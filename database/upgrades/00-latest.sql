-- v0 -> v3: Latest revision

CREATE TABLE portal (
    thread_id   BIGINT  NOT NULL,
    receiver    BIGINT  NOT NULL,
    thread_type INTEGER NOT NULL,
    mxid        TEXT,

    name        TEXT    NOT NULL,
    avatar_id   TEXT    NOT NULL,
    avatar_url  TEXT    NOT NULL,
    name_set    BOOLEAN NOT NULL DEFAULT false,
    avatar_set  BOOLEAN NOT NULL DEFAULT false,

    encrypted     BOOLEAN NOT NULL DEFAULT false,
    relay_user_id TEXT    NOT NULL,

    oldest_message_id TEXT    NOT NULL,
    oldest_message_ts BIGINT  NOT NULL,
    more_to_backfill  BOOLEAN NOT NULL,

    PRIMARY KEY (thread_id, receiver),
    CONSTRAINT portal_mxid_unique UNIQUE(mxid)
);

CREATE TABLE puppet (
    id         BIGINT  NOT NULL PRIMARY KEY,
    name       TEXT    NOT NULL,
    username   TEXT    NOT NULL,
    avatar_id  TEXT    NOT NULL,
    avatar_url TEXT    NOT NULL,
    name_set   BOOLEAN NOT NULL DEFAULT false,
    avatar_set BOOLEAN NOT NULL DEFAULT false,

    contact_info_set BOOLEAN NOT NULL DEFAULT false,

    custom_mxid  TEXT,
    access_token TEXT NOT NULL,

    CONSTRAINT puppet_custom_mxid_unique UNIQUE(custom_mxid)
);

CREATE TABLE "user" (
    mxid    TEXT   NOT NULL PRIMARY KEY,
    meta_id BIGINT,
    cookies jsonb,

    management_room TEXT,
    space_room      TEXT,

    CONSTRAINT user_meta_id_unique UNIQUE(meta_id)
);

CREATE TABLE user_portal (
    user_mxid        TEXT NOT NULL,
    portal_thread_id BIGINT NOT NULL,
    portal_receiver  BIGINT NOT NULL,

    in_space BOOLEAN NOT NULL DEFAULT false,

    PRIMARY KEY (user_mxid, portal_thread_id, portal_receiver),
    CONSTRAINT user_portal_user_fkey FOREIGN KEY (user_mxid)
        REFERENCES "user"(mxid) ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT user_portal_portal_fkey FOREIGN KEY (portal_thread_id, portal_receiver)
        REFERENCES portal(thread_id, receiver) ON UPDATE CASCADE ON DELETE CASCADE
);

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

CREATE TABLE message (
    id              TEXT    NOT NULL,
    part_index      INTEGER NOT NULL,
    thread_id       BIGINT  NOT NULL,
    thread_receiver BIGINT  NOT NULL,
    msg_sender      BIGINT  NOT NULL,
    otid            BIGINT  NOT NULL,

    mxid    TEXT NOT NULL,
    mx_room TEXT NOT NULL,

    timestamp  BIGINT NOT NULL,
    edit_count BIGINT NOT NULL,

    PRIMARY KEY (id, part_index, thread_receiver),
    CONSTRAINT message_portal_fkey FOREIGN KEY (thread_id, thread_receiver)
        REFERENCES portal(thread_id, receiver) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT message_puppet_fkey FOREIGN KEY (msg_sender)
        REFERENCES puppet(id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT message_mxid_unique UNIQUE (mxid)
);

CREATE TABLE reaction (
    message_id      TEXT    NOT NULL,
    -- Part index is not used by reactions, but is required for the foreign key
    _part_index     INTEGER NOT NULL DEFAULT 0,
    thread_id       BIGINT  NOT NULL,
    thread_receiver BIGINT  NOT NULL,
    reaction_sender BIGINT  NOT NULL,

    emoji TEXT NOT NULL,

    mxid    TEXT NOT NULL,
    mx_room TEXT NOT NULL,

    PRIMARY KEY (message_id, thread_receiver, reaction_sender),
    CONSTRAINT reaction_message_fkey FOREIGN KEY (message_id, _part_index, thread_receiver)
        REFERENCES message (id, part_index, thread_receiver) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT reaction_puppet_fkey FOREIGN KEY (reaction_sender)
        REFERENCES puppet(id) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT reaction_portal_fkey FOREIGN KEY (thread_id, thread_receiver)
        REFERENCES portal(thread_id, receiver) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT reaction_mxid_unique UNIQUE (mxid)
);
