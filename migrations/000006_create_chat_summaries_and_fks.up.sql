-- Foreign key from messages to chats
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_messages_chat'
    ) THEN
        ALTER TABLE messages
        ADD CONSTRAINT fk_messages_chat
        FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Foreign key from alerts to chats
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_alerts_chat'
    ) THEN
        ALTER TABLE alerts
        ADD CONSTRAINT fk_alerts_chat
        FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Chat Summaries
CREATE TABLE IF NOT EXISTS chat_summaries (
    chat_id BIGINT PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    topic TEXT NOT NULL DEFAULT '',
    agreements TEXT NOT NULL DEFAULT '',
    next_step TEXT NOT NULL DEFAULT '',
    open_questions TEXT NOT NULL DEFAULT '',
    risk VARCHAR(32) NOT NULL DEFAULT 'low' CHECK (risk IN ('low', 'medium', 'high')),
    last_message_id BIGINT NOT NULL DEFAULT 0,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_summaries_generated_at ON chat_summaries(generated_at DESC);

