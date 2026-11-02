package metadb

import (
	"context"
	"database/sql"
	"errors"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

func (db *MetaDB) GetPollMessageID(ctx context.Context, portal networkid.PortalKey, pollID int64) (networkid.MessageID, error) {
	var messageID networkid.MessageID
	err := db.QueryRow(ctx, `
		SELECT message.id FROM meta_poll
		JOIN message ON message.rowid = meta_poll.message_rowid
		WHERE meta_poll.bridge_id = $1 AND meta_poll.portal_id = $2
			AND (meta_poll.portal_receiver = $3 OR meta_poll.portal_receiver = '') AND meta_poll.poll_id = $4
		ORDER BY meta_poll.portal_receiver = $3 DESC LIMIT 1
	`, db.BridgeID, portal.ID, portal.Receiver, pollID).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return messageID, err
}

func (db *MetaDB) PutPollMessage(ctx context.Context, portal networkid.PortalKey, pollID, messageRowID int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO meta_poll (bridge_id, portal_id, poll_id, portal_receiver, message_rowid)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (bridge_id, portal_id, poll_id, portal_receiver) DO UPDATE
		SET message_rowid = excluded.message_rowid
	`, db.BridgeID, portal.ID, pollID, portal.Receiver, messageRowID)
	return err
}
