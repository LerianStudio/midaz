# Test Helpers

This package provides comprehensive test utilities and helper functions for integration testing of the Midaz ledger system.

## Overview

The `helpers` package contains reusable test utilities that simplify integration test development by providing:

- Environment configuration and service discovery
- HTTP client utilities with authentication
- Test data generation and payload creation
- Balance tracking and verification
- Docker container management
- Test isolation and cleanup utilities

## Package Structure

### Environment Management

**File:** `env.go`

Provides environment configuration for test execution:

- Service URL discovery (onboarding, transaction)
- HTTP client configuration
- Stack management (start/stop services)
- Health check utilities

**Key Types:**

- `Environment` - Holds service URLs and configuration
- `LoadEnvironment()` - Loads configuration with sensible defaults

### HTTP Utilities

**File:** `http.go`

HTTP client utilities for API testing:

- Authenticated HTTP requests (GET, POST, PUT, PATCH, DELETE)
- Response parsing and validation
- Error handling and retries
- Request/response logging

**Key Functions:**

- `GET()`, `POST()`, `PUT()`, `PATCH()`, `DELETE()` - HTTP methods with auth
- `ParseResponse()` - Parse JSON responses
- `ExpectStatus()` - Assert HTTP status codes

### Authentication

**File:** `auth.go`

Authentication and authorization utilities:

- Token generation and management
- Authorization header injection
- Test user creation
- Permission testing

**Key Functions:**

- `AuthHeaders()` - Generate authentication headers
- `CreateTestUser()` - Create test users with permissions

### Test Data Generation

**File:** `payloads.go`

Generates valid test payloads for API requests:

- Organization creation payloads
- Ledger creation payloads
- Account creation payloads
- Asset creation payloads
- Transaction payloads

**Key Functions:**

- `OrgPayload()` - Generate organization payload
- `LedgerPayload()` - Generate ledger payload
- `AccountPayload()` - Generate account payload
- `AssetPayload()` - Generate asset payload
- `TransactionPayload()` - Generate transaction payload

**File:** `random.go`

Random data generation for test uniqueness:

- Random strings
- Random numbers
- Random UUIDs
- Random dates

**Key Functions:**

- `RandomString()` - Generate random alphanumeric strings
- `RandomInt()` - Generate random integers
- `RandomUUID()` - Generate random UUIDs

### Balance Verification

**File:** `balances.go`

Balance assertion and verification utilities:

- Fetch account balances
- Assert balance values
- Compare balance changes
- Verify balance consistency

**Key Functions:**

- `GetBalance()` - Fetch account balance
- `AssertBalance()` - Assert expected balance
- `AssertBalanceChange()` - Verify balance delta

**File:** `balance_tracking.go`

Advanced balance tracking for transaction verification:

- Track balance changes across transactions
- Verify double-entry bookkeeping
- Detect balance inconsistencies
- Generate balance reports

**Key Types:**

- `BalanceTracker` - Tracks balances across multiple accounts
- `BalanceSnapshot` - Captures balance state at a point in time

**Key Functions:**

- `NewBalanceTracker()` - Create balance tracker
- `RecordTransaction()` - Record transaction for tracking
- `VerifyBalances()` - Verify all tracked balances

### Test Isolation

**File:** `isolation.go`

Test isolation utilities to ensure test independence:

- Unique test identifiers
- Test namespace generation
- Resource cleanup
- Test data isolation

**Key Functions:**

- `TestID()` - Generate unique test identifier
- `TestNamespace()` - Create isolated test namespace
- `Cleanup()` - Clean up test resources

### Test Setup

**File:** `setup.go`

Test setup and teardown utilities:

- Initialize test environment
- Create test organizations
- Create test ledgers
- Teardown test data

**Key Functions:**

- `SetupTestOrg()` - Create test organization
- `SetupTestLedger()` - Create test ledger
- `TeardownTest()` - Clean up test data

### Docker Management

**File:** `docker.go`

Docker container management for test infrastructure:

- Start/stop Docker containers
- Check container health
- Manage test databases
- Manage message queues

**Key Functions:**

- `StartStack()` - Start Docker Compose stack
- `StopStack()` - Stop Docker Compose stack
- `CheckHealth()` - Verify service health

### File Utilities

**File:** `files.go`

File handling utilities for test data:

- Read test fixtures
- Write test output
- Manage temporary files
- Clean up test files

**Key Functions:**

- `ReadFixture()` - Read test fixture file
- `WriteTestFile()` - Write test output file
- `TempFile()` - Create temporary file

### Multipart Forms

**File:** `multipart.go`

Multipart form data utilities for file upload testing:

- Create multipart requests
- Add files to multipart forms
- Add form fields
- Send multipart requests

**Key Functions:**

- `CreateMultipartRequest()` - Create multipart form request
- `AddFile()` - Add file to multipart form
- `AddField()` - Add field to multipart form

