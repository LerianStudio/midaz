# pkg/errors - Typed Business Errors

**Location**: `pkg/errors.go:1`
**Priority**: ⚠️ CRITICAL - Core error handling system
**Status**: Production-ready, HTTP-mapped error types

Structured error types for domain-specific errors that automatically map to HTTP status codes and provide user-facing messages.

## Design Philosophy

**Use typed errors for business failures** - Conditions users can cause (invalid input, not found, conflicts)
**Use assert for programming bugs** - Conditions that should never happen if code is correct
**Always include context** - EntityType, Title, Message, Code for debugging

## Error Type Hierarchy

All error types implement `error` interface and support Go 1.13+ error wrapping via `Unwrap()`.

### HTTP 404 - Not Found
```go
type EntityNotFoundError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error  // wrapped error
}
```

### HTTP 400 - Bad Request / Validation
```go
type ValidationError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}

// With field-level errors
type ValidationKnownFieldsError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
    Fields     FieldValidations  // map[string]string
}

// With unknown fields
type ValidationUnknownFieldsError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
    Fields     UnknownFields  // map[string]any
}
```

### HTTP 409 - Conflict
```go
type EntityConflictError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

### HTTP 401 - Unauthorized
```go
type UnauthorizedError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

### HTTP 403 - Forbidden
```go
type ForbiddenError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

### HTTP 422 - Unprocessable Entity
```go
type UnprocessableOperationError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

### HTTP 412 - Precondition Failed
```go
type FailedPreconditionError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

### HTTP 500 - Internal Server Error
```go
type InternalServerError struct {
    EntityType string
    Title      string
    Message    string
    Code       string
    Err        error
}
```

## Helper Functions

### ValidateBusinessError

Map constant error codes to typed business errors with user-facing messages:

```go
func ValidateBusinessError(err error, entityType string, args ...any) error
```

**Usage:**
```go
// Maps constant.ErrLedgerIDNotFound → EntityNotFoundError with message
return pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, "account")

// With format args
return pkg.ValidateBusinessError(constant.ErrAliasUnavailability, "account", aliasValue)

// Maps constant.ErrInsufficientFunds → UnprocessableOperationError
return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "transaction")
```

**Error Code Mappings (examples):**
- `constant.ErrDuplicateLedger` → `EntityConflictError` (409)
- `constant.ErrEntityNotFound` → `EntityNotFoundError` (404)
- `constant.ErrInvalidCountryCode` → `ValidationError` (400)
- `constant.ErrTokenMissing` → `UnauthorizedError` (401)
- `constant.ErrInsufficientPrivileges` → `ForbiddenError` (403)
- `constant.ErrInsufficientFunds` → `UnprocessableOperationError` (422)

See `pkg/errors.go:324` for complete mapping (100+ error codes).

### ValidateBadRequestFieldsError

Create validation errors with field-level details:

```go
func ValidateBadRequestFieldsError(
    requiredFields, knownInvalidFields map[string]string,
    entityType string,
    unknownFields map[string]any,
) error
```

**Usage:**
```go
// Missing required fields
requiredFields := map[string]string{
    "name": "Name is required",
    "asset_code": "Asset code is required",
}
return pkg.ValidateBadRequestFieldsError(requiredFields, nil, "account", nil)

// Invalid field values
invalidFields := map[string]string{
    "email": "Invalid email format",
    "amount": "Amount must be positive",
}
return pkg.ValidateBadRequestFieldsError(nil, invalidFields, "transaction", nil)

// Unknown fields in request
unknownFields := map[string]any{
    "unexpected_field": "value",
}
return pkg.ValidateBadRequestFieldsError(nil, nil, "account", unknownFields)
```

### ValidateInternalError

Create internal server errors:

```go
func ValidateInternalError(err error, entityType string) error
```

**Usage:**
```go
if err := db.Ping(); err != nil {
    return pkg.ValidateInternalError(err, "account")
}
```

### ValidateUnmarshallingError

Create JSON unmarshalling errors:

```go
func ValidateUnmarshallingError(err error) error
```

**Usage:**
```go
var input CreateAccountInput
if err := c.BodyParser(&input); err != nil {
    return pkg.ValidateUnmarshallingError(err)
}
```

## Usage Patterns

### Use Case Pattern - Return Business Errors

```go
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID,
    input *CreateAccountInput) (*Account, error) {

    // Check if ledger exists
    _, err := uc.LedgerRepo.Find(ctx, organizationID, ledgerID)
    if err != nil {
        // Not found → EntityNotFoundError
        return nil, pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, "account")
    }

    // Check for duplicate alias
    exists, _ := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, input.Alias)
    if exists {
        // Duplicate → EntityConflictError
        return nil, pkg.ValidateBusinessError(constant.ErrAliasUnavailability,
            "account", input.Alias)
    }

    // Create account
    account, err := uc.AccountRepo.Create(ctx, &Account{...})
    if err != nil {
        // Infrastructure error → InternalServerError
        return nil, pkg.ValidateInternalError(err, "account")
    }

    return account, nil
}
```

### Handler Pattern - Map Errors to HTTP Responses

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
    if err != nil {
        // http.WithError automatically maps typed errors to HTTP status codes:
        // - EntityNotFoundError → 404
        // - ValidationError → 400
        // - EntityConflictError → 409
        // - UnauthorizedError → 401
        // - ForbiddenError → 403
        // - UnprocessableOperationError → 422
        // - InternalServerError → 500
        if httpErr := http.WithError(c, err); httpErr != nil {
            return fmt.Errorf("http response error: %w", httpErr)
        }
        return nil
    }

    return http.Created(c, account)
}
```

