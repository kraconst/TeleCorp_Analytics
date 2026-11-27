-- Deduplicate any pre-existing unresolved alerts per (chat_id, type), keeping the earliest id.
UPDATE alerts
SET resolved_at = NOW()
WHERE resolved_at IS NULL
  AND id NOT IN (
      SELECT MIN(id)
      FROM alerts
      WHERE resolved_at IS NULL
      GROUP BY chat_id, type
  );

CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_chat_type_active_unique
    ON alerts (chat_id, type)
    WHERE resolved_at IS NULL;
