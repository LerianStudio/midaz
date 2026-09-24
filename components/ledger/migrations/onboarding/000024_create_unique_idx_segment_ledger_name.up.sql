-- Enforce segment name uniqueness per ledger, case-insensitive, over live rows
-- only. The partial predicate lets a soft-deleted segment release its name,
-- matching the semantics the create and update paths already advertise.
--
-- Pre-existing duplicates abort this statement (SQLSTATE 23505) and therefore the
-- whole migration, leaving data untouched. Cleanup is manual and must happen
-- before the deploy. Detect duplicates with:
--
--   SELECT organization_id, ledger_id, LOWER(name), COUNT(*) FROM segment
--    WHERE deleted_at IS NULL GROUP BY 1, 2, 3 HAVING COUNT(*) > 1;
--
-- Built without CONCURRENTLY on purpose: a concurrent build that meets a
-- duplicate leaves an INVALID index behind that a later run must drop by hand,
-- while this form aborts clean. The segment table holds a handful of rows per
-- ledger, so the brief write lock is acceptable.

CREATE UNIQUE INDEX IF NOT EXISTS idx_segment_ledger_name_unique
    ON segment (organization_id, ledger_id, LOWER(name))
    WHERE deleted_at IS NULL;
