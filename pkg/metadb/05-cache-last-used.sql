-- v5 (compatible with v1+): Add last used time for reconnection state
ALTER TABLE meta_reconnection_state ADD COLUMN last_used BIGINT;
