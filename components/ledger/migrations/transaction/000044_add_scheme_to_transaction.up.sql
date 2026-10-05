-- scheme records the payment scheme the client declared on a /v2 create
-- (CARD, WIRE, PIX, CRYPTO) so a revert can carry the scheme of the
-- transaction it reverses.
--
-- Nullable with no default, so the ALTER is metadata-only: no table rewrite,
-- pre-existing rows read NULL. IF NOT EXISTS keeps the ALTER idempotent.
ALTER TABLE transaction ADD COLUMN IF NOT EXISTS scheme VARCHAR(16) NULL;
