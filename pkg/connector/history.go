package connector

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow/types"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

type historyReadState struct {
	readWatermark int64
	lastActivity  int64
}

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
		if err = m.Main.Bridge.DB.BackfillTask.EnsureExists(ctx, portal.PortalKey, m.UserLogin.ID); err != nil {
			return err
		}
		if err = m.Main.Bridge.DB.BackfillTask.MarkNotDone(ctx, portal.PortalKey, m.UserLogin.ID); err != nil {
			return err
		}
		m.Main.Bridge.WakeupBackfillQueue(&bridgev2.ManualBackfill{Source: m.UserLogin, Portal: portal})
	}
	return nil
}

func (m *MetaClient) fetchEncryptedHistory(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	device := m.historyDevice()
	if device == nil || device.ID == "" || params.ThreadRoot != "" {
		return nil, nil
	}
	timestamp := time.Now().Add(time.Minute).UnixMilli()
	if !params.Forward {
		if params.AnchorMessage != nil {
			timestamp = params.AnchorMessage.Timestamp.UnixMilli()
		}
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
	var newestTimestamp int64
	for _, message := range messages {
		converted, err := m.convertArchiveMessage(ctx, params.Portal, message)
		if err != nil {
			return nil, err
		} else if converted != nil {
			response.Messages = append(response.Messages, converted)
		}
		newestTimestamp = max(newestTimestamp, message.Message.TimestampMS)
	}
	slices.SortFunc(response.Messages, func(a, b *bridgev2.BackfillMessage) int {
		return cmp.Or(a.Timestamp.Compare(b.Timestamp), cmp.Compare(a.ID, b.ID))
	})
	if (params.Forward || params.AnchorMessage == nil) && len(response.Messages) > 0 {
		readState, _ := m.historyReadStates.Get(metaid.ParseFBPortalID(params.Portal.ID))
		response.MarkRead = readState.lastActivity > 0 && readState.readWatermark >= max(readState.lastActivity, newestTimestamp)
	}
	return response, nil
}
