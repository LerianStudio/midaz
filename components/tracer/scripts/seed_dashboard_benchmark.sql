-- Benchmark fixture for the /v1/dashboard reads.
--
-- Seeds 1,000,000 transaction_validations spread over 365 days, plus the rule
-- and limit rows the active counts read. It exists so the cost claims in
-- docs/dashboard.md and migration 000024 can be re-measured rather than
-- trusted: run it against a throwaway database, ANALYZE, then EXPLAIN
-- (ANALYZE, BUFFERS) the statements in
-- internal/adapters/postgres/dashboard_repository.go.
--
--   docker run -d --name tracer-bench -e POSTGRES_PASSWORD=tracer \
--     -e POSTGRES_USER=tracer -e POSTGRES_DB=tracer -p 55432:5432 postgres:17
--   for f in migrations/0000*.up.sql; do psql ... -f "$f"; done
--   psql ... -f scripts/seed_dashboard_benchmark.sql
--   psql ... -c 'VACUUM (ANALYZE) transaction_validations;'
--
-- The VACUUM is not optional. The dashboard's index-only plans need the
-- visibility map set, and a freshly bulk-loaded table has none of it — skip it
-- and every measurement reports the slower bitmap-heap plan instead.
--
-- NEVER run this against a database that holds real validations. The trail is
-- append-only (migration 000003 installs an ON DELETE DO INSTEAD NOTHING rule),
-- so these rows cannot be deleted afterwards.
--
-- Decision mix is 5% DENY, 5% REVIEW, 90% ALLOW across 4 transaction types and
-- 3 assets, with matched/evaluated rule arrays drawn from the 20 active rules.

-- 20 ACTIVE rules + 6 INACTIVE, 12 ACTIVE limits + 3 DELETED
INSERT INTO rules (id, name, description, expression, action, scopes, status, activated_at)
SELECT gen_random_uuid(), 'rule-' || i, 'seeded', 'amount > ' || (i * 100), 
       (ARRAY['DENY','REVIEW','ALLOW'])[1 + (i % 3)]::decision_enum, '[]'::jsonb,
       CASE WHEN i <= 20 THEN 'ACTIVE' ELSE 'INACTIVE' END::rule_status_enum,
       NOW() - INTERVAL '400 days'
FROM generate_series(1, 26) i;

INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status, deleted_at)
SELECT gen_random_uuid(), 'limit-' || i, 'DAILY'::limit_type_enum, 100000, 'USD', '[]'::jsonb,
       CASE WHEN i <= 12 THEN 'ACTIVE' ELSE 'DELETED' END::limit_status_enum,
       CASE WHEN i <= 12 THEN NULL ELSE NOW() END
FROM generate_series(1, 15) i;

-- 1,000,000 validations spread over 365 days
INSERT INTO transaction_validations
  (request_id, transaction_type, sub_type, amount, asset, transaction_timestamp,
   account, decision, reason, matched_rule_ids, evaluated_rule_ids,
   limit_usage_details, processing_time_ms, created_at)
SELECT
  gen_random_uuid(),
  (ARRAY['CARD','WIRE','PIX','CRYPTO'])[1 + (i % 4)]::transaction_type_enum,
  (ARRAY['purchase','withdrawal','transfer',NULL])[1 + (i % 4)],
  round((random() * 5000 + 1)::numeric, 2),
  (ARRAY['USD','BRL','EUR'])[1 + (i % 3)],
  ts, '{"accountId":"11111111-1111-1111-1111-111111111111","type":"CHECKING"}'::jsonb,
  CASE WHEN i % 20 = 0 THEN 'DENY' WHEN i % 20 = 1 THEN 'REVIEW' ELSE 'ALLOW' END::decision_enum,
  'seeded',
  CASE WHEN i % 20 IN (0,1) THEN ARRAY[r1,r2] WHEN i % 7 = 0 THEN ARRAY[r1] ELSE '{}'::uuid[] END,
  ARRAY[r1,r2,r3],
  '[]'::jsonb,
  (random() * 90 + 2)::double precision,
  ts
FROM (
  SELECT i, NOW() - (random() * INTERVAL '365 days') AS ts,
         ra[1 + (i % 20)] AS r1, ra[1 + ((i+7) % 20)] AS r2, ra[1 + ((i+13) % 20)] AS r3
  FROM generate_series(1, 1000000) i,
       (SELECT array_agg(id ORDER BY name) AS ra FROM rules WHERE status='ACTIVE') a
) s;
