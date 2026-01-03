# lib-commons - External Observability Library

**Package**: `github.com/LerianStudio/lib-commons/v2`
**Version**: `v2.6.1` (see `go.mod`)

`lib-commons` is an external library providing observability, database utilities, and common patterns for Midaz services. It's the foundation for logging, tracing, metrics, and database operations.

## Quick Reference

| Package | Purpose | Import Alias |
|---------|---------|--------------|
| `commons` | Tracking context extraction | `libCommons` |
| `commons/opentelemetry` | OpenTelemetry utilities | `libOpentelemetry` |
| `commons/postgres` | PostgreSQL connection & pagination | `libPostgres` |
| `commons/redis` | Redis/Valkey utilities | `libRedis` |
| `commons/log` | Logging interface | - |
| `commons/constants` | Common constants | `libConstants` |
| `commons/net/http` | HTTP utilities | `libHTTP` |
| `commons/pointers` | Pointer helpers | `libPointers` |

---

## Core Pattern: NewTrackingFromContext

**Most Important Function** - Extract observability tools from context.

```go
logger, tracer, ctx, metricFactory := libCommons.NewTrackingFromContext(ctx)
```

Returns:
- `logger` (`log.Logger`) - Structured logger (Infof, Errorf, etc.)
- `tracer` (`trace.Tracer`) - OpenTelemetry tracer for creating spans
- `ctx` (`context.Context`) - Context (usually same as input, but can be enhanced)
- `metricFactory` - Metrics recorder with domain-specific metrics

### Usage Pattern

**Every handler, use case, and repository should start with this**:

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()

    // Extract tracking tools
    logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

    // Use logger for structured logging
    logger.Infof("Request to create account: %#v", payload)

    // Use tracer to create spans
    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Use metricFactory to record metrics
    metricFactory.RecordAccountCreated(ctx, organizationID.String(), ledgerID.String())

    // ... handler logic
}
```

---

## libCommons - Tracking & Context

**Import**: `libCommons "github.com/LerianStudio/lib-commons/v2/commons"`

### Key Functions

```go
// Extract logger, tracer, context, and metrics from context
func NewTrackingFromContext(ctx context.Context) (
    logger log.Logger,
    tracer trace.Tracer,
    ctx context.Context,
    metricFactory any,
)

// Check if string pointer is nil or empty
func IsNilOrEmpty(s *string) bool
```

### Usage Examples

#### Standard handler pattern

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()

    logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)

    logger.Infof("Request to create Account: %#v", payload)

    // Check optional field
    if !libCommons.IsNilOrEmpty(payload.PortfolioID) {
        logger.Infof("Creating account with Portfolio ID: %s", *payload.PortfolioID)
    }

    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
        logger.Errorf("Create account failed: %v", err)
        return http.WithError(c, err)
    }

    metricFactory.RecordAccountCreated(ctx, organizationID.String(), ledgerID.String())
    logger.Infof("Successfully created Account: %s", account.ID)

    return http.Created(c, account)
}
```

#### Repository pattern

```go
func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "postgres.create_account")
    defer span.End()

    db, err := r.connection.GetDB()
    if err != nil {
        libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
        logger.Errorf("Failed to get database connection: %v", err)
        return nil, fmt.Errorf("failed to get database connection: %w", err)
    }

    // ... SQL operations

    logger.Infof("Created account with ID: %s", result.ID)
    return result, nil
}
```

---

## libOpentelemetry - OpenTelemetry Utilities

**Import**: `libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"`

### Span Management Functions

```go
// Record business error event on span (for expected errors)
func HandleSpanBusinessErrorEvent(span *trace.Span, message string, err error)

// Record system error on span and set status to Error (for infrastructure errors)
func HandleSpanError(span *trace.Span, message string, err error)

// Set span attributes from struct (converts struct to JSON attributes)
func SetSpanAttributesFromStruct(span *trace.Span, key string, value any) error
```

### Usage Examples

#### Tracing handler with request payload

