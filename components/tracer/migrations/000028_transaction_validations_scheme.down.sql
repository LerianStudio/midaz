-- ============================================
-- Migration: 000028_transaction_validations_scheme (DOWN)
-- Description: Drop transaction_validation_scheme and the scheme column, and
--              restore NOT NULL on transaction_type when every row still
--              carries one.
-- Date: 2026-10-06
-- ============================================
--
-- This down is safe only before a writer that stores scheme alone has run:
-- transaction_validations forbids UPDATE and DELETE, so a row whose
-- transaction_type is NULL can neither be backfilled nor removed, and SET NOT
-- NULL over it would fail. The probe therefore reads the table itself (never
-- the catalog) and restores NOT NULL only when no such row exists; otherwise
-- transaction_type stays nullable, and the function and the scheme column are
-- dropped either way.
--
-- Lock wait: lock_timeout = 5s, transaction-local, for the same reason as the
-- up migration.

DROP FUNCTION IF EXISTS transaction_validation_scheme(VARCHAR, transaction_type_enum);

-- ACKNOWLEDGE: symmetric rollback of the expand step above; it removes only the column this migration adds
ALTER TABLE transaction_validations DROP COLUMN IF EXISTS scheme;

DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    IF NOT EXISTS (SELECT 1 FROM transaction_validations WHERE transaction_type IS NULL) THEN
        -- ACKNOWLEDGE: the column is named transaction_type; this restores NOT NULL and leaves the column type unchanged
        ALTER TABLE transaction_validations ALTER COLUMN transaction_type SET NOT NULL;
    END IF;
END $$;
