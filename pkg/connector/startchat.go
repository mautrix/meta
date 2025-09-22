package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/messagix/socket"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.IdentifierResolvingNetworkAPI = (*MetaClient)(nil)
	_ bridgev2.UserSearchingNetworkAPI       = (*MetaClient)(nil)
	_ bridgev2.IdentifierValidatingNetwork   = (*MetaConnector)(nil)
)

func (m *MetaConnector) ValidateUserID(id networkid.UserID) bool {
	parsed := metaid.ParseUserID(id)
	return parsed > 0
}

func (m *MetaClient) ResolveIdentifier(ctx context.Context, identifier string, createChat bool) (*bridgev2.ResolveIdentifierResponse, error) {
	if m.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	} else if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
		return nil, ErrNotConnected
	}
	log := zerolog.Ctx(ctx)

	id, err := metaid.ParseIDFromString(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to parse identifier: %w", err)
	}

	var chat *bridgev2.CreateChatResponse
	if createChat {
		resp, err := m.Client.ExecuteTasks(ctx, &socket.CreateThreadTask{
			ThreadFBID:                id,
			ForceUpsert:               0,
			UseOpenMessengerTransport: 0,
			SyncGroup:                 1,
			MetadataOnly:              0,
			PreviewOnly:               0,
		})

		log.Debug().Any("response_data", resp).Err(err).Msg("Create chat response")
		chat = &bridgev2.CreateChatResponse{
			PortalKey:  m.makeFBPortalKey(id, table.ONE_TO_ONE),
			PortalInfo: m.makeMinimalChatInfo(id, table.ONE_TO_ONE),
		}
	}
	ghost, _ := m.Main.Bridge.GetGhostByID(ctx, metaid.MakeUserID(id))
	return &bridgev2.ResolveIdentifierResponse{
		UserID: metaid.MakeUserID(id),
		Ghost:  ghost,
		Chat:   chat,
	}, nil
}

func (m *MetaClient) CreateWhatsAppDM(ctx context.Context, threadID int64) error {
	log := zerolog.Ctx(ctx)
	resp, err := m.Client.ExecuteTasks(ctx, &socket.CreateWhatsAppThreadTask{
		WAJID:            threadID,
		OfflineThreadKey: methods.GenerateEpochID(),
		ThreadType:       table.ENCRYPTED_OVER_WA_ONE_TO_ONE,
		FolderType:       table.INBOX,
		BumpTimestampMS:  time.Now().UnixMilli(),
		TAMThreadSubtype: 0,
	})
	if err != nil {
		return err
	}
	log.Trace().Any("create_resp", resp).Msg("Create WhatsApp thread response")
	if len(resp.LSIssueNewTask) > 0 {
		tasks := make([]socket.Task, len(resp.LSIssueNewTask))
		for i, task := range resp.LSIssueNewTask {
			log.Trace().Any("task", task).Msg("Create WhatsApp thread response task")
			tasks[i] = task
		}
		resp, err = m.Client.ExecuteTasks(ctx, tasks...)
		if err != nil {
			return fmt.Errorf("failed to run WhatsApp thread create subtasks: %w", err)
		} else {
			log.Trace().Any("create_resp", resp).Msg("Create thread response")
		}
	}
	return nil
}

func (m *MetaClient) SearchUsers(ctx context.Context, search string) ([]*bridgev2.ResolveIdentifierResponse, error) {
	if m.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	} else if !m.connectWaiter.WaitTimeout(ConnectWaitTimeout) {
		return nil, ErrNotConnected
	}
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
		resp, err := m.Client.ExecuteTasks(ctx, secondaryTask)
		log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier secondary response")
		// The secondary response doesn't seem to have anything important, so just ignore it
	}()

	resp, err := m.Client.ExecuteTasks(ctx, task)
	log.Trace().Any("response_data", resp).Err(err).Msg("Resolve identifier primary response")
	if err != nil {
		return nil, fmt.Errorf("failed to search for user: %w", err)
	}

	users := make([]*bridgev2.ResolveIdentifierResponse, 0)

	for _, result := range resp.LSInsertSearchResult {
		if result.ThreadType == table.ONE_TO_ONE && result.CanViewerMessage && result.GetFBID() != 0 {
			userID := metaid.MakeUserID(result.GetFBID())
			ghost, _ := m.Main.Bridge.GetGhostByID(ctx, userID)
			users = append(users, &bridgev2.ResolveIdentifierResponse{
				UserID:   userID,
				Ghost:    ghost,
				UserInfo: m.wrapUserInfo(result),
			})
		}
	}

	return users, nil
}
