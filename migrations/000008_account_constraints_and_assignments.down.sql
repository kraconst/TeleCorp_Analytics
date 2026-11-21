DROP INDEX IF EXISTS idx_account_assignments_active_unique;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS uq_accounts_company_phone;
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_status;

