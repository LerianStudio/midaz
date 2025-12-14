# Error Handling Guide

## Overview

Midaz uses a **typed error system** with consistent error wrapping and context propagation. The system distinguishes between business errors (expected failures like "account not found") and technical errors (unexpected failures like database connection errors).

## Error Types

### Location: `pkg/errors.go`

All business errors are typed and defined in the shared errors package.

### Business Error Types

```go
// Entity not found (404)
type EntityNotFoundError struct {
    EntityType string
    ID         interface{}
    Err        error
}

// Validation failure (400)
type ValidationError struct {
    Code    string
    Message string
    Field   string
    Err     error
}

// Entity already exists (409)
type EntityConflictError struct {
    EntityType string
    Field      string
    Value      interface{}
    Err        error
}

// Unauthorized access (401)
type UnauthorizedError struct {
    Message string
    Err     error
}

// Forbidden access (403)
type ForbiddenError struct {
    Message string
    Err     error
}
```

### Creating Business Errors

```go
// Entity not found
return pkg.EntityNotFoundError{
    EntityType: "Account",
    ID:         accountID,
    Err:        errors.New("account does not exist"),
}

// Validation error
return pkg.ValidationError{
    Code:    constant.ErrInvalidAccountType,
    Message: "account type must be DEPOSIT, SAVINGS, or INVESTMENT",
    Field:   "type",
}

// Conflict error
return pkg.EntityConflictError{
    EntityType: "Account",
    Field:      "name",
    Value:      input.Name,
    Err:        errors.New("account with this name already exists"),
}
```

### Error Constants

**Location**: `pkg/constant/errors.go`

All error codes are centralized as constants:

```go
const (
    // Validation errors
    ErrInvalidAccountType      = "INVALID_ACCOUNT_TYPE"
    ErrInvalidAmount          = "INVALID_AMOUNT"
    ErrInsufficientBalance    = "INSUFFICIENT_BALANCE"

    // Business logic errors
    ErrAccountNotFound        = "ACCOUNT_NOT_FOUND"
    ErrDuplicateAccount       = "DUPLICATE_ACCOUNT"
    ErrParentAccountNotFound  = "PARENT_ACCOUNT_NOT_FOUND"

    // System errors
    ErrDatabaseConnection     = "DATABASE_CONNECTION_ERROR"
    ErrServiceUnavailable     = "SERVICE_UNAVAILABLE"
)
```

## Error Wrapping Pattern

### Always Wrap Errors with Context

**Rule**: Every layer that handles an error MUST add context about what operation failed.

```go
// ❌ BAD - No context
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    account, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        return nil, err  // Lost context about what we were doing
    }
    return account, nil
}

// ✅ GOOD - Adds context at each layer
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    account, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        return nil, fmt.Errorf("creating account in repository: %w", err)
    }
    return account, nil
}
```

### Wrapping Pattern by Layer

**Handler Layer** - Converts errors to HTTP responses

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
    if err != nil {
        // Handler doesn't wrap - it converts to HTTP response
        return http.WithError(c, err)
    }
    return http.Created(c, account)
}
```

**Use Case Layer** - Adds business context

```go
func (uc *UseCase) CreateAccount(ctx context.Context, orgID, ledgerID uuid.UUID, input Input) (*Account, error) {
    // Validate parent account exists
    if input.ParentAccountID != nil {
        parent, err := uc.AccountRepo.Find(ctx, orgID, ledgerID, *input.ParentAccountID)
        if err != nil {
            // Add context about what business operation failed
            return nil, fmt.Errorf("validating parent account %s: %w", *input.ParentAccountID, err)
        }
        if parent == nil {
            return pkg.EntityNotFoundError{
                EntityType: "ParentAccount",
                ID:         *input.ParentAccountID,
            }
        }
    }

    // Create account
    account := &mmodel.Account{...}
    if err := uc.AccountRepo.Create(ctx, account); err != nil {
        return nil, fmt.Errorf("creating account %s: %w", account.Name, err)
    }

    return account, nil
}
```

**Repository Layer** - Adds technical context

```go
func (r *Repository) Create(ctx context.Context, account *mmodel.Account) error {
    query := `INSERT INTO accounts (id, name, type, organization_id, ledger_id) VALUES ($1, $2, $3, $4, $5)`

    _, err := r.db.ExecContext(ctx, query, account.ID, account.Name, account.Type, account.OrganizationID, account.LedgerID)
    if err != nil {
        // Check for specific database errors
        if pqErr, ok := err.(*pq.Error); ok {
            if pqErr.Code == "23505" { // Unique violation
                return pkg.EntityConflictError{
                    EntityType: "Account",
                    Field:      "name",
                    Value:      account.Name,
                    Err:        err,
                }
            }
        }
        // Add SQL context
        return fmt.Errorf("executing insert query for account %s: %w", account.ID, err)
    }

    return nil
}
```

### Error Chain Example

Full error chain from repository to handler:

```
Database Error: pq: duplicate key value violates unique constraint "accounts_name_key"
    ↓ Wrapped by Repository
Repository Error: executing insert query for account abc123: <db error>
    ↓ Wrapped by Use Case
