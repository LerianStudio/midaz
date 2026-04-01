# CLAUDE.md — Midaz Deep Technical Reference

This file is a deep technical reference for AI coding agents working on the Midaz codebase.
For a quick-start overview, see [AGENTS.md](AGENTS.md).
For full API and env var reference, see [llms-full.txt](llms-full.txt).
For project rules and coding standards, see [docs/PROJECT_RULES.md](docs/PROJECT_RULES.md).

---

## What Is Midaz

Midaz is an enterprise-grade open-source **double-entry ledger system**. It is the core ledger component of a banking platform. The hierarchy is: Organization → Ledger → (Assets, Portfolios, Segments) → Accounts → Transactions → Operations → Balances.

- **Module path**: `github.com/LerianStudio/midaz/v3`
- **Go version**: 1.25+
- **License**: Elastic License 2.0 (ELv2) — not MIT/Apache

## Repository Layout

```
midaz/
├── components/
│   ├── ledger/           # Main component — unified onboarding + transaction API
│   │   ├── cmd/app/      # main.go
│   │   ├── internal/
│   │   │   ├── adapters/ # http/in, postgres, mongodb, redis, rabbitmq
│   │   │   ├── bootstrap/# Config struct, InitServers, DI, workers
│   │   │   └── services/ # command/ (writes), query/ (reads)
│   │   ├── migrations/   # onboarding/ and transaction/ SQL migrations
│   │   └── api/          # Swagger docs
│   ├── crm/              # CRM plugin (MongoDB-backed)
│   └── infra/            # Docker Compose infrastructure
├── pkg/                  # Shared packages — imported by all components
│   ├── mmodel/           # Domain models (Organization, Ledger, Account, Transaction, Balance, etc.)
│   ├── constant/         # Error codes (0001–0161), action constants, HTTP constants
│   ├── errors.go         # Typed error structs + factory functions
│   ├── gold/             # ANTLR4 transaction DSL (grammar + parser + builder)
│   ├── net/http/         # Fiber middleware, pagination, protected route chain
│   ├── transaction/      # Transaction processing utilities
│   └── ...               # mongo, repository, pagination, utils
├── tests/                # Integration + chaos test suites
├── docs/PROJECT_RULES.md # 1130-line coding standards (DO NOT overwrite)
├── Makefile              # Root orchestrator
└── go.mod                # Single go.mod for entire monorepo
```

## Architecture Patterns

### Hexagonal Architecture with CQRS

```
HTTP Handlers → Command/Query Use Cases → Repository Interfaces → Adapters (Postgres/Mongo/Redis/RabbitMQ)
```

- **Handlers** (`components/ledger/internal/adapters/http/in/`): Parse HTTP, validate input, call use cases
- **Services** (`components/ledger/internal/services/command/` and `query/`): Business logic, one file per operation
- **Repositories**: Interfaces defined where used (in services), implemented in adapters
- **Models** (`pkg/mmodel/`): Shared domain types, no business logic

### Key Principles
- Dependencies flow inward: handlers → services → repos → DB drivers
- No `/internal/domain` folder — entities live in `pkg/mmodel/`
- Interfaces defined in the package that USES them, not in `/port` folders
- One handler per entity, one service operation per file

## Bootstrap & Configuration

The Config struct is in `components/ledger/internal/bootstrap/config.go`. It loads all env vars via `libCommons.SetConfigFromEnvVars(cfg)`.

**InitServersWithOptions** (config.go:234) is the composition root:
1. Loads Config from env vars
2. Validates multi-tenant + auth coupling
3. Creates logger, telemetry
4. Initializes tenant client (if multi-tenant)
5. Initializes PG (onboarding + transaction), MongoDB (onboarding + transaction), Redis, RabbitMQ
6. Creates Command + Query use cases
7. Creates HTTP handlers
8. Registers routes (onboarding, transaction, metadata)
9. Creates workers (RedisQueueConsumer, BalanceSyncWorker)
10. Returns Service struct

## HTTP Framework

Uses **Fiber v2** (`github.com/gofiber/fiber/v2`).

