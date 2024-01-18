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
)

const (
	puppetBaseSelect = `
        SELECT id, name, username, avatar_id, avatar_url, name_set, avatar_set,
               contact_info_set, custom_mxid, access_token
        FROM puppet
	`
	getPuppetByMetaIDQuery     = puppetBaseSelect + `WHERE id=$1`
	getPuppetByCustomMXIDQuery = puppetBaseSelect + `WHERE custom_mxid=$1`
	getPuppetsWithCustomMXID   = puppetBaseSelect + `WHERE custom_mxid<>''`
	updatePuppetQuery          = `
		UPDATE puppet SET
			name=$2, username=$3, avatar_id=$4, avatar_url=$5, name_set=$6, avatar_set=$7,
			contact_info_set=$8, custom_mxid=$9, access_token=$10
		WHERE id=$1
	`
	insertPuppetQuery = `
		INSERT INTO puppet (
			id, name, username, avatar_id, avatar_url, name_set, avatar_set,
			contact_info_set, custom_mxid, access_token
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
)

type PuppetQuery struct {
	*dbutil.QueryHelper[*Puppet]
}

type Puppet struct {
	qh *dbutil.QueryHelper[*Puppet]

	ID        int64
	Name      string
	Username  string
	AvatarID  string
	AvatarURL id.ContentURI
	NameSet   bool
	AvatarSet bool

	ContactInfoSet bool

	CustomMXID  id.UserID
	AccessToken string
}

func newPuppet(qh *dbutil.QueryHelper[*Puppet]) *Puppet {
	return &Puppet{qh: qh}
}

func (pq *PuppetQuery) GetByID(ctx context.Context, id int64) (*Puppet, error) {
	return pq.QueryOne(ctx, getPuppetByMetaIDQuery, id)
}

func (pq *PuppetQuery) GetByCustomMXID(ctx context.Context, mxid id.UserID) (*Puppet, error) {
	return pq.QueryOne(ctx, getPuppetByCustomMXIDQuery, mxid)
}

func (pq *PuppetQuery) GetAllWithCustomMXID(ctx context.Context) ([]*Puppet, error) {
	return pq.QueryMany(ctx, getPuppetsWithCustomMXID)
}

func (p *Puppet) Scan(row dbutil.Scannable) (*Puppet, error) {
	var customMXID sql.NullString
	err := row.Scan(
		&p.ID,
		&p.Name,
		&p.Username,
		&p.AvatarID,
		&p.AvatarURL,
		&p.NameSet,
		&p.AvatarSet,
		&p.ContactInfoSet,
		&customMXID,
		&p.AccessToken,
	)
	if err != nil {
		return nil, nil
	}
	p.CustomMXID = id.UserID(customMXID.String)
	return p, nil
}

func (p *Puppet) sqlVariables() []any {
	return []any{
		p.ID,
		p.Name,
		p.Username,
		p.AvatarID,
		&p.AvatarURL,
		p.NameSet,
		p.AvatarSet,
		p.ContactInfoSet,
		dbutil.StrPtr(p.CustomMXID),
		p.AccessToken,
	}
}

func (p *Puppet) Insert(ctx context.Context) error {
	return p.qh.Exec(ctx, insertPuppetQuery, p.sqlVariables()...)
}

func (p *Puppet) Update(ctx context.Context) error {
	return p.qh.Exec(ctx, updatePuppetQuery, p.sqlVariables()...)
}
