-- v5 (compatible with v3+): Store WhatsApp server name for portals and puppets
ALTER TABLE portal ADD COLUMN whatsapp_server TEXT NOT NULL DEFAULT '';
ALTER TABLE puppet ADD COLUMN whatsapp_server TEXT NOT NULL DEFAULT '';
