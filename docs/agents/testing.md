# Testing Guide

## Testing Philosophy

Midaz maintains **comprehensive test coverage** across multiple testing levels:
- **Unit Tests**: Fast, isolated tests for business logic
- **Integration Tests**: API-level tests with real infrastructure
- **Chaos Tests**: Infrastructure failure scenarios
- **Fuzz Tests**: Robustness testing with random inputs
- **Property-Based Tests**: Generative testing for invariants

## Test Structure

### Directory Organization

```
midaz/
├── components/
│   ├── onboarding/
│   │   └── internal/
│   │       ├── services/
│   │       │   ├── command/
│   │       │   │   ├── create-account.go
│   │       │   │   └── create-account_test.go      # Unit tests
│   │       │   └── query/
│   │       │       ├── get-account.go
│   │       │       └── get-account_test.go
│   │       └── adapters/
│   │           └── postgres/
│   │               └── account/
│   │                   ├── account.postgresql.go
│   │                   ├── account.postgresql_test.go
│   │                   └── account.postgresql_mock.go   # Generated mock
│   └── transaction/
│       └── internal/
│           └── ... (same structure)
├── tests/
│   ├── integration/           # API integration tests
│   │   ├── onboarding/
│   │   │   ├── account_test.go
│   │   │   └── ledger_test.go
│   │   └── transaction/
│   │       └── transaction_test.go
│   ├── chaos/                 # Chaos engineering tests
│   │   ├── database_failure_test.go
│   │   └── rabbitmq_failure_test.go
│   ├── fuzzy/                 # Fuzz testing
│   │   └── transaction_fuzzer_test.go
│   └── property/              # Property-based tests
│       └── ledger_properties_test.go
└── pkg/
    └── assert/
        └── assert_test.go     # Tests for shared packages
```

## Running Tests

### Unit Tests

```bash
# Run all unit tests
make test

# Run unit tests with coverage
make test-unit

# Run tests for specific component
make onboarding COMMAND=test

# Run specific test file
go test ./components/onboarding/internal/services/command -run TestCreateAccount

# Run tests with race detector
go test -race ./...

# Generate coverage report
make cover  # Creates coverage.html
```

### Integration Tests

```bash
# Start infrastructure (PostgreSQL, MongoDB, RabbitMQ, Valkey)
make up

# Run integration tests
make test-integration

# Run specific integration test
go test ./tests/integration/onboarding -run TestAccountCreation
```

### Specialized Tests

```bash
# Chaos engineering tests
make test-chaos

# Fuzz testing
make test-fuzzy

# Property-based tests
make test-property

# E2E tests with Apidog
make test-e2e
```

## Unit Testing Patterns

### Table-Driven Tests (Preferred)

```go
func TestCreateAccount(t *testing.T) {
    t.Parallel()  // REQUIRED by paralleltest linter

    tests := []struct {
        name          string
        input         mmodel.CreateAccountInput
        setupMock     func(*gomock.Controller) *mock.MockAccountRepository
        want          *mmodel.Account
        wantErr       bool
        expectedError error
    }{
        {
            name: "success - create checking account",
            input: mmodel.CreateAccountInput{
                Name:           "Checking Account",
                Type:           "DEPOSIT",
                OrganizationID: testOrgID,
                LedgerID:       testLedgerID,
            },
            setupMock: func(ctrl *gomock.Controller) *mock.MockAccountRepository {
                repo := mock.NewMockAccountRepository(ctrl)
                repo.EXPECT().
                    Create(gomock.Any(), gomock.Any()).
                    Return(nil)
                return repo
            },
            want: &mmodel.Account{
                Name: "Checking Account",
                Type: "DEPOSIT",
            },
            wantErr: false,
        },
        {
            name: "error - duplicate account name",
            input: mmodel.CreateAccountInput{
                Name: "Existing Account",
                Type: "DEPOSIT",
            },
            setupMock: func(ctrl *gomock.Controller) *mock.MockAccountRepository {
                repo := mock.NewMockAccountRepository(ctrl)
                repo.EXPECT().
                    Create(gomock.Any(), gomock.Any()).
                    Return(pkg.EntityConflictError{
                        EntityType: "Account",
                        Field:      "name",
                    })
                return repo
            },
            wantErr:       true,
            expectedError: pkg.EntityConflictError{},
        },
        {
            name: "error - invalid account type",
            input: mmodel.CreateAccountInput{
                Name: "Invalid Account",
                Type: "INVALID_TYPE",
            },
            setupMock: func(ctrl *gomock.Controller) *mock.MockAccountRepository {
                // No expectations - validation fails before repo call
                return mock.NewMockAccountRepository(ctrl)
            },
            wantErr:       true,
            expectedError: pkg.ValidationError{},
        },
    }

    for _, tt := range tests {
        tt := tt  // Capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // Run sub-tests in parallel

            // Setup
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()

            mockRepo := tt.setupMock(ctrl)
            uc := command.NewUseCase(mockRepo)

            // Execute
            got, err := uc.CreateAccount(context.Background(), tt.input)

            // Assert
            if tt.wantErr {
                assert.Error(t, err)
                if tt.expectedError != nil {
                    assert.IsType(t, tt.expectedError, err)
                }
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, got)
                assert.Equal(t, tt.want.Name, got.Name)
                assert.Equal(t, tt.want.Type, got.Type)
            }
        })
    }
}
```

