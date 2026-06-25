-- v7 (compatible with v1+): Add Instagram chat ID table
CREATE TABLE meta_instagram_chat_id (
    igid TEXT PRIMARY KEY,
    fbid BIGINT NOT NULL UNIQUE
);
