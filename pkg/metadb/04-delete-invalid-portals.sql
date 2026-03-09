-- v4 (compatible with v1+): Delete invalid portals with no thread type

-- only: postgres
DELETE FROM portal WHERE (metadata->>'thread_type')::INTEGER = 0 AND mxid IS NULL;
-- only: sqlite
DELETE FROM portal WHERE metadata->>'thread_type' = 0 AND mxid IS NULL;
