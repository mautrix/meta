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
	"sync"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"

	"go.mau.fi/mautrix-meta/messagix/cookies"
)

const (
	getUserByMXIDQuery       = `SELECT mxid, meta_id, wa_device_id, cookies, inbox_fetched, management_room, space_room FROM "user" WHERE mxid=$1`
	getUserByMetaIDQuery     = `SELECT mxid, meta_id, wa_device_id, cookies, inbox_fetched, management_room, space_room FROM "user" WHERE meta_id=$1`
	getAllLoggedInUsersQuery = `SELECT mxid, meta_id, wa_device_id, cookies, inbox_fetched, management_room, space_room FROM "user" WHERE cookies IS NOT NULL`
	insertUserQuery          = `INSERT INTO "user" (mxid, meta_id, wa_device_id, cookies, inbox_fetched, management_room, space_room) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	updateUserQuery          = `UPDATE "user" SET meta_id=$2, wa_device_id=$3, cookies=$4, inbox_fetched=$5, management_room=$6, space_room=$7 WHERE mxid=$1`
)

type UserQuery struct {
	*dbutil.QueryHelper[*User]
}

type User struct {
	qh *dbutil.QueryHelper[*User]

	MXID           id.UserID
	MetaID         int64
	WADeviceID     uint16
	Cookies        cookies.Cookies
	InboxFetched   bool
	ManagementRoom id.RoomID
	SpaceRoom      id.RoomID

	inSpaceCache     map[PortalKey]bool
	inSpaceCacheLock sync.Mutex
}

func newUser(qh *dbutil.QueryHelper[*User]) *User {
	return &User{
		qh: qh,

		inSpaceCache: make(map[PortalKey]bool),
	}
}

func (uq *UserQuery) GetByMXID(ctx context.Context, mxid id.UserID) (*User, error) {
	return uq.QueryOne(ctx, getUserByMXIDQuery, mxid)
}

func (uq *UserQuery) GetByMetaID(ctx context.Context, id int64) (*User, error) {
	return uq.QueryOne(ctx, getUserByMetaIDQuery, id)
}

func (uq *UserQuery) GetAllLoggedIn(ctx context.Context) ([]*User, error) {
	return uq.QueryMany(ctx, getAllLoggedInUsersQuery)
}

func (u *User) sqlVariables() []any {
	return []any{u.MXID, dbutil.NumPtr(u.MetaID), u.WADeviceID, dbutil.JSON{Data: u.Cookies}, u.InboxFetched, dbutil.StrPtr(u.ManagementRoom), dbutil.StrPtr(u.SpaceRoom)}
}

func (u *User) Insert(ctx context.Context) error {
	return u.qh.Exec(ctx, insertUserQuery, u.sqlVariables()...)
}

func (u *User) Update(ctx context.Context) error {
	return u.qh.Exec(ctx, updateUserQuery, u.sqlVariables()...)
}

var NewCookies func() cookies.Cookies

func (u *User) Scan(row dbutil.Scannable) (*User, error) {
	var managementRoom, spaceRoom sql.NullString
	var metaID sql.NullInt64
	var waDeviceID sql.NullInt32
	scannedCookies := NewCookies()
	err := row.Scan(
		&u.MXID,
		&metaID,
		&waDeviceID,
		&dbutil.JSON{Data: scannedCookies},
		&u.InboxFetched,
		&managementRoom,
		&spaceRoom,
	)
	if err != nil {
		return nil, err
	}
	if scannedCookies.IsLoggedIn() {
		u.Cookies = scannedCookies
	}
	u.MetaID = metaID.Int64
	u.WADeviceID = uint16(waDeviceID.Int32)
	u.ManagementRoom = id.RoomID(managementRoom.String)
	u.SpaceRoom = id.RoomID(spaceRoom.String)
	return u, nil
}
