-- Insert test ledger for E2E tests
-- Uses ORGANIZATION_ID from environment variable (passed as parameter)
INSERT INTO ledger (
    id,
    name,
    organization_id,
    status,
    status_description,
    created_at,
    updated_at
) VALUES (
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'Test Ledger E2E',
    :'organization_id',
    'ACTIVE',
    'Test ledger for E2E tests',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    organization_id = EXCLUDED.organization_id,
    status = EXCLUDED.status,
    status_description = EXCLUDED.status_description,
    updated_at = NOW();
