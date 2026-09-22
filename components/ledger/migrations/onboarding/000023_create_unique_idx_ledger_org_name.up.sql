-- Enforce ledger name uniqueness per organization, case-insensitive, over live
-- rows only. The partial predicate lets a soft-deleted ledger release its name,
-- matching the semantics the create path already advertises.
--
-- Pre-existing duplicates abort this statement (SQLSTATE 23505) and therefore the
-- whole migration, leaving data untouched. Cleanup is manual and must happen
-- before the deploy. Detect duplicates with:
--
--   SELECT organization_id, LOWER(name), COUNT(*) FROM ledger
--    WHERE deleted_at IS NULL GROUP BY 1, 2 HAVING COUNT(*) > 1;
--
-- Built without CONCURRENTLY on purpose: a concurrent build that meets a
-- duplicate leaves an INVALID index behind that a later run must drop by hand,
-- while this form aborts clean. The ledger table holds one row per ledger, so
-- the brief write lock is acceptable.

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_org_name_unique
    ON ledger (organization_id, LOWER(name))
    WHERE deleted_at IS NULL;
