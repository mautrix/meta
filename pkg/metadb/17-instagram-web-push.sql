-- v17 (compatible with v11+): Track Instagram web push delivery
CREATE TABLE meta_instagram_web_push (
    bridge_id     TEXT    NOT NULL,
    login_id      TEXT    NOT NULL,
    last_received BIGINT  NOT NULL,
    stale         BOOLEAN NOT NULL,

    PRIMARY KEY (bridge_id, login_id),
    CONSTRAINT meta_instagram_web_push_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);
