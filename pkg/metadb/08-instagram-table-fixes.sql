-- v8 (compatible with v1+): Fix scoping of Instagram chat and thread ID tables
DROP TABLE meta_instagram_chat_id;
DROP TABLE meta_instagram_thread_id;

CREATE TABLE meta_instagram_chat_id (
    igid  TEXT   NOT NULL,
    fbid  BIGINT NOT NULL,
    login TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT ig_chat_fbid_unique UNIQUE (fbid, login)
);

CREATE TABLE meta_instagram_thread_id (
    igid      TEXT   NOT NULL,
    fbid      BIGINT NOT NULL,
    login     TEXT   NOT NULL,

    PRIMARY KEY (igid, login),
    CONSTRAINT ig_thread_fbid_unique UNIQUE (fbid, login)
);
