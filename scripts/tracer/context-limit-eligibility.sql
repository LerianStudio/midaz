-- Read-only preflight for each Tracer tenant primary before enabling shared Reserve.
-- A successful preflight returns zero rows. Compare scope_count and scope_bytes
-- with the rendered CONTEXT_LIMIT_MAX_SCOPES and CONTEXT_LIMIT_MAX_SCOPE_BYTES.
WITH active_limits AS (
    SELECT
        l.id,
        l.name,
        l.scopes,
        a.limit_id AS mapped_limit_id,
        a.asset_namespace,
        a.asset_id,
        a.asset_code
    FROM limits l
    LEFT JOIN limit_asset_references a ON a.limit_id = l.id
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
)
SELECT
    l.id,
    l.name,
    l.asset_namespace,
    l.asset_id,
    l.asset_code,
    f.scope_count,
    f.scope_bytes,
    array_remove(ARRAY[
        CASE WHEN l.mapped_limit_id IS NULL THEN 'missing_asset_reference' END,
        CASE WHEN coalesce(l.asset_namespace, '') = '' THEN 'missing_asset_namespace' END,
        CASE WHEN coalesce(l.asset_id, '') = '' THEN 'missing_asset_id' END,
        CASE WHEN coalesce(l.asset_code, '') = '' THEN 'missing_asset_code' END,
        CASE WHEN jsonb_typeof(l.scopes) IS DISTINCT FROM 'array' THEN 'scopes_not_array' END,
        CASE WHEN f.scope_count = 0 THEN 'empty_scopes' END,
        CASE WHEN NOT f.account_scoped THEN 'non_account_or_invalid_scope' END,
        CASE WHEN NOT f.accounts_unique THEN 'duplicate_account_scope' END
    ], NULL) AS reasons
FROM active_limits l
JOIN scope_facts f ON f.id = l.id
WHERE l.mapped_limit_id IS NULL
   OR coalesce(l.asset_namespace, '') = ''
   OR coalesce(l.asset_id, '') = ''
   OR coalesce(l.asset_code, '') = ''
   OR jsonb_typeof(l.scopes) IS DISTINCT FROM 'array'
   OR f.scope_count = 0
   OR NOT f.account_scoped
   OR NOT f.accounts_unique
ORDER BY l.id;
