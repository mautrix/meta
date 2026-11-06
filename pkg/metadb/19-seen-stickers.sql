-- v19 (compatible with v11+): Per-login seen Facebook stickers
CREATE TABLE meta_seen_sticker (
    bridge_id  TEXT    NOT NULL,
    login_id   TEXT    NOT NULL,
    sticker_id BIGINT  NOT NULL,
    seen_at    BIGINT  NOT NULL,
    cdn_url    TEXT    NOT NULL,
    mxc        TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    mime       TEXT    NOT NULL,
    width      INTEGER NOT NULL,
    height     INTEGER NOT NULL,

    PRIMARY KEY (bridge_id, login_id, sticker_id),
    CONSTRAINT meta_seen_sticker_user_login_fkey FOREIGN KEY (bridge_id, login_id)
        REFERENCES user_login (bridge_id, id) ON DELETE CASCADE ON UPDATE CASCADE
);
