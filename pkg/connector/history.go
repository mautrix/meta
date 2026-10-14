package connector

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

func (m *MetaClient) historyDevice() *securehistory.Device {
	m.backfillLock.Lock()
	defer m.backfillLock.Unlock()
	return m.LoginMeta.HistoryDevice
}

func (m *MetaClient) canBackfillEncrypted() bool {
	device := m.historyDevice()
	return device != nil && device.ID != ""
}

func (m *MetaClient) saveHistoryEpochs(ctx context.Context, fetchedDevice *securehistory.Device, epochs *securehistory.EpochDerivationSet) (*securehistory.Device, error) {
	if epochs == nil || len(epochs.Edges) == 0 {
		return fetchedDevice, nil
	}
	m.backfillLock.Lock()
	defer m.backfillLock.Unlock()
	previous := m.LoginMeta.HistoryDevice
	if previous == nil || previous.ID != fetchedDevice.ID {
		return nil, fmt.Errorf("history enrollment changed while fetching messages")
	}
	device, err := previous.WithEpochs(epochs)
	if err != nil {
		return nil, err
	}
	m.LoginMeta.HistoryDevice = device
	if err = m.UserLogin.Save(ctx); err != nil {
		m.LoginMeta.HistoryDevice = previous
		return nil, fmt.Errorf("failed to save Messenger history epochs: %w", err)
	}
	return device, nil
}

func (m *MetaClient) queueEncryptedBackfill(ctx context.Context) error {
	if !m.Main.Bridge.Config.Backfill.Enabled || !m.Main.Bridge.Config.Backfill.Queue.AnyEnabled() {
		return nil
	}
	portals, err := m.Main.Bridge.DB.UserPortal.GetAllForLogin(ctx, m.UserLogin.UserLogin)
	if err != nil {
		return err
	}
	for _, userPortal := range portals {
		portal, err := m.Main.Bridge.GetExistingPortalByKey(ctx, userPortal.Portal)
		if err != nil {
			return err
		} else if portal == nil || portal.MXID == "" || !portal.Metadata.(*metaid.PortalMetadata).ThreadType.IsWhatsApp() {
			continue
		}
		task, err := m.Main.Bridge.DB.BackfillTask.GetNextForPortal(ctx, portal.PortalKey, true)
		if err != nil {
			return err
		} else if task != nil && task.UserLoginID != "" && task.UserLoginID != m.UserLogin.ID {
			continue
		}
		if err = m.Main.Bridge.DB.BackfillTask.Upsert(ctx, &database.BackfillTask{
			PortalKey:         portal.PortalKey,
			UserLoginID:       m.UserLogin.ID,
			NextDispatchMinTS: time.Now(),
		}); err != nil {
			return err
		}
		m.Main.Bridge.WakeupBackfillQueue(&bridgev2.ManualBackfill{Source: m.UserLogin, Portal: portal})
	}
	return nil
}

type messengerHistoryCursor struct {
	Timestamp    int64  `json:"timestamp,omitempty"`
	OpenThreadID int64  `json:"open_thread_id,omitempty"`
	MessageID    string `json:"message_id,omitempty"`
}

func (m *MetaClient) fetchMessengerHistory(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	if params.ThreadRoot != "" {
		return nil, nil
	} else if params.Forward {
		return m.fetchEncryptedHistory(ctx, params)
	}
	var cursor messengerHistoryCursor
	if params.Cursor != "" {
		if err := json.Unmarshal([]byte(params.Cursor), &cursor); err != nil {
			return nil, fmt.Errorf("invalid Messenger history cursor: %w", err)
		}
	}
	openThreadID, err := m.Main.DB.GetOpenThreadKey(ctx, m.UserLogin.ID, metaid.ParseFBPortalID(params.Portal.ID))
	if err != nil {
		return nil, err
	}
	if cursor.OpenThreadID == 0 && !m.canBackfillEncrypted() {
		cursor.OpenThreadID = openThreadID
	}
	if cursor.OpenThreadID != 0 {
		if cursor.OpenThreadID != openThreadID {
			return nil, fmt.Errorf("history predecessor changed")
		}
		params.AnchorMessage = nil
		if cursor.MessageID != "" {
			params.AnchorMessage = &database.Message{
				ID:        metaid.MakeFBMessageID(cursor.MessageID),
				Timestamp: time.UnixMilli(cursor.Timestamp),
			}
		}
		return m.fetchFacebookHistory(ctx, params, openThreadID, true)
	}
	params.Cursor = ""
	if cursor.Timestamp != 0 {
		params.Cursor = networkid.PaginationCursor(strconv.FormatInt(cursor.Timestamp, 10))
	}
	response, err := m.fetchEncryptedHistory(ctx, params)
	if err != nil || response == nil {
		return response, err
	}
	cursor.Timestamp, err = strconv.ParseInt(string(response.Cursor), 10, 64)
	if err != nil {
		return nil, err
	}
	if !response.HasMore && openThreadID != 0 {
		cursor = messengerHistoryCursor{OpenThreadID: openThreadID}
		response.HasMore = true
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return nil, err
	}
	response.Cursor = networkid.PaginationCursor(encoded)
	response.AggressiveDeduplication = true
	return response, nil
}

func (m *MetaClient) fetchEncryptedHistory(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	device := m.historyDevice()
	if device == nil || device.ID == "" || params.ThreadRoot != "" {
		return nil, nil
	}
	timestamp := time.Now().Add(time.Minute).UnixMilli()
	if !params.Forward {
		if params.Cursor != "" {
			var err error
			timestamp, err = strconv.ParseInt(string(params.Cursor), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid Messenger history cursor: %w", err)
			}
		}
	}
	chat := params.Portal.Metadata.(*metaid.PortalMetadata).JID(params.Portal.ID)
	page, err := m.Client.GetSecureHistoryClient().FetchMessages(ctx, device, chat.User, true, timestamp, params.Count)
	if err != nil {
		return nil, err
	}
	next := page.Range.Before
	if *page.Range.HasMoreBefore && next >= timestamp {
		return nil, fmt.Errorf("history cursor did not move backwards")
	}
	device, err = m.saveHistoryEpochs(ctx, device, page.Epochs)
	if err != nil {
		return nil, err
	}
	response := &bridgev2.FetchMessagesResponse{
		Messages: make([]*bridgev2.BackfillMessage, 0, len(page.Messages)),
		Cursor:   networkid.PaginationCursor(strconv.FormatInt(next, 10)),
		HasMore:  *page.Range.HasMoreBefore,
		Forward:  params.Forward,
	}
	messages, err := page.Decrypt(device, chat.User, metaid.ParseUserLoginID(m.UserLogin.ID), chat.Server == types.GroupServer)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt Messenger history: %w", err)
	}
	for _, message := range messages {
		converted, err := m.convertArchiveMessage(ctx, params.Portal, message)
		if err != nil {
			return nil, err
		} else if converted != nil {
			response.Messages = append(response.Messages, converted)
		}
	}
	slices.SortFunc(response.Messages, func(a, b *bridgev2.BackfillMessage) int {
		return cmp.Or(a.Timestamp.Compare(b.Timestamp), cmp.Compare(a.ID, b.ID))
	})
	return response, nil
}
