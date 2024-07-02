package connector

import (
	"context"
	"strconv"

	"go.mau.fi/mautrix-meta/messagix"
	//"go.mau.fi/mautrix-meta/pkg/store"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type MetaClient struct {
	Main *MetaConnector
	//session *store.MetaSession
	client *messagix.Client
}

func NewMetaClient(ctx context.Context, main *MetaConnector, login *bridgev2.UserLogin) (*MetaClient, error) {
	FBID, err := strconv.ParseInt(string(login.ID), 10, 64)
	if err != nil {
		return nil, err
	}
	session, err := main.store.GetSessionQuery().GetByMetaID(ctx, FBID)
	if err != nil {
		return nil, err
	}

	log := login.User.Log.With().Str("component", "messagix").Logger()
	client := messagix.NewClient(session.Cookies, log)

	return &MetaClient{
		Main: main,
		//session: session,
		client: client,
	}, nil
}

// Connect implements bridgev2.NetworkAPI.
func (m *MetaClient) Connect(ctx context.Context) error {
	panic("unimplemented")
}

// Disconnect implements bridgev2.NetworkAPI.
func (m *MetaClient) Disconnect() {
	panic("unimplemented")
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