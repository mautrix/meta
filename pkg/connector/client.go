package connector

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type MetaClient struct {
	Main      *MetaConnector
	Client    *messagix.Client
	LoginMeta *metaid.UserLoginMetadata
	UserLogin *bridgev2.UserLogin
	Ghost     *bridgev2.Ghost

	stopHandlingTables atomic.Pointer[context.CancelFunc]
	initialTable       atomic.Pointer[table.LSTable]
	incomingTables     chan *table.LSTable

	E2EEClient      *whatsmeow.Client
	WADevice        *store.Device
	e2eeConnectLock sync.Mutex

	metaState status.BridgeState
	waState   status.BridgeState
}

func (m *MetaConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	loginMetadata := login.Metadata.(*metaid.UserLoginMetadata)
	loginMetadata.Cookies.Platform = loginMetadata.Platform
	c := &MetaClient{
		Main:      m,
		Client:    messagix.NewClient(loginMetadata.Cookies, login.Log.With().Str("component", "messagix").Logger()),
		LoginMeta: loginMetadata,
		UserLogin: login,

		incomingTables: make(chan *table.LSTable, 16),
	}
	c.Client.SetEventHandler(c.handleMetaEvent)
	login.Client = c
	return nil
}

var _ bridgev2.NetworkAPI = (*MetaClient)(nil)

func (m *MetaClient) Connect(ctx context.Context) error {
	currentUser, initialTable, err := m.Client.LoadMessagesPage()
	if err != nil {
		return fmt.Errorf("failed to load messages page: %w", err)
	}
	return m.connectWithTable(ctx, initialTable, currentUser)
}

func (m *MetaClient) connectWithTable(ctx context.Context, initialTable *table.LSTable, currentUser types.UserInfo) error {
	go m.handleTableLoop()

	var err error
	m.Ghost, err = m.Main.Bridge.GetGhostByID(ctx, networkid.UserID(m.UserLogin.ID))
	if err != nil {
		return fmt.Errorf("failed to get own ghost: %w", err)
	}
	m.Ghost.UpdateInfo(ctx, m.wrapUserInfo(currentUser))
	m.UserLogin.RemoteProfile.Name = currentUser.GetName()
	if !m.LoginMeta.Platform.IsMessenger() {
		m.UserLogin.RemoteProfile.Username = currentUser.GetUsername()
	}
	m.UserLogin.RemoteProfile.Avatar = m.Ghost.AvatarMXC

	m.initialTable.Store(initialTable)

	err = m.Client.Connect()
	if err != nil {
		return err
	}

	return nil
}

func (m *MetaClient) connectE2EE() error {
	m.e2eeConnectLock.Lock()
	defer m.e2eeConnectLock.Unlock()
	if m.E2EEClient != nil {
		return fmt.Errorf("already connected to e2ee")
	}
	log := m.UserLogin.Log.With().Str("component", "e2ee").Logger()
	ctx := log.WithContext(context.TODO())
	var err error
	if m.WADevice == nil && m.LoginMeta.WADeviceID != 0 {
		m.WADevice, err = m.Main.DeviceStore.GetDevice(waTypes.JID{User: string(m.UserLogin.ID), Device: m.LoginMeta.WADeviceID, Server: waTypes.MessengerServer})
		if err != nil {
			return fmt.Errorf("failed to get whatsmeow device: %w", err)
		} else if m.WADevice == nil {
			log.Warn().Uint16("device_id", m.LoginMeta.WADeviceID).Msg("Existing device not found in store")
		}
	}
	isNew := false
	if m.WADevice == nil {
		isNew = true
		m.WADevice = m.Main.DeviceStore.NewDevice()
	}
	m.Client.SetDevice(m.WADevice)

	if isNew {
		fbid := metaid.ParseUserLoginID(m.UserLogin.ID)
		log.Info().Msg("Registering new e2ee device")
		err = m.Client.RegisterE2EE(ctx, fbid)
		if err != nil {
			return fmt.Errorf("failed to register e2ee device: %w", err)
		}
		m.LoginMeta.WADeviceID = m.WADevice.ID.Device
		err = m.WADevice.Save()
		if err != nil {
			return fmt.Errorf("failed to save whatsmeow device store: %w", err)
		}
		err = m.UserLogin.Save(ctx)
		if err != nil {
			return fmt.Errorf("failed to save device ID to user login: %w", err)
		}
	}
	m.E2EEClient = m.Client.PrepareE2EEClient()
	m.E2EEClient.AddEventHandler(m.e2eeEventHandler)
	err = m.E2EEClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to e2ee socket: %w", err)
	}
	return nil
}

func (m *MetaClient) Disconnect() {
	if cli := m.Client; cli != nil {
		cli.Disconnect()
		m.Client = nil
	}
	if ecli := m.E2EEClient; ecli != nil {
		ecli.Disconnect()
		m.E2EEClient = nil
	}
	m.metaState = status.BridgeState{}
	m.waState = status.BridgeState{}
	if stopTableLoop := m.stopHandlingTables.Load(); stopTableLoop != nil {
		(*stopTableLoop)()
	}
}

var metaCaps = &bridgev2.NetworkRoomCapabilities{
	FormattedText: true,
	UserMentions:  true,
	Replies:       true,
	Edits:         true,
	EditMaxCount:  10,
	EditMaxAge:    24 * time.Hour,
	Reactions:     true,
	ReactionCount: 1,
}

func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *bridgev2.NetworkRoomCapabilities {
	return metaCaps
}

var ErrServerRejectedMessage = bridgev2.WrapErrorInStatus(errors.New("server rejected message")).WithErrorAsMessage().WithSendNotice(true)

func (m *MetaClient) IsLoggedIn() bool {
	return m.Client != nil
}

func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return networkid.UserLoginID(userID) == m.UserLogin.ID
}

func (m *MetaClient) LogoutRemote(ctx context.Context) {
	panic("unimplemented")
}

func (m *MetaClient) canReconnect() bool {
	//return time.Since(user.lastFullReconnect) > time.Duration(user.bridge.Config.Meta.MinFullReconnectIntervalSeconds)*time.Second
	return false
}

func (m *MetaClient) FullReconnect() {
	panic("unimplemented")
}

func (m *MetaClient) resetWADevice() {
	m.WADevice = nil
	m.LoginMeta.WADeviceID = 0
}
