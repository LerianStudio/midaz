-- Drop the transaction_backup_quarantine table and its index.
--
-- Uses IF EXISTS for idempotent rollback. Dropping the table discards any
-- quarantined financial copies; only run this rollback when the data has been
-- exported or is known to be reconciled.

DROP INDEX IF EXISTS idx_transaction_backup_quarantine_org_ledger;
DROP TABLE IF EXISTS transaction_backup_quarantine;
