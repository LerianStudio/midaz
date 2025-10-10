-- Insert test organization for E2E tests
-- Uses ORGANIZATION_ID from environment variable (passed as parameter)
INSERT INTO organization (
    id,
    parent_organization_id,
    legal_name,
    doing_business_as,
    legal_document,
    address,
    status,
    status_description,
    created_at,
    updated_at
) VALUES (
    :'organization_id',
    NULL,
    'Test Organization E2E',
    'Test Org',
    '00000000000000',
    '{"street": "123 Test Street", "city": "Test City", "state": "TS", "country": "Test Country", "zipCode": "00000"}',
    'ACTIVE',
    'Test organization for E2E tests',
    NOW(),
    NOW()
)
ON CONFLICT (id) DO UPDATE SET
    legal_name = EXCLUDED.legal_name,
    doing_business_as = EXCLUDED.doing_business_as,
    legal_document = EXCLUDED.legal_document,
    address = EXCLUDED.address,
    status = EXCLUDED.status,
    status_description = EXCLUDED.status_description,
    updated_at = NOW();