### Route Registration
- `RegisterOnboardingRoutesToApp()` — organizations, ledgers, assets, portfolios, segments, accounts, account types
- `RegisterTransactionRoutesToApp()` — transactions, operations, asset-rates, balances, operation-routes, transaction-routes
- `RegisterMetadataRoutesToApp()` — metadata indexes

### Route Protection
All routes use `http.ProtectedRouteChain()` which applies:
1. Auth middleware (RBAC via lib-auth)
2. Optional post-auth middlewares (multi-tenant DB resolution)
3. Body parsing, UUID path parameter validation, handler

### Middleware
- Telemetry middleware (`libHTTP.NewTelemetryMiddleware`)
- CORS (`cors.New()`)
- HTTP logging (`libHTTP.WithHTTPLogging`)
- Body limit (`http.WithBodyLimit(SettingsMaxPayloadSize)`)

## Error Handling

### Error Types (pkg/errors.go)
| Type | HTTP | Use |
|------|------|-----|
| `EntityNotFoundError` | 404 | Entity not found |
| `ValidationError` | 400 | Input validation |
| `EntityConflictError` | 409 | Duplicates |
| `UnauthorizedError` | 401 | Auth missing |
| `ForbiddenError` | 403 | Insufficient privileges |
| `UnprocessableOperationError` | 422 | Business rule violation |
| `InternalServerError` | 500 | Unexpected failures |
| `ServiceUnavailableError` | 503 | Infrastructure down |

### Error Constants (pkg/constant/errors.go)
Numeric codes as `errors.New("0007")`. Factory: `pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")`.

### Pattern
```go
// Business error — return directly (expected)
if errors.Is(err, constant.ErrEntityNotFound) {
    return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Account")
}
// Technical error — wrap with context
return nil, fmt.Errorf("failed to query accounts: %w", err)
```

## Domain Models (pkg/mmodel/)

Key entities and their files:
- `organization.go` — Organization, Address, CreateOrganizationInput, UpdateOrganizationInput
- `ledger.go` — Ledger, LedgerSettings, CreateLedgerInput, UpdateLedgerInput
- `account.go` — Account, CreateAccountInput, UpdateAccountInput
- `asset.go` — Asset, CreateAssetInput, UpdateAssetInput
- `balance.go` — Balance, UpdateBalance, CreateAdditionalBalance
- `operation.go` — Operation
- `portfolio.go` — Portfolio
- `segment.go` — Segment
- `status.go` — Status struct (code-based status pattern)
- `transaction-route.go` — TransactionRoute, CreateTransactionRouteInput
- `operation-route.go` — OperationRoute, CreateOperationRouteInput
- `account-type.go` — AccountType, CreateAccountTypeInput
- `holder.go` — Holder (CRM)
- `queue.go` — Queue message models
- `settings.go` — LedgerSettings
- `alias.go` — Alias models
- `date.go` — Date range utilities
- `metadata.go` — Metadata index models

### Status Pattern
```go
type Status struct {
    Code        *string `json:"code"`
    Description *string `json:"description,omitempty"`
}
```
Common codes: `ACTIVE`, `INACTIVE`, `DELETED`, `PENDING`, `CANCELLED`

### Metadata
- Flat key-value pairs (no nesting allowed)
- Key max: 100 chars, Value max: 2000 chars
- Stored in MongoDB, queried via metadata indexes

## Transaction Processing

### Creation Modes
1. **JSON** (`POST .../transactions/json`): Full source/destination specification
2. **DSL** (`POST .../transactions/dsl`): Gold DSL text format
3. **Inflow** (`POST .../transactions/inflow`): Simplified credit-only
4. **Outflow** (`POST .../transactions/outflow`): Simplified debit-only
5. **Annotation** (`POST .../transactions/annotation`): Informational entries

### Transaction Lifecycle
- `ACTIVE`: Completed immediately
- `PENDING`: Created with pending status → `commit` or `cancel`
- `revert`: Creates a reverse transaction (parentTransactionID linkage)

### Async Processing
When `RABBITMQ_TRANSACTION_ASYNC=true`:
1. Transaction created, published to RabbitMQ
2. Workers consume messages, update balances
3. Bulk recorder batches inserts for 10x throughput
4. Circuit breaker protects against RabbitMQ outages

