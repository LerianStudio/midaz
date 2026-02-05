# Midaz Project Rules

> **Auto-generated from codebase exploration on 2026-02-02**
> This document captures the coding standards, architectural patterns, and conventions discovered in the Midaz open-source ledger project.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Project Structure](#2-project-structure)
3. [Coding Standards](#3-coding-standards)
4. [Error Handling](#4-error-handling)
5. [Validation Patterns](#5-validation-patterns)
6. [API Design](#6-api-design)
7. [Database Patterns](#7-database-patterns)
8. [Configuration & Environment](#8-configuration--environment)
9. [Testing Standards](#9-testing-standards)
10. [Build & CI/CD](#10-build--cicd)
11. [Git Conventions](#11-git-conventions)
12. [Background Workers](#12-background-workers)
13. [Inter-Module Ports](#13-inter-module-ports)

---

## 1. Architecture Overview

### Primary Pattern: Hexagonal Architecture (Ports & Adapters)

Midaz implements hexagonal architecture with clear separation between:
- **Domain Layer**: Business logic in `services/command/` and `services/query/`
- **Port Layer**: Interfaces in `pkg/mbootstrap/` defining contracts
- **Adapter Layer**: Technology-specific implementations in `adapters/`
- **Bootstrap Layer**: Dependency injection in `bootstrap/`

### Secondary Pattern: CQRS (Command Query Responsibility Segregation)

Services split into:
- `services/command/` - Write operations (mutations)
- `services/query/` - Read operations (projections)

### Deployment Modes

| Mode | Description | Port |
|------|-------------|------|
| **Unified Ledger** | Single process composing onboarding + transaction via in-process calls | 3002 |
| **Microservices** | Independent services communicating via gRPC | 3000/3001 |

```
┌─────────────────────────────────────────────────────────────┐
│                    UNIFIED LEDGER MODE                      │
│  UnifiedServer (Fiber App) - Single HTTP Port :3002         │
├─────────────────────────────────────────────────────────────┤
│  /v1/organizations/*  → Onboarding Routes                   │
│  /v1/organizations/*/transactions/* → Transaction Routes    │
│  /v1/settings/metadata-indexes/* → Metadata Index Routes    │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    MICROSERVICES MODE                       │
├──────────────────────────┬──────────────────────────────────┤
│  Onboarding :3000        │  Transaction :3001 (HTTP)        │
│  - Organizations         │  - Transactions                  │
│  - Ledgers               │  - Operations                    │
│  - Accounts              │  - Balances                      │
│  - Assets                │  - Asset Rates                   │
│  - Portfolios            │                                  │
│  - Segments              │  gRPC :3011 (Balance Service)    │
└──────────────────────────┴──────────────────────────────────┘
```

### Component Communication

In **Unified Mode**, the `ledger` component composes both modules with in-process calls:

```go
// Transaction module initialized first to expose BalancePort
transactionService := transaction.InitServiceWithOptionsOrError(&transaction.Options{Logger: logger})
balancePort := transactionService.GetBalancePort()

// Onboarding module uses BalancePort for direct in-process calls (no gRPC overhead)
onboardingService := onboarding.InitServiceWithOptionsOrError(&onboarding.Options{
    UnifiedMode: true,
    BalancePort: balancePort,  // Direct UseCase reference
})
```

In **Microservices Mode**, onboarding calls transaction via gRPC:

```go
// gRPC adapter wraps network calls
grpcConnection := &mgrpc.GRPCConnection{Addr: "midaz-transaction:3011"}
balancePort := grpcout.NewBalanceAdapter(grpcConnection)
```

---

## 2. Project Structure

### Component Layout

```
components/{service}/
├── cmd/app/main.go           # Entry point
├── internal/
│   ├── adapters/
│   │   ├── http/in/          # HTTP inbound handlers
│   │   ├── grpc/in/          # gRPC inbound handlers
│   │   ├── grpc/out/         # gRPC outbound clients
│   │   ├── postgres/         # PostgreSQL repositories
│   │   ├── mongodb/          # MongoDB repositories
│   │   ├── redis/            # Cache layer
│   │   └── rabbitmq/         # Message queue
│   ├── services/
│   │   ├── command/          # Write operations (CQRS)
│   │   └── query/            # Read operations (CQRS)
│   └── bootstrap/            # DI & initialization
├── migrations/               # Database migrations
├── api/                      # OpenAPI/Swagger docs
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── .env.example
```

### Shared Packages (`pkg/`)

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `mmodel/` | Shared domain models (Account, Balance, Transaction, Organization, etc.) | 23 model files |
| `mbootstrap/` | Service composition interfaces (BalancePort, MetadataIndexRepository, Service) | `balance.go`, `interfaces.go` |
| `constant/` | Error codes (90+ errors), enums, constants | `errors.go`, `account.go`, `transaction.go` |
| `net/http/` | HTTP utilities, error handling, Fiber middleware | HTTP response helpers |
| `mgrpc/` | gRPC infrastructure, proto definitions, connection management | `balance/` proto package |
| `transaction/` | Transaction DSL parsing and validation | `validations.go`, `transaction.go` |
| `gold/` | Golden file test utilities | Test comparison helpers |
| `mongo/` | MongoDB connection utilities | `ExtractMongoPortAndParameters` |
| `shell/` | Shell execution utilities | Script helpers |
| `utils/` | Common utilities (encryption, pointers) | General helpers |

### Components

| Component | Port | Responsibility | Architecture |
|-----------|------|----------------|--------------|
| `onboarding` | 3000 | Entity management (organizations, ledgers, accounts, assets, portfolios, segments) | Hexagonal + CQRS |
| `transaction` | 3001 (HTTP), 3011 (gRPC) | Double-entry accounting, balances, operations, asset rates | Hexagonal + CQRS |
| `ledger` | 3002 | Unified mode composing onboarding + transaction | Composition layer |
| `crm` | 4003 | Customer/holder management, aliases | Hexagonal (flat services) |
| `infra` | - | Docker infrastructure (PostgreSQL, MongoDB, Redis, RabbitMQ) | Infrastructure-only |

### Component Capabilities Matrix

| Component | PostgreSQL | MongoDB | Redis | RabbitMQ | gRPC In | gRPC Out | Migrations |
|-----------|------------|---------|-------|----------|---------|----------|------------|
| onboarding | Yes | Yes (metadata) | Yes (cache) | No | Yes | Yes (to transaction) | 16 files |
| transaction | Yes | Yes (metadata) | Yes (cache/sync) | Yes (async balance) | Yes | No | 34 files |
| ledger | No (composition) | No | No | No | No | No | Inherits from modules |
| crm | No | Yes (holders, aliases) | No | No | No | No | None |

---

## 3. Coding Standards

### File Naming Conventions

| Type | Pattern | Example |
|------|---------|---------|
| Source files | `lowercase_or_kebab.go` | `create-account.go` |
| PostgreSQL adapter | `{entity}.postgresql.go` | `organization.postgresql.go` |
| MongoDB adapter | `{entity}.mongodb.go` | `metadata.mongodb.go` |
| Unit tests | `{filename}_test.go` | `config_test.go` |
| Integration tests | `{filename}_integration_test.go` | `organization.postgresql_integration_test.go` |
| Mocks | `{entity}.{technology}_mock.go` | `transaction.postgresql_mock.go` |

### Naming Conventions

```go
// Types: PascalCase
type CreateAccountInput struct {}
type EntityNotFoundError struct {}

// Functions: PascalCase for exported, camelCase for unexported
func CreateTransaction() {}      // Exported
func validateFromBalances() {}   // Unexported

// Variables: camelCase
ctx := context.Background()
logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

// Constants: PascalCase for exported errors
var ErrDuplicateLedger = errors.New("0001")
```

### Import Organization

Organize imports in groups separated by blank lines:

```go
import (
    // 1. Standard library
    "context"
    "encoding/json"
    "errors"

    // 2. Third-party generic packages
    "github.com/shopspring/decimal"

    // 3. Internal: lib-commons (with lib prefix)
    libCommons "github.com/LerianStudio/lib-commons/v2/commons"
    libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

    // 4. Internal: midaz project packages
    "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"

    // 5. External: frameworks
    "github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
)
```

### Context Propagation

**Every function accepts `context.Context` as first parameter:**

```go
func (uc *UseCase) CreateTransaction(ctx context.Context, input *mmodel.CreateTransactionInput) (*mmodel.Transaction, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "command.create_transaction")
    defer span.End()

    // Pass context to all downstream calls
    result, err := uc.TransactionRepo.Create(ctx, transaction)
}
```

### Observability Integration

**Always instrument with OpenTelemetry:**

```go
// Extract tracking from context
logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

// Create span for operation
ctx, span := tracer.Start(ctx, "layer.operation_name")
defer span.End()

// On error: instrument span AND log
if err != nil {
    libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation description", err)
    logger.Errorf("Operation failed: %v", err)
    return nil, err
}
```

**Span naming convention:** `<layer>.<operation>` (e.g., `command.create_transaction`, `postgres.find_organization`)

---

## 4. Error Handling

### Error Catalog

**Location:** `pkg/constant/errors.go`

Errors use 4-digit numeric codes:

```go
var (
    ErrDuplicateLedger         = errors.New("0001")
    ErrEntityNotFound          = errors.New("0007")
    ErrInsufficientFunds       = errors.New("0018")
    ErrTokenMissing            = errors.New("0041")
    ErrInternalServer          = errors.New("0046")
)
```

### Domain Error Types

**Location:** `pkg/errors.go`

```go
type EntityNotFoundError struct {
    EntityType string `json:"entityType,omitempty"`
    Title      string `json:"title,omitempty"`
    Message    string `json:"message,omitempty"`
    Code       string `json:"code,omitempty"`
    Err        error  `json:"err,omitempty"`
}

func (e EntityNotFoundError) Error() string { return e.Message }
func (e EntityNotFoundError) Unwrap() error { return e.Err }
```

**Available error types:**
- `EntityNotFoundError` → 404
- `ValidationError` → 400
- `EntityConflictError` → 409
- `UnauthorizedError` → 401
- `ForbiddenError` → 403
- `UnprocessableOperationError` → 422
- `InternalServerError` → 500

### Error Mapping

Use `ValidateBusinessError()` to map constants to typed errors:

```go
// In service layer
if err != nil {
    return nil, pkg.ValidateBusinessError(constant.ErrDuplicateLedger, "Organization", organizationName)
}
```

### Error Propagation Rules

1. **Use `errors.As()` for type checking:**
   ```go
   var notFound pkg.EntityNotFoundError
   if errors.As(err, &notFound) {
       // Handle not found case
   }
   ```

2. **Log at EVERY error point:**
   ```go
   logger.Errorf("Failed to create organization: %v", err)
   ```

3. **Instrument spans BEFORE returning:**
   ```go
   libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create organization", err)
   return nil, err
   ```

4. **Never swallow errors silently**

---

## 5. Validation Patterns

### Struct Tag Validation

Use `go-playground/validator` tags:

```go
type CreateAccountInput struct {
    Name      string          `json:"name" validate:"max=256"`
    AssetCode string          `json:"assetCode" validate:"required,max=100"`
    Alias     *string         `json:"alias" validate:"omitempty,max=100,invalidaliascharacters"`
    Type      string          `json:"type" validate:"required,max=256,invalidstrings=external"`
    Metadata  map[string]any  `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}
```

**Common tags:**
- `required` - Field must be present
- `max=N` - Maximum length
- `uuid` - Valid UUID format
- `omitempty` - Allow empty with other rules
- `dive` - Validate nested elements
- `keymax=N` / `valuemax=N` - Map key/value limits
- `nonested` - Forbid nested maps (metadata)

**Custom validators:**
- `prohibitedexternalaccountprefix` - Alias prefix restrictions
- `invalidaliascharacters` - Character set validation
- `invalidstrings=external` - Disallowed string values
- `cpf` / `cnpj` - Brazilian document validation

### HTTP Layer Validation

```go
// WithBody decorator handles:
// 1. JSON unmarshaling
// 2. Struct validation
// 3. Unknown field detection (rejects extra fields)
// 4. Null byte protection

f.Post("/path", http.WithBody(new(CreateAccountInput), handler.Create))
```

### Validation Error Response

```json
{
    "code": "0003",
    "title": "Validation Error",
    "message": "Request validation failed",
    "entityType": "Account",
    "fields": {
        "name": "name cannot exceed 256 characters",
        "assetCode": "assetCode is required"
    }
}
```

---

## 6. API Design

### URL Structure

```
/v1/organizations/{organization_id}/ledgers/{ledger_id}/{resources}/{action}
```

**Conventions:**
- API version prefix: `/v1/`
- Plural resource names: `transactions`, `balances`, `operations`
- Path parameters: snake_case with `_id` suffix
- Actions via POST: `/transactions/{id}/commit`, `/transactions/{id}/revert`

### HTTP Methods and Status Codes

| Method | Usage | Success Status |
|--------|-------|----------------|
| POST | Create resource, state transitions | 201 Created |
| GET | Read resource(s) | 200 OK |
| PATCH | Partial update | 200 OK |
| DELETE | Remove resource | 204 No Content |

**Error Status Codes:**
- 400 Bad Request - Validation errors
- 401 Unauthorized - Missing/invalid auth
- 403 Forbidden - Insufficient permissions
- 404 Not Found - Resource not found
- 409 Conflict - State conflict
- 422 Unprocessable Entity - Business rule violation
- 500 Internal Server Error

### Handler Structure

```go
type TransactionHandler struct {
    Command *command.UseCase  // For write operations
    Query   *query.UseCase    // For read operations
}

// With body parsing
func (h *TransactionHandler) CreateTransaction(p any, c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "handler.create_transaction")
    defer span.End()

    input := p.(*transaction.CreateTransactionInput)
    result, err := h.Command.CreateTransaction(ctx, input)
    if err != nil {
        return http.WithError(c, err)
    }
    return http.Created(c, result)
}
```

### Swagger Documentation

```go
// @Summary      Create a Transaction
// @Description  Create a Transaction with the input payload
// @Tags         Transactions
// @Accept       json
// @Produce      json
// @Param        Authorization    header  string  true   "Authorization Bearer Token"
// @Param        organization_id  path    string  true   "Organization ID"
// @Param        transaction      body    CreateTransactionInput  true  "Transaction Input"
// @Success      201  {object}  Transaction
// @Failure      400  {object}  mmodel.Error
// @Failure      422  {object}  mmodel.Error
// @Router       /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json [post]
func (h *TransactionHandler) CreateTransactionJSON(p any, c *fiber.Ctx) error
```

### Pagination

**Query parameters:**
- `limit` - Results per page (default: 10, max: configurable)
- `cursor` - Base64-encoded pagination token
- `sort_order` - `asc` or `desc`
- `start_date` / `end_date` - Date range filtering
- `metadata.key=value` - Metadata filtering

**Response format:**
```json
{
    "items": [...],
    "next_cursor": "base64token",
    "prev_cursor": "base64token",
    "limit": 10
}
```

---

## 7. Database Patterns

### Repository Interface Pattern

**Interface and implementation co-located in same package:**

```go
// organization.go - Interface
type Repository interface {
    Create(ctx context.Context, org *mmodel.Organization) (*mmodel.Organization, error)
    Update(ctx context.Context, id uuid.UUID, org *mmodel.Organization) (*mmodel.Organization, error)
    Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)
    FindAll(ctx context.Context, filter Filter) ([]*mmodel.Organization, error)
    Delete(ctx context.Context, id uuid.UUID) error
}

// organization.postgresql.go - Implementation
type OrganizationPostgreSQLRepository struct {
    connection *libPostgres.PostgresConnection
    tableName  string
}
```

### Query Building with Squirrel

```go
findQuery := squirrel.Select(columnList...).
    From("organization").
    Where(squirrel.Eq{"id": id}).
    Where(squirrel.Eq{"deleted_at": nil}).
    PlaceholderFormat(squirrel.Dollar)

query, args, err := findQuery.ToSql()
row := db.QueryRowContext(ctx, query, args...)
```

### Soft Deletes

All queries must filter `WHERE deleted_at IS NULL`:

```go
// Delete operation
UPDATE organization SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL
```

### UUID Generation

Use UUID v7 for new records:

```go
ID: libCommons.GenerateUUIDv7().String()
```

### Migration Naming

```
{sequence}_{description}.{up|down}.sql

Examples:
000000_create_organization_table.up.sql
000000_create_organization_table.down.sql
000001_create_ledger_table.up.sql
```

### PostgreSQL Error Handling

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) {
    return services.ValidatePGError(pgErr, reflect.TypeOf(mmodel.Organization{}).Name())
}
```

---

## 8. Configuration & Environment

### Configuration Loading

```go
cfg := &Config{}
if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
    return nil, fmt.Errorf("failed to load config: %w", err)
}
```

### Struct Tags for Environment Variables

```go
type Config struct {
    ServerAddress string `env:"SERVER_ADDRESS" envDefault:":3002"`
    RedisDB       int    `env:"REDIS_DB" default:"0"`
    RedisTLS      bool   `env:"REDIS_TLS" default:"false"`
}
```

### Prefixed Variables for Unified Mode

Support both unified and standalone deployments:

```go
// Prefixed (unified mode) - takes precedence
PrefixedPrimaryDBHost string `env:"DB_ONBOARDING_HOST"`

// Non-prefixed (standalone mode) - fallback
PrimaryDBHost string `env:"DB_HOST"`

// Resolution
dbHost := envFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
```

### Safe Defaults Requirements

**CRITICAL: Environment variables must NEVER break the application if missing.**

1. **Always provide defaults:**
   ```go
   ServerAddress string `env:"SERVER_ADDRESS" envDefault:":3002"`
   ```

2. **Runtime validation with logging:**
   ```go
   if cfg.ProtoAddress == "" {
       cfg.ProtoAddress = ":3011"
       logger.Warn("PROTO_ADDRESS not set, using default: :3011")
   }
   ```

3. **Never crash on missing optional config**

### Duration Conversion

Store as integers, convert to `time.Duration`:

```go
RedisReadTimeout int `env:"REDIS_READ_TIMEOUT" default:"3"` // seconds

// Usage
ReadTimeout: time.Duration(cfg.RedisReadTimeout) * time.Second
```

---

## 9. Testing Standards

### Test File Organization

| Suffix | Purpose | Build Tag |
|--------|---------|-----------|
| `_test.go` | Unit tests | None |
| `_integration_test.go` | Integration tests | `//go:build integration` |
| `_benchmark_test.go` | Benchmarks | None |
| `_mock.go` | Generated mocks | None |

### Unit Test Structure

```go
func TestValidateBalancesRules(t *testing.T) {
    t.Parallel()  // Enable parallel execution

    // Table-driven tests
    tests := []struct {
        name        string
        input       Input
        expectError bool
    }{
        {
            name:        "valid input",
            input:       validInput,
            expectError: false,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()

            err := ValidateFunction(tc.input)

            if tc.expectError {
                require.Error(t, err)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Mock Generation

Use `mockgen` directive:

```go
//go:generate mockgen --destination={entity}.postgresql_mock.go --package={package} . Repository
```

### Stubs vs Mocks

| Type | Use Case | Package |
|------|----------|---------|
| **Stubs** | Integration tests - fixed behavior | `tests/utils/stubs/` |
| **Mocks** | Unit tests - verifiable interactions | Generated with mockgen |
| **Test utilities** | Postgres containers, test helpers | `tests/utils/postgres/`, `tests/helpers/` |
| **Chaos tests** | Resilience testing | `tests/chaos/` |

### Test Assertions

```go
// Use require for flow control (fails immediately)
require.NoError(t, err)
require.Equal(t, expected, actual)

// Use assert for result verification (continues)
assert.NotNil(t, result)
assert.Equal(t, expected, actual)
```

### Integration Tests with Testcontainers

```go
//go:build integration

func TestIntegration_CountLedgers_Monotonic(t *testing.T) {
    // Setup container with automatic cleanup
    container := pgtestutil.SetupContainer(t)
    
    // Setup repository with migrations
    logger := libZap.InitializeLogger()
    migrationsPath := pgtestutil.FindMigrationsPath(t, "onboarding")
    connStr := pgtestutil.BuildConnectionString(container.Host, container.Port, container.Config)
    
    conn := &libPostgres.PostgresConnection{
        ConnectionStringPrimary: connStr,
        ConnectionStringReplica: connStr,
        MigrationsPath:          migrationsPath,
        Logger:                  logger,
    }
    
    ledgerRepo := ledger.NewLedgerPostgreSQLRepository(conn)
    
    // Mock external dependencies
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()
    mockMetadata := mongodb.NewMockRepository(ctrl)
    mockMetadata.EXPECT().FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
        Return(nil, nil).AnyTimes()
    
    uc := &UseCase{LedgerRepo: ledgerRepo, MetadataRepo: mockMetadata}
    
    // Test invariants
    ctx := context.Background()
    orgID := pgtestutil.CreateTestOrganization(t, container.DB)
    
    lastCount, err := uc.CountLedgers(ctx, orgID)
    require.NoError(t, err)
    
    // Property-based: count never decreases
    for i := 0; i < 5; i++ {
        pgtestutil.CreateTestLedgerWithParams(t, container.DB, orgID, params)
        newCount, _ := uc.CountLedgers(ctx, orgID)
        assert.GreaterOrEqual(t, newCount, lastCount)
        lastCount = newCount
    }
}
```

**Test utilities available:**
- `pgtestutil.SetupContainer(t)` - Postgres testcontainer with cleanup
- `pgtestutil.FindMigrationsPath(t, component)` - Locate migrations
- `pgtestutil.CreateTestOrganization(t, db)` - Create test fixtures
- `pgtestutil.BuildConnectionString(...)` - Build DSN from container

---

## 10. Build & CI/CD

### Makefile Targets

**Root Makefile:**
```bash
make help           # Show all targets
make build          # Build all components
make test           # Run all tests
make lint           # Run linters
make format         # Format code
make tidy           # go mod tidy
make sec            # Security scanning
make up             # Start Docker services
make down           # Stop Docker services
make dev-setup      # Complete dev environment
```

**Component-level:**
```bash
make ledger COMMAND=build
make ledger COMMAND=test
make ledger COMMAND=lint
```

### Docker Multi-Stage Build

```dockerfile
# Stage 1: Builder (multi-platform support)
FROM --platform=$BUILDPLATFORM golang:1.25.6-alpine AS builder
WORKDIR /ledger-app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -tags netgo \
    -ldflags '-s -w -extldflags "-static"' \
    -o /app components/ledger/cmd/app/main.go

# Stage 2: Runtime (distroless for security)
FROM gcr.io/distroless/static-debian12
COPY --from=builder /app /app
COPY --from=builder /ledger-app/components/onboarding/migrations /components/onboarding/migrations
COPY --from=builder /ledger-app/components/transaction/migrations /components/transaction/migrations
EXPOSE 3002
ENTRYPOINT ["/app"]
```

**Build flags explained:**
- `-tags netgo`: Use Go's native DNS resolver
- `-ldflags '-s -w'`: Strip debug info, reduce binary size
- `-extldflags "-static"`: Static linking for distroless compatibility

### CI/CD Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `build.yml` | Tag push | Build & publish Docker images |
| `go-combined-analysis.yml` | PR | CodeQL, lint, gosec, tests |
| `pr-security-scan.yml` | PR | Security scanning |
| `go-integration-e2e.yml` | PR | Integration and E2E tests |
| `midaz-e2e-tests.yml` | PR | Full E2E test suite |
| `pr-validation.yml` | PR | PR format validation |
| `release.yml` | Release | Release automation |
| `release-notification.yml` | Release | Discord/Slack notifications |
| `env-vars-pr-notification.yml` | PR | Environment variable change alerts |

### Code Quality Gates

Required checks before merge:
1. CodeQL security analysis
2. golangci-lint (must pass)
3. gosec security scanning
4. Unit tests (must pass)
5. Migration linting (if migrations changed)

### Linter Configuration

**Key settings in `.golangci.yml`:**
- `gocyclo`: max complexity 18
- `depguard`: blocks `io/ioutil` (deprecated)
- `wsl_v5`: whitespace linting
- Test files excluded from certain checks

---

## 11. Git Conventions

### Branch Naming

```
feature/task-id-or-name
fix/bug-description
hotfix/critical-issue
docs/documentation-update
refactor/code-improvement
```

**Protected branches:** `main`, `develop`, `release/*`

### Commit Message Format

```
<type>(<scope>): <description>

Types: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
```

**Examples:**
```
feat(auth): add login functionality
fix(ledger): correct balance calculation
docs: update README
test(crm): add user validation tests
```

### Pre-commit Hooks

Located in `.githooks/`:
- `pre-commit`: Branch protection, commit message validation
- `pre-push`: Branch naming enforcement
- `commit-msg`: Message format validation

---

## Quick Reference

### Creating a New Feature

1. **Create branch:** `git checkout -b feature/my-feature`
2. **Add to component:** Follow hexagonal structure
3. **Define interface:** In adapter package
4. **Implement:** PostgreSQL/MongoDB adapter
5. **Add service:** Command (write) or Query (read)
6. **Add HTTP handler:** With Swagger docs
7. **Add tests:** Unit + integration
8. **Run quality checks:** `make lint test sec`
9. **Create PR:** Against `develop`

### Adding a New Endpoint

1. Define input/output in `pkg/mmodel/`
2. Add validation tags
3. Create handler in `adapters/http/in/`
4. Register route with auth middleware
5. Add Swagger annotations
6. Add unit tests
7. Update OpenAPI spec

### Adding a New Error

1. Add constant in `pkg/constant/errors.go`
2. Add mapping in `pkg/errors.go` `ValidateBusinessError()`
3. Document in API error reference

---

---

## 12. Background Workers

### Balance Sync Worker (Transaction Component)

The transaction component includes a background worker for synchronous balance processing:

```go
type Config struct {
    BalanceSyncWorkerEnabled bool `env:"BALANCE_SYNC_WORKER_ENABLED" default:"true"`
    BalanceSyncMaxWorkers    int  `env:"BALANCE_SYNC_MAX_WORKERS"`  // default: 5
}
```

**Worker responsibilities:**
- Process balance updates from Redis queue
- Configurable worker pool size
- Can be disabled for testing or specific deployments

### RabbitMQ Consumer (Transaction Component)

Multi-queue consumer for async balance creation:

```go
routes := rabbitmq.NewConsumerRoutes(connection, 
    cfg.RabbitMQNumbersOfWorkers, 
    cfg.RabbitMQNumbersOfPrefetch, 
    logger, telemetry)
multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)
```

---

## 13. Inter-Module Ports

### BalancePort Interface

Defines transport-agnostic balance operations:

```go
// pkg/mbootstrap/balance.go
type BalancePort interface {
    CreateBalanceSync(ctx context.Context, input mmodel.CreateBalanceInput) (*mmodel.Balance, error)
    DeleteAllBalancesByAccountID(ctx context.Context, orgID, ledgerID, accountID uuid.UUID, requestID string) error
    CheckHealth(ctx context.Context) error
}
```

**Implementations:**
- `transaction.UseCase` - Direct in-process (unified mode)
- `grpcout.BalanceGRPCRepository` - Network calls via gRPC (microservices mode)

### MetadataIndexRepository Interface

For cross-module metadata index operations:

```go
type MetadataIndexRepository interface {
    // Metadata index operations
}
```

Both onboarding and transaction modules expose their metadata repositories for the ledger component.

---

## References

- **API Documentation:** https://docs.midaz.io/
- **Error Catalog:** https://docs.midaz.io/midaz/api-reference/resources/errors-list
- **Project Structure:** `STRUCTURE.md`
- **Linter Config:** `.golangci.yml`
- **Go Version:** 1.24.2+ (toolchain 1.25.6)