Use Case Error: creating account "Checking Account": <repository error>
    ↓ Converted by Handler
HTTP Response: 409 Conflict - Account with name "Checking Account" already exists
```

## Business Error Validation

### Use `pkg.ValidateBusinessError()`

This function maps error codes to error types and ensures consistency:

```go
// In use case
if existingAccount != nil {
    err := pkg.ValidateBusinessError(constant.ErrDuplicateAccount, "Account")
    return nil, err
}
```

### Function signature:

```go
func ValidateBusinessError(code string, entityType string) error
```

Maps error codes to appropriate error types:
- `*_NOT_FOUND` → `EntityNotFoundError`
- `DUPLICATE_*` → `EntityConflictError`
- `INVALID_*` → `ValidationError`

## Error Logging Pattern

### Log Errors at Boundaries Only

**Rule**: Only log errors at the boundary (handlers), not in every layer.

```go
// ❌ BAD - Logging at every layer
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    account, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        logger.Errorf("Failed to create account: %v", err)  // Don't log here!
        return nil, fmt.Errorf("creating account: %w", err)
    }
    return account, nil
}

// ✅ GOOD - Logging at handler boundary
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    tracking := libCommons.NewTrackingFromContext(ctx)

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
    if err != nil {
        // Log once at the boundary with full context
        tracking.Logger.Errorf("Failed to create account: %v", err)
        return http.WithError(c, err)
    }

    tracking.Logger.Infof("Account created successfully: %s", account.ID)
    return http.Created(c, account)
}
```

### Structured Logging

Always use structured logging with context:

```go
// ✅ GOOD - Structured logging
tracking.Logger.WithFields(map[string]interface{}{
    "account_id":      account.ID,
    "organization_id": organizationID,
    "ledger_id":       ledgerID,
    "account_type":    account.Type,
}).Infof("Account created")

// ❌ BAD - String interpolation loses structure
tracking.Logger.Infof("Account %s created in org %s", account.ID, organizationID)
```

## OpenTelemetry Error Integration

### Record Business Errors in Spans

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    ctx, span := tracer.Start(ctx, "usecase.create_account")
    defer span.End()

    account, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        // Record business error event in span
        if _, ok := err.(pkg.EntityConflictError); ok {
            libOpentelemetry.HandleSpanBusinessErrorEvent(span, err, "Account already exists")
        } else {
            // Record technical error
            span.RecordError(err)
            span.SetStatus(codes.Error, "Failed to create account")
        }
        return nil, fmt.Errorf("creating account: %w", err)
    }

    return account, nil
}
```

### Business vs Technical Error Distinction

```go
// Business error - expected failure, don't mark span as error
libOpentelemetry.HandleSpanBusinessErrorEvent(span, err, "Entity not found")

// Technical error - unexpected failure, mark span as error
span.RecordError(err)
span.SetStatus(codes.Error, err.Error())
```

## HTTP Error Response Pattern

### Using `http.WithError()`

Location: `pkg/net/http/response.go` (or similar)

```go
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    account, err := h.Command.CreateAccount(ctx, input)
    if err != nil {
        // Automatically converts error type to appropriate HTTP status
        return http.WithError(c, err)
    }
    return http.Created(c, account)
}
```

### Error to HTTP Status Mapping

```go
EntityNotFoundError    → 404 Not Found
ValidationError        → 400 Bad Request
EntityConflictError    → 409 Conflict
UnauthorizedError      → 401 Unauthorized
ForbiddenError         → 403 Forbidden
Default (other errors) → 500 Internal Server Error
```

### Error Response Format

```json
{
  "error": {
    "code": "ACCOUNT_NOT_FOUND",
    "message": "Account with ID abc123 not found",
    "details": {
      "entity_type": "Account",
      "id": "abc123"
    }
  }
}
```

## Assert Package Integration

### Domain Invariant Assertions

Location: `pkg/assert/assert.go`

The assert package is used for validating domain invariants and preconditions.

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    // Validate inputs - panics with detailed message if fails
    assert.NotEmpty(input.Name, "account name")
    assert.ValidUUID(input.OrganizationID, "organization ID")
    assert.That(input.Balance >= 0, "balance must be non-negative", "balance", input.Balance)

    // If we reach here, all assertions passed
    // ... continue with business logic
}
```

### Assert Functions

```go
// Check condition is true
assert.That(condition bool, message string, keyValuePairs ...interface{})

// Check value is not nil
assert.NotNil(value interface{}, message string)

// Check string/slice is not empty
assert.NotEmpty(value interface{}, message string)

// Check error is nil
assert.NoError(err error, message string)

// Mark code path that should never execute
assert.Never(message string, keyValuePairs ...interface{})

