-- v6 (compatible with v1+): Delete folder portals

-- only: postgres
DELETE FROM portal WHERE (metadata->>'thread_type')::INTEGER = 6;
-- only: sqlite
DELETE FROM portal WHERE metadata->>'thread_type' = 6;
