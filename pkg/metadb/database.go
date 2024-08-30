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
	"errors"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
)

type MetaDB struct {
	*dbutil.Database
}

func New(db *dbutil.Database, log zerolog.Logger) *MetaDB {
	db = db.Child("meta_version", table, dbutil.ZeroLogger(log))
	return &MetaDB{
		Database: db,
	}
}

var table dbutil.UpgradeTable

//go:embed *.sql
var upgrades embed.FS

func init() {
	table.RegisterFS(upgrades)
}

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