```go
func (h *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    payload := http.Payload[*mmodel.CreateAccountInput](c, i)

    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Add request payload as span attributes
    err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to serialize payload", err)
        logger.Errorf("Failed to serialize payload: %v", err)
        return http.WithError(c, err)
    }

    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
    if err != nil {
        // Business error (validation, not found, etc.)
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
        logger.Errorf("Create account failed: %v", err)
        return http.WithError(c, err)
    }

    return http.Created(c, account)
}
```

#### Tracing repository with database errors

```go
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Account, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "postgres.find_account")
    defer span.End()

    db, err := r.connection.GetDB()
    if err != nil {
        // Infrastructure error - set span status to Error
        libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
        logger.Errorf("Failed to get database connection: %v", err)
        return nil, fmt.Errorf("failed to get database connection: %w", err)
    }

    var record AccountPostgreSQLModel
    err = db.QueryRowContext(ctx, query, id).Scan(&record)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            // Business error (not found)
            libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Account not found", err)
            return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "account")
        }

        // Infrastructure error (database failure)
        libOpentelemetry.HandleSpanError(&span, "Query failed", err)
        logger.Errorf("Query failed: %v", err)
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return record.ToEntity(), nil
}
```

### When to Use Each Function

| Function | When to Use | Effect |
|----------|-------------|--------|
| `HandleSpanBusinessErrorEvent` | Expected errors (validation, not found, conflicts) | Records event, does NOT set span status to Error |
| `HandleSpanError` | Unexpected errors (database down, network failure) | Records event AND sets span status to Error |
| `SetSpanAttributesFromStruct` | Add structured data to span (request payload, entity) | Converts to JSON attributes |

---

## libPostgres - PostgreSQL Utilities

**Import**: `libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"`

### Key Types

```go
// PostgreSQL connection wrapper
type PostgresConnection struct {
    // ... internal fields
}

// Get underlying *sql.DB connection
func (pc *PostgresConnection) GetDB() (*sql.DB, error)

// Pagination response structure
type Pagination struct {
    Items      any    `json:"items"`
    Page       int    `json:"page"`
    Limit      int    `json:"limit"`
    TotalItems int64  `json:"totalItems,omitempty"`
    TotalPages int    `json:"totalPages,omitempty"`
}

// Set items and calculate totals
func (p *Pagination) SetItems(items any)
```

### Usage Examples

#### Repository constructor with connection

```go
type AccountPostgreSQLRepository struct {
    connection *libPostgres.PostgresConnection
    tableName  string
}

func NewAccountPostgreSQLRepository(pc *libPostgres.PostgresConnection) *AccountPostgreSQLRepository {
    assert.NotNil(pc, "PostgreSQL connection must not be nil",
        "repository", "AccountPostgreSQLRepository")

    c := &AccountPostgreSQLRepository{
        connection: pc,
        tableName:  "account",
    }

    // Verify connection is working
    db, err := c.connection.GetDB()
    assert.NoError(err, "database connection required for AccountPostgreSQLRepository",
        "repository", "AccountPostgreSQLRepository")
    assert.NotNil(db, "database handle must not be nil",
        "repository", "AccountPostgreSQLRepository")

    return c
}
```

#### Query execution

```go
func (r *AccountPostgreSQLRepository) Find(ctx context.Context, id uuid.UUID) (*mmodel.Account, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "postgres.find_account")
    defer span.End()

    // Get database connection
    db, err := r.connection.GetDB()
    if err != nil {
        libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
        logger.Errorf("Failed to get database connection: %v", err)
        return nil, fmt.Errorf("failed to get database connection: %w", err)
    }

    // Execute query
    var record AccountPostgreSQLModel
    query := `SELECT * FROM account WHERE id = $1 AND deleted_at IS NULL`
    err = db.QueryRowContext(ctx, query, id).Scan(&record.ID, &record.Name, /* ... */)

    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "account")
        }
        logger.Errorf("Query failed: %v", err)
        return nil, fmt.Errorf("query failed: %w", err)
    }

    return record.ToEntity(), nil
}
```

