-- Partial unique index for active account assignment
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_assignments_active_unique
ON account_assignments (account_id)
WHERE to_time IS NULL;

-- Unique constraint on (company_id, phone) for accounts
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_accounts_company_phone'
    ) THEN
        ALTER TABLE accounts
        ADD CONSTRAINT uq_accounts_company_phone
        UNIQUE (company_id, phone);
    END IF;
END $$;

-- Check constraint on account status
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_status'
    ) THEN
        ALTER TABLE accounts
        ADD CONSTRAINT chk_accounts_status
        CHECK (status IN ('connecting', 'active', 'disconnected', 'banned'));
    END IF;
END $$;

