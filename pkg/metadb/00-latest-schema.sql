-- v0 -> v3 (compatible with v1+): Latest schema
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

-- On Instagram every FBID has two IGIDs associated with it
-- e.g.         FBID 17843099136558503
--      "short" IGID 76594782502
--      "long"  IGID 340282366841710301244276018116713645479
--
-- It appears these are user IDs and thread IDs respectively
--
-- These tables let us cache those values as they are not currently
-- known to ever change, and then we don't need to fetch them again
-- for a user/thread
--
-- Primary key on the igid since we primarily use fbid everywhere and
-- only need a way to translate from igid when we receive igid from
-- legacy API endpoints
--
-- Index on the fbid because occasionally we have to go the other way,
-- for example looking up FB contacts on Instagram

CREATE TABLE meta_instagram_user_id (
  igid TEXT PRIMARY KEY,
  fbid BIGINT NOT NULL UNIQUE
);

CREATE TABLE meta_instagram_thread_id (
  igid TEXT PRIMARY KEY,
  fbid BIGINT NOT NULL UNIQUE
);
