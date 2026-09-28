-- Read-only preflight for each Tracer tenant primary before enabling shared Reserve.
-- A successful preflight returns zero rows. Compare scope_count and scope_bytes
-- with the rendered CONTEXT_LIMIT_MAX_SCOPES and CONTEXT_LIMIT_MAX_SCOPE_BYTES.
--
-- Each ACTIVE limit must be scoped to distinct accounts and carry an asset code
-- that follows the ledger rule (1-100 uppercase letters). The asset predicate is
-- the limits_asset_code_format CHECK plus explicit character bounds, so rows
-- restored from a dump taken without that constraint are reported too.
WITH active_limits AS (
    SELECT
        l.id,
        l.name,
        l.asset,
        l.scopes
    FROM limits l
    WHERE l.status = 'ACTIVE'
      AND l.deleted_at IS NULL
),
scope_rows AS (
    SELECT
        l.id,
        scope.value,
        scope.value->>'accountId' AS account_id,
        CASE
            WHEN pg_input_is_valid(scope.value->>'accountId', 'uuid')
                THEN (scope.value->>'accountId')::uuid::text
        END AS canonical_account_id
    FROM active_limits l
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(l.scopes) = 'array' THEN l.scopes ELSE '[]'::jsonb END
    ) AS scope(value)
),
scope_facts AS (
    SELECT
        l.id,
        count(s.value) AS scope_count,
        octet_length(l.scopes::text) AS scope_bytes,
        coalesce(bool_and(
            jsonb_typeof(s.value) = 'object'
            AND s.account_id IS NOT NULL
            AND s.value - 'accountId' = '{}'::jsonb
            AND s.canonical_account_id IS NOT NULL
            AND s.canonical_account_id <> '00000000-0000-0000-0000-000000000000'
        ), false) AS account_scoped,
        count(s.canonical_account_id) = count(DISTINCT s.canonical_account_id) AS accounts_unique
    FROM active_limits l
    LEFT JOIN scope_rows s ON s.id = l.id
    GROUP BY l.id, l.scopes
),
limit_facts AS (
    SELECT
        l.id,
        l.name,
        l.scopes,
        f.scope_count,
        f.scope_bytes,
        f.account_scoped,
        f.accounts_unique,
        coalesce(
            char_length(l.asset) BETWEEN 1 AND 100
            AND l.asset ~ '^[^\x01-\x40\x5B-\x7F]{1,100}$',
            false
        ) AS asset_well_formed
    FROM active_limits l
    JOIN scope_facts f ON f.id = l.id
)
SELECT
    l.id,
    l.name,
    l.scope_count,
    l.scope_bytes,
    array_remove(ARRAY[
        CASE WHEN NOT l.asset_well_formed THEN 'malformed_asset_code' END,
        CASE WHEN jsonb_typeof(l.scopes) IS DISTINCT FROM 'array' THEN 'scopes_not_array' END,
        CASE WHEN l.scope_count = 0 THEN 'empty_scopes' END,
        CASE WHEN NOT l.account_scoped THEN 'non_account_or_invalid_scope' END,
        CASE WHEN NOT l.accounts_unique THEN 'duplicate_account_scope' END
    ], NULL) AS reasons
FROM limit_facts l
WHERE NOT l.asset_well_formed
   OR jsonb_typeof(l.scopes) IS DISTINCT FROM 'array'
   OR l.scope_count = 0
   OR NOT l.account_scoped
   OR NOT l.accounts_unique
ORDER BY l.id;