### Validation Pattern - Field-Level Errors

```go
func ValidateCreateAccountInput(input *CreateAccountInput) error {
    requiredFields := make(map[string]string)
    invalidFields := make(map[string]string)

    if input.Name == "" {
        requiredFields["name"] = "Name is required"
    }

    if input.AssetCode == "" {
        requiredFields["asset_code"] = "Asset code is required"
    }

    if input.Type != "" && !isValidAccountType(input.Type) {
        invalidFields["type"] = "Invalid account type"
    }

    if len(requiredFields) > 0 || len(invalidFields) > 0 {
        return pkg.ValidateBadRequestFieldsError(requiredFields, invalidFields,
            "account", nil)
    }

    return nil
}
```

### Repository Pattern - Distinguish Infrastructure vs Business Errors

```go
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*Account, error) {
    db, err := r.connection.GetDB()
    if err != nil {
        // Infrastructure error
        return nil, fmt.Errorf("failed to get database connection: %w", err)
    }

    var account Account
    err = db.QueryRowContext(ctx, query, id).Scan(&account)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            // Business error - not found
            return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "account")
        }
        // Infrastructure error - database failure
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return &account, nil
}
```

## Error Wrapping Pattern

Always wrap infrastructure errors with context:

```go
// ✅ GOOD - Wrap with context
if err := db.Query(...); err != nil {
    return fmt.Errorf("failed to query accounts: %w", err)
}

// ❌ BAD - Return raw error
if err := db.Query(...); err != nil {
    return err  // Loses context
}
```

## Integration with HTTP Package

The `pkg/net/http.WithError()` function automatically maps typed errors:

```go
// In handler
if err != nil {
    if httpErr := http.WithError(c, err); httpErr != nil {
        return fmt.Errorf("http response error: %w", httpErr)
    }
    return nil
}
```

**Mapping Table:**

| Error Type | HTTP Status | JSON Response |
|------------|-------------|---------------|
| `EntityNotFoundError` | 404 | `{"code": "0007", "title": "...", "message": "..."}` |
| `ValidationError` | 400 | `{"code": "0009", "title": "...", "message": "..."}` |
| `ValidationKnownFieldsError` | 400 | `{"code": "0009", "fields": {...}}` |
| `ValidationUnknownFieldsError` | 400 | `{"code": "...", "fields": {...}}` |
| `EntityConflictError` | 409 | `{"code": "0001", "title": "...", "message": "..."}` |
| `UnauthorizedError` | 401 | `{"code": "...", "title": "...", "message": "..."}` |
| `ForbiddenError` | 403 | `{"code": "...", "title": "...", "message": "..."}` |
| `UnprocessableOperationError` | 422 | `{"code": "0018", "title": "...", "message": "..."}` |
| `FailedPreconditionError` | 412 | `{"code": "...", "title": "...", "message": "..."}` |
| `InternalServerError` | 500 | `{"code": "...", "title": "...", "message": "..."}` |

See [`http.md`](./http.md) for HTTP response helpers.

## References

- **Source**: `pkg/errors.go:1`
- **Tests**: `pkg/errors_test.go:1`
- **Constants**: See `pkg/constant/errors.go` for error codes
- **Related**: [`http.md`](./http.md) for HTTP error handling
- **Related**: [`assert.md`](./assert.md) for programming bug assertions
- **Related**: `docs/agents/error-handling.md` for error patterns

## Summary

`pkg/errors` provides typed business errors:

1. **Use for expected failures** - User input, not found, conflicts
2. **Map to HTTP automatically** - via `http.WithError()`
3. **Always wrap infrastructure errors** - with `fmt.Errorf("context: %w", err)`
4. **Include rich context** - EntityType, Title, Message, Code
5. **Combine with assert** - assert for bugs, typed errors for business failures

The error system ensures users get helpful messages while developers get debugging context.
