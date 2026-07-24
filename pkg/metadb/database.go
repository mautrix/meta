// mautrix-meta - A Matrix-Facebook Messenger and Instagram DM puppeting bridge.
// Copyright (C) 2024 Tulir Asokan
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

package metadb

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exsync"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type MetaDB struct {
	*dbutil.Database

	BridgeID networkid.BridgeID

	igUserMap   *exsync.Map[string, int64]
	igChatMap   *exsync.Map[keyWithLogin, int64]
	igThreadMap *exsync.Map[keyWithLogin, int64]
}

type keyWithLogin struct {
	IGID  string
	Login networkid.UserLoginID
}

func New(bridgeID networkid.BridgeID, db *dbutil.Database, log zerolog.Logger) *MetaDB {
	db = db.Child("meta_version", table, dbutil.ZeroLogger(log))
	return &MetaDB{
		BridgeID: bridgeID,
		Database: db,

		igUserMap:   exsync.NewMap[string, int64](),
		igChatMap:   exsync.NewMap[keyWithLogin, int64](),
		igThreadMap: exsync.NewMap[keyWithLogin, int64](),
	}
}

var table = dbutil.BuildUpgradeTable().WithFS(upgrades).Finish()

//go:embed *.sql
var upgrades embed.FS

func (db *MetaDB) PutThread(ctx context.Context, parentKey, threadKey int64, messageID string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_thread (parent_key, thread_key, message_id)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, parentKey, threadKey, messageID)
	return err
}

func (db *MetaDB) GetThreadByKey(ctx context.Context, threadKey int64) (parentKey int64, messageID string, err error) {
	err = db.QueryRow(ctx, "SELECT parent_key, message_id FROM meta_thread WHERE thread_key = $1", threadKey).
		Scan(&parentKey, &messageID)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) GetThreadByMessage(ctx context.Context, messageID string) (threadKey int64, err error) {
	err = db.QueryRow(ctx, "SELECT thread_key FROM meta_thread WHERE message_id = $1", messageID).
		Scan(&threadKey)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) PutHybridThreadMapping(ctx context.Context, loginID networkid.UserLoginID, fbThreadKey, threadJID, threadType int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_hybrid_thread (bridge_id, login_id, fb_thread_key, thread_jid, thread_type)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bridge_id, login_id, fb_thread_key) DO UPDATE SET
			thread_jid = excluded.thread_jid,
			thread_type = CASE WHEN excluded.thread_type = 0 THEN meta_hybrid_thread.thread_type ELSE excluded.thread_type END
	`, db.BridgeID, loginID, fbThreadKey, threadJID, threadType)
	return err
}

func (db *MetaDB) GetHybridThreadJID(ctx context.Context, loginID networkid.UserLoginID, fbThreadKey int64) (threadJID, threadType int64, err error) {
	err = db.QueryRow(ctx, `
		SELECT thread_jid, thread_type FROM meta_hybrid_thread
		WHERE bridge_id = $1 AND login_id = $2 AND fb_thread_key = $3
	`, db.BridgeID, loginID, fbThreadKey).Scan(&threadJID, &threadType)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) SetHybridThreadMessageRequest(ctx context.Context, loginID networkid.UserLoginID, fbThreadKey int64, messageRequest bool) error {
	_, err := db.Exec(ctx, `
		UPDATE meta_hybrid_thread SET message_request = $4
		WHERE bridge_id = $1 AND login_id = $2 AND fb_thread_key = $3
	`, db.BridgeID, loginID, fbThreadKey, messageRequest)
	return err
}

func (db *MetaDB) GetHybridThreadInfoByJID(ctx context.Context, loginID networkid.UserLoginID, threadJID int64) (fbThreadKey int64, messageRequest bool, err error) {
	err = db.QueryRow(ctx, `
		SELECT fb_thread_key, message_request FROM meta_hybrid_thread
		WHERE bridge_id = $1 AND login_id = $2 AND thread_jid = $3
	`, db.BridgeID, loginID, threadJID).Scan(&fbThreadKey, &messageRequest)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (db *MetaDB) GetIGSeqID(ctx context.Context, loginID networkid.UserLoginID) (int64, time.Time, error) {
	var seqID, ts int64
	err := db.QueryRow(ctx, "SELECT seq_id, timestamp FROM meta_instagram_seq_id WHERE bridge_id = $1 AND login_id = $2", db.BridgeID, loginID).Scan(&seqID, &ts)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return 0, time.Time{}, err
	}
	return seqID, time.UnixMilli(ts), nil
}

func (db *MetaDB) PutIGSeqID(ctx context.Context, loginID networkid.UserLoginID, seqID int64, ts time.Time) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_instagram_seq_id (bridge_id, login_id, seq_id, timestamp)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (bridge_id, login_id) DO UPDATE SET seq_id = excluded.seq_id, timestamp = excluded.timestamp
		WHERE meta_instagram_seq_id.seq_id <= excluded.seq_id
	`, db.BridgeID, loginID, seqID, ts.UnixMilli())
	return err
}

