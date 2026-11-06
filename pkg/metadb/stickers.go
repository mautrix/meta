package metadb

import (
	"context"
	"time"

	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/id"
)

type SeenSticker struct {
	ID     int64
	SeenAt time.Time
	CDNURL string
	MXC    id.ContentURIString
	Body   string
	MIME   string
	Width  int
	Height int
}

func (db *MetaDB) PutSeenSticker(ctx context.Context, login networkid.UserLoginID, s SeenSticker) error {
	db.seenStickerLock.Lock(login)
	defer db.seenStickerLock.Unlock(login)
	return db.DoTxn(ctx, nil, func(ctx context.Context) error {
		_, err := db.Exec(ctx, `INSERT INTO meta_seen_sticker
			(bridge_id, login_id, sticker_id, seen_at, cdn_url, mxc, body, mime, width, height)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (bridge_id, login_id, sticker_id) DO UPDATE SET
			seen_at=CASE WHEN excluded.seen_at > meta_seen_sticker.seen_at THEN excluded.seen_at ELSE meta_seen_sticker.seen_at END,
			cdn_url=excluded.cdn_url,
			mxc=CASE WHEN meta_seen_sticker.mxc='' THEN excluded.mxc ELSE meta_seen_sticker.mxc END,
			body=excluded.body, mime=CASE WHEN meta_seen_sticker.mxc='' THEN excluded.mime ELSE meta_seen_sticker.mime END,
			width=excluded.width, height=excluded.height`,
			db.BridgeID, login, s.ID, s.SeenAt.UnixMilli(), s.CDNURL, s.MXC, s.Body, s.MIME, s.Width, s.Height)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, `DELETE FROM meta_seen_sticker WHERE bridge_id=$1 AND login_id=$2
			AND sticker_id NOT IN (SELECT sticker_id FROM meta_seen_sticker
			WHERE bridge_id=$1 AND login_id=$2 ORDER BY seen_at DESC, sticker_id DESC LIMIT 500)`, db.BridgeID, login)
		return err
	})
}

func (db *MetaDB) GetSeenStickers(ctx context.Context, login networkid.UserLoginID) ([]SeenSticker, error) {
	rows, err := db.Query(ctx, `SELECT sticker_id, cdn_url, mxc, body, mime, width, height FROM meta_seen_sticker
		WHERE bridge_id=$1 AND login_id=$2 ORDER BY seen_at DESC, sticker_id DESC LIMIT 500`, db.BridgeID, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stickers := make([]SeenSticker, 0)
	for rows.Next() {
		var s SeenSticker
		if err = rows.Scan(&s.ID, &s.CDNURL, &s.MXC, &s.Body, &s.MIME, &s.Width, &s.Height); err != nil {
			return nil, err
		}
		stickers = append(stickers, s)
	}
	return stickers, rows.Err()
}

func (db *MetaDB) SetSeenStickerMXC(ctx context.Context, login networkid.UserLoginID, stickerID int64, mxc id.ContentURIString, mime string) error {
	_, err := db.Exec(ctx, `UPDATE meta_seen_sticker SET mxc=$4, mime=$5
	WHERE bridge_id=$1 AND login_id=$2 AND sticker_id=$3 AND mxc=''`, db.BridgeID, login, stickerID, mxc, mime)
	return err
}
