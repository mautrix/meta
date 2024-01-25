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
	putBackfillTask = `
		INSERT INTO backfill_task (
			portal_id, portal_receiver, user_mxid, priority, page_count, finished,
			dispatched_at, completed_at, cooldown_until
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (portal_id, portal_receiver, user_mxid) DO UPDATE
			SET priority=excluded.priority, page_count=excluded.page_count, finished=excluded.finished,
			    dispatched_at=excluded.dispatched_at, completed_at=excluded.completed_at, cooldown_until=excluded.cooldown_until
	`
	getNextBackfillTask = `
		SELECT portal_id, portal_receiver, user_mxid, priority, page_count, finished, dispatched_at, completed_at, cooldown_until
		FROM backfill_task
		WHERE user_mxid=$1 AND finished=false AND cooldown_until<$2 AND (dispatched_at<$3 OR completed_at<>0)
		ORDER BY priority DESC, completed_at, dispatched_at LIMIT 1
	`
)

type BackfillTaskQuery struct {
	*dbutil.QueryHelper[*BackfillTask]
}

type BackfillTask struct {
	qh *dbutil.QueryHelper[*BackfillTask]

	Key           PortalKey
	UserMXID      id.UserID
	Priority      int
	PageCount     int
	Finished      bool
	DispatchedAt  time.Time
	CompletedAt   time.Time
	CooldownUntil time.Time
}

func newBackfillTask(qh *dbutil.QueryHelper[*BackfillTask]) *BackfillTask {
	return &BackfillTask{qh: qh}
}

func (btq *BackfillTaskQuery) NewWithValues(portalKey PortalKey, userID id.UserID) *BackfillTask {
	return &BackfillTask{
		qh: btq.QueryHelper,

		Key:          portalKey,
		UserMXID:     userID,
		DispatchedAt: time.Now(),
		CompletedAt:  time.Now(),
	}
}

func (btq *BackfillTaskQuery) GetNext(ctx context.Context, userID id.UserID) (*BackfillTask, error) {
	return btq.QueryOne(ctx, getNextBackfillTask, userID, time.Now().UnixMilli(), time.Now().Add(-15*time.Minute).UnixMilli())
}

func (task *BackfillTask) Scan(row dbutil.Scannable) (*BackfillTask, error) {
	var dispatchedAt, completedAt, cooldownUntil int64
	err := row.Scan(&task.Key.ThreadID, &task.Key.Receiver, &task.UserMXID, &task.Priority, &task.PageCount, &task.Finished, &dispatchedAt, &completedAt, &cooldownUntil)
	if err != nil {
		return nil, err
	}
	task.DispatchedAt = timeFromUnixMilli(dispatchedAt)
	task.CompletedAt = timeFromUnixMilli(completedAt)
	task.CooldownUntil = timeFromUnixMilli(cooldownUntil)
	return task, nil
}

func timeFromUnixMilli(unix int64) time.Time {
	if unix == 0 {
		return time.Time{}
	}
	return time.UnixMilli(unix)
}

func unixMilliOrZero(time time.Time) int64 {
	if time.IsZero() {
		return 0
	}
	return time.UnixMilli()
}

func (task *BackfillTask) sqlVariables() []any {
	return []any{
		task.Key.ThreadID,
		task.Key.Receiver,
		task.UserMXID,
		task.Priority,
		task.PageCount,
		task.Finished,
		unixMilliOrZero(task.DispatchedAt),
		unixMilliOrZero(task.CompletedAt),
		unixMilliOrZero(task.CooldownUntil),
	}
}

func (task *BackfillTask) Upsert(ctx context.Context) error {
	return task.qh.Exec(ctx, putBackfillTask, task.sqlVariables()...)
}
