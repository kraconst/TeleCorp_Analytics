CREATE TABLE IF NOT EXISTS alerts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    -- Like messages.chat_id, this references the future chats table logically.
    -- The contacts/chats migration is developed on a separate feature branch.
    chat_id BIGINT NOT NULL CHECK (chat_id > 0),
    type VARCHAR(32) NOT NULL CHECK (type IN ('no_reply', 'complaint', 'refusal', 'negative', 'urgent')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_company_active
    ON alerts (company_id, created_at DESC, id DESC)
    WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_alerts_chat_id ON alerts (chat_id);
