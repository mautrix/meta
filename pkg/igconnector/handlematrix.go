// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2026 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package igconnector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/jsontime"
	"go.mau.fi/util/ptr"
	"go.mau.fi/util/variationselector"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/pkg/instameow"
	"go.mau.fi/mautrix-meta/pkg/instameow/slidetypes"
	"go.mau.fi/mautrix-meta/pkg/messagix/methods"
	"go.mau.fi/mautrix-meta/pkg/metaid"
)

var (
	_ bridgev2.EditHandlingNetworkAPI        = (*IGClient)(nil)
	_ bridgev2.ReactionHandlingNetworkAPI    = (*IGClient)(nil)
	_ bridgev2.RedactionHandlingNetworkAPI   = (*IGClient)(nil)
	_ bridgev2.ReadReceiptHandlingNetworkAPI = (*IGClient)(nil)
	//_ bridgev2.ChatViewingNetworkAPI             = (*IGClient)(nil)
	_ bridgev2.TypingHandlingNetworkAPI          = (*IGClient)(nil)
	_ bridgev2.MessageRequestAcceptingNetworkAPI = (*IGClient)(nil)
	_ bridgev2.DeleteChatHandlingNetworkAPI      = (*IGClient)(nil)
	_ bridgev2.MuteHandlingNetworkAPI            = (*IGClient)(nil)
	_ bridgev2.TagHandlingNetworkAPI             = (*IGClient)(nil)
	_ bridgev2.RoomNameHandlingNetworkAPI        = (*IGClient)(nil)
	_ bridgev2.RoomAvatarHandlingNetworkAPI      = (*IGClient)(nil)
)

var _ bridgev2.TransactionIDGeneratingNetwork = (*IGConnector)(nil)

func (ic *IGConnector) GenerateTransactionID(userID id.UserID, roomID id.RoomID, eventType event.Type) networkid.RawTransactionID {
	return networkid.RawTransactionID(strconv.FormatInt(methods.GenerateEpochID(), 10))
}

func getOTID(inputTxnID networkid.RawTransactionID) int64 {
	if inputTxnID != "" {
		otid, err := strconv.ParseInt(string(inputTxnID), 10, 64)
		if err == nil && otid > 0 {
			return otid
		}
	}
	return methods.GenerateEpochID()
}

var ErrIGIDNotFound = bridgev2.WrapErrorInStatus(errors.New("IGID of thread not available")).
	WithIsCertain(true).WithSendNotice(true)

func (ic *IGClient) fetchIGIDs(ctx context.Context, portal *bridgev2.Portal) error {
	fbid := metaid.ParseFBPortalID(portal.ID)
	if fbid == 0 {
		return fmt.Errorf("invalid portal ID")
	}
	res, err := ic.fetchIGIDsDirect(ctx, fbid)
	if err != nil {
		return err
	}
	meta := portal.Metadata.(*metaid.PortalMetadata)
	meta.IGThreadID = res.LongID
	meta.IGID = res.ShortID
	return nil
}

func (ic *IGClient) ensureIGIDDirect(ctx context.Context, fbid int64) (string, error) {
	igid, err := ic.Main.DB.GetIGChatForFBID(ctx, fbid, ic.UserLogin.ID)
	if err != nil {
		return "", err
	} else if igid != "" {
		return igid, nil
	} else if ids, err := ic.fetchIGIDsDirect(ctx, fbid); err != nil {
		return "", err
	} else {
		return ids.ShortID, nil
	}
}

func (ic *IGClient) fetchIGIDsDirect(ctx context.Context, fbid int64) (*instameow.ThreadIGIDs, error) {
	resp, err := ic.Client.FetchThreadIDs(ctx, fbid)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch IGIDs: %w", err)
	}
	res, ok := resp[fbid]
	if !ok {
		return nil, fmt.Errorf("server didn't return route definition for %d", fbid)
	} else if res == nil {
		return nil, fmt.Errorf("server returned nil route definition for %d", fbid)
	}
	err = ic.Main.DB.PutFBIDForIGThread(ctx, res.LongID, fbid, ic.UserLogin.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to save IG thread ID mapping %s<->%d: %w", res.LongID, fbid, err)
	}
	err = ic.Main.DB.PutFBIDForIGChat(ctx, res.ShortID, fbid, ic.UserLogin.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to save IG chat ID mapping %s<->%d: %w", res.ShortID, fbid, err)
	}
	return res, nil
}