### Mocking with uber/mock (gomock)

**1. Define Mock Directive in Interface**

```go
// File: components/onboarding/internal/adapters/postgres/account/account.go

//go:generate mockgen -source=account.go -destination=account_mock.go -package=account

type Repository interface {
    Create(ctx context.Context, account *mmodel.Account) error
    Find(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (*mmodel.Account, error)
    FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID) ([]*mmodel.Account, error)
    Update(ctx context.Context, account *mmodel.Account) error
    Delete(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error
}
```

**2. Generate Mocks**

```bash
# Generate all mocks
go generate ./...

# Or for specific package
cd components/onboarding/internal/adapters/postgres/account
go generate
```

**3. Use Mocks in Tests**

```go
func TestCreateAccount_RepositoryError(t *testing.T) {
    // Setup mock controller
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Create mock repository
    mockRepo := mock.NewMockAccountRepository(ctrl)

    // Set expectations
    mockRepo.EXPECT().
        Create(
            gomock.Any(),  // Any context
            gomock.Any(),  // Any account
        ).
        Return(errors.New("database connection failed"))

    // Create use case with mock
    uc := command.NewUseCase(mockRepo)

    // Execute and assert
    _, err := uc.CreateAccount(context.Background(), testInput)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "database connection failed")
}
```

### Testing with sqlmock (Database Tests)

```go
func TestAccountRepository_Create(t *testing.T) {
    // Create mock database
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    // Set up expectations
    mock.ExpectExec(`INSERT INTO accounts`).
        WithArgs(
            sqlmock.AnyArg(),  // ID
            "Checking Account",
            "DEPOSIT",
            testOrgID,
            testLedgerID,
        ).
        WillReturnResult(sqlmock.NewResult(1, 1))

    // Create repository with mock db
    repo := NewAccountRepository(db)

    // Execute
    account := &mmodel.Account{
        ID:             uuid.New(),
        Name:           "Checking Account",
        Type:           "DEPOSIT",
        OrganizationID: testOrgID,
        LedgerID:       testLedgerID,
    }

    err = repo.Create(context.Background(), account)

    // Assert
    assert.NoError(t, err)
    assert.NoError(t, mock.ExpectationsWereMet())
}
```

## Integration Testing

### Setup Pattern

```go
// tests/integration/onboarding/setup_test.go

var (
    baseURL    string
    httpClient *http.Client
)

func TestMain(m *testing.M) {
    // Setup: Read environment variables
    baseURL = os.Getenv("ONBOARDING_URL")
    if baseURL == "" {
        baseURL = "http://localhost:3000"
    }

    httpClient = &http.Client{
        Timeout: 30 * time.Second,
    }

    // Run tests
    code := m.Run()

    // Teardown (if needed)

    os.Exit(code)
}
```

