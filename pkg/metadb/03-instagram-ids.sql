-- v3 (compatible with v1+): Add Instagram legacy ID tables
CREATE TABLE meta_instagram_user_id (
  igid TEXT PRIMARY KEY,
  fbid BIGINT NOT NULL UNIQUE
);

CREATE TABLE meta_instagram_thread_id (
  igid TEXT PRIMARY KEY,
  fbid BIGINT NOT NULL UNIQUE
);