func (ic *IGClient) ensureIGID(ctx context.Context, portal *bridgev2.Portal) (*metaid.PortalMetadata, error) {
	meta := portal.Metadata.(*metaid.PortalMetadata)
	if meta.IGID == "" || meta.IGThreadID == "" {
		err := ic.fetchIGIDs(ctx, portal)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrIGIDNotFound, err)
		}
		err = portal.Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to save portal after fetching IGID: %w", err)
		}
	}
	return meta, nil
}

func (ic *IGClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	if ic.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	_, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return nil, err
	}
	log := zerolog.Ctx(ctx)

	otid := getOTID(msg.InputTransactionID)

	log.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Int64("otid", otid)
	})
	log.Debug().Msg("Sending Matrix message to Meta")
	converted, err := ic.Main.MsgConv.ToInstagram(ctx, ic.Client, msg.Event, msg.Content, msg.ReplyTo, otid, msg.OrigSender != nil, msg.Portal)
	if err != nil {
		return nil, err
	}

	var resp interface {
		GetMessage() slidetypes.SentMessage
	}
	switch req := converted.(type) {
	case *slidetypes.SendTextRequest:
		resp, err = ic.Client.SendMessage(ctx, req)
	case *slidetypes.SendMediaRequest:
		resp, err = ic.Client.SendMedia(ctx, req)
	default:
		return nil, fmt.Errorf("unexpected converted request type %T", converted)
	}
	if err != nil {
		return nil, err
	}

	respMsg := resp.GetMessage()
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        metaid.MakeFBMessageID(respMsg.ID),
			SenderID:  networkid.UserID(ic.UserLogin.ID),
			Timestamp: respMsg.TimestampMS.Time,
		},
		StreamOrder: respMsg.TimestampMS.UnixMilli(),
	}, nil
}

func (ic *IGClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	return bridgev2.MatrixReactionPreResponse{
		SenderID:     networkid.UserID(ic.UserLogin.ID),
		EmojiID:      "",
		Emoji:        variationselector.Remove(msg.Content.RelatesTo.Key),
		MaxReactions: 1,
	}, nil
}

func (ic *IGClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (*database.Reaction, error) {
	if ic.LoginMeta.Cookies == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return nil, err
	}
	msgID, ok := metaid.ParseMessageID(msg.TargetMessage.ID).(metaid.ParsedFBMessageID)
	if !ok {
		return nil, fmt.Errorf("unexpected parsed message ID type")
	}
	resp, err := ic.Client.SendReaction(ctx, &slidetypes.CreateReactionRequest{Input: slidetypes.ReactionInput{
		Emoji:          msg.PreHandleResp.Emoji,
		ItemID:         "",
		MessageID:      msgID.ID,
		ReactionStatus: slidetypes.ReactionStatusCreated,
		ThreadID:       meta.IGID,
	}})
	if err != nil {
		return nil, err
	}
	var reactionTS time.Time
	for _, react := range resp.Message.Reactions {
		if react.SenderFBID == metaid.ParseUserLoginID(ic.UserLogin.ID) {
			reactionTS = react.ReactionTimestampMS.Time
			break
		}
	}
	return &database.Reaction{
		Timestamp: reactionTS,
	}, nil
}

func (ic *IGClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	if ic.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	meta := msg.Portal.Metadata.(*metaid.PortalMetadata)
	if meta.IGID == "" {
		// TODO fetch?
		return fmt.Errorf("portal metadata missing IGID")
	}
	msgID, ok := metaid.ParseMessageID(msg.TargetReaction.MessageID).(metaid.ParsedFBMessageID)
	if !ok {
		return fmt.Errorf("unexpected parsed message ID type")
	}
	_, err := ic.Client.SendReaction(ctx, &slidetypes.CreateReactionRequest{Input: slidetypes.ReactionInput{
		Emoji:          msg.TargetReaction.Emoji,
		ItemID:         "",
		MessageID:      msgID.ID,
		ReactionStatus: slidetypes.ReactionStatusDeleted,
		ThreadID:       meta.IGID,
	}})
	return err
}

