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
	"time"

	"github.com/rs/zerolog"
)

const (
	getIsInSpaceQuery = `SELECT in_space FROM user_portal WHERE user_mxid=$1 AND portal_thread_id=$2 AND portal_receiver=$3`
	setIsInSpaceQuery = `
		INSERT INTO user_portal (user_mxid, portal_thread_id, portal_receiver, in_space) VALUES ($1, $2, $3, true)
		ON CONFLICT (user_mxid, portal_thread_id, portal_receiver) DO UPDATE SET in_space=true
	`
	putBackfillTask = `
		INSERT INTO user_portal (user_mxid, portal_thread_id, portal_receiver, backfill_priority, backfill_max_pages, backfill_dispatched_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_mxid, portal_thread_id, portal_receiver) DO UPDATE
			SET backfill_priority=excluded.backfill_priority, backfill_max_pages=excluded.backfill_max_pages, backfill_dispatched_at=excluded.backfill_dispatched_at
	`
	getNextBackfillTask = `
		SELECT portal_thread_id, portal_receiver, backfill_priority, backfill_max_pages, backfill_dispatched_at
		FROM user_portal
		WHERE backfill_max_pages=-1 OR backfill_max_pages>0
		ORDER BY backfill_priority DESC, backfill_dispatched_at LIMIT 1
	`
)

func (u *User) IsInSpace(ctx context.Context, portal PortalKey) bool {
	u.inSpaceCacheLock.Lock()
	defer u.inSpaceCacheLock.Unlock()
	if cached, ok := u.inSpaceCache[portal]; ok {
		return cached
	}
	var inSpace bool
	err := u.qh.GetDB().QueryRow(ctx, getIsInSpaceQuery, u.MXID, portal.ThreadID, portal.Receiver).Scan(&inSpace)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		zerolog.Ctx(ctx).Err(err).
			Str("user_id", u.MXID.String()).
			Any("portal_key", portal).
			Msg("Failed to query in space status")
		return false
	}
	u.inSpaceCache[portal] = inSpace
	return inSpace
}

func (u *User) MarkInSpace(ctx context.Context, portal PortalKey) {
	u.inSpaceCacheLock.Lock()
	defer u.inSpaceCacheLock.Unlock()
	err := u.qh.Exec(ctx, setIsInSpaceQuery, u.MXID, portal.ThreadID, portal.Receiver)
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Str("user_id", u.MXID.String()).
			Any("portal_key", portal).
			Msg("Failed to update in space status")
	} else {
		u.inSpaceCache[portal] = true
	}
}

type BackfillTask struct {
	Key          PortalKey
	Priority     int
	MaxPages     int
	DispatchedAt time.Time
}

func (u *User) PutBackfillTask(ctx context.Context, task BackfillTask) {
	err := u.qh.Exec(ctx, putBackfillTask, u.MXID, task.Key.ThreadID, task.Key.Receiver, task.Priority, task.MaxPages, task.DispatchedAt.UnixMilli())
	if err != nil {
		zerolog.Ctx(ctx).Err(err).
			Str("user_id", u.MXID.String()).
			Any("portal_key", task.Key).
			Msg("Failed to save backfill task")
	}
}

func (u *User) GetNextBackfillTask(ctx context.Context) (*BackfillTask, error) {
	var task BackfillTask
	var dispatchedAt int64
	err := u.qh.GetDB().QueryRow(ctx, getNextBackfillTask).Scan(&task.Key.ThreadID, &task.Key.Receiver, &task.Priority, &task.MaxPages, &dispatchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	task.DispatchedAt = time.UnixMilli(dispatchedAt)
	return &task, nil
}

func (u *User) RemoveInSpaceCache(key PortalKey) {
	u.inSpaceCacheLock.Lock()
	defer u.inSpaceCacheLock.Unlock()
	delete(u.inSpaceCache, key)
}
