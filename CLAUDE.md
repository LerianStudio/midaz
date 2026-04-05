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

## Environment Variables — Complete Reference

Source: `components/ledger/internal/bootstrap/config.go` (Ledger) and `components/crm/internal/bootstrap/config.go` (CRM).
Defaults shown in parentheses where set by `applyConfigDefaults()` or env tags.

### Application

| Variable | Default | Description |
|----------|---------|-------------|
| `APPLICATION_NAME` | — | Application identity (used for tenant-manager service name) |
| `ENV_NAME` | development | Environment: development, staging, uat, production, local |
| `VERSION` | — | Application version string |
| `LOG_LEVEL` | debug | Log level |
| `SERVER_ADDRESS` | :3002 | Listen address (Ledger) |

### Auth / Casdoor

| Variable | Default | Description |
|----------|---------|-------------|
| `PLUGIN_AUTH_ENABLED` | false | Enable auth middleware |
| `PLUGIN_AUTH_HOST` | — | Auth service host (Ledger) |
| `CASDOOR_JWK_ADDRESS` | — | JWK endpoint for Casdoor JWT validation |

### PostgreSQL — Onboarding (Primary + Replica)

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_ONBOARDING_HOST` | midaz-postgres-primary | Primary host |
| `DB_ONBOARDING_USER` | midaz | Username |
| `DB_ONBOARDING_PASSWORD` | lerian | Password |
| `DB_ONBOARDING_NAME` | onboarding | Database name |
| `DB_ONBOARDING_PORT` | 5701 | Port |
| `DB_ONBOARDING_SSLMODE` | disable | SSL mode |
| `DB_ONBOARDING_REPLICA_HOST` | midaz-postgres-replica | Replica host |
| `DB_ONBOARDING_REPLICA_USER` | midaz | Replica username |
| `DB_ONBOARDING_REPLICA_PASSWORD` | lerian | Replica password |
| `DB_ONBOARDING_REPLICA_NAME` | onboarding | Replica database name |
| `DB_ONBOARDING_REPLICA_PORT` | 5702 | Replica port |
| `DB_ONBOARDING_REPLICA_SSLMODE` | disable | Replica SSL mode |
| `DB_ONBOARDING_MAX_OPEN_CONNS` | 3000 | Max open connections |
| `DB_ONBOARDING_MAX_IDLE_CONNS` | 3000 | Max idle connections |

### PostgreSQL — Transaction (Primary + Replica)

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_TRANSACTION_HOST` | midaz-postgres-primary | Primary host |
| `DB_TRANSACTION_USER` | midaz | Username |
| `DB_TRANSACTION_PASSWORD` | lerian | Password |
| `DB_TRANSACTION_NAME` | transaction | Database name |
| `DB_TRANSACTION_PORT` | 5701 | Port |
| `DB_TRANSACTION_SSLMODE` | disable | SSL mode |
| `DB_TRANSACTION_REPLICA_HOST` | midaz-postgres-replica | Replica host |
| `DB_TRANSACTION_REPLICA_USER` | midaz | Replica username |
| `DB_TRANSACTION_REPLICA_PASSWORD` | lerian | Replica password |
| `DB_TRANSACTION_REPLICA_NAME` | transaction | Replica database name |
| `DB_TRANSACTION_REPLICA_PORT` | 5702 | Replica port |
| `DB_TRANSACTION_REPLICA_SSLMODE` | disable | Replica SSL mode |
| `DB_TRANSACTION_MAX_OPEN_CONNS` | 3000 | Max open connections |
| `DB_TRANSACTION_MAX_IDLE_CONNS` | 3000 | Max idle connections |

