-- v15 (compatible with v11+): Keep hybrid thread mappings when a login is deleted and added again
ALTER TABLE meta_hybrid_thread ADD COLUMN user_mxid TEXT;
UPDATE meta_hybrid_thread SET user_mxid = (
    SELECT user_mxid FROM user_login
    WHERE user_login.bridge_id = meta_hybrid_thread.bridge_id AND user_login.id = meta_hybrid_thread.login_id
);
ALTER TABLE meta_hybrid_thread DROP CONSTRAINT meta_hybrid_thread_user_login_fkey;
ALTER TABLE meta_hybrid_thread ADD CONSTRAINT meta_hybrid_thread_user_fkey FOREIGN KEY (bridge_id, user_mxid)
    REFERENCES "user" (bridge_id, mxid) ON DELETE CASCADE ON UPDATE CASCADE;
