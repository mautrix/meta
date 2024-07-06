package connector

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/messagix/types"

	"maunium.net/go/mautrix/bridge/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
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
	log := zerolog.Ctx(ctx).With().Str("component", "meta_client").Logger()
	log.Debug().Any("metadata", login.Metadata.Extra).Msg("Creating new Meta client")

	var c *cookies.Cookies
	if _, ok := login.Metadata.Extra["cookies"].(map[string]interface{}); ok {
		c = cookiesFromMetadata(login.Metadata.Extra)
	} else {
		c = login.Metadata.Extra["cookies"].(*cookies.Cookies)
	}

	return &MetaClient{
		Main:    main,
		cookies: c,
		log:     log,
		login:   login,
	}, nil
}

func (m *MetaClient) Update(ctx context.Context) error {
	m.login.Metadata.Extra["cookies"] = m.cookies
	err := m.login.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to save updated cookies: %w", err)
	}
	zerolog.Ctx(ctx).Debug().Msg("Updated cookies")
	return nil
}

func (m *MetaClient) handleMetaEvent(rawEvt any) {
	// Create a new context for this event
	ctx := m.log.WithContext(context.TODO())
	log := zerolog.Ctx(ctx)

	switch evt := rawEvt.(type) {
	case *messagix.Event_PublishResponse:
		log.Trace().Any("table", &evt.Table).Msg("Got new event table")
		m.handleTable(ctx, evt.Table)
	case *messagix.Event_Ready:
		log.Trace().Msg("Initial connect to Meta socket completed, sending connected BridgeState")
		m.login.BridgeState.Send(status.BridgeState{StateEvent: status.StateConnected})
	default:
		log.Warn().Type("event_type", evt).Msg("Unrecognized event type from messagix")
	}
}

func (m *MetaClient) handleTable(ctx context.Context, tbl *table.LSTable) {
	tblValue := reflect.ValueOf(*tbl)
	for i := 0; i < tblValue.NumField(); i++ {
		field := tblValue.Field(i)
		if field.Kind() == reflect.Slice {
			for j := 0; j < field.Len(); j++ {
				m.handleTableEvent(ctx, field.Index(j).Interface())
			}
		}
	}
}

func (m *MetaClient) handleTableEvent(ctx context.Context, tblEvt any) {
	log := zerolog.Ctx(ctx)
	switch evt := tblEvt.(type) {
	case *table.LSInsertMessage:
		log.Info().Any("text", evt.Text).Any("sender", evt.SenderId).Msg("Got new message")
		m.Main.Bridge.QueueRemoteEvent(m.login, &bridgev2.SimpleRemoteEvent[string]{
			Type: bridgev2.RemoteEventMessage,
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.
					Str("message_id", evt.MessageId).
					Str("sender", strconv.Itoa(int(evt.SenderId))).
					//Str("sender_login", string(evt.Sender)).
					Bool("is_from_me", strconv.Itoa(int(evt.SenderId)) == string(m.login.ID))
			},
			ID: networkid.MessageID(evt.MessageId),
			Sender: bridgev2.EventSender{
				IsFromMe:    strconv.Itoa(int(evt.SenderId)) == string(m.login.ID),
				Sender:      networkid.UserID(strconv.Itoa(int(evt.SenderId))),
				SenderLogin: networkid.UserLoginID(strconv.Itoa(int(evt.SenderId))),
			},
			PortalKey: networkid.PortalKey{
				ID: networkid.PortalID("test"),
			},
			Data:         evt.Text,
			CreatePortal: true,
			ConvertMessageFunc: func(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, data string) (*bridgev2.ConvertedMessage, error) {
				return &bridgev2.ConvertedMessage{
					Parts: []*bridgev2.ConvertedMessagePart{
						{
							ID:   networkid.PartID("test"),
							Type: event.EventMessage,
							Content: &event.MessageEventContent{
								MsgType: event.MsgText,
								Body:    data,
							},
						},
					},
				}, nil
			},
		})
	default:
		log.Warn().Type("event_type", evt).Msg("Unrecognized event type from table")
	}
}

func (m *MetaClient) Connect(ctx context.Context) error {
	client := messagix.NewClient(m.cookies, m.log.With().Str("component", "messagix").Logger())
	m.client = client

	_, initialTable, err := m.client.LoadMessagesPage()
	if err != nil {
		return fmt.Errorf("failed to load messages page: %w", err)
	}

	m.handleTable(ctx, initialTable)

	m.client.SetEventHandler(m.handleMetaEvent)

	err = m.client.Connect()
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
func (m *MetaClient) GetChatInfo(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
	return &bridgev2.ChatInfo{
		Name:         &[]string{"test"}[0],
		IsSpace:      &[]bool{false}[0],
		IsDirectChat: &[]bool{true}[0],
	}, nil
}

// GetUserInfo implements bridgev2.NetworkAPI.
func (m *MetaClient) GetUserInfo(ctx context.Context, ghost *bridgev2.Ghost) (*bridgev2.UserInfo, error) {
	return nil, nil
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