### Balance Model
- `Available`: Spendable balance
- `OnHold`: Pending transaction holds
- `Scale`: Decimal precision
- `Version`: Optimistic concurrency (lock version)
- History queries: balance state at any timestamp

## Multi-Tenancy

Enabled via `MULTI_TENANT_ENABLED=true`. Requires auth enabled.

**Architecture**:
- Tenant ID extracted from JWT by auth middleware
- `TenantMiddleware.WithTenantDB` resolves per-tenant PG/Mongo connections
- `tmpostgres.Manager` / `tmmongo.Manager` manage connection pools per tenant
- `TenantCache` + `TenantLoader` provide process-local caching
- `TenantEventListener` subscribes to Redis Pub/Sub for tenant lifecycle events
- On tenant add: start RabbitMQ consumer, warm cache
- On tenant remove: stop consumer, close all connections, invalidate cache

**Modules**: Each module (`onboarding`, `transaction`) has independent PG and Mongo managers.

## lib-commons v4 Integration

Import prefix: `github.com/LerianStudio/lib-commons/v4/commons/...`

Key packages used:
- `libCommons` — `SetConfigFromEnvVars`, `NewTrackingFromContext`
- `libLog` — Structured logging interface
- `libZap` — Zap logger implementation
- `libOpentelemetry` — Telemetry, span management
- `libRedis` — Redis client (standalone/sentinel/cluster)
- `libHTTP` — HTTP response helpers, middleware, Fiber error handler
- `libCircuitBreaker` — Circuit breaker pattern
- `tmclient` — Tenant manager HTTP client
- `tmcore` — Tenant manager core types/errors
- `tmevent` — Tenant event dispatcher/listener
- `tmmiddleware` — Fiber middleware for tenant DB injection
- `tmpostgres` / `tmmongo` / `tmredis` — Per-tenant connection managers

## Database Schemas

### PostgreSQL
- **onboarding** database: organizations, ledgers, assets, portfolios, segments, accounts, account_types
- **transaction** database: transactions, operations, balances, asset_rates, operation_routes, transaction_routes

Migrations in `components/ledger/migrations/onboarding/` and `components/ledger/migrations/transaction/`.

### MongoDB
- **onboarding** database: metadata collections
- **transaction** database: metadata collections
- **crm** database: holders, aliases, related parties

## Build Commands

```bash
make set-env          # Setup .env files from .env.example
make build            # Build ledger + CRM
make up               # Start infra → ledger → CRM
make down             # Stop all
make lint             # Lint all components (golangci-lint v2)
make test             # Run tests
make test-unit        # Unit tests only
make test-integration # Integration tests (testcontainers)
make coverage-unit    # Unit test coverage
make generate-docs    # Swagger docs
make migrate-lint     # Lint SQL migrations
make sec              # Security scans (gosec + govulncheck)
```

## Coding Conventions

Full rules in `docs/PROJECT_RULES.md` (1130 lines). Key points:

1. **Go 1.25+**: Use `any` not `interface{}`, use generics for utilities
2. **File naming**: `snake_case.go`, one handler/service per file
3. **Import groups**: stdlib → external → internal (blank-line separated)
4. **Context**: Always first param, check `ctx.Err()` before work
5. **Error wrapping**: `%w` for technical, direct return for business errors
6. **Validation**: Normalize → Apply defaults → Validate → Execute
7. **Struct tags**: `json`, `validate`, `example`, `format`, `maxLength`
8. **Metadata**: Flat only (no nesting), key max 100, value max 2000
9. **IDs**: Use `uuid.UUID` type, not strings
10. **Soft delete**: `DeletedAt` field, status `DELETED`
11. **Pagination**: Page-based (page + limit), max 100 per page

## What NOT To Do

- Do NOT overwrite `docs/PROJECT_RULES.md` — it is maintained separately
- Do NOT use `interface{}` — use `any`
- Do NOT use offset-based pagination for new endpoints
- Do NOT create domain logic in handler or repository layers
- Do NOT panic — return errors from constructors
- Do NOT use `time.Now()` in tests — use fixed time utilities
- Do NOT store nested metadata values
- Do NOT expose internal error details to API clients
- Do NOT import outer layers from inner layers
- Do NOT use string literals for HTTP methods — use `http.Method*` constants
