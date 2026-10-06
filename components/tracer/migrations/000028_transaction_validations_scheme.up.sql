-- ============================================
-- Migration: 000028_transaction_validations_scheme
-- Description: Open the payment scheme of a validated transaction to any
--              normalized value without rewriting transaction_validations.
--              scheme holds the scheme as free-form text (A-Z, 0-9, _ and -,
--              up to 50 chars). transaction_type stays for rows written before
--              this column existed and for values inside its enum; readers
--              coalesce scheme over transaction_type through
--              transaction_validation_scheme(scheme, transaction_type).
-- Date: 2026-10-06
-- ============================================
--
-- Both ALTERs are metadata-only: dropping NOT NULL and adding a nullable column
-- without a default touch the catalog, never the heap, so the lock is held for
-- a catalog write and no row is rewritten.
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md):
-- DROP NOT NULL on a column that is already nullable is a no-op, the column is
-- added with IF NOT EXISTS, and the function uses CREATE OR REPLACE.
--
-- Lock wait: the ALTERs run in one DO block with lock_timeout = 5s, set
-- transaction-locally, so they fail fast with SQLSTATE 55P03 instead of queueing
-- the validate hot path behind an ACCESS EXCLUSIVE request.

DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    -- ACKNOWLEDGE: the column is named transaction_type; this drops NOT NULL and leaves the column type unchanged
    ALTER TABLE transaction_validations ALTER COLUMN transaction_type DROP NOT NULL;

    ALTER TABLE transaction_validations ADD COLUMN IF NOT EXISTS scheme VARCHAR(50) NULL;
END $$;

-- The effective scheme of a row. PostgreSQL classifies an enum-to-text cast
-- as STABLE, so COALESCE(scheme, transaction_type::text) cannot be indexed
-- directly; this wrapper is declared IMMUTABLE so 000029 can index it, and
-- filters that want that index must call it the same way. The declaration is
-- honest for as long as the labels of transaction_type_enum are never renamed,
-- which is the only thing that would change its output for a stored row.
CREATE OR REPLACE FUNCTION transaction_validation_scheme(
    scheme VARCHAR,
    transaction_type transaction_type_enum
) RETURNS TEXT
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT COALESCE($1, $2::text)
$$;
