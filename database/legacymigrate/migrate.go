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

package legacymigrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/exerrors"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/database"
	"go.mau.fi/mautrix-meta/messagix/table"
)

type Insertable interface {
	Insert(context.Context) error
}

type ToNewable[T Insertable] interface {
	ToNew(*database.Database) T
}

type LegacyUser struct {
	MXID       id.UserID
	NoticeRoom id.RoomID
}

func (lu *LegacyUser) ToNew(db *database.Database) *database.User {
	dbUser := db.User.New()
	dbUser.MXID = lu.MXID
	dbUser.ManagementRoom = lu.NoticeRoom
	return dbUser
}

type LegacyPortal struct {
	FBID       int64
	FBReceiver int64
	FBType     string
	MXID       sql.NullString
	Name       sql.NullString
	PhotoID    sql.NullString
	AvatarURL  sql.NullString
	Encrypted  bool
	NameSet    bool
	AvatarSet  bool
}

func (lp *LegacyPortal) ToNew(db *database.Database) *database.Portal {
	dbPortal := db.Portal.New()
	dbPortal.PortalKey.ThreadID = lp.FBID
	dbPortal.PortalKey.Receiver = lp.FBReceiver
	switch lp.FBType {
	case "USER":
		dbPortal.ThreadType = table.ONE_TO_ONE
	case "GROUP":
		dbPortal.ThreadType = table.GROUP_THREAD
	default:
		dbPortal.ThreadType = table.UNKNOWN_THREAD_TYPE
	}
	dbPortal.MXID = id.RoomID(lp.MXID.String)
	dbPortal.Name = lp.Name.String
	dbPortal.AvatarID = lp.PhotoID.String
	dbPortal.AvatarURL, _ = id.ParseContentURI(lp.AvatarURL.String)
	dbPortal.Encrypted = lp.Encrypted
	dbPortal.NameSet = lp.NameSet
	dbPortal.AvatarSet = lp.AvatarSet
	return dbPortal
}

type LegacyPuppet struct {
	FBID      int64
	Name      sql.NullString
	PhotoID   sql.NullString
	PhotoMXC  sql.NullString
	NameSet   bool
	AvatarSet bool
}

func (lp *LegacyPuppet) ToNew(db *database.Database) *database.Puppet {
	dbPuppet := db.Puppet.New()
	dbPuppet.ID = lp.FBID
	dbPuppet.Name = lp.Name.String
	dbPuppet.AvatarID = lp.PhotoID.String
	dbPuppet.AvatarURL, _ = id.ParseContentURI(lp.PhotoMXC.String)
	dbPuppet.NameSet = lp.NameSet
	dbPuppet.AvatarSet = lp.AvatarSet
	return dbPuppet
}

type LegacyMessage struct {
	FBID       string
	FBTxnID    sql.NullInt64
	Index      int
	FBChat     int64
	FBReceiver int64
	FBSender   int64
	Timestamp  int64
	MXID       id.EventID
	MXRoom     id.RoomID
}

func (lm *LegacyMessage) ToNew(db *database.Database) *database.Message {
	dbMessage := db.Message.New()
	dbMessage.ID = lm.FBID
	dbMessage.OTID = lm.FBTxnID.Int64
	dbMessage.PartIndex = lm.Index
	dbMessage.ThreadID = lm.FBChat
	dbMessage.ThreadReceiver = lm.FBReceiver
	dbMessage.Sender = lm.FBSender
	dbMessage.Timestamp = time.UnixMilli(lm.Timestamp)
	dbMessage.MXID = lm.MXID
	dbMessage.RoomID = lm.MXRoom
	return dbMessage
}

type LegacyReaction struct {
	FBMsgID    string
	FBThreadID int64
	FBReceiver int64
	FBSender   int64
	Reaction   string
	MXID       id.EventID
	MXRoom     id.RoomID
}

func (lr *LegacyReaction) ToNew(db *database.Database) *database.Reaction {
	dbReaction := db.Reaction.New()
	dbReaction.MessageID = lr.FBMsgID
	dbReaction.ThreadID = lr.FBThreadID
	dbReaction.ThreadReceiver = lr.FBReceiver
	dbReaction.Sender = lr.FBSender
	dbReaction.Emoji = lr.Reaction
	dbReaction.MXID = lr.MXID
	dbReaction.RoomID = lr.MXRoom
	return dbReaction
}