func (db *MetaDB) PutReconnectionState(ctx context.Context, loginID networkid.UserLoginID, state json.RawMessage) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_reconnection_state (bridge_id, login_id, state)
		VALUES ($1, $2, $3)
		ON CONFLICT (bridge_id, login_id) DO UPDATE SET state = excluded.state, last_used = NULL
	`, db.BridgeID, loginID, string(state))
	return err
}

func (db *MetaDB) DeleteReconnectionState(ctx context.Context, loginID networkid.UserLoginID) error {
	_, err := db.Exec(ctx, `
		DELETE FROM meta_reconnection_state WHERE bridge_id = $1 AND login_id = $2
	`, db.BridgeID, loginID)
	if err != nil {
		return err
	}
	return db.DeleteIGSeqID(ctx, loginID)
}

func (db *MetaDB) DeleteIGSeqID(ctx context.Context, loginID networkid.UserLoginID) error {
	_, err := db.Exec(ctx, `
		DELETE FROM meta_instagram_seq_id WHERE bridge_id = $1 AND login_id = $2
	`, db.BridgeID, loginID)
	return err
}

func (db *MetaDB) GetReconnectionState(ctx context.Context, loginID networkid.UserLoginID) (state json.RawMessage, lastUsed time.Time, err error) {
	var stateStr string
	var lastUsedInt sql.NullInt64
	err = db.QueryRow(ctx, "SELECT state, last_used FROM meta_reconnection_state WHERE bridge_id = $1 AND login_id = $2", db.BridgeID, loginID).
		Scan(&stateStr, &lastUsedInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
		return
	}
	if lastUsedInt.Int64 != 0 {
		lastUsed = time.UnixMilli(lastUsedInt.Int64)
	}
	state = []byte(stateStr)
	_, err = db.Exec(ctx, "UPDATE meta_reconnection_state SET last_used = $3 WHERE bridge_id = $1 AND login_id = $2", db.BridgeID, loginID, time.Now().UnixMilli())
	if err != nil {
		err = fmt.Errorf("failed to update last used ts: %w", err)
	}
	return
}

func (db *MetaDB) GetFBIDForIGUser(ctx context.Context, igid string) (fbid int64, err error) {
	var ok bool
	fbid, ok = db.igUserMap.Get(igid)
	if ok {
		return fbid, nil
	}
	err = db.QueryRow(ctx, "SELECT fbid FROM meta_instagram_user_id WHERE igid = $1", igid).Scan(&fbid)
	if errors.Is(err, sql.ErrNoRows) {
		// return 0 if not cached
		err = nil
	} else {
		db.igUserMap.Set(igid, fbid)
	}
	return
}

func (db *MetaDB) GetIGUserForFBID(ctx context.Context, fbid int64) (igid string, err error) {
	err = db.QueryRow(ctx, "SELECT igid FROM meta_instagram_user_id WHERE fbid = $1", fbid).Scan(&igid)
	if errors.Is(err, sql.ErrNoRows) {
		// return "" if not cached
		err = nil
	}
	return
}

func (db *MetaDB) GetIGChatForFBID(ctx context.Context, fbid int64, login networkid.UserLoginID) (igid string, err error) {
	err = db.QueryRow(ctx, "SELECT igid FROM meta_instagram_chat_id WHERE fbid = $1 AND login = $2 AND bridge_id = $3", fbid, login, db.BridgeID).Scan(&igid)
	if errors.Is(err, sql.ErrNoRows) {
		// return "" if not cached
		err = nil
	}
	return
}

func (db *MetaDB) GetFBIDForIGThread(ctx context.Context, igid string, login networkid.UserLoginID) (fbid int64, err error) {
	var ok bool
	fbid, ok = db.igThreadMap.Get(keyWithLogin{igid, login})
	if ok {
		return fbid, nil
	}
	err = db.QueryRow(ctx, "SELECT fbid FROM meta_instagram_thread_id WHERE igid = $1 AND login = $2 AND bridge_id = $3", igid, login, db.BridgeID).Scan(&fbid)
	if errors.Is(err, sql.ErrNoRows) {
		// return 0 if not cached
		err = nil
	} else {
		db.igThreadMap.Set(keyWithLogin{igid, login}, fbid)
	}
	return
}

func (db *MetaDB) GetFBIDForIGChat(ctx context.Context, igid string, login networkid.UserLoginID) (fbid int64, err error) {
	var ok bool
	fbid, ok = db.igChatMap.Get(keyWithLogin{igid, login})
	if ok {
		return fbid, nil
	}
	err = db.QueryRow(ctx, "SELECT fbid FROM meta_instagram_chat_id WHERE igid = $1 AND login = $2 AND bridge_id = $3", igid, login, db.BridgeID).Scan(&fbid)
	if errors.Is(err, sql.ErrNoRows) {
		// return 0 if not cached
		err = nil
	} else {
		db.igChatMap.Set(keyWithLogin{igid, login}, fbid)
	}
	return
}

func (db *MetaDB) PutFBIDForIGUser(ctx context.Context, igid string, fbid int64) error {
	if igid == "" || igid == "0" || fbid == 0 {
		return nil
	}
	_, exists := db.igUserMap.GetOrSet(igid, fbid)
	if exists {
		return nil
	}
	// If the fbid gets set to a new value for an existing row,
	// that would be surprising. We don't currently expect these
	// values ever to change.
	_, err := db.Exec(ctx, "INSERT INTO meta_instagram_user_id (igid, fbid) VALUES ($1, $2) ON CONFLICT DO NOTHING", igid, fbid)
	return err
}

func (db *MetaDB) PutFBIDForIGThread(ctx context.Context, igid string, fbid int64, login networkid.UserLoginID) error {
	if igid == "" || igid == "0" || fbid == 0 {
		return nil
	}
	_, exists := db.igThreadMap.GetOrSet(keyWithLogin{igid, login}, fbid)
	if exists {
		return nil
	}
	_, err := db.Exec(ctx, "INSERT INTO meta_instagram_thread_id (bridge_id, igid, fbid, login) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", db.BridgeID, igid, fbid, login)
	return err
}

func (db *MetaDB) PutFBIDForIGChat(ctx context.Context, igid string, fbid int64, login networkid.UserLoginID) error {
	if igid == "" || igid == "0" || fbid == 0 {
		return nil
	}
	_, exists := db.igChatMap.GetOrSet(keyWithLogin{igid, login}, fbid)
	if exists {
		return nil
	}
	_, err := db.Exec(ctx, "INSERT INTO meta_instagram_chat_id (bridge_id, igid, fbid, login) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING", db.BridgeID, igid, fbid, login)
	return err
}

const (
	putIGReactionQuery = `
		INSERT INTO meta_instagram_reaction (bridge_id, portal_id, portal_receiver, target_message_id, reaction_sender, reaction_message_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT DO NOTHING
	`
	getIGReactionQuery = `
		SELECT target_message_id, reaction_sender
		FROM meta_instagram_reaction
		WHERE bridge_id = $1 AND portal_id = $2 AND portal_receiver = $3 AND reaction_message_id = $4
	`
)

func (db *MetaDB) PutIGReaction(ctx context.Context, portalKey networkid.PortalKey, targetMsgID string, sender int64, reactionMsgID string) error {
	_, err := db.Exec(ctx, putIGReactionQuery, db.BridgeID, portalKey.ID, portalKey.Receiver, targetMsgID, sender, reactionMsgID)
	return err
}

type IGReactionEntry struct {
	TargetMsgID   string
	Sender        int64
	ReactionMsgID string
}

func (ire *IGReactionEntry) GetMassInsertValues() [3]any {
	return [3]any{ire.TargetMsgID, ire.Sender, ire.ReactionMsgID}
}

var putIGReactionMassInsertBuilder = dbutil.NewMassInsertBuilder[*IGReactionEntry, [3]any](putIGReactionQuery, "($1, $2, $3, $%d, $%d, $%d)")

func (db *MetaDB) PutManyIGReactions(ctx context.Context, portalKey networkid.PortalKey, reactions []*IGReactionEntry) error {
	if len(reactions) == 0 {
		return nil
	}
	query, values := putIGReactionMassInsertBuilder.Build([3]any{db.BridgeID, portalKey.ID, portalKey.Receiver}, reactions)
	_, err := db.Exec(ctx, query, values...)
	return err
}

func (db *MetaDB) GetIGReactionTarget(ctx context.Context, portalKey networkid.PortalKey, reactionMsgID string) (targetMsgID string, sender int64, err error) {
	err = db.QueryRow(ctx, getIGReactionQuery, db.BridgeID, portalKey.ID, portalKey.Receiver, reactionMsgID).Scan(&targetMsgID, &sender)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
