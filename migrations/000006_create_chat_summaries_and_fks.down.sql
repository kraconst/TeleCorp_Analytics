DROP TABLE IF EXISTS chat_summaries;
ALTER TABLE alerts DROP CONSTRAINT IF EXISTS fk_alerts_chat;
ALTER TABLE messages DROP CONSTRAINT IF EXISTS fk_messages_chat;