#### Pagination in handler

```go
func (handler *AccountHandler) GetAllAccounts(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
    defer span.End()

    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")

    // Parse query parameters
    headerParams, err := http.ValidateParameters(c.AllParams())
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid query parameters", err)
        return http.WithError(c, err)
    }

    // Create pagination object
    pagination := &libPostgres.Pagination{
        Limit: headerParams.Limit,
        Page:  headerParams.Page,
    }

    // Get accounts from repository
    accounts, err := handler.Query.GetAllAccounts(ctx, organizationID, ledgerID, *headerParams)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve accounts", err)
        return http.WithError(c, err)
    }

    // Set items (calculates TotalPages if TotalItems is set)
    pagination.SetItems(accounts)

    logger.Infof("Successfully retrieved %d accounts", len(accounts))
    return http.OK(c, pagination)
}
```

---

## libRedis - Redis/Valkey Utilities

**Import**: `libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"`

Redis/Valkey connection utilities for caching and session management.

### Usage

```go
// Used in httputils.go for query parameter validation
// See: pkg/net/http/httputils.go
```

---

## libConstants - Common Constants

**Import**: `libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"`

Shared constants across lib-commons.

---

## libHTTP - HTTP Utilities

**Import**: `libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"`

HTTP client and server utilities from lib-commons.

**Note**: Midaz primarily uses its own `pkg/net/http` utilities for Fiber handlers.

---

## libPointers - Pointer Helpers

**Import**: `libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"`

Pointer utility functions (similar to `pkg/utils/ptr.go`).

**Note**: Prefer using `pkg/utils` pointer helpers for consistency within Midaz codebase.

---

## Standard Usage Patterns

### Complete Handler Pattern with lib-commons

```go
package in

import (
    libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
    libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
    "github.com/LerianStudio/midaz/v3/pkg/assert"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
    "github.com/LerianStudio/midaz/v3/pkg/net/http"
    "github.com/gofiber/fiber/v2"
)

type AccountHandler struct {
    Command *command.UseCase
    Query   *query.UseCase
}

func (h *AccountHandler) CreateAccount(i any, c *fiber.Ctx) error {
    // 1. Get context from Fiber
    ctx := c.UserContext()

    // 2. Extract tracking context (logger, tracer, metrics)
    logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

    // 3. Extract path parameters (using pkg/net/http)
    organizationID := http.LocalUUID(c, "organization_id")
    ledgerID := http.LocalUUID(c, "ledger_id")

    // 4. Extract validated payload (using pkg/net/http)
    payload := http.Payload[*mmodel.CreateAccountInput](c, i)
    logger.Infof("Request to create Account: %#v", payload)

    // 5. Check optional fields (using libCommons)
    if !libCommons.IsNilOrEmpty(payload.PortfolioID) {
        logger.Infof("Creating account with Portfolio ID: %s", *payload.PortfolioID)
    }

    // 6. Start tracing span (using libCommons tracer)
    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // 7. Add request payload to span (using libOpentelemetry)
    err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to serialize payload", err)
        logger.Errorf("Failed to serialize payload: %v", err)
        return http.WithError(c, err)
    }

    // 8. Call use case
    account, err := h.Command.CreateAccount(ctx, organizationID, ledgerID, payload)
    if err != nil {
        // 9. Handle error (using libOpentelemetry + pkg/net/http)
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
        logger.Errorf("Create account failed: %v", err)

        if httpErr := http.WithError(c, err); httpErr != nil {
            return fmt.Errorf("http response error: %w", httpErr)
        }
        return nil
    }

    // 10. Record metrics (using libCommons metricFactory)
    metricFactory.RecordAccountCreated(ctx, organizationID.String(), ledgerID.String())

    // 11. Return success response (using pkg/net/http)
    logger.Infof("Successfully created Account: %s", account.ID)
    return http.Created(c, account)
}
```

### Complete Use Case Pattern with lib-commons

