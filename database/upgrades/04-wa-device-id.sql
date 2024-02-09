-- v4 (compatible with v3+): Store WhatsApp device ID for users
ALTER TABLE "user" ADD COLUMN wa_device_id INTEGER;