// UUID validation
assert.ValidUUID(value interface{}, fieldName string)
```

### Assert vs Error Returns

**Use Assert for**: Programmer errors, invariants that should never be violated
```go
// These indicate bugs if they fail
assert.NotNil(repository, "repository must be injected")
assert.ValidUUID(id, "ID")
```

**Use Error Returns for**: Business validation, expected failures
```go
// These are valid business scenarios
if input.Balance < 0 {
    return pkg.ValidationError{Code: constant.ErrInvalidAmount, Message: "balance cannot be negative"}
}
```

## Repository Error Handling

### Nil vs Error Pattern

```go
// ✅ GOOD - Return both entity and error
func (r *Repository) Find(ctx context.Context, id uuid.UUID) (*Account, error) {
    var account Account
    err := r.db.GetContext(ctx, &account, "SELECT * FROM accounts WHERE id = $1", id)
    if err != nil {
        if err == sql.ErrNoRows {
            // Not found is not an error - return nil entity
            return nil, nil
        }
        return nil, fmt.Errorf("querying account %s: %w", id, err)
    }
    return &account, nil
}

// In use case
account, err := repo.Find(ctx, id)
if err != nil {
    return nil, fmt.Errorf("finding account: %w", err)
}
if account == nil {
    // Not found - return business error
    return nil, pkg.EntityNotFoundError{EntityType: "Account", ID: id}
}
```

### PostgreSQL-Specific Error Handling

```go
import "github.com/lib/pq"

func (r *Repository) Create(ctx context.Context, account *Account) error {
    _, err := r.db.ExecContext(ctx, query, args...)
    if err != nil {
        // Check for PostgreSQL-specific errors
        if pqErr, ok := err.(*pq.Error); ok {
            switch pqErr.Code {
            case "23505": // unique_violation
                return pkg.EntityConflictError{EntityType: "Account", Field: extractField(pqErr)}
            case "23503": // foreign_key_violation
                return pkg.ValidationError{Code: constant.ErrInvalidForeignKey}
            case "23514": // check_violation
                return pkg.ValidationError{Code: constant.ErrConstraintViolation}
            }
        }
        return fmt.Errorf("executing query: %w", err)
    }
    return nil
}
```

## Testing Error Scenarios

### Table-Driven Error Tests

```go
func TestCreateAccount_Errors(t *testing.T) {
    tests := []struct {
        name          string
        input         Input
        setupMock     func(*mock.MockRepository)
        expectedError error
    }{
        {
            name: "validation error - empty name",
            input: Input{Name: "", Type: "DEPOSIT"},
            setupMock: func(m *mock.MockRepository) {},
            expectedError: pkg.ValidationError{Code: constant.ErrInvalidAccountName},
        },
        {
            name: "conflict error - duplicate name",
            input: Input{Name: "Checking", Type: "DEPOSIT"},
            setupMock: func(m *mock.MockRepository) {
                m.EXPECT().Create(gomock.Any(), gomock.Any()).Return(
                    pkg.EntityConflictError{EntityType: "Account", Field: "name"},
                )
            },
            expectedError: pkg.EntityConflictError{},
        },
        {
            name: "not found error - parent account missing",
            input: Input{Name: "Sub Account", ParentAccountID: &someID},
            setupMock: func(m *mock.MockRepository) {
                m.EXPECT().Find(gomock.Any(), someID).Return(nil, nil)
            },
            expectedError: pkg.EntityNotFoundError{EntityType: "ParentAccount"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            ctrl := gomock.NewController(t)
            defer ctrl.Finish()
            mockRepo := mock.NewMockRepository(ctrl)
            tt.setupMock(mockRepo)

            uc := NewUseCase(mockRepo)

            // Execute
            _, err := uc.CreateAccount(context.Background(), tt.input)

            // Assert
            assert.Error(t, err)
            assert.IsType(t, tt.expectedError, err)
        })
    }
}
```

## Error Handling Checklist

✅ **Always wrap errors** with `fmt.Errorf("context: %w", err)` at each layer

✅ **Use typed business errors** from `pkg/errors.go` for expected failures

✅ **Use error constants** from `pkg/constant/errors.go` for error codes

✅ **Log errors only at boundaries** (handlers), not in every function

✅ **Use structured logging** with key-value pairs, not string interpolation

✅ **Record errors in OpenTelemetry spans** with appropriate status

✅ **Distinguish business vs technical errors** in spans and logs

✅ **Return nil entity + nil error** for "not found" in repositories

✅ **Use assert package** for invariants and preconditions

✅ **Convert typed errors to HTTP status** using `http.WithError()`

✅ **Test error scenarios** with table-driven tests

## Common Anti-Patterns

❌ **Ignoring errors**
```go
account, _ := repo.Find(ctx, id)  // Never ignore errors!
```

❌ **Logging and returning error** (double reporting)
```go
if err != nil {
    log.Error(err)  // Don't log here
    return err      // AND return - log at boundary only
}
```

❌ **Using panic for business logic**
```go
if account == nil {
    panic("account not found")  // Use typed error instead!
}
```

❌ **Returning error strings instead of typed errors**
```go
return errors.New("account not found")  // Use EntityNotFoundError
```

❌ **Not wrapping errors**
```go
return err  // Lost context - wrap it!
```

## Related Documentation

- Architecture: `docs/agents/architecture.md`
- Testing: `docs/agents/testing.md`
- Observability: `docs/agents/observability.md`
- Concurrency: `docs/agents/concurrency.md`
