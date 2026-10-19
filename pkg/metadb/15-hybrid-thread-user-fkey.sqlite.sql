-- v15 (compatible with v11+): Keep hybrid thread mappings when a login is deleted and added again
CREATE TABLE meta_hybrid_thread_new (
    bridge_id       TEXT    NOT NULL,
    login_id        TEXT    NOT NULL,
    user_mxid       TEXT,
    fb_thread_key   BIGINT  NOT NULL,
    thread_jid      BIGINT  NOT NULL,
    thread_type     BIGINT  NOT NULL,
    message_request BOOLEAN NOT NULL DEFAULT false,

    PRIMARY KEY (bridge_id, login_id, fb_thread_key),
    CONSTRAINT meta_hybrid_thread_user_fkey FOREIGN KEY (bridge_id, user_mxid)
        REFERENCES "user" (bridge_id, mxid) ON DELETE CASCADE ON UPDATE CASCADE
);

INSERT INTO meta_hybrid_thread_new (bridge_id, login_id, user_mxid, fb_thread_key, thread_jid, thread_type, message_request)
SELECT meta_hybrid_thread.bridge_id, login_id, user_login.user_mxid, fb_thread_key, thread_jid, thread_type, message_request
FROM meta_hybrid_thread
LEFT JOIN user_login ON user_login.bridge_id = meta_hybrid_thread.bridge_id AND user_login.id = meta_hybrid_thread.login_id;

DROP TABLE meta_hybrid_thread;
ALTER TABLE meta_hybrid_thread_new RENAME TO meta_hybrid_thread;
