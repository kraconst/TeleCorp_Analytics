CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_contacts_name_trgm ON contacts USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_contacts_username_trgm ON contacts USING gin (username gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_messages_text_trgm ON messages USING gin (text gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_chats_awaiting_reply_since ON chats (awaiting_reply_since ASC) WHERE awaiting_reply_since IS NOT NULL;
