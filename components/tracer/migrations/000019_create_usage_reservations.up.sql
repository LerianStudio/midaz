-- ============================================
-- Migration: 000019_create_usage_reservations
-- Description: Per-reservation ledger for the two-phase reservation seam.
--              One row per (transaction, limit, scope, period) reservation.
--              Carries TTL for the reaper, idempotency for retried reserves,
--              and correlation back to the ledger transaction for audit.
-- Date: 2026-06-05
-- ============================================

-- usage_reservations table (depends on limits)
-- amount is stored in the smallest currency unit (e.g., cents), matching
-- usage_counters.current_usage / reserved_usage.
-- scope_key / period_key mirror usage_counters semantics so a reservation
-- targets exactly one counter bucket.
-- transaction_id is the ledger transaction correlation id. It is NOT a foreign
-- key: the ledger transaction lives in a different service/database, so the
-- reference is by value only.
-- status is constrained by a CHECK (not a PG enum type) to avoid the
-- ALTER TYPE ... ADD VALUE migration friction hit in 000009; the Go-side enum
-- in pkg/model/reservation.go is the authoritative source.
CREATE TABLE IF NOT EXISTS usage_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    limit_id UUID NOT NULL REFERENCES limits(id) ON DELETE CASCADE,
    scope_key VARCHAR(255) NOT NULL,
    period_key VARCHAR(50) NOT NULL,
    amount BIGINT NOT NULL CHECK (amount >= 0),
    status VARCHAR(16) NOT NULL DEFAULT 'RESERVED'
        CHECK (status IN ('RESERVED', 'CONFIRMED', 'RELEASED', 'EXPIRED')),
    transaction_id UUID NOT NULL,
    reservation_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMP WITH TIME ZONE,
    released_at TIMESTAMP WITH TIME ZONE
);

-- Idempotency index: a retried reserve for the same ledger transaction touching
-- the same limit/scope/period collapses onto the existing row (INSERT ... ON
-- CONFLICT DO NOTHING in the repository). The grain MUST be the full 4-tuple:
-- a single ledger transaction touching one limit across two scopes or periods
-- legitimately yields two reservation rows, so (transaction_id, limit_id) alone
-- would wrongly collide them.
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_reservations_request
    ON usage_reservations(transaction_id, limit_id, scope_key, period_key);

-- Reaper index: the TTL sweep scans only outstanding RESERVED rows by expiry.
-- Partial index keeps it small — CONFIRMED/RELEASED/EXPIRED rows are excluded.
CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
    ON usage_reservations(reservation_expires_at)
    WHERE status = 'RESERVED';