func (ic *IGClient) HandleMatrixEdit(ctx context.Context, edit *bridgev2.MatrixEdit) error {
	if ic.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	meta, err := ic.ensureIGID(ctx, edit.Portal)
	if err != nil {
		return err
	}
	otid := getOTID(edit.InputTransactionID)
	messageID, ok := metaid.ParseMessageID(edit.EditTarget.ID).(metaid.ParsedFBMessageID)
	if !ok {
		return fmt.Errorf("unexpected parsed message ID type")
	}
	_, err = ic.Client.EditMessage(ctx, &slidetypes.EditMessageRequest{
		ThreadID:           meta.IGID,
		TargetItemID:       "",
		Body:               slidetypes.SensitiveString{Value: edit.Content.Body},
		OfflineThreadingID: strconv.FormatInt(otid, 10),
		TargetMessageID:    messageID.ID,
	})
	if err != nil {
		return err
	}
	edit.EditTarget.EditCount++
	return nil
}

func (ic *IGClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	if ic.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return err
	}
	messageID, ok := metaid.ParseMessageID(msg.TargetMessage.ID).(metaid.ParsedFBMessageID)
	if !ok {
		return fmt.Errorf("unexpected parsed message ID type")
	}
	_, err = ic.Client.UnsendMessage(ctx, &slidetypes.UnsendMessageRequest{
		MessageID: messageID.ID,
		SendData: slidetypes.SendData{
			ThreadID: meta.IGThreadID,
		},
	})
	return err
}

func (ic *IGClient) HandleMatrixReadReceipt(ctx context.Context, receipt *bridgev2.MatrixReadReceipt) error {
	if ic.LoginMeta.Cookies == nil {
		return bridgev2.ErrNotLoggedIn
	}
	meta, err := ic.ensureIGID(ctx, receipt.Portal)
	if err != nil {
		return err
	}
	if !receipt.ReadUpTo.After(receipt.LastRead) {
		return nil
	}
	readUpTo := receipt.ExactMessage
	if readUpTo == nil {
		var err error
		readUpTo, err = receipt.Portal.Bridge.DB.Message.GetLastNonFakePartAtOrBeforeTime(ctx, receipt.Portal.PortalKey, receipt.ReadUpTo)
		if err != nil {
			return fmt.Errorf("failed to get last read message: %w", err)
		} else if readUpTo == nil {
			return fmt.Errorf("last read message not found")
		}
	}
	messageID, ok := metaid.ParseMessageID(readUpTo.ID).(metaid.ParsedFBMessageID)
	if !ok {
		return fmt.Errorf("unexpected parsed message ID type")
	}
	resp1, err := ic.Client.MarkRead(ctx, &slidetypes.MarkReadRequest{
		Metadata: slidetypes.MarkReadMetadata{IGThreadIGID: meta.IGThreadID},
		Data: slidetypes.MarkReadData{
			MessageID: messageID.ID,
			ItemID:    ptr.Ptr(""),
		},
	})
	if err != nil {
		return err
	}
	resp2, err := ic.Client.MarkReadValidation(ctx, &slidetypes.MarkReadRequest{
		Metadata: slidetypes.MarkReadMetadata{IGThreadIGID: meta.IGThreadID},
		Data: slidetypes.MarkReadData{
			MessageID:          messageID.ID,
			MessageTimestampMS: jsontime.UnixMilliString{Time: readUpTo.Timestamp},
		},
	})
	if err != nil {
		return err
	}
	zerolog.Ctx(ctx).Trace().
		Any("mark_read_resp", resp1).
		Any("validation_resp", resp2).
		Msg("Sent read receipt to Instagram")
	return nil
}

func (ic *IGClient) HandleMatrixDeleteChat(ctx context.Context, chat *bridgev2.MatrixDeleteChat) error {
	meta, err := ic.ensureIGID(ctx, chat.Portal)
	if err != nil {
		// TODO treat 404 as success? (thread already deleted)
		return err
	}

	_, err = ic.Client.DeleteThread(ctx, &slidetypes.DeleteThreadRequest{
		ThreadID:   meta.IGID,
		MarkAsSpam: false,
	})
	return err
}

func (ic *IGClient) HandleMatrixAcceptMessageRequest(ctx context.Context, msg *bridgev2.MatrixAcceptMessageRequest) error {
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		// TODO treat 404 as success? (thread already deleted)
		return err
	}

	_, err = ic.Client.AcceptMessageRequest(ctx, &slidetypes.AcceptMessageRequestRequest{
		ThreadID:           meta.IGID,
		IGInboxFolder:      nil,
		OfflineThreadingID: strconv.FormatInt(getOTID(msg.InputTransactionID), 10),
	})
	return err
}

