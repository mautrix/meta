-- v15 (compatible with v11+): Store Messenger thread cutover mappings
CREATE TABLE meta_thread_cutover (
    bridge_id                 TEXT    NOT NULL,
    login_id                  TEXT    NOT NULL,
    thread_jid                BIGINT  NOT NULL,
    open_thread_key           BIGINT  NOT NULL,
    show_open_message_history BOOLEAN NOT NULL,

    PRIMARY KEY (bridge_id, login_id, thread_jid),
    CONSTRAINT meta_thread_cutover_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);

UPDATE backfill_task
SET batch_count = 0, is_done = false, queue_done = false, cursor = NULL,
    oldest_message_id = NULL, dispatched_at = NULL, completed_at = NULL,
    next_dispatch_min_ts = 0
WHERE EXISTS (
    SELECT 1 FROM portal
    JOIN user_login ON user_login.bridge_id = portal.bridge_id
        AND user_login.id = backfill_task.user_login_id
    WHERE portal.bridge_id = backfill_task.bridge_id
        AND portal.id = backfill_task.portal_id
        AND portal.receiver = backfill_task.portal_receiver
        AND CAST(portal.metadata->>'thread_type' AS INTEGER) IN (15, 16)
        AND COALESCE(user_login.metadata->'history_device'->>'id', '') <> ''
);
