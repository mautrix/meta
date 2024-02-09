-- v6 (compatible with v3+): Store last read timestamp for chats
ALTER TABLE user_portal ADD COLUMN last_read_ts BIGINT NOT NULL DEFAULT 0;