### MongoDB — Onboarding

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGO_ONBOARDING_URI` | mongodb | Connection URI scheme |
| `MONGO_ONBOARDING_HOST` | midaz-mongodb | Host |
| `MONGO_ONBOARDING_NAME` | onboarding | Database name |
| `MONGO_ONBOARDING_USER` | midaz | Username |
| `MONGO_ONBOARDING_PASSWORD` | lerian | Password |
| `MONGO_ONBOARDING_PORT` | 5703 | Port |
| `MONGO_ONBOARDING_PARAMETERS` | — | Extra connection params (appended to URI) |
| `MONGO_ONBOARDING_MAX_POOL_SIZE` | 1000 | Max pool size |

### MongoDB — Transaction

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGO_TRANSACTION_URI` | mongodb | Connection URI scheme |
| `MONGO_TRANSACTION_HOST` | midaz-mongodb | Host |
| `MONGO_TRANSACTION_NAME` | transaction | Database name |
| `MONGO_TRANSACTION_USER` | midaz | Username |
| `MONGO_TRANSACTION_PASSWORD` | lerian | Password |
| `MONGO_TRANSACTION_PORT` | 5703 | Port |
| `MONGO_TRANSACTION_PARAMETERS` | — | Extra connection params |
| `MONGO_TRANSACTION_MAX_POOL_SIZE` | 1000 | Max pool size |

