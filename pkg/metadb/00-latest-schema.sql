-- v0 -> v12 (compatible with v11+): Latest schema
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
    last_used BIGINT,

    PRIMARY KEY (bridge_id, login_id),
    CONSTRAINT meta_reconnection_state_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE
);

CREATE TABLE meta_instagram_seq_id (
    bridge_id TEXT   NOT NULL,
    login_id  TEXT   NOT NULL,
    seq_id    BIGINT NOT NULL,
    timestamp BIGINT NOT NULL,

    PRIMARY KEY (bridge_id, login_id),
    CONSTRAINT meta_reconnection_state_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE
);

-- Short IG user ID <-> FB user ID
CREATE TABLE meta_instagram_user_id (
    igid TEXT PRIMARY KEY,
    fbid BIGINT NOT NULL UNIQUE
);

-- Long IG thread ID <-> FB thread key
CREATE TABLE meta_instagram_thread_id (
    bridge_id TEXT   NOT NULL,
    igid      TEXT   NOT NULL,
    fbid      BIGINT NOT NULL,
    login     TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT meta_ig_thread_fbid_unique UNIQUE (fbid, login),
    CONSTRAINT meta_ig_thread_user_login_fkey FOREIGN KEY (bridge_id, login)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);

-- Short IG chat ID <-> FB thread key.
-- For groups, these two are usually the same value.
-- For DMs, the fbid is the recipient user ID, but the igid is a different short ID.
CREATE TABLE meta_instagram_chat_id (
    bridge_id TEXT   NOT NULL,
    igid      TEXT   NOT NULL,
    fbid      BIGINT NOT NULL,
    login     TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT meta_ig_chat_fbid_unique UNIQUE (fbid, login),
    CONSTRAINT meta_ig_chat_user_login_fkey FOREIGN KEY (bridge_id, login)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE TABLE meta_instagram_reaction (
    bridge_id           TEXT   NOT NULL,
    portal_id           TEXT   NOT NULL,
    portal_receiver     TEXT   NOT NULL,
    target_message_id   TEXT   NOT NULL,
    reaction_sender     BIGINT NOT NULL,
    reaction_message_id TEXT   NOT NULL,

    PRIMARY KEY (bridge_id, portal_receiver, reaction_message_id),
    CONSTRAINT ig_reaction_portal_fkey FOREIGN KEY (bridge_id, portal_id, portal_receiver)
        REFERENCES portal (bridge_id, id, receiver) ON DELETE CASCADE ON UPDATE CASCADE
);

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
