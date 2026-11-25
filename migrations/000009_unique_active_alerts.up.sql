CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_chat_type_active_unique
    ON alerts (chat_id, type)
    WHERE resolved_at IS NULL;