### Redis / Valkey

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_HOST` | midaz-valkey:5704 | Host(s); comma-separated for cluster/sentinel |
| `REDIS_MASTER_NAME` | — | Sentinel master name (enables sentinel mode) |
| `REDIS_PASSWORD` | lerian | Password |
| `REDIS_DB` | 0 | Database index |
| `REDIS_PROTOCOL` | (3) | RESP protocol version |
| `REDIS_TLS` | false | Enable TLS |
| `REDIS_CA_CERT` | — | CA certificate (base64) for TLS |
| `REDIS_USE_GCP_IAM` | false | Use GCP IAM auth instead of password |
| `REDIS_SERVICE_ACCOUNT` | — | GCP service account for IAM auth |
| `GOOGLE_APPLICATION_CREDENTIALS` | — | GCP credentials (base64) for IAM auth |
| `REDIS_TOKEN_LIFETIME` | (60) | GCP IAM token lifetime (minutes) |
| `REDIS_TOKEN_REFRESH_DURATION` | (45) | GCP IAM token refresh interval (minutes) |
| `REDIS_POOL_SIZE` | (10) | Connection pool size |
| `REDIS_MIN_IDLE_CONNS` | 0 | Minimum idle connections |
| `REDIS_READ_TIMEOUT` | (3) | Read timeout (seconds) |
| `REDIS_WRITE_TIMEOUT` | (3) | Write timeout (seconds) |
| `REDIS_DIAL_TIMEOUT` | (5) | Dial timeout (seconds) |
| `REDIS_POOL_TIMEOUT` | (2) | Pool wait timeout (seconds) |
| `REDIS_MAX_RETRIES` | (3) | Max retries per command |
| `REDIS_MIN_RETRY_BACKOFF` | (8) | Min retry backoff (milliseconds) |
| `REDIS_MAX_RETRY_BACKOFF` | (1) | Max retry backoff (seconds) |

### RabbitMQ

| Variable | Default | Description |
|----------|---------|-------------|
| `RABBITMQ_URI` | amqp | Protocol scheme (amqp/amqps) |
| `RABBITMQ_HOST` | midaz-rabbitmq | Host |
| `RABBITMQ_PORT_HOST` | 3003 | Management port |
| `RABBITMQ_PORT_AMQP` | 3004 | AMQP port |
| `RABBITMQ_DEFAULT_USER` | transaction | Producer username |
| `RABBITMQ_DEFAULT_PASS` | lerian | Producer password |
| `RABBITMQ_CONSUMER_USER` | consumer | Consumer username |
| `RABBITMQ_CONSUMER_PASS` | lerian | Consumer password |
| `RABBITMQ_VHOST` | — | Virtual host (empty = default "/") |
| `RABBITMQ_NUMBERS_OF_WORKERS` | 5 | Consumer worker count |
| `RABBITMQ_NUMBERS_OF_PREFETCH` | 10 | Prefetch count per worker |
| `RABBITMQ_HEALTH_CHECK_URL` | — | Health check URL |
| `RABBITMQ_TLS` | false | Enable TLS |
| `RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE` | — | Balance operation queue name |
| `RABBITMQ_TRANSACTION_ASYNC` | false | Enable async transaction processing |
| `RABBITMQ_OPERATION_TIMEOUT` | — | Operation timeout (e.g., "30s") |
| `RABBITMQ_TRANSACTION_EVENTS_ENABLED` | false | Enable transaction event exchange |
| `RABBITMQ_TRANSACTION_EVENTS_EXCHANGE` | — | Events exchange name |
| `AUDIT_LOG_ENABLED` | false | Enable audit log publishing |
| `RABBITMQ_AUDIT_EXCHANGE` | — | Audit exchange name |
| `RABBITMQ_AUDIT_KEY` | — | Audit routing key |

### RabbitMQ Circuit Breaker

| Variable | Default | Description |
|----------|---------|-------------|
| `RABBITMQ_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES` | 15 | Consecutive failures before open |
| `RABBITMQ_CIRCUIT_BREAKER_FAILURE_RATIO` | 50 | Failure % to trigger open (0-100) |
| `RABBITMQ_CIRCUIT_BREAKER_INTERVAL` | 120 | Failure counting window (seconds) |
| `RABBITMQ_CIRCUIT_BREAKER_MAX_REQUESTS` | 3 | Requests allowed in half-open |
| `RABBITMQ_CIRCUIT_BREAKER_MIN_REQUESTS` | 10 | Min requests before ratio evaluated |
| `RABBITMQ_CIRCUIT_BREAKER_TIMEOUT` | 30 | Open → half-open wait (seconds) |
| `RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL` | 30 | Health check interval (seconds) |
| `RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_TIMEOUT` | 10 | Health check timeout (seconds) |

### Bulk Recorder

Activates only when `RABBITMQ_TRANSACTION_ASYNC=true` AND `BULK_RECORDER_ENABLED=true`.

| Variable | Default | Description |
|----------|---------|-------------|
| `BULK_RECORDER_ENABLED` | (true) | Enable bulk mode |
| `BULK_RECORDER_SIZE` | (workers×prefetch) | Batch size (0 = auto-calculated) |
| `BULK_RECORDER_FLUSH_TIMEOUT_MS` | (100) | Max wait before flush (ms) |
| `BULK_RECORDER_MAX_ROWS_PER_INSERT` | (1000) | Max rows per INSERT statement |

### Balance Sync / Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `BALANCE_SYNC_BATCH_SIZE` | (50) | Keys accumulated before flush (SIZE trigger) |
| `BALANCE_SYNC_FLUSH_TIMEOUT_MS` | (500) | Max ms before flushing incomplete batch (TIMEOUT trigger) |
| `BALANCE_SYNC_POLL_INTERVAL_MS` | (50) | ZSET polling interval when draining |
| `SETTINGS_CACHE_TTL` | (5m) | Settings cache duration (Go duration) |

### Pagination

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_PAGINATION_LIMIT` | 100 | Max items per page |
| `MAX_PAGINATION_MONTH_DATE_RANGE` | 3 | Max date range for queries (months) |

### Telemetry (OpenTelemetry)

| Variable | Default | Description |
|----------|---------|-------------|
| `OTEL_RESOURCE_SERVICE_NAME` | ledger | Service name in traces |
| `OTEL_LIBRARY_NAME` | — | Instrumentation library name |
| `OTEL_RESOURCE_SERVICE_VERSION` | — | Service version in traces |
| `OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT` | — | Deployment environment in traces |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | midaz-otel-lgtm:4317 | OTLP gRPC collector endpoint |
| `ENABLE_TELEMETRY` | false | Enable telemetry export |

### Multi-Tenant (Ledger)

