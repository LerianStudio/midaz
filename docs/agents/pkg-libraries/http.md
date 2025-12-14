# pkg/net/http - HTTP Utilities for Fiber

**Location**: `pkg/net/http/`
**Priority**: 🔥 High Use - Every HTTP handler uses this
**Status**: Production-ready Fiber utilities

Fiber handler utilities for request parsing, response building, and error handling with panic-safe extractors.

## Core Modules

- `locals.go` - Extract typed values from `c.Locals()` (panic-safe)
- `response.go` - HTTP response helpers (Created, OK, WithError, etc.)
- `httputils.go` - Query parameter validation and pagination
- `withBody.go` - Request body validation middleware
- `errors.go` - Error response mapping

## Local Extractors (Panic-Safe)

### LocalUUID

Extract UUID from `c.Locals()` - panics with rich context if not set or wrong type:

```go
func LocalUUID(c *fiber.Ctx, key string) uuid.UUID
```

**Usage:**
```go
organizationID := http.LocalUUID(c, "organization_id")
ledgerID := http.LocalUUID(c, "ledger_id")
accountID := http.LocalUUID(c, "account_id")
```

**Panic output if missing:**
```
assertion failed: middleware must set locals key
    key=organization_id
    path=/v1/organizations/:organization_id/ledgers
    method=GET
[stack trace]
```

### LocalUUIDOptional

Extract optional UUID - returns `uuid.Nil` if not set:

```go
func LocalUUIDOptional(c *fiber.Ctx, key string) uuid.UUID
```

**Usage:**
```go
portfolioID := http.LocalUUIDOptional(c, "portfolio_id")
if portfolioID != uuid.Nil {
    // Portfolio specified
}
```

### LocalStringSlice

Extract string slice from `c.Locals()`:

```go
func LocalStringSlice(c *fiber.Ctx, key string) []string
```

**Usage:**
```go
fieldsToRemove := http.LocalStringSlice(c, "patchRemove")
```

### Payload[T]

Assert payload type after validation middleware:

```go
func Payload[T any](c *fiber.Ctx, p any) T
```

**Usage:**
```go
func (h *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
    // Extract and assert type
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
    // ...
}
```

## Response Helpers

### Success Responses

```go
func OK(c *fiber.Ctx, s any) error           // 200
func Created(c *fiber.Ctx, s any) error      // 201
func Accepted(c *fiber.Ctx, s any) error     // 202
func NoContent(c *fiber.Ctx) error           // 204
func PartialContent(c *fiber.Ctx, s any) error // 206
```

**Usage:**
```go
// Success response with entity
return http.Created(c, account)

// Success response with list
return http.OK(c, accounts)

// Success response without body
return http.NoContent(c)
```

### Error Responses

```go
func BadRequest(c *fiber.Ctx, s any) error   // 400
func Unauthorized(c *fiber.Ctx, code, title, message string) error // 401
func Forbidden(c *fiber.Ctx, code, title, message string) error    // 403
func NotFound(c *fiber.Ctx, code, title, message string) error     // 404
func Conflict(c *fiber.Ctx, code, title, message string) error     // 409
func UnprocessableEntity(c *fiber.Ctx, code, title, message string) error // 422
func InternalServerError(c *fiber.Ctx, code, title, message string) error // 500
func NotImplemented(c *fiber.Ctx, message string) error // 501
```

**Usage:**
```go
// Manual error response
return http.NotFound(c, "0007", "Entity Not Found", "Account not found")

// Better: Use WithError for automatic mapping
return http.WithError(c, err)
```

### WithError - Automatic Error Mapping

Maps typed errors to HTTP responses:

```go
func WithError(c *fiber.Ctx, err error) error
```

**Usage:**
```go
account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, input)
if err != nil {
    // Automatically maps:
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
```

See [`errors.md`](./errors.md) for error types.

## Query Parameter Validation

### Pagination

```go
type Pagination struct {
    Limit     int
    Page      int
    Cursor    string
    SortOrder string
    StartDate time.Time
    EndDate   time.Time
}
```

### QueryHeader

Extended query parameters with metadata and filters:

```go
type QueryHeader struct {
    Pagination
    Metadata              *bson.M
    UseMetadata           bool
    PortfolioID           string
    OperationType         string
    ToAssetCodes          []string
    HolderID              *string
    ExternalID            *string
    Document              *string
    AccountID             *string
    LedgerID              *string
    BankingDetailsBranch  *string
    BankingDetailsAccount *string
    BankingDetailsIban    *string
}
```

### ValidateParameters

Validate and parse query parameters:

```go
func ValidateParameters(params map[string]string) (*QueryHeader, error)
```

