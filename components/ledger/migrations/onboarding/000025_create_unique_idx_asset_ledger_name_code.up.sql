-- Enforce asset name and code uniqueness per ledger over live rows only. The
-- name is compared case-insensitively; the code is compared exactly, since
-- validation already requires codes to be uppercase. The partial predicate lets
-- a soft-deleted asset release its name and code, matching the semantics the
-- create and update paths already advertise.
--
-- Pre-existing duplicates abort the statement (SQLSTATE 23505) and therefore the
-- whole migration, leaving data untouched. Cleanup is manual and must happen
-- before the deploy. Detect duplicates with:
--
--   SELECT organization_id, ledger_id, LOWER(name), COUNT(*) FROM asset
--    WHERE deleted_at IS NULL GROUP BY 1, 2, 3 HAVING COUNT(*) > 1;
--
--   SELECT organization_id, ledger_id, code, COUNT(*) FROM asset
--    WHERE deleted_at IS NULL GROUP BY 1, 2, 3 HAVING COUNT(*) > 1;
--
-- Built without CONCURRENTLY on purpose: a concurrent build that meets a
-- duplicate leaves an INVALID index behind that a later run must drop by hand,
-- while this form aborts clean. The asset table holds a handful of rows per
-- ledger, so the brief write lock is acceptable.

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_ledger_name_unique
    ON asset (organization_id, ledger_id, LOWER(name))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_ledger_code_unique
    ON asset (organization_id, ledger_id, code)
    WHERE deleted_at IS NULL;
