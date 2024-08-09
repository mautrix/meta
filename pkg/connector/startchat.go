package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"

	"go.mau.fi/mautrix-meta/messagix/socket"
	"go.mau.fi/mautrix-meta/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.IdentifierResolvingNetworkAPI = (*MetaClient)(nil)
	_ bridgev2.UserSearchingNetworkAPI       = (*MetaClient)(nil)
)

func (m *MetaClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	log := zerolog.Ctx(ctx)

	id, err := metaid.ParseIDFromString(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}

	var chat *bridgev2.CreateChatResponse
	if createChat {
		resp, err := m.Client.ExecuteTasks(
			&socket.CreateThreadTask{
				ThreadFBID:                id,
				ForceUpsert:               0,
				UseOpenMessengerTransport: 0,
				SyncGroup:                 1,
				MetadataOnly:              0,
				PreviewOnly:               0,
			},
		)

		log.Debug().Any("response_data", resp).Err(err).Msg("Create chat response")
		chat = &bridgev2.CreateChatResponse{
			PortalKey:  m.makeFBPortalKey(id, table.ONE_TO_ONE),
			PortalInfo: m.makeMinimalChatInfo(id, table.ONE_TO_ONE),
		}
	}
	return &bridgev2.ResolveIdentifierResponse{
		UserID: metaid.MakeUserID(id),
		Chat:   chat,
	}, nil
}

func (m *MetaClient) SearchUsers(ctx context.Context, search string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	log := zerolog.Ctx(ctx)

	task := &socket.SearchUserTask{
		Query: search,
		SupportedTypes: []table.SearchType{
			table.SearchTypeContact, table.SearchTypeGroup, table.SearchTypePage, table.SearchTypeNonContact,
			table.SearchTypeIGContactFollowing, table.SearchTypeIGContactNonFollowing,
			table.SearchTypeIGNonContactFollowing, table.SearchTypeIGNonContactNonFollowing,
		},
		SurfaceType: 15,
		Secondary:   false,
	}
	if m.LoginMeta.Platform.IsMessenger() {
		task.SurfaceType = 5
		task.SupportedTypes = append(task.SupportedTypes, table.SearchTypeCommunityMessagingThread)
	}
	taskCopy := *task
	taskCopy.Secondary = true
	secondaryTask := &taskCopy

	go func() {
		time.Sleep(10 * time.Millisecond)
		resp, err := m.Client.ExecuteTasks(secondaryTask)
		log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier secondary response")
		// The secondary response doesn't seem to have anything important, so just ignore it
	}()

	resp, err := m.Client.ExecuteTasks(task)
	log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier primary response")
	if err != nil {
		return nil, fmt.Errorf("failed to search for user: %w", err)
	}

	users := make([]*bridgev2.ResolveIdentifierResponse, 0)

	for _, result := range resp.LSInsertSearchResult {
		if result.ThreadType == table.ONE_TO_ONE && result.CanViewerMessage && result.GetFBID() != 0 {
			users = append(users, &bridgev2.ResolveIdentifierResponse{
				UserID:   metaid.MakeUserID(result.GetFBID()),
				UserInfo: m.wrapUserInfo(result),
			})
		}
	}

	return users, nil
}
