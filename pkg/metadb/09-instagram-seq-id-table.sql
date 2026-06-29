-- v9 (compatible with v8+): Add separate table for Instagram socket seq IDs
CREATE TABLE meta_instagram_seq_id (
    bridge_id TEXT   NOT NULL,
    login_id  TEXT   NOT NULL,
    seq_id    BIGINT NOT NULL,
    timestamp BIGINT NOT NULL,

    PRIMARY KEY (bridge_id, login_id),
    CONSTRAINT meta_reconnection_state_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE
);
