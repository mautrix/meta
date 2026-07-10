-- v11 (compatible with v11+): Fix Instagram chat and thread ID tables again

CREATE TABLE new_meta_instagram_thread_id (
    bridge_id TEXT   NOT NULL,
    igid      TEXT   NOT NULL,
    fbid      BIGINT NOT NULL,
    login     TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT meta_ig_thread_fbid_unique UNIQUE (fbid, login),
    CONSTRAINT meta_ig_thread_user_login_fkey FOREIGN KEY (bridge_id, login)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE TABLE new_meta_instagram_chat_id (
    bridge_id TEXT   NOT NULL,
    igid      TEXT   NOT NULL,
    fbid      BIGINT NOT NULL,
    login     TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT meta_ig_chat_fbid_unique UNIQUE (fbid, login),
    CONSTRAINT meta_ig_chat_user_login_fkey FOREIGN KEY (bridge_id, login)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);

INSERT INTO new_meta_instagram_thread_id (bridge_id, igid, fbid, login)
SELECT '', igid, fbid, login FROM meta_instagram_thread_id;

INSERT INTO new_meta_instagram_chat_id (bridge_id, igid, fbid, login)
SELECT '', igid, fbid, login FROM meta_instagram_chat_id;

DROP TABLE meta_instagram_chat_id;
DROP TABLE meta_instagram_thread_id;

ALTER TABLE new_meta_instagram_chat_id RENAME TO meta_instagram_chat_id;
ALTER TABLE new_meta_instagram_thread_id RENAME TO meta_instagram_thread_id;
