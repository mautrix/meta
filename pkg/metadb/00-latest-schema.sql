-- v0 -> v2 (compatible with v1+): Latest schema
CREATE TABLE meta_thread (
    parent_key BIGINT NOT NULL,
    thread_key BIGINT NOT NULL,
    message_id TEXT   NOT NULL,

    PRIMARY KEY (thread_key),
    CONSTRAINT meta_thread_message_id_unique UNIQUE (message_id)
);

CREATE TABLE meta_reconnection_state (
    bridge_id TEXT  NOT NULL,
    login_id  TEXT  NOT NULL,
    state     jsonb NOT NULL,

    PRIMARY KEY (bridge_id, login_id),
    CONSTRAINT meta_reconnection_state_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE
);