| Variable | Default | Description |
|----------|---------|-------------|
| `MULTI_TENANT_ENABLED` | false | Enable multi-tenant mode |
| `MULTI_TENANT_URL` | — | Tenant Manager API URL |
| `MULTI_TENANT_SERVICE_API_KEY` | — | Service API key for tenant-manager |
| `MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD` | (5) | CB failures before open |
| `MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC` | (30) | CB open → half-open (seconds) |
| `MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC` | — | Interval for revalidation checks |
| `MULTI_TENANT_CACHE_TTL_SEC` | (120) | Tenant config cache TTL (seconds) |
| `MULTI_TENANT_REDIS_HOST` | — | Redis for tenant Pub/Sub events |
| `MULTI_TENANT_REDIS_PORT` | 6379 | Redis port for Pub/Sub |
| `MULTI_TENANT_REDIS_PASSWORD` | — | Redis password for Pub/Sub |
| `MULTI_TENANT_REDIS_TLS` | false | Enable TLS for Pub/Sub Redis |

### CRM-Specific Variables

Source: `components/crm/internal/bootstrap/config.go`

| Variable | Default | Description |
|----------|---------|-------------|
| `ENV_NAME` | development | Environment name |
| `PROTO_ADDRESS` | — | gRPC address (CRM-only) |
| `SERVER_ADDRESS` | :4003 | HTTP listen address |
| `LOG_LEVEL` | debug | Log level |
| `MONGO_URI` | mongodb | Connection URI scheme |
| `MONGO_HOST` | midaz-mongodb | MongoDB host |
| `MONGO_NAME` | crm | Database name |
| `MONGO_USER` | midaz | Username |
| `MONGO_PASSWORD` | lerian | Password |
| `MONGO_PORT` | 5703 | Port |
| `MONGO_PARAMETERS` | — | Extra connection parameters |
| `MONGO_MAX_POOL_SIZE` | 1000 | Max pool size |
| `LCRYPTO_HASH_SECRET_KEY` | — | PII hash key (data security) |
| `LCRYPTO_ENCRYPT_SECRET_KEY` | — | PII encryption key (data security) |
| `PLUGIN_AUTH_ADDRESS` | — | Auth service address (CRM uses different var from Ledger) |
| `PLUGIN_AUTH_ENABLED` | false | Enable auth |
| `APPLICATION_NAME` | ledger | Application identity |
| `MULTI_TENANT_ENABLED` | false | Enable multi-tenant |
| `MULTI_TENANT_URL` | — | Tenant Manager API URL |
| `MULTI_TENANT_TIMEOUT` | — | HTTP client timeout (seconds) |
| `MULTI_TENANT_IDLE_TIMEOUT_SEC` | — | Idle connection eviction (seconds) |
| `MULTI_TENANT_MAX_TENANT_POOLS` | — | Max concurrent tenant pools |
| `MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD` | — | CB failures before open |
| `MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC` | — | CB open → half-open (seconds) |
| `MULTI_TENANT_SERVICE_API_KEY` | — | Service API key |
| `MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC` | — | Revalidation interval (seconds) |
| `MULTI_TENANT_CACHE_TTL_SEC` | (120) | Tenant config cache TTL (seconds) |
| `MULTI_TENANT_REDIS_HOST` | — | Redis for Pub/Sub |
| `MULTI_TENANT_REDIS_PORT` | 6379 | Redis port |
| `MULTI_TENANT_REDIS_PASSWORD` | — | Redis password |
| `MULTI_TENANT_REDIS_TLS` | false | TLS for Redis |

## Build & Make Targets — Complete Reference

### Setup
```bash
make set-env              # Copy .env.example → .env for all components
make clear-envs           # Remove all .env files
make dev-setup            # Install tools (gitleaks, gofumpt, goimports, gosec, golangci-lint) + git hooks
make setup-git-hooks      # Configure git hooks (core.hooksPath = .githooks)
```