### API Integration Test

```go
func TestCreateAccount_Integration(t *testing.T) {
    // Skip if integration tests disabled
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Arrange
    reqBody := map[string]interface{}{
        "name": "Integration Test Account",
        "type": "DEPOSIT",
    }
    body, _ := json.Marshal(reqBody)

    url := fmt.Sprintf("%s/api/v1/organizations/%s/ledgers/%s/accounts",
        baseURL, testOrgID, testLedgerID)

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
    require.NoError(t, err)
    req.Header.Set("Content-Type", "application/json")

    // Act
    resp, err := httpClient.Do(req)
    require.NoError(t, err)
    defer resp.Body.Close()

    // Assert
    assert.Equal(t, http.StatusCreated, resp.StatusCode)

    var account mmodel.Account
    err = json.NewDecoder(resp.Body).Decode(&account)
    require.NoError(t, err)

    assert.Equal(t, "Integration Test Account", account.Name)
    assert.Equal(t, "DEPOSIT", account.Type)
    assert.NotEmpty(t, account.ID)
}
```

## Chaos Testing

**Location**: `tests/chaos/`

Tests infrastructure failure scenarios.

```go
func TestDatabaseFailover(t *testing.T) {
    // Simulate primary database failure
    // Verify system continues with replica

    // 1. Create account with primary DB healthy
    account := createTestAccount(t)

    // 2. Simulate primary DB failure
    stopPrimaryDatabase()
    defer startPrimaryDatabase()

    // 3. Verify reads still work (from replica)
    retrieved := getAccount(t, account.ID)
    assert.Equal(t, account.ID, retrieved.ID)

    // 4. Verify writes fail gracefully
    _, err := updateAccount(t, account.ID, updates)
    assert.Error(t, err)
}

func TestRabbitMQReconnection(t *testing.T) {
    // 1. Verify events are published
    publishEvent(t, testEvent)

    // 2. Stop RabbitMQ
    stopRabbitMQ()

    // 3. Publish should fail or queue locally
    err := publishEvent(t, testEvent)
    // System should handle gracefully, not crash

    // 4. Restart RabbitMQ
    startRabbitMQ()

    // 5. Verify reconnection and queued messages delivered
    waitForReconnection()
    assertEventDelivered(t, testEvent)
}
```

## Fuzz Testing

**Location**: `tests/fuzzy/`

```go
func FuzzTransactionParsing(f *testing.F) {
    // Seed corpus with valid inputs
    f.Add("send 100 from @account1 to @account2")
    f.Add("distribute 1000 from @source to @dest1 @dest2 by 50% 50%")

    f.Fuzz(func(t *testing.T, input string) {
        // Parse transaction DSL
        tx, err := parser.Parse(input)

        // Should never panic, even with invalid input
        if err != nil {
            // Invalid input is expected
            return
        }

        // If parsed successfully, validate structure
        assert.NotNil(t, tx)
        assert.NotEmpty(t, tx.Source)
    })
}
```

## Property-Based Testing

**Location**: `tests/property/`

```go
import "github.com/leanovate/gopter"

func TestBalanceInvariant(t *testing.T) {
    properties := gopter.NewProperties(nil)

    properties.Property("sum of account balances equals ledger balance", prop.ForAll(
        func(transactions []Transaction) bool {
            // Setup ledger with accounts
            ledger := setupTestLedger()

            // Apply all transactions
            for _, tx := range transactions {
                applyTransaction(ledger, tx)
            }

            // Verify invariant: sum of all account balances equals total
            accountSum := sumAccountBalances(ledger)
            ledgerTotal := ledger.TotalBalance()

            return accountSum == ledgerTotal
        },
        generateTransactions(),
    ))

    properties.TestingRun(t)
}
```

## Test Helpers

### Creating Test Helpers