func (ic *IGClient) HandleMatrixRoomName(ctx context.Context, msg *bridgev2.MatrixRoomName) (bool, error) {
	if msg.Portal.RoomType == database.RoomTypeDM {
		return false, fmt.Errorf("renaming not supported in DMs")
	}
	// Note: technically this should use the IGID, but only groups can be renamed
	// and the IGID is always the same as the FB thread key there.
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	err := ic.Client.EditGroupTitle(ctx, strconv.FormatInt(threadID, 10), msg.Content.Name)
	return err == nil, err
}

func (ic *IGClient) HandleMatrixRoomAvatar(ctx context.Context, msg *bridgev2.MatrixRoomAvatar) (bool, error) {
	if msg.Portal.RoomType == database.RoomTypeDM {
		return false, fmt.Errorf("changing avatar not supported in DMs")
	}
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	if msg.Content.URL == "" {
		err := ic.Client.RemoveGroupAvatar(ctx, strconv.FormatInt(threadID, 10))
		if err != nil {
			return false, fmt.Errorf("failed to remove Instagram avatar: %w", err)
		}
		return true, nil
	}
	data, err := ic.Main.Bridge.Bot.DownloadMedia(ctx, msg.Content.URL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to download avatar: %w", err)
	}
	err = ic.Client.EditGroupAvatar(ctx, strconv.FormatInt(threadID, 10), data)
	if err != nil {
		return false, fmt.Errorf("failed to set Instagram avatar: %w", err)
	}
	return true, nil
}

func (ic *IGClient) HandleMatrixMembership(ctx context.Context, msg *bridgev2.MatrixMembershipChange) (*bridgev2.MatrixMembershipResult, error) {
	/*if msg.Portal.RoomType == database.RoomTypeDM {
		return nil, errors.New("cannot change members for DM")
	}

	var targetID int64
	switch target := msg.Target.(type) {
	case *bridgev2.Ghost:
		targetID = metaid.ParseUserID(target.ID)
	case *bridgev2.UserLogin:
		targetID = metaid.ParseUserLoginID(target.ID)
	default:
		return nil, fmt.Errorf("unknown membership target type %T", target)
	}
	if targetID == 0 {
		return nil, fmt.Errorf("invalid target user ID")
	}

	portalMeta := msg.Portal.Metadata.(*metaid.PortalMetadata)
	threadID := metaid.ParseFBPortalID(msg.Portal.ID)
	var task socket.Task
	switch msg.Type {
	case bridgev2.Invite:
		task = &socket.AddParticipantsTask{
			ThreadKey:  threadID,
			ContactIDs: []int64{targetID},
			SyncGroup:  1,
		}
	case bridgev2.Kick:
		task = &socket.RemoveParticipantTask{
			ThreadID:  threadID,
			ContactID: targetID,
		}
	default:
		return nil, nil
	}
	_, err := m.Client.ExecuteTasks(ctx, task)
	if err != nil {
		return nil, err
	}*/
	return &bridgev2.MatrixMembershipResult{}, fmt.Errorf("not implemented")
}

func (ic *IGClient) HandleMatrixTyping(ctx context.Context, msg *bridgev2.MatrixTyping) error {
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return err
	}
	return ic.Client.SetTyping(ctx, meta.IGID, msg.IsTyping)
}

func (ic *IGClient) HandleMute(ctx context.Context, msg *bridgev2.MatrixMute) error {
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return err
	}
	var muteSeconds int
	dur := msg.Content.GetMuteDuration()
	if dur <= 0 {
		muteSeconds = int(dur)
	} else {
		muteSeconds = int(dur.Seconds())
	}
	_, err = ic.Client.MuteThread(ctx, &slidetypes.MuteThreadRequest{
		ThreadID:           meta.IGID,
		MuteSeconds:        muteSeconds,
		OfflineThreadingID: strconv.FormatInt(getOTID(msg.InputTransactionID), 10),
	})
	return err
}

func (ic *IGClient) HandleRoomTag(ctx context.Context, msg *bridgev2.MatrixRoomTag) error {
	meta, err := ic.ensureIGID(ctx, msg.Portal)
	if err != nil {
		return err
	}
	_, pinned := msg.Content.Tags[event.RoomTagFavourite]
	_, err = ic.Client.PinThread(ctx, &slidetypes.PinThreadRequest{
		ThreadID: meta.IGID,
		Pin:      pinned,
	})
	return err
}