### Logging

**File:** `logs.go`

Logging utilities for test output and debugging:

- Capture test logs
- Format log output
- Filter log messages
- Debug test failures

**Key Functions:**

- `StartLogCapture()` - Start capturing logs
- `StopLogCapture()` - Stop capturing logs
- `PrintLogs()` - Print captured logs

### HTTP Headers

**File:** `testheaders.go`

HTTP header utilities for test requests:

- Generate authentication headers
- Add request ID headers
- Add custom headers
- Merge header sets

**Key Functions:**

- `AuthHeaders()` - Generate auth headers with request ID
- `CustomHeaders()` - Create custom header set
- `MergeHeaders()` - Merge multiple header sets

## Usage Example

```go
package integration_test

import (
    "testing"
    "github.com/LerianStudio/midaz/tests/helpers"
)

func TestCreateOrganization(t *testing.T) {
    // Load environment
    env := helpers.LoadEnvironment()

    // Generate test data
    orgName := helpers.RandomString(10)
    payload := helpers.OrgPayload(orgName, "12345678000190")

    // Make authenticated request
    requestID := helpers.TestID()
    headers := helpers.AuthHeaders(requestID)

    resp, err := helpers.POST(
        env.OnboardingURL+"/organizations",
        payload,
        headers,
    )
    if err != nil {
        t.Fatalf("Failed to create organization: %v", err)
    }

    // Verify response
    helpers.ExpectStatus(t, resp, 201)

    // Parse response
    var org map[string]any
    helpers.ParseResponse(t, resp, &org)

    // Verify organization
    if org["legalName"] != orgName {
        t.Errorf("Expected name %s, got %s", orgName, org["legalName"])
    }
}
```

## Best Practices

### Test Isolation

Always use unique identifiers for test data:

```go
// Good - unique identifier
orgName := "test-org-" + helpers.RandomString(8)

// Bad - fixed name (causes conflicts)
orgName := "test-org"
```

### Cleanup

Always clean up test resources:

```go
func TestSomething(t *testing.T) {
    orgID := helpers.SetupTestOrg(t)
    defer helpers.TeardownTest(t, orgID)

    // Test code...
}
```

### Balance Verification

Use balance tracking for transaction tests:

```go
tracker := helpers.NewBalanceTracker()
tracker.RecordTransaction(txn)

// After transaction
if err := tracker.VerifyBalances(); err != nil {
    t.Errorf("Balance verification failed: %v", err)
}
```

### Error Handling

Always check errors and provide context:

```go
resp, err := helpers.POST(url, payload, headers)
if err != nil {
    t.Fatalf("Failed to create resource: %v", err)
}

if resp.StatusCode != 201 {
    body, _ := io.ReadAll(resp.Body)
    t.Fatalf("Expected 201, got %d: %s", resp.StatusCode, body)
}
```

## Environment Variables

The helpers package respects these environment variables:

- `MIDAZ_ONBOARDING_URL` - Onboarding service URL (default: http://localhost:3000)
- `MIDAZ_TRANSACTION_URL` - Transaction service URL (default: http://localhost:3001)
- `TEST_AUTH_HEADER` - Authorization header value for tests
- `TEST_MANAGE_STACK` - If "true", tests can start/stop Docker stack
- `TEST_HTTP_TIMEOUT` - HTTP client timeout (default: 30s)

## Integration with Test Suites

### Property-Based Tests

The helpers integrate with property-based testing:

```go
func TestBalanceProperties(t *testing.T) {
    env := helpers.LoadEnvironment()
    tracker := helpers.NewBalanceTracker()

    // Property: Sum of all operations equals zero
    // (double-entry bookkeeping)
    for i := 0; i < 100; i++ {
        amount := helpers.RandomInt(1, 1000)
        // Create transaction...
        tracker.RecordTransaction(txn)
    }

    if err := tracker.VerifyBalances(); err != nil {
        t.Errorf("Property violated: %v", err)
    }
}
```

### Chaos Testing

The helpers support chaos testing scenarios:

```go
func TestServiceResilience(t *testing.T) {
    env := helpers.LoadEnvironment()

    // Simulate service failure
    helpers.StopStack()
    defer helpers.StartStack()

    // Verify graceful degradation
    _, err := helpers.GET(env.OnboardingURL+"/health", nil)
    if err == nil {
        t.Error("Expected error when service is down")
    }
}
```

## Contributing

When adding new test utilities:

1. Add package-level comment explaining the utility's purpose
2. Document all exported functions with examples
3. Follow existing naming conventions
4. Add usage examples to this README
5. Ensure utilities are reusable across test suites

## Related Documentation

- [Integration Tests](../integration/README.md)
- [Property-Based Tests](../property/README.md)
- [Chaos Tests](../chaos/README.md)
- [Test Fixtures](../fixtures/README.md)