```go
package command

import (
    libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
    "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/assert"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

type UseCase struct {
    AccountRepo account.Repository
    LedgerRepo  ledger.Repository
}

func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID,
    input *mmodel.CreateAccountInput) (*mmodel.Account, error) {

    // 1. Extract tracking context
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    // 2. Start span
    ctx, span := tracer.Start(ctx, "use_case.create_account")
    defer span.End()

    // 3. Validate preconditions (using pkg/assert)
    assert.NotNil(input, "input must not be nil")
    assert.NotEmpty(input.Name, "account name required")
    assert.NotEmpty(input.AssetCode, "asset code required")

    // 4. Verify ledger exists
    _, err := uc.LedgerRepo.Find(ctx, organizationID, ledgerID)
    if err != nil {
        libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Ledger not found", err)
        logger.Errorf("Ledger not found: %v", err)
        return nil, pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, "account")
    }

    // 5. Check for duplicate alias (using libCommons)
    if !libCommons.IsNilOrEmpty(input.Alias) {
        exists, _ := uc.AccountRepo.FindByAlias(ctx, organizationID, ledgerID, *input.Alias)
        if exists {
            err := pkg.ValidateBusinessError(constant.ErrAliasUnavailability, "account", *input.Alias)
            libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Alias already exists", err)
            logger.Errorf("Alias already exists: %s", *input.Alias)
            return nil, err
        }
    }

    // 6. Create entity
    account := &mmodel.Account{
        ID:             uuid.New(),
        Name:           input.Name,
        AssetCode:      input.AssetCode,
        OrganizationID: organizationID,
        LedgerID:       ledgerID,
        Status:         mmodel.Status{Code: mmodel.ACTIVE},
        CreatedAt:      time.Now(),
    }

    // 7. Persist to database
    created, err := uc.AccountRepo.Create(ctx, account)
    if err != nil {
        libOpentelemetry.HandleSpanError(&span, "Failed to create account in repository", err)
        logger.Errorf("Failed to create account: %v", err)
        return nil, fmt.Errorf("failed to create account: %w", err)
    }

    // 8. Validate postconditions (using pkg/assert)
    assert.NotNil(created, "created account must not be nil")
    assert.NotEmpty(created.ID.String(), "created account must have ID")

    logger.Infof("Successfully created account: %s", created.ID)
    return created, nil
}
```

### Complete Repository Pattern with lib-commons

