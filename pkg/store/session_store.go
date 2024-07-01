package store

import (
	"context"
	"database/sql"
	"fmt"

	"go.mau.fi/util/dbutil"

	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/types"
)

type MetaSession struct {
	MetaID     int64
	WADeviceID uint16
	Cookies    *cookies.Cookies
}

func (s *MetaSession) Scan(row dbutil.Scannable) (*MetaSession, error) {
	var metaID sql.NullInt64
	var waDeviceID sql.NullInt32
	var platform sql.NullString
	var newCookies cookies.Cookies

	err := row.Scan(
		&metaID,
		&platform,
		&waDeviceID,
		&dbutil.JSON{Data: &newCookies},
	)
	if err != nil {
		return nil, err
	}

	if platform.String == "instagram" {
		newCookies.Platform = types.Instagram
	} else if platform.String == "facebook" {
		newCookies.Platform = types.Facebook
	} else {
		// TODO: Fix this err
		return nil, fmt.Errorf("unknown platform %s", platform.String)
	}

	if newCookies.IsLoggedIn() {
		s.Cookies = &newCookies
	}

	s.MetaID = metaID.Int64
	s.WADeviceID = uint16(waDeviceID.Int32)

	return s, nil
}

const (
	getUserByMetaIDQuery = `SELECT meta_id, platform, wa_device_id, cookies FROM "meta_session" WHERE meta_id=$1`
	insertUserQuery      = `INSERT INTO "meta_session" (meta_id, platform, wa_device_id, cookies) VALUES ($1, $2, $3, $4)`
)

type MetaSessionQuery struct {
	*dbutil.QueryHelper[*MetaSession]
}

func (q *MetaSessionQuery) GetByMetaID(ctx context.Context, metaID int64) (*MetaSession, error) {
	return q.QueryOne(ctx, getUserByMetaIDQuery, metaID)
}

func (q *MetaSessionQuery) Insert(ctx context.Context, session *MetaSession) error {
	platform := "instagram"
	if session.Cookies.Platform == types.Facebook {
		platform = "facebook"
	}
	return q.Exec(ctx, insertUserQuery, session.MetaID, platform, session.WADeviceID, dbutil.JSON{Data: &session.Cookies})
}

func (q *MetaSessionQuery) Delete(ctx context.Context, metaID int64) error {
	return q.Exec(ctx, `DELETE FROM "meta_session" WHERE meta_id=$1`, metaID)
}