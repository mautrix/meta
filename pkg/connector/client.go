package connector

import (
	"context"
	"fmt"
	"reflect"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type MetaClient struct {
	Main    *MetaConnector
	client  *messagix.Client
	log     zerolog.Logger
	cookies *cookies.Cookies
	login   *bridgev2.UserLogin
}

func cookiesFromMetadata(metadata map[string]interface{}) *cookies.Cookies {
	platform := types.Platform(metadata["platform"].(float64))

	m := make(map[string]string)
	for k, v := range metadata["cookies"].(map[string]interface{}) {
		m[k] = v.(string)
	}

	c := &cookies.Cookies{
		Platform: platform,
	}
	c.UpdateValues(m)

	return c
}

func NewMetaClient(ctx context.Context, main *MetaConnector, login *bridgev2.UserLogin) (*MetaClient, error) {
	login.User.Log.Debug().Any("metadata", login.Metadata.Extra).Msg("Creating new Meta client")

	var c *cookies.Cookies
	if _, ok := login.Metadata.Extra["cookies"].(map[string]interface{}); ok {
		c = cookiesFromMetadata(login.Metadata.Extra)
	} else {
		c = login.Metadata.Extra["cookies"].(*cookies.Cookies)
	}

	return &MetaClient{
		Main:    main,
		cookies: c,
		log:     login.Log,
		login:   login,
	}, nil
}

func (m *MetaClient) Update(ctx context.Context) error {
	m.login.Metadata.Extra["cookies"] = m.cookies
	err := m.login.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save updated cookies: %w", err)
	}
	m.log.Debug().Msg("Updated cookies")
	return nil
}

func (m *MetaClient) eventHandler(rawEvt any) {
	switch evt := rawEvt.(type) {
	case *messagix.Event_PublishResponse:
		m.log.Trace().Any("table", &evt.Table).Msg("Got new event table")
		m.handleTable(evt.Table)
	case *messagix.Event_Ready:
		m.log.Trace().Msg("Initial connect to Meta socket completed, sending connected BridgeState")
		m.login.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	default:
		m.log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (m *MetaClient) handleTable(tbl *table.LSTable) {
	tblValue := reflect.ValueOf(*tbl)
	for i := 0; i < tblValue.NumField(); i++ {
		field := tblValue.Field(i)
		if field.Kind() == reflect.Slice {
			for j := 0; j < field.Len(); j++ {
				m.handleTableEvent(field.Index(j).Interface())
			}
		}
	}
}

func (m *MetaClient) handleTableEvent(tblEvt any) {
	switch evt := tblEvt.(type) {
	case *table.LSInsertMessage:
		m.log.Info().Any("text", evt.Text).Any("sender", evt.SenderId).Msg("Got new message")
	default:
		m.log.Warn().Type("event_type", evt).Msg("Unrecognized event type from table")
	}

}

func (m *MetaClient) Connect(ctx context.Context) error {
	if m.client == nil {
		log := m.login.User.Log.With().Str("component", "messagix").Logger()
		client := messagix.NewClient(m.cookies, log)
		m.client = client
		// We have to call this before calling `Connect`, even if we don't use the result
		_, _, err := m.client.LoadMessagesPage()
		if err != nil {
			return fmt.Errorf("failed to load messages page: %w", err)
		}
	}

	m.client.SetEventHandler(m.eventHandler)
	err := m.client.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to messagix: %w", err)
	}
	err = m.Update(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (m *MetaClient) Disconnect() {
	if m.client != nil {
		m.client.Disconnect()
	}
	m.client = nil
}

// GetCapabilities implements bridgev2.NetworkAPI.
func (m *MetaClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *bridgev2.NetworkRoomCapabilities {
	panic("unimplemented")
}

// GetChatInfo implements bridgev2.NetworkAPI.
func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.PortalInfo, error) {
	panic("unimplemented")
}

// GetUserInfo implements bridgev2.NetworkAPI.
func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	panic("unimplemented")
}

// HandleMatrixMessage implements bridgev2.NetworkAPI.
func (m *MetaClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (message *bridgev2.MatrixMessageResponse, err error) {
	panic("unimplemented")
}

// IsLoggedIn implements bridgev2.NetworkAPI.
func (m *MetaClient) IsLoggedIn() bool {
	panic("unimplemented")
}

// IsThisUser implements bridgev2.NetworkAPI.
func (m *MetaClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	panic("unimplemented")
}

// LogoutRemote implements bridgev2.NetworkAPI.
func (m *MetaClient) LogoutRemote(ctx context.Context) {
	panic("unimplemented")
}

var (
	_ bridgev2.NetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.EditHandlingNetworkAPI        = (*MetaClient)(nil)
	// _ bridgev2.ReactionHandlingNetworkAPI    = (*MetaClient)(nil)
	// _ bridgev2.RedactionHandlingNetworkAPI   = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.ReadReceiptHandlingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.TypingHandlingNetworkAPI      = (*MetaClient)(nil)
	// _ bridgev2.IdentifierResolvingNetworkAPI = (*MetaClient)(nil)
	// _ bridgev2.GroupCreatingNetworkAPI       = (*MetaClient)(nil)
	// _ bridgev2.ContactListingNetworkAPI      = (*MetaClient)(nil)
)
