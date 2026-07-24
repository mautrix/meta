-- v12 (compatible with v11+): Persist Facebook thread key to E2EE JID mappings

CREATE TABLE meta_hybrid_thread (
    bridge_id     TEXT   NOT NULL,
    login_id      TEXT   NOT NULL,
    fb_thread_key BIGINT NOT NULL,
    thread_jid    BIGINT NOT NULL,
    thread_type   BIGINT NOT NULL,
    message_request BOOLEAN NOT NULL DEFAULT false,

    PRIMARY KEY (bridge_id, login_id, fb_thread_key),
    CONSTRAINT meta_hybrid_thread_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);