```go
package account

import (
    libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
    libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
    "github.com/LerianStudio/midaz/v3/pkg/assert"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

type AccountPostgreSQLRepository struct {
    connection *libPostgres.PostgresConnection
    tableName  string
}

func NewAccountPostgreSQLRepository(pc *libPostgres.PostgresConnection) *AccountPostgreSQLRepository {
    // Validate constructor arguments (using pkg/assert)
    assert.NotNil(pc, "PostgreSQL connection must not be nil",
        "repository", "AccountPostgreSQLRepository")

    c := &AccountPostgreSQLRepository{
        connection: pc,
        tableName:  "account",
    }

    // Verify database connection (using libPostgres)
    db, err := c.connection.GetDB()
    assert.NoError(err, "database connection required for AccountPostgreSQLRepository",
        "repository", "AccountPostgreSQLRepository")
    assert.NotNil(db, "database handle must not be nil",
        "repository", "AccountPostgreSQLRepository")

    return c
}

func (r *AccountPostgreSQLRepository) Create(ctx context.Context, acc *mmodel.Account) (*mmodel.Account, error) {
    // 1. Validate preconditions (using pkg/assert)
    assert.NotNil(acc, "account entity must not be nil for Create",
        "repository", "AccountPostgreSQLRepository")

    // 2. Extract tracking context (using libCommons)
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    // 3. Start span (using libCommons tracer)
    ctx, span := tracer.Start(ctx, "postgres.create_account")
    defer span.End()

    // 4. Get database connection (using libPostgres)
    db, err := r.connection.GetDB()
    if err != nil {
        // Infrastructure error (using libOpentelemetry)
        libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
        logger.Errorf("Failed to get database connection: %v", err)
        return nil, fmt.Errorf("failed to get database connection: %w", err)
    }

    // 5. Build and execute SQL query
    record := &AccountPostgreSQLModel{}
    record.FromEntity(acc)

    query := `
        INSERT INTO account (id, name, asset_code, organization_id, ledger_id, status, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING id, created_at
    `

    err = db.QueryRowContext(ctx, query, record.ID, record.Name, record.AssetCode,
        record.OrganizationID, record.LedgerID, record.Status, record.CreatedAt).
        Scan(&record.ID, &record.CreatedAt)

    if err != nil {
        // Database error (using libOpentelemetry)
        libOpentelemetry.HandleSpanError(&span, "Failed to insert account", err)
        logger.Errorf("Failed to insert account: %v", err)
        return nil, fmt.Errorf("failed to insert account: %w", err)
    }

    // 6. Convert result back to entity
    result := record.ToEntity()

    // 7. Validate postconditions (using pkg/assert)
    assert.NotNil(result, "repository must return non-nil account")
    assert.NotEmpty(result.ID.String(), "created account must have ID")

    logger.Infof("Created account with ID: %s", result.ID)
    return result, nil
}
```

---

## Integration Points

### lib-commons + pkg/ Integration

| Scenario | lib-commons | pkg/ |
|----------|-------------|------|
| **Observability** | `NewTrackingFromContext` extracts logger/tracer | `assert` for invariants, `mruntime` for panic recovery |
| **Tracing** | `libOpentelemetry.HandleSpan*` for span events | `pkg/net/http` handlers create spans |
| **Database** | `libPostgres.PostgresConnection` for connections | `pkg/net/http` for pagination, `pkg/assert` for validation |
| **Errors** | Logging with `logger.Errorf()` | `pkg/errors` for typed business errors |
| **HTTP** | `libPostgres.Pagination` for response | `pkg/net/http` for request/response helpers |

### Typical Stack Layers

```
┌─────────────────────────────────────────┐
│  HTTP Handler (Fiber)                   │
│  - libCommons.NewTrackingFromContext()  │
│  - pkg/net/http.LocalUUID()             │
│  - pkg/net/http.Payload()               │
│  - libOpentelemetry.HandleSpan*()       │
│  - pkg/net/http.Created()/WithError()   │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│  Use Case (Command/Query)               │
│  - libCommons.NewTrackingFromContext()  │
│  - pkg/assert for preconditions         │
│  - pkg/errors for business errors       │
│  - libOpentelemetry.HandleSpan*()       │
└─────────────────────────────────────────┘
                  ↓
┌─────────────────────────────────────────┐
│  Repository (PostgreSQL)                │
│  - libPostgres.PostgresConnection       │
│  - libCommons.NewTrackingFromContext()  │
│  - pkg/assert for nil checks            │
│  - libOpentelemetry.HandleSpan*()       │
└─────────────────────────────────────────┘
```

---

## Summary

`lib-commons` is the **observability backbone** of Midaz:

1. **Always start with** `libCommons.NewTrackingFromContext(ctx)` in handlers, use cases, and repositories
2. **Use** `libOpentelemetry.HandleSpan*()` for recording span events (business vs infrastructure errors)
3. **Use** `libPostgres.PostgresConnection` for database access
4. **Combine with** `pkg/` libraries:
   - `pkg/assert` for invariants
   - `pkg/errors` for typed business errors
   - `pkg/net/http` for HTTP utilities
   - `pkg/mruntime` for safe goroutines

The combination of `lib-commons` (external) and `pkg/` (internal) provides a complete foundation for building observable, reliable Midaz services.

---

## External Library

**Important**: `lib-commons` is maintained in a separate repository. For detailed API documentation, refer to:
- GitHub: `https://github.com/LerianStudio/lib-commons` (if public)
- Internal documentation (if available)
- Source code: `$GOPATH/pkg/mod/github.com/!lerian!studio/lib-commons@v2.6.0-beta.4/`
