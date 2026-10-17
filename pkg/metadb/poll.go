package metadb

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"maunium.net/go/mautrix/bridgev2/networkid"
)

func (db *MetaDB) GetPollMessageID(ctx context.Context, portal networkid.PortalKey, pollID int64) (networkid.MessageID, error) {
	var messageID networkid.MessageID
	err := db.QueryRow(ctx, `
  SELECT id FROM message
  WHERE bridge_id=$1 AND room_id=$2 AND (room_receiver=$3 OR room_receiver='')
    AND CAST(metadata->>'poll_id' AS TEXT)=$4
  ORDER BY part_id LIMIT 1
 `, db.BridgeID, portal.ID, portal.Receiver, strconv.FormatInt(pollID, 10)).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return messageID, err
}
