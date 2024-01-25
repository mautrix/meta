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
) WHERE EXISTS(SELECT 1 FROM message WHERE thread_id = portal.thread_id AND thread_receiver = portal.receiver);
-- only: postgres for next 3 lines
ALTER TABLE portal ALTER COLUMN oldest_message_id DROP DEFAULT;
ALTER TABLE portal ALTER COLUMN oldest_message_ts DROP DEFAULT;
ALTER TABLE portal ALTER COLUMN more_to_backfill DROP DEFAULT;

CREATE TABLE backfill_task (
    portal_id       BIGINT NOT NULL,
    portal_receiver BIGINT NOT NULL,
    user_mxid       TEXT NOT NULL,

    priority       INTEGER NOT NULL,
    page_count     INTEGER NOT NULL,
    finished       BOOLEAN NOT NULL,
    dispatched_at  BIGINT  NOT NULL,
    completed_at   BIGINT  NOT NULL,
    cooldown_until BIGINT  NOT NULL,

    PRIMARY KEY (portal_id, portal_receiver, user_mxid),
    CONSTRAINT backfill_task_user_fkey FOREIGN KEY (user_mxid)
        REFERENCES "user" (mxid) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT backfill_task_portal_fkey FOREIGN KEY (portal_id, portal_receiver)
        REFERENCES portal (thread_id, receiver) ON UPDATE CASCADE ON DELETE CASCADE
);

ALTER TABLE "user" ADD COLUMN inbox_fetched BOOLEAN NOT NULL DEFAULT false;
-- only: postgres
ALTER TABLE "user" ALTER COLUMN inbox_fetched DROP DEFAULT;