**Usage:**
```go
func (h *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
    // Parse and validate query params
    headerParams, err := http.ValidateParameters(c.AllParams())
    if err != nil {
        return http.WithError(c, err)
    }

    // Use validated params
    accounts, err := h.Query.GetAllAccounts(ctx, organizationID, ledgerID, *headerParams)
    if err != nil {
        return http.WithError(c, err)
    }

    // Build pagination response
    pagination := &libPostgres.Pagination{
        Limit: headerParams.Limit,
        Page:  headerParams.Page,
    }
    pagination.SetItems(accounts)

    return http.OK(c, pagination)
}
```

**Default Values:**
- `limit`: 10
- `page`: 1
- `maxLimit`: 100
- `sortOrder`: "asc" or "desc"

## Common Handler Patterns

### Complete Handler with Error Handling

```go
func (h *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

    // Extract path parameters
    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")

    // Extract validated payload
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)
    logger.Infof("Request to create account: %#v", payload)

    // Start tracing
    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Call use case
    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
        if httpErr := http.WithError(c, err); httpErr != nil {
            return fmt.Errorf("http response error: %w", httpErr)
        }
        return nil
    }

    // Record metrics
    metricFactory.RecordAccountCreated(ctx, organizationID.String(), ledgerID.String())

    // Success response
    logger.Infof("Successfully created account: %s", account.ID)
    return http.Created(c, account)
}
```

### List Handler with Pagination

```go
func (h *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")

    // Parse query parameters
    headerParams, err := http.ValidateParameters(c.AllParams())
    if err != nil {
        return http.WithError(c, err)
    }

    ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
    defer span.End()

    // Get accounts
    accounts, err := h.Query.GetAllAccounts(ctx, organizationID, ledgerID, *headerParams)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve accounts", err)
        return http.WithError(c, err)
    }

    // Build pagination response
    pagination := &libPostgres.Pagination{
        Limit: headerParams.Limit,
        Page:  headerParams.Page,
    }
    pagination.SetItems(accounts)

    logger.Infof("Successfully retrieved %d accounts", len(accounts))
    return http.OK(c, pagination)
}
```

### Get By ID Handler

```go
func (h *AccountHandler) GetAccountByID(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")
    accountID := http.LocalUUID(c, "account_id")

    ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
    defer span.End()

    account, err := h.Query.GetAccountByID(ctx, organizationID, ledgerID, accountID)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Account not found", err)
        return http.WithError(c, err)
    }

    logger.Infof("Successfully retrieved account: %s", accountID)
    return http.OK(c, account)
}
```

### Update Handler

```go
func (h *AccountHandler) UpdateAccount(i any, c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")
    accountID := http.LocalUUID(c, "account_id")

    payload := http.Payload[*mmodel.UpdateAccountInput](c, i)

    ctx, span := tracer.Start(ctx, "handler.update_account")
    defer span.End()

    account, err := h.Command.UpdateAccount(ctx, organizationID, ledgerID, accountID, payload)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account", err)
        return http.WithError(c, err)
    }

    logger.Infof("Successfully updated account: %s", accountID)
    return http.OK(c, account)
}
```

### Delete Handler

```go
func (h *AccountHandler) DeleteAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")
    accountID := http.LocalUUID(c, "account_id")

    ctx, span := tracer.Start(ctx, "handler.delete_account")
    defer span.End()

    err := h.Command.DeleteAccount(ctx, organizationID, ledgerID, accountID)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete account", err)
        return http.WithError(c, err)
    }

    logger.Infof("Successfully deleted account: %s", accountID)
    return http.NoContent(c)
}
```

## Panic Safety

Functions in this package may panic on programming errors (e.g., missing middleware, wrong payload types). These panics are caught by Fiber's recover middleware and converted to 500 responses with logging.

**Example:**
```go
// If middleware doesn't set "organization_id", LocalUUID panics:
organizationID := http.LocalUUID(c, "organization_id")

// Panic is caught by Fiber middleware:
// - Logs panic with stack trace
// - Returns 500 Internal Server Error
// - Includes panic message in response (if debug mode)
```

This design ensures fast-fail on wiring mistakes during development while providing safe error responses in production.

## References

- **Source**: `pkg/net/http/*.go`
- **Locals**: `pkg/net/http/locals.go:1`
- **Response**: `pkg/net/http/response.go:1`
- **Validation**: `pkg/net/http/httputils.go:1`
- **Error Handling**: `pkg/net/http/errors.go:1`
- **WithBody**: `pkg/net/http/withBody.go:1`
- **Related**: [`errors.md`](./errors.md) for error types
- **Related**: [`../libcommons.md`](../libcommons.md) for observability

## Summary

`pkg/net/http` provides Fiber handler utilities:

1. **Panic-safe extractors** - LocalUUID, Payload with rich error context
2. **Response helpers** - Created, OK, WithError for consistent responses
3. **Error mapping** - Automatic typed error → HTTP status mapping
4. **Query validation** - ValidateParameters for pagination and filters
5. **Standard patterns** - Complete handler examples for CRUD operations

Every HTTP handler should use these utilities for consistency and safety.
