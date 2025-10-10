# Test Helpers

This package provides a comprehensive set of utilities and helper functions to
streamline the development of integration and end-to-end tests for the Midaz
ledger system.

## Overview

The `helpers` package is designed to simplify test development by offering:

- **Environment Management:** Configuration and service discovery for test environments.
- **HTTP Client:** A convenient wrapper for making authenticated API requests.
- **Test Data Generation:** Utilities for creating random and structured test data.
- **Balance Verification:** Tools for tracking and asserting account balances.
- **Docker Integration:** Functions for managing the lifecycle of Docker containers.
- **Test Isolation:** Mechanisms to ensure that tests can run in parallel without conflicts.

## Package Structure

### Environment Management (`env.go`)

- **Service URL Discovery:** Onboarding, transaction, etc.
- **HTTP Client Configuration:** Timeouts and other settings.
- **Stack Management:** Functions to start and stop the Docker Compose stack.
- **Health Checks:** Utilities to wait for services to become healthy.

**Key Functions:**

- `LoadEnvironment()`: Loads configuration from environment variables with sensible defaults.
- `WaitForHTTP200()`: Polls a URL until it returns a 200 OK status.

### HTTP Utilities (`http.go`)

- **Authenticated Requests:** Simplified functions for GET, POST, PUT, PATCH, DELETE.
- **Response Handling:** Utilities for parsing JSON responses and asserting status codes.
- **Error Handling:** Automatic retries for transient network errors.

**Key Functions:**

- `Request()`: A simplified method for making authenticated requests.
- `RequestFull()`: Returns the full HTTP response, including headers.

### Authentication (`auth.go`)

- **Token Management:** Functions to obtain and manage authentication tokens.
- **Header Injection:** Utilities for adding authorization headers to requests.

**Key Functions:**

- `AuthHeaders()`: Creates a standard set of authentication and request ID headers.
- `RunTestsWithAuth()`: A wrapper for `TestMain` that handles authentication.

### Test Data Generation

**`payloads.go`**

- **Payload Creation:** Functions to generate valid payloads for creating organizations,
  ledgers, accounts, assets, and transactions.

**Key Functions:**

- `OrgPayload()`: Generates a minimal, valid organization payload.

**`random.go`**

- **Random Data:** Utilities for generating random strings, numbers, UUIDs, and dates.

**Key Functions:**

- `RandString()`: Generates a random alphanumeric string.
- `RandHex()`: Generates a random hexadecimal string.

### Balance Verification

**`balances.go`**

- **Balance Assertions:** Utilities for fetching and asserting account balances.

**Key Functions:**

- `GetAvailableSumByAlias()`: Returns the total available balance for an account.
- `WaitForAvailableSumByAlias()`: Polls until an account's balance reaches an expected value.

**`balance_tracking.go`**

- **Advanced Tracking:** A more sophisticated system for tracking balance changes across
  multiple transactions to verify correctness.

**Key Types:**

- `BalanceTracker`: Tracks balances across multiple accounts.
- `BalanceSnapshot`: Captures the state of a balance at a specific point in time.

### Test Isolation (`isolation.go`)

- **Unique Identifiers:** Generates unique IDs for test runs, organizations, and other entities.
- **Resource Cleanup:** (Future) Utilities to assist with cleaning up test data.

**Key Functions:**

- `NewTestIsolation()`: Creates a new isolation helper for a test run.
- `UniqueOrgName()`, `UniqueLedgerName()`, etc.: Generate unique names for test entities.

### Test Setup (`setup.go`)

- **Test Environment:** High-level functions for setting up and tearing down the test environment.

**Key Functions:**

- `CreateUSDAsset()`: A convenience function for creating a standard USD asset.

### Docker Management (`docker.go`)

- **Container Lifecycle:** Functions for starting, stopping, and restarting Docker containers.
- **Health Checks:** Utilities to check the health of containers.

**Key Functions:**

- `ComposeUpBackend()`, `ComposeDownBackend()`: Start and stop the Docker Compose stack.
- `RestartWithWait()`: Restarts a container and waits for it to become healthy.

### File Utilities (`files.go`)

- **File Handling:** Utilities for reading test fixtures and writing test output.

**Key Functions:**

- `WriteTextFile()`: Writes content to a file, creating the directory if needed.

### Multipart Forms (`multipart.go`)

- **File Uploads:** Utilities for creating and sending `multipart/form-data` requests.

**Key Functions:**

- `RequestMultipart()`: Sends a multipart request with files and fields.
- `PostDSL()`: A convenience function for uploading a Gold DSL file.

### Logging (`logs.go`)

- **Debug Logging:** Utilities for capturing and formatting test logs.

**Key Functions:**

- `StartLogCapture()`: Returns a deferred function to capture Docker logs for a test.

### HTTP Headers (`testheaders.go`)

- **Header Generation:** Functions for creating standard and custom HTTP headers.

**Key Functions:**

- `AuthHeaders()`: Generates a standard set of authentication and request ID headers.

## Best Practices

### Test Isolation

Always use unique identifiers for test data to prevent conflicts between parallel test runs:
`orgName := "test-org-" + helpers.RandString(8)`

### Balance Verification

For complex transaction tests, use the `BalanceTracker` to verify that balances change as expected.

### Error Handling

Always check for errors from helper functions and provide informative failure messages:
`if err != nil { t.Fatalf("Failed to create resource: %v", err) }`

## Environment Variables

The helpers package respects the following environment variables:

- `MIDAZ_ONBOARDING_URL`: Onboarding service URL (default: `http://localhost:3000`).
- `MIDAZ_TRANSACTION_URL`: Transaction service URL (default: `http://localhost:3001`).
- `TEST_AUTH_HEADER`: The full `Authorization` header value for test requests.
- `TEST_MANAGE_STACK`: If `true`, allows tests to start and stop the Docker stack.
- `TEST_HTTP_TIMEOUT`: The timeout for HTTP client requests (default: `30s`).

## Related Documentation

- [Integration Tests](../integration/README.md)
- [Property-Based Tests](../property/README.md)
- [Chaos Tests](../chaos/README.md)
- [Test Fixtures](../fixtures/README.md)