### Build & Run
```bash
make build                # Build ledger + CRM binaries
make up                   # Start all services (infra → ledger → CRM)
make down                 # Stop all services (CRM → ledger → infra)
make start                # Start existing containers
make stop                 # Stop containers (no removal)
make restart              # down + up
make rebuild-up           # Rebuild images and restart
make logs                 # Show logs for all services (tail 50)
make clean                # Clean build artifacts (./scripts/clean-artifacts.sh)
make clean-docker         # Clean Docker resources (containers, networks, volumes, prune)
```

### Code Quality
```bash
make lint                 # Lint all components + tests/ + pkg/ (golangci-lint v2.4.0)
make format               # Format code in all components
make tidy                 # go mod tidy
make check-logs           # Verify error logging in usecases
make check-tests          # Verify test coverage for components
make check-hooks          # Verify git hooks installation
make check-envs           # Check hooks + secret env files not exposed
make sec                  # Run gosec + govulncheck
make sec-gosec            # Run gosec only (SARIF=1 for GitHub Security tab)
make sec-govulncheck      # Run govulncheck only
```

### Testing
```bash
make test                 # Run all tests (scripts/run-tests.sh)
make test-unit            # Unit tests only (excludes tests/ and api/ dirs)
make test-all             # Unit + integration tests
make test-integration     # Integration tests (testcontainers, -p=1; RUN=, PKG=, CHAOS=1)
make test-fuzz            # Native Go fuzz tests (FUZZ=target, FUZZTIME=10s)
make test-bench           # Benchmark tests (BENCH=pattern, BENCH_PKG=./...)
make test-chaos-system    # Chaos tests with full Docker stack (starts/stops services)
```

### Coverage
```bash
make cover                # Legacy coverage (scripts/coverage.sh → coverage.html)
make coverage             # All coverage targets (unit + integration)
make coverage-unit        # Unit test coverage (PKG=, uses .ignorecoverunit)
make coverage-integration # Integration test coverage (PKG=, CHAOS=1)
```

### Component Delegation
```bash
make infra COMMAND=<cmd>          # Run make target in infra component
make ledger COMMAND=<cmd>         # Run make target in ledger component
make all-components COMMAND=<cmd> # Run make target across all components
```

### Documentation & Migrations
```bash
make generate-docs                 # Generate Swagger docs (scripts/generate-docs.sh)
make migrate-lint                  # Lint SQL migrations for dangerous patterns
make migrate-create COMPONENT=<onboarding|transaction> NAME=<name>  # Create new migration
```

### Test Tooling
```bash
make tools                # Install test tools (gotestsum)
make tools-gotestsum      # Install gotestsum
make wait-for-services    # Wait for backend services healthy (TEST_HEALTH_WAIT=60s)
```

## Coding Conventions

Full rules in `docs/PROJECT_RULES.md` (1130 lines). Key points:

1. **Go 1.25+**: Use `any` not `interface{}`, use generics for utilities
2. **File naming**: `snake_case.go` with dot-separated component types (e.g., `balance_sync.worker.go`, `redis.consumer.go`)
3. **Import groups**: stdlib → external → internal (blank-line separated)
4. **Context**: Always first param, check `ctx.Err()` before work
5. **Error wrapping**: `%w` for technical, direct return for business errors
6. **Validation**: Normalize → Apply defaults → Validate → Execute
7. **Struct tags**: `json`, `validate`, `example`, `format`, `maxLength`
8. **Metadata**: Flat only (no nesting), key max 100, value max 2000
9. **IDs**: Use `uuid.UUID` type, not strings
10. **Soft delete**: `DeletedAt` field, status `DELETED`
11. **Pagination**: Page-based (page + limit), max 100 per page
12. **Structured logging**: Use `libLog.Err(err)`, `libLog.String()`, `libLog.Int()` fields instead of `fmt.Sprintf` inside log calls
13. **MT naming**: Multi-tenant code uses `MT` suffix (`NewFooMT`, `runFooMT`, `mtEnabled`, `isMTReady`). Default (single-tenant) uses no qualifier.

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
