-- v2: Store message edit count
ALTER TABLE message ADD COLUMN edit_count BIGINT NOT NULL DEFAULT 0;
-- only: postgres
ALTER TABLE message ALTER COLUMN edit_count DROP DEFAULT;
