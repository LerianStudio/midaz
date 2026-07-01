-- ============================================
-- Remove Development Seed Data
-- WARNING: This removes ALL seed data
-- ============================================

BEGIN;

-- Delete in reverse order (respecting FK constraints)
DELETE FROM transaction_validations WHERE id IN (
    '40000000-0000-0000-0000-000000000001'
);

DELETE FROM usage_counters WHERE id IN (
    '30000000-0000-0000-0000-000000000001',
    '30000000-0000-0000-0000-000000000002'
);

DELETE FROM limits WHERE id IN (
    '20000000-0000-0000-0000-000000000001',
    '20000000-0000-0000-0000-000000000002'
);

DELETE FROM rules WHERE id IN (
    '10000000-0000-0000-0000-000000000001',
    '10000000-0000-0000-0000-000000000002',
    '10000000-0000-0000-0000-000000000003'
);

COMMIT;

-- Verification
SELECT 'Rules' AS entity, COUNT(*) AS count FROM rules
UNION ALL
SELECT 'Limits', COUNT(*) FROM limits
UNION ALL
SELECT 'UsageCounters', COUNT(*) FROM usage_counters
UNION ALL
SELECT 'TransactionValidations', COUNT(*) FROM transaction_validations;
