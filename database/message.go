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
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"
)

const (
	getMessageByMXIDQuery = `
		SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
		WHERE mxid=$1
	`
	getMessagePartByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
        WHERE id=$1 AND part_index=$2 AND thread_receiver=$3
	`
	getLastMessagePartByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
        WHERE id=$1 AND thread_receiver=$2
        ORDER BY part_index DESC LIMIT 1
	`
	getLastPartByTimestampQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
        WHERE thread_id=$1 AND thread_receiver=$2 AND timestamp<=$3
        ORDER BY timestamp DESC, part_index DESC LIMIT 1
	`
	getAllMessagePartsByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
        WHERE id=$1 AND thread_receiver=$2
	`
	getMessagesBetweenTimeQuery = `
		SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count FROM message
		WHERE thread_id=$1 AND thread_receiver=$2 AND timestamp>$3 AND timestamp<=$4 AND part_index=0
		ORDER BY timestamp ASC
	`
	findEditTargetPortalFromMessageQuery = `
        SELECT thread_id, thread_receiver FROM message
        WHERE id=$1 AND (thread_receiver=$2 OR thread_receiver=0) AND part_index=0
	`
	insertMessageQuery = `
		INSERT INTO message (id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp, edit_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	insertQueryValuePlaceholder   = `($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	bulkInsertPlaceholderTemplate = `($%d, $%d, $1, $2, $%d, $%d, $%d, $3, $%d, $%d)`
	deleteMessageQuery            = `
        DELETE FROM message
        WHERE id=$1 AND thread_receiver=$2 AND part_index=$3
	`
	updateMessageEditCountQuery = `
		UPDATE message SET edit_count=$4 WHERE id=$1 AND thread_receiver=$2 AND part_index=$3
	`
)

func init() {
	if strings.ReplaceAll(insertMessageQuery, insertQueryValuePlaceholder, "meow") == insertMessageQuery {
		panic("Bulk insert query placeholder not found")
	}
}

type MessageQuery struct {
	*dbutil.QueryHelper[*Message]
}

type Message struct {
	qh *dbutil.QueryHelper[*Message]

	ID             string
	PartIndex      int
	ThreadID       int64
	ThreadReceiver int64
	Sender         int64
	OTID           int64

	MXID   id.EventID
	RoomID id.RoomID

	Timestamp time.Time
	EditCount int64
}

func newMessage(qh *dbutil.QueryHelper[*Message]) *Message {
	return &Message{qh: qh}
}

func (mq *MessageQuery) GetByMXID(ctx context.Context, mxid id.EventID) (*Message, error) {
	return mq.QueryOne(ctx, getMessageByMXIDQuery, mxid)
}

func (mq *MessageQuery) GetByID(ctx context.Context, id string, partIndex int, receiver int64) (*Message, error) {
	return mq.QueryOne(ctx, getMessagePartByIDQuery, id, partIndex, receiver)
}

func (mq *MessageQuery) GetLastByTimestamp(ctx context.Context, key PortalKey, timestamp time.Time) (*Message, error) {
	return mq.QueryOne(ctx, getLastPartByTimestampQuery, key.ThreadID, key.Receiver, timestamp.UnixMilli())
}

func (mq *MessageQuery) GetLastPartByID(ctx context.Context, id string, receiver int64) (*Message, error) {
	return mq.QueryOne(ctx, getLastMessagePartByIDQuery, id, receiver)
}

func (mq *MessageQuery) GetAllPartsByID(ctx context.Context, id string, receiver int64) ([]*Message, error) {
	return mq.QueryMany(ctx, getAllMessagePartsByIDQuery, id, receiver)
}

func (mq *MessageQuery) GetAllBetweenTimestamps(ctx context.Context, key PortalKey, min, max time.Time) ([]*Message, error) {
	return mq.QueryMany(ctx, getMessagesBetweenTimeQuery, key.ThreadID, key.Receiver, min.UnixMilli(), max.UnixMilli())
}

func (mq *MessageQuery) FindEditTargetPortal(ctx context.Context, id string, receiver int64) (key PortalKey, err error) {
	err = mq.GetDB().QueryRow(ctx, findEditTargetPortalFromMessageQuery, id, receiver).Scan(&key.ThreadID, &key.Receiver)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

type bulkInserter[T any] interface {
	GetDB() *dbutil.Database
	BulkInsertChunk(context.Context, PortalKey, id.RoomID, []T) error
}

const BulkInsertChunkSize = 100

func doBulkInsert[T any](q bulkInserter[T], ctx context.Context, thread PortalKey, roomID id.RoomID, entries []T) error {
	if len(entries) == 0 {
		return nil
	}
	return q.GetDB().DoTxn(ctx, nil, func(ctx context.Context) error {
		for i := 0; i < len(entries); i += BulkInsertChunkSize {
			messageChunk := entries[i:]
			if len(messageChunk) > BulkInsertChunkSize {
				messageChunk = messageChunk[:BulkInsertChunkSize]
			}
			err := q.BulkInsertChunk(ctx, thread, roomID, messageChunk)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (mq *MessageQuery) BulkInsert(ctx context.Context, thread PortalKey, roomID id.RoomID, messages []*Message) error {
	return doBulkInsert[*Message](mq, ctx, thread, roomID, messages)
}

func (mq *MessageQuery) BulkInsertChunk(ctx context.Context, thread PortalKey, roomID id.RoomID, messages []*Message) error {
	if len(messages) == 0 {
		return nil
	}
	placeholders := make([]string, len(messages))
	values := make([]any, 3+len(messages)*7)
	values[0] = thread.ThreadID
	values[1] = thread.Receiver
	values[2] = roomID
	for i, msg := range messages {
		baseIndex := 3 + i*7
		placeholders[i] = fmt.Sprintf(bulkInsertPlaceholderTemplate, baseIndex+1, baseIndex+2, baseIndex+3, baseIndex+4, baseIndex+5, baseIndex+6, baseIndex+7)
		values[baseIndex] = msg.ID
		values[baseIndex+1] = msg.PartIndex
		values[baseIndex+2] = msg.Sender
		values[baseIndex+3] = msg.OTID
		values[baseIndex+4] = msg.MXID
		values[baseIndex+5] = msg.Timestamp.UnixMilli()
		values[baseIndex+6] = msg.EditCount
	}
	query := strings.ReplaceAll(insertMessageQuery, insertQueryValuePlaceholder, strings.Join(placeholders, ","))
	return mq.Exec(ctx, query, values...)
}

func (msg *Message) Scan(row dbutil.Scannable) (*Message, error) {
	var timestamp int64
	err := row.Scan(
		&msg.ID, &msg.PartIndex, &msg.ThreadID, &msg.ThreadReceiver, &msg.Sender, &msg.OTID, &msg.MXID, &msg.RoomID, &timestamp, &msg.EditCount,
	)
	if err != nil {
		return nil, err
	}
	msg.Timestamp = time.UnixMilli(timestamp)
	return msg, nil
}

func (msg *Message) sqlVariables() []any {
	return []any{msg.ID, msg.PartIndex, msg.ThreadID, msg.ThreadReceiver, msg.Sender, msg.OTID, msg.MXID, msg.RoomID, msg.Timestamp.UnixMilli(), msg.EditCount}
}

func (msg *Message) Insert(ctx context.Context) error {
	return msg.qh.Exec(ctx, insertMessageQuery, msg.sqlVariables()...)
}

func (msg *Message) Delete(ctx context.Context) error {
	return msg.qh.Exec(ctx, deleteMessageQuery, msg.ID, msg.ThreadReceiver, msg.PartIndex)
}

func (msg *Message) UpdateEditCount(ctx context.Context, count int64) error {
	msg.EditCount = count
	return msg.qh.Exec(ctx, updateMessageEditCountQuery, msg.ID, msg.ThreadReceiver, msg.PartIndex, msg.EditCount)
}

func (msg *Message) EditTimestamp() int64 {
	return msg.EditCount
}

func (msg *Message) UpdateEditTimestamp(ctx context.Context, ts int64) error {
	return msg.UpdateEditCount(ctx, ts)
}
