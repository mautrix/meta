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
	"time"

	"go.mau.fi/util/dbutil"
	"maunium.net/go/mautrix/id"
)

const (
	getMessageByMXIDQuery = `
		SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp FROM message
		WHERE mxid=$1
	`
	getMessagePartByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp FROM message
        WHERE id=$1 AND part_index=$2 AND thread_receiver=$3
	`
	getLastMessagePartByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp FROM message
        WHERE id=$1 AND thread_receiver=$2
        ORDER BY part_index DESC LIMIT 1
	`
	getLastPartByTimestampQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp FROM message
        WHERE thread_id=$1 AND thread_receiver=$2 AND timestamp<=$3
        ORDER BY timestamp DESC, part_index DESC LIMIT 1
	`
	getAllMessagePartsByIDQuery = `
        SELECT id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp FROM message
        WHERE id=$1 AND thread_receiver=$2
	`
	insertMessageQuery = `
		INSERT INTO message (id, part_index, thread_id, thread_receiver, msg_sender, otid, mxid, mx_room, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	deleteMessageQuery = `
        DELETE FROM message
        WHERE id=$1 AND thread_receiver=$2 AND part_index=$3
	`
)

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

func (msg *Message) Scan(row dbutil.Scannable) (*Message, error) {
	var timestamp int64
	err := row.Scan(
		&msg.ID, &msg.PartIndex, &msg.ThreadID, &msg.ThreadReceiver, &msg.Sender, &msg.OTID, &msg.MXID, &msg.RoomID, &timestamp,
	)
	if err != nil {
		return nil, err
	}
	msg.Timestamp = time.UnixMilli(timestamp)
	return msg, nil
}

func (msg *Message) sqlVariables() []any {
	return []any{msg.ID, msg.PartIndex, msg.ThreadID, msg.ThreadReceiver, msg.Sender, msg.OTID, msg.MXID, msg.RoomID, msg.Timestamp.UnixMilli()}
}

func (msg *Message) Insert(ctx context.Context) error {
	return msg.qh.Exec(ctx, insertMessageQuery, msg.sqlVariables()...)
}

func (msg *Message) Delete(ctx context.Context) error {
	return msg.qh.Exec(ctx, deleteMessageQuery, msg.ID, msg.ThreadReceiver, msg.PartIndex)
}
