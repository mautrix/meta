-- v13 (compatible with v12+): Persist the native Instagram app-installation identity

CREATE TABLE meta_instagram_login_device (
	bridge_id         TEXT NOT NULL,
	user_mxid         TEXT NOT NULL,
	phone_id          TEXT NOT NULL,
	device_id         TEXT NOT NULL,
	advertising_id    TEXT NOT NULL,
	android_device_id TEXT NOT NULL,
	machine_id        TEXT NOT NULL,

	PRIMARY KEY (bridge_id, user_mxid),
	CONSTRAINT meta_instagram_login_device_user_fkey FOREIGN KEY (bridge_id, user_mxid)
		REFERENCES "user" (bridge_id, mxid) ON DELETE CASCADE ON UPDATE CASCADE
);