```go
// tests/helpers/account.go

// CreateTestAccount creates an account for testing
func CreateTestAccount(t *testing.T, name string) *mmodel.Account {
    t.Helper()  // REQUIRED by thelper linter

    return &mmodel.Account{
        ID:             uuid.New(),
        Name:           name,
        Type:           "DEPOSIT",
        OrganizationID: testOrgID,
        LedgerID:       testLedgerID,
        CreatedAt:      time.Now(),
    }
}

// AssertAccountEqual compares accounts
func AssertAccountEqual(t *testing.T, expected, actual *mmodel.Account) {
    t.Helper()

    assert.Equal(t, expected.ID, actual.ID)
    assert.Equal(t, expected.Name, actual.Name)
    assert.Equal(t, expected.Type, actual.Type)
}
```

## Testing Anti-Patterns (Enforced by Linters)

### Anti-Pattern 1: Testing Mock Behavior

```go
// ❌ BAD - Testing the mock, not the code
func TestBadTest(t *testing.T) {
    mockRepo := mock.NewMockRepository(ctrl)
    mockRepo.EXPECT().Find(gomock.Any()).Return(account, nil)

    // Only testing that mock returns what we told it to
    result, _ := mockRepo.Find(ctx, id)
    assert.Equal(t, account, result)  // Meaningless test!
}

// ✅ GOOD - Testing actual business logic
func TestGoodTest(t *testing.T) {
    mockRepo := mock.NewMockRepository(ctrl)
    mockRepo.EXPECT().Find(gomock.Any()).Return(account, nil)

    // Testing the use case, which uses the mock
    uc := NewUseCase(mockRepo)
    result, err := uc.ProcessAccount(ctx, id)

    // Testing business logic behavior
    assert.NoError(t, err)
    assert.Equal(t, expected.Status, result.Status)
}
```

### Anti-Pattern 2: Not Using t.Parallel()

```go
// ❌ BAD - Caught by paralleltest linter
func TestSlowTest(t *testing.T) {
    // Missing t.Parallel() - tests run sequentially
    time.Sleep(1 * time.Second)
}

// ✅ GOOD - Tests run in parallel
func TestFastTest(t *testing.T) {
    t.Parallel()  // Tests run concurrently
    time.Sleep(1 * time.Second)
}
```

### Anti-Pattern 3: Not Calling t.Helper()

```go
// ❌ BAD - Caught by thelper linter
func createAccount(t *testing.T) *Account {
    // Missing t.Helper() - error line numbers wrong
    return &Account{...}
}

// ✅ GOOD - Proper helper function
func createAccount(t *testing.T) *Account {
    t.Helper()  // Error line numbers point to caller
    return &Account{...}
}
```

## Test Coverage

### Measuring Coverage

```bash
# Generate coverage report
make cover

# View coverage in browser
open coverage.html

# Coverage by package
go test -cover ./...

# Detailed coverage profile
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Coverage Targets

- **Unit Tests**: Aim for 80%+ coverage of business logic
- **Integration Tests**: Cover all API endpoints
- **Critical Paths**: 100% coverage for financial calculations
- **Error Paths**: Test all error branches

## Testing Checklist

✅ **Use table-driven tests** for multiple scenarios

✅ **Include t.Parallel()** in all test functions (enforced by linter)

✅ **Call t.Helper()** in test helper functions (enforced by linter)

✅ **Generate mocks** with `go generate ./...` before running tests

✅ **Test error paths** - Don't just test happy path

✅ **Use descriptive test names** - `TestFunction_Scenario_ExpectedBehavior`

✅ **Arrange-Act-Assert** pattern in test bodies

✅ **Clean up resources** with `defer` or `t.Cleanup()`

✅ **Use require** for setup assertions (stops test on failure)

✅ **Use assert** for test assertions (continues on failure)

✅ **Test business logic**, not mock behavior

✅ **Run tests with race detector** - `go test -race`

✅ **Maintain test isolation** - No shared state between tests

## Running Tests in CI

```yaml
# .github/workflows/test.yml (example)
name: Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      - run: make test
      - run: make test GOFLAGS=-race

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17
      mongodb:
        image: mongo:8
      rabbitmq:
        image: rabbitmq:4.1.3
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: make test-integration
```

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Error Handling: `docs/agents/error-handling.md`
- Concurrency: `docs/agents/concurrency.md`
- Database: `docs/agents/database.md`