type reinserter[T ToNewable[I], I Insertable] struct {
	db  *database.Database
	ctx context.Context
}

func (r reinserter[T, I]) do(m T) (bool, error) {
	return true, m.ToNew(r.db).Insert(r.ctx)
}

func Migrate(ctx context.Context, targetDB *database.Database, sourceDialect, sourceURI string) {
	log := zerolog.Ctx(ctx)
	sourceDB := exerrors.Must(dbutil.NewWithDialect(sourceURI, sourceDialect))
	sourceDB.Log = dbutil.ZeroLoggerPtr(log)

	oldDBOwner := exerrors.Must(dbutil.ScanSingleColumn[string](sourceDB.QueryRow(ctx, "SELECT owner FROM database_owner")))
	if oldDBOwner != "mautrix-facebook" {
		panic(fmt.Errorf("source database is %s, not mautrix-facebook", oldDBOwner))
	}
	oldDBVersion := exerrors.Must(dbutil.ScanSingleColumn[int](sourceDB.QueryRow(ctx, "SELECT version FROM version")))
	if oldDBVersion != 12 {
		panic(fmt.Errorf("source database is not on latest version (got %d, expected 12)", oldDBVersion))
	}
	log.Debug().
		Str("owner", oldDBOwner).
		Int("version", oldDBVersion).
		Msg("Source database version confirmed")

	log.Info().Msg("Upgrading target database")
	exerrors.PanicIfNotNil(targetDB.Upgrade(ctx))

	log.Info().Msg("Migrating data")
	origCtx := ctx
	exerrors.PanicIfNotNil(targetDB.DoTxn(ctx, nil, func(ctx context.Context) error {
		err := dbutil.NewSimpleReflectRowIter[LegacyUser](sourceDB.Query(origCtx, `
			SELECT mxid, notice_room FROM "user" WHERE notice_room<>''
		`)).Iter(reinserter[*LegacyUser, *database.User]{targetDB, ctx}.do)
		if err != nil {
			log.Error().Msg("Failed to copy users")
			return err
		}
		err = dbutil.NewSimpleReflectRowIter[LegacyPortal](sourceDB.Query(origCtx, `
			SELECT fbid, fb_receiver, fb_type, mxid, name, photo_id, avatar_url, encrypted, name_set, avatar_set FROM portal
		`)).Iter(reinserter[*LegacyPortal, *database.Portal]{targetDB, ctx}.do)
		if err != nil {
			log.Error().Msg("Failed to copy portals")
			return err
		}
		err = dbutil.NewSimpleReflectRowIter[LegacyPuppet](sourceDB.Query(origCtx, `
			SELECT fbid, name, photo_id, photo_mxc, name_set, avatar_set FROM puppet
		`)).Iter(reinserter[*LegacyPuppet, *database.Puppet]{targetDB, ctx}.do)
		if err != nil {
			log.Error().Msg("Failed to copy puppets")
			return err
		}
		err = dbutil.NewSimpleReflectRowIter[LegacyMessage](sourceDB.Query(origCtx, `
			SELECT fbid, fb_txn_id, "index", fb_chat, fb_receiver, fb_sender, timestamp, mxid, mx_room
			FROM message WHERE fbid<>'' AND fb_sender<>0
		`)).Iter(reinserter[*LegacyMessage, *database.Message]{targetDB, ctx}.do)
		if err != nil {
			log.Error().Msg("Failed to copy messages")
			return err
		}
		err = dbutil.NewSimpleReflectRowIter[LegacyReaction](sourceDB.Query(origCtx, `
			SELECT reaction.fb_msgid, message.fb_chat, reaction.fb_receiver,
			       reaction.fb_sender, reaction.reaction, reaction.mxid, reaction.mx_room
			FROM reaction
			JOIN message ON reaction.fb_msgid=message.fbid AND message."index"=0
			WHERE reaction.fb_sender<>0 AND message.fb_chat<>0 AND reaction.fb_msgid='mid.$gABC3ypFJHACT3lHXb2NrQyNVgAAq'
		`)).Iter(reinserter[*LegacyReaction, *database.Reaction]{targetDB, ctx}.do)
		if err != nil {
			log.Error().Msg("Failed to copy reactions")
			return err
		}
		return nil
	}))
	log.Info().Msg("Migration complete")
}
