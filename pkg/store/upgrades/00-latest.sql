-- v0 -> v1: Latest revision

CREATE TABLE meta_session (
    meta_id      BIGINT PRIMARY KEY,
    platform     TEXT NOT NULL,
    wa_device_id INTEGER,
    cookies      jsonb,

    -- platform must be "instagram" or "facebook"
    CONSTRAINT platform_check CHECK (platform = 'instagram' OR platform = 'facebook'),
    CONSTRAINT user_meta_id_unique UNIQUE(meta_id)
);
