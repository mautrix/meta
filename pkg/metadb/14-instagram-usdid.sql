-- v14 (compatible with v13+): Persist Instagram's signed installation identity

ALTER TABLE meta_instagram_login_device ADD COLUMN usdid TEXT NOT NULL DEFAULT '';
ALTER TABLE meta_instagram_login_device ADD COLUMN usdid_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE meta_instagram_login_device ADD COLUMN usdid_private_key TEXT NOT NULL DEFAULT '';
ALTER TABLE meta_instagram_login_device ADD COLUMN usdid_registered BOOLEAN NOT NULL DEFAULT false;
