-- v19 (compatible with v11+): Persist the Messenger app-installation identity used for logins
CREATE TABLE meta_messenger_login_device (
    bridge_id        TEXT NOT NULL,
    user_mxid        TEXT NOT NULL,
    app_id           TEXT NOT NULL,
    device_id        TEXT NOT NULL,
    family_device_id TEXT NOT NULL,
    machine_id       TEXT NOT NULL,
    attestation_key  TEXT NOT NULL,
    usdid            TEXT NOT NULL,

    PRIMARY KEY (bridge_id, user_mxid, app_id),
    CONSTRAINT meta_messenger_login_device_user_fkey FOREIGN KEY (bridge_id, user_mxid)
        REFERENCES "user" (bridge_id, mxid) ON DELETE CASCADE ON UPDATE CASCADE
);
