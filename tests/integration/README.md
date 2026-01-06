# Integration Tests

This directory contains integration tests for the Midaz platform, including multi-tenant functionality tests.

## Running Integration Tests

### Basic Run

```bash
go test -v ./tests/integration/...
```

### Run Specific Test

```bash
go test -v ./tests/integration/... -run "TestIntegration_CoreOrgLedgerAccountAndTransactions"
```

## Multi-Tenant Tests

Multi-tenant tests verify tenant data isolation and proper handling of tenant context in the Midaz platform.

### Test Categories

| Test | Description |
|------|-------------|
| `TestMultiTenant_BackwardCompatibility` | Verifies single-tenant mode works when `MULTI_TENANT_ENABLED=false` |
| `TestMultiTenant_TenantIsolation_Organizations` | Verifies organizations are isolated by tenant |
| `TestMultiTenant_TenantIsolation_Ledgers` | Verifies ledgers are isolated by tenant |
| `TestMultiTenant_TenantIsolation_Accounts` | Verifies accounts are isolated by tenant |
| `TestMultiTenant_TenantIsolation_Transactions` | Verifies transactions are isolated by tenant |
| `TestMultiTenant_CrossTenantAccessDenied` | Verifies one tenant cannot access another's data |
| `TestMultiTenant_MissingTenantContext` | Verifies proper error when tenant context is missing |
| `TestMultiTenant_InvalidTenantToken` | Verifies proper error for invalid JWT tokens |
| `TestMultiTenant_ExpiredTenantToken` | Verifies proper error for expired JWT tokens |
| `TestMultiTenant_HealthEndpointNoAuth` | Verifies health endpoints work without auth |

### Running Multi-Tenant Tests

#### Single-Tenant Mode (Default)

When `MULTI_TENANT_ENABLED` is not set or set to `false`, only backward compatibility tests run:

```bash
go test -v ./tests/integration/... -run "MultiTenant"
```

#### Multi-Tenant Mode

To run full multi-tenant isolation tests, you need:

1. **Pool Manager running**: The tenant configuration service
2. **Multi-tenant enabled services**: Onboarding and Transaction services configured with `MULTI_TENANT_ENABLED=true`

```bash
# Set environment variables
export MULTI_TENANT_ENABLED=true
export POOL_MANAGER_URL=http://localhost:8080
export ONBOARDING_URL=http://localhost:3000
export TRANSACTION_URL=http://localhost:3001

# Run multi-tenant tests
go test -v ./tests/integration/... -run "MultiTenant"
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MULTI_TENANT_ENABLED` | `false` | Enable multi-tenant mode |
| `POOL_MANAGER_URL` | - | URL of the pool manager service (required when multi-tenant is enabled) |
| `ONBOARDING_URL` | `http://localhost:3000` | Onboarding service URL |
| `TRANSACTION_URL` | `http://localhost:3001` | Transaction service URL |
| `TEST_AUTH_URL` | - | OAuth token endpoint (optional) |
| `TEST_AUTH_USERNAME` | - | Auth username (required if `TEST_AUTH_URL` is set) |
| `TEST_AUTH_PASSWORD` | - | Auth password (required if `TEST_AUTH_URL` is set) |

### Test Helpers

The test helpers in `tests/helpers/` provide utilities for multi-tenant testing:

#### JWT Generation (`jwt.go`)

```go
// Generate a JWT with tenant context
token, err := helpers.GenerateTestJWT(tenantID, tenantSlug, userID)

// Generate JWT without tenant claims
token, err := helpers.GenerateTestJWTWithoutTenant(userID)

// Generate expired JWT for error testing
token, err := helpers.GenerateExpiredTestJWT(tenantID, userID)
```

#### Tenant Headers (`testheaders.go`)

```go
// Get headers with tenant context
headers := helpers.TenantAuthHeaders(requestID, tenantID)

// Get headers with additional tenant attributes
headers := helpers.TenantAuthHeadersWithSlug(requestID, tenantID, tenantSlug, userID)
```

### Test Architecture

```
tests/
├── helpers/
│   ├── jwt.go           # JWT generation for multi-tenant tests
│   ├── testheaders.go   # Header helpers including tenant context
│   ├── auth.go          # Authentication helpers
│   └── ...
├── integration/
│   ├── multi_tenant_test.go  # Multi-tenant specific tests
│   ├── core_flow_test.go     # Core functionality tests
│   └── ...
└── README.md
```

### JWT Token Structure

Test JWTs include the following claims for tenant identification:

```json
{
  "iss": "midaz-test",
  "sub": "user-id",
  "aud": ["midaz"],
  "exp": 1234567890,
  "iat": 1234567890,
  "tenantId": "tenant-id",     // Default tenant claim key
  "owner": "tenant-id",        // Alternative tenant claim key
  "tenantSlug": "tenant-slug", // Optional human-readable identifier
  "name": "test-user",
  "email": "test@example.com",
  "roles": ["admin"]
}
```

The tenant ID is extracted from the JWT using the claim key configured via `TENANT_CLAIM_KEY` (default: "tenantId").

### Expected Behavior

#### Single-Tenant Mode (`MULTI_TENANT_ENABLED=false`)

- All requests work without tenant context
- Data is stored in default database
- No tenant isolation is enforced

#### Multi-Tenant Mode (`MULTI_TENANT_ENABLED=true`)

- Requests must include valid JWT with tenant claim
- Tenant A cannot see or access Tenant B's data
- Missing tenant context returns 401/400 error
- Each tenant's data is isolated in separate databases

### Troubleshooting

1. **"Skipping ... - multi-tenant mode is not enabled"**
   - Set `MULTI_TENANT_ENABLED=true` to run isolation tests

2. **Pool Manager connection errors**
   - Ensure Pool Manager is running at `POOL_MANAGER_URL`
   - Verify tenant configurations exist in Pool Manager

3. **JWT validation errors**
   - Check that the auth service accepts test tokens
   - Verify `TENANT_CLAIM_KEY` matches your JWT structure
