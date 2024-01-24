-- v3: Add backfill queue
ALTER TABLE portal ADD COLUMN oldest_message_id TEXT NOT NULL DEFAULT '';
ALTER TABLE portal ADD COLUMN oldest_message_ts BIGINT NOT NULL DEFAULT 0;
ALTER TABLE portal ADD COLUMN more_to_backfill BOOL NOT NULL DEFAULT true;
UPDATE portal SET (oldest_message_id, oldest_message_ts) = (
    SELECT id, timestamp
    FROM message
    WHERE thread_id = portal.thread_id
      AND thread_receiver = portal.receiver
    ORDER BY timestamp ASC
    LIMIT 1
);
-- only: postgres for next 3 lines
ALTER TABLE portal ALTER COLUMN oldest_message_id DROP DEFAULT;
ALTER TABLE portal ALTER COLUMN oldest_message_ts DROP DEFAULT;
ALTER TABLE portal ALTER COLUMN more_to_backfill DROP DEFAULT;

ALTER TABLE user_portal ADD COLUMN backfill_priority INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_portal ADD COLUMN backfill_max_pages INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_portal ADD COLUMN backfill_dispatched_at BIGINT NOT NULL DEFAULT 0;
