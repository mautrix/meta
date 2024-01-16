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

package database

import (
	"context"
	"database/sql"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/messagix/table"
)

const (
	portalBaseSelect = `
		SELECT thread_id, receiver, thread_type, mxid,
		       name, avatar_id, avatar_url, name_set, avatar_set,
		       encrypted, relay_user_id
		FROM portal
	`
	getPortalByMXIDQuery       = portalBaseSelect + `WHERE mxid=$1`
	getPortalByThreadIDQuery   = portalBaseSelect + `WHERE thread_id=$1 AND (receiver=$2 OR receiver=0)`
	getPortalsByReceiver       = portalBaseSelect + `WHERE receiver=$1`
	getPortalsByOtherUser      = portalBaseSelect + `WHERE thread_id=$1 AND thread_type IN (1, 7, 10, 13, 15)`
	getAllPortalsWithMXIDQuery = portalBaseSelect + `WHERE mxid IS NOT NULL`
	getChatsNotInSpaceQuery    = `
		SELECT thread_id FROM portal
		    LEFT JOIN user_portal ON portal.thread_id=user_portal.portal_thread_id AND portal.receiver=user_portal.portal_receiver
		WHERE mxid<>'' AND receiver=$1 AND (user_portal.in_space=false OR user_portal.in_space IS NULL)
	`
	insertPortalQuery = `
		INSERT INTO portal (
			thread_id, receiver, thread_type, mxid,
			name, avatar_id, avatar_url, name_set, avatar_set,
			encrypted, relay_user_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	updatePortalQuery = `
		UPDATE portal SET
			thread_type=$3, mxid=$4,
			name=$5, avatar_id=$6, avatar_url=$7, name_set=$8, avatar_set=$9,
			encrypted=$10, relay_user_id=$11
		WHERE thread_id=$1 AND receiver=$2
	`
	deletePortalQuery = `DELETE FROM portal WHERE thread_id=$1 AND receiver=$2`
)

type PortalQuery struct {
	*dbutil.QueryHelper[*Portal]
}

type PortalKey struct {
	ThreadID int64
	Receiver int64
}

type Portal struct {
	qh *dbutil.QueryHelper[*Portal]

	PortalKey
	ThreadType  table.ThreadType
	MXID        id.RoomID
	Name        string
	AvatarID    string
	AvatarURL   id.ContentURI
	NameSet     bool
	AvatarSet   bool
	Encrypted   bool
	RelayUserID id.UserID
}

func newPortal(qh *dbutil.QueryHelper[*Portal]) *Portal {
	return &Portal{qh: qh}
}

func (pq *PortalQuery) GetByMXID(ctx context.Context, mxid id.RoomID) (*Portal, error) {
	return pq.QueryOne(ctx, getPortalByMXIDQuery, mxid)
}

func (pq *PortalQuery) GetByThreadID(ctx context.Context, pk PortalKey) (*Portal, error) {
	return pq.QueryOne(ctx, getPortalByThreadIDQuery, pk.ThreadID, pk.Receiver)
}

func (pq *PortalQuery) FindPrivateChatsWith(ctx context.Context, userID int64) ([]*Portal, error) {
	return pq.QueryMany(ctx, getPortalsByOtherUser, userID)
}

func (pq *PortalQuery) FindPrivateChatsOf(ctx context.Context, receiver int64) ([]*Portal, error) {
	return pq.QueryMany(ctx, getPortalsByReceiver, receiver)
}

func (pq *PortalQuery) GetAllWithMXID(ctx context.Context) ([]*Portal, error) {
	return pq.QueryMany(ctx, getAllPortalsWithMXIDQuery)
}

func (pq *PortalQuery) FindPrivateChatsNotInSpace(ctx context.Context, receiver int64) ([]PortalKey, error) {
	rows, err := pq.GetDB().Query(ctx, getChatsNotInSpaceQuery, receiver)
	if err != nil {
		return nil, err
	}
	return dbutil.NewRowIter(rows, func(rows dbutil.Scannable) (key PortalKey, err error) {
		err = rows.Scan(&key.ThreadID)
		key.Receiver = receiver
		return
	}).AsList()
}

func (p *Portal) IsPrivateChat() bool {
	return p.ThreadType.IsOneToOne()
}

func (p *Portal) Scan(row dbutil.Scannable) (*Portal, error) {
	var mxid sql.NullString
	err := row.Scan(
		&p.ThreadID,
		&p.Receiver,
		&p.ThreadType,
		&mxid,
		&p.Name,
		&p.AvatarID,
		&p.AvatarURL,
		&p.NameSet,
		&p.AvatarSet,
		&p.Encrypted,
		&p.RelayUserID,
	)
	if err != nil {
		return nil, err
	}
	p.MXID = id.RoomID(mxid.String)
	return p, nil
}

func (p *Portal) sqlVariables() []any {
	return []any{
		p.ThreadID,
		p.Receiver,
		p.ThreadType,
		dbutil.StrPtr(p.MXID),
		p.Name,
		p.AvatarID,
		&p.AvatarURL,
		p.NameSet,
		p.AvatarSet,
		p.Encrypted,
		p.RelayUserID,
	}
}

func (p *Portal) Insert(ctx context.Context) error {
	return p.qh.Exec(ctx, insertPortalQuery, p.sqlVariables()...)
}

func (p *Portal) Update(ctx context.Context) error {
	return p.qh.Exec(ctx, updatePortalQuery, p.sqlVariables()...)
}

func (p *Portal) Delete(ctx context.Context) error {
	return p.qh.Exec(ctx, deletePortalQuery, p.ThreadID, p.Receiver)
}
