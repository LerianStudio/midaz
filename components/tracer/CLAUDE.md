# CLAUDE.md

Comprehensive reference for AI coding agents working in the Tracer codebase. Read [AGENTS.md](AGENTS.md) first for a concise overview, then use this file for deep patterns and conventions.

## Quick Start (30 seconds)

```bash
cp .env.example .env   # Create environment file
make up                # Start Tracer via Docker Compose (joins the shared infra-network)
make test              # Run all tests
make lint              # Run golangci-lint v2
```

Health check: `GET http://localhost:4020/health`
Readiness: `GET http://localhost:4020/readyz`

## Project Identity

| Attribute | Value |
|-----------|-------|
| **Project** | Real-time transaction validation and fraud prevention API |
| **Component** | Co-located deploy unit in the `midaz` monorepo (module `github.com/LerianStudio/midaz/v4`, single root `go.mod`, no own `go.mod`) |
| **Language** | Go (root `go.mod`: `go 1.26.3`, toolchain `go1.26.4`; Dockerfile builder: `golang:1.26.3-alpine`) |
| **Architecture** | Hexagonal (Ports & Adapters) + CQRS |
| **Database** | PostgreSQL 16 |
| **Rule Engine** | Google CEL (cel-go v0.28.1) |
| **Auth** | lib-auth v2 (API Key + Access Manager plugin) |
| **Observability** | OpenTelemetry via lib-observability |
| **Testing** | TDD; testify + sqlmock + gomock + testcontainers + Godog (BDD) |
| **License** | Elastic License 2.0 |

## Essential Commands

### Development

```bash
make build            # Build binary to .bin/tracer
make run              # Run locally with .env config (go run cmd/app/main.go)
make clean            # Remove build artifacts
make tidy             # go mod tidy
make dev-setup        # Install tools (golangci-lint, swag, mockgen, gosec) + set-env + tidy
```

### Testing

```bash
make test             # All tests (go test -v ./...)
make test-unit        # Unit tests with race detector (excludes tests/ and api/)
make test-integration # Integration tests with testcontainers (build tag: integration, -p=1)
make test-e2e         # E2E BDD tests with Godog (resets Docker, build tag: e2e)
make test-all         # Unit + integration
make test-bench       # Benchmarks (BENCH=pattern BENCH_PKG=./path)
make check-tests      # Quick coverage verification

# Single test
go test -v -run TestFunctionName ./internal/services/command/...

# Integration with specific test
make test-integration RUN=TestIntegration_PostgresRepo_Create

# Low-resource mode (CI)
make test-integration LOW_RESOURCE=1
```

### Coverage

```bash
make coverage-unit          # Unit coverage report (uses .ignorecoverunit for exclusions)
make coverage-integration   # Integration coverage report
make coverage               # All coverage targets
```

### Quality

```bash
make lint             # golangci-lint v2 with --fix (auto-installs if missing)
make format           # go fmt
make sec              # gosec + govulncheck security scan
make sec SARIF=1      # SARIF output for GitHub Security tab
make quality          # lint + test aggregator
make generate         # go generate (mocks via mockgen)
make generate-docs    # Generate Swagger/OpenAPI docs to api/ (uses swaggo/swag under the hood)
make verify-api-docs  # Check annotation coverage
make validate-api-docs # Validate generated OpenAPI spec
```

### Docker

```bash
make up               # Start Tracer (docker compose up -d; uses shared infra-network for PostgreSQL)
make down             # Stop and remove containers/volumes
make start            # Start existing containers (without recreating)
make stop             # Stop containers (without removing)
make restart          # Stop + start
make rebuild-up       # Full rebuild and restart
make logs             # Tail all service logs
make logs-api         # Tail tracer service only
make ps               # List container status
```

### Database

```bash
make migrate          # Apply all pending migrations
make migrate-down     # Rollback last migration
make migrate-down-all # Rollback ALL (5s confirmation, FORCE=1 to skip)
make migrate-version  # Show current version
make migrate-force VERSION=N  # Force set version
make ensure-migrate   # Ensure migration tool is installed
make seed             # Load development seed data
make seed-down        # Remove seed data
```

### Git Hooks

```bash
make setup-git-hooks  # Install pre-commit hooks from .githooks/
make check-hooks      # Verify hooks installation
make check-envs       # Verify no secrets in tracked files
```

## Architecture

### Directory Structure

```
tracer/
├── cmd/app/main.go                 # Entry point: loads .env, calls bootstrap.InitServers()
├── internal/
│   ├── bootstrap/                  # Composition root
│   │   ├── config.go              # Config struct, InitServers(), all DI wiring
│   │   ├── http_server.go         # Fiber server with graceful shutdown
│   │   └── service.go             # Service struct with Run()/Shutdown()
│   ├── adapters/
│   │   ├── http/in/               # Fiber handlers, routes, middleware, validation
│   │   │   ├── routes.go          # Route registration with auth guard
│   │   │   ├── validation_handler.go  # POST /v1/validations
│   │   │   ├── rule_handler.go    # Rule CRUD + lifecycle handlers
│   │   │   ├── limit_handler.go   # Limit CRUD + lifecycle handlers
│   │   │   ├── audit_event_handler.go # Audit event read handlers
│   │   │   ├── health.go          # Health/readiness probes
│   │   │   ├── middleware/        # Auth (lib-auth v2: API Key + Access Manager), CORS, IP extraction
│   │   │   └── *_validation.go    # Input validation per resource
│   │   ├── postgres/              # Repository implementations
│   │   │   ├── rule_repository.go           # Rules CRUD
│   │   │   ├── limit_repository.go          # Limits CRUD
│   │   │   ├── usage_counter_repository.go  # Usage counter ops
│   │   │   ├── transaction_validation_repository.go # Validation records
│   │   │   ├── audit_event_repository.go    # Audit events + hash chain
│   │   │   ├── rule_sync_repository.go      # Delta queries for cache sync
│   │   │   ├── db/                # DB/Connection/TxBeginner interfaces
│   │   │   └── *_postgresql_model.go  # DB ↔ domain model conversions
│   │   └── cel/                   # CEL expression engine adapter
│   │       ├── adapter.go         # Compile + Evaluate with cost limiting
│   │       ├── environment.go     # CEL environment setup (variables, types)
│   │       └── program.go         # Compiled program wrapper
│   ├── services/                  # Business logic
│   │   ├── validation_service.go  # Orchestrates rules + limits + audit in single tx
│   │   ├── rule_service.go        # Rule facade (commands + queries)
│   │   ├── limit_service.go       # Limit facade (commands + queries + usage)
│   │   ├── audit_event_service.go # Read-only audit facade
│   │   ├── transaction_validation_service.go # Validation history facade
│   │   ├── audit_writer.go        # AuditWriter interface (services package)
│   │   ├── metrics.go             # Prometheus-style metrics helpers
│   │   ├── command/               # Write operations
│   │   │   ├── create_rule.go, update_rule.go, activate_rule.go, ...
│   │   │   ├── create_limit.go, update_limit.go, activate_limit.go, ...
│   │   │   ├── record_audit_event.go  # AuditWriter implementation
│   │   │   ├── repository.go     # RuleRepository interface
│   │   │   ├── limit_repository.go # LimitRepository interface
│   │   │   ├── audit_writer.go   # AuditWriter interface (command package)
│   │   │   └── rule_cache_writer.go # RuleCacheWriter interface
│   │   ├── query/                 # Read operations
│   │   │   ├── evaluate_rules.go  # Rule evaluation orchestration
│   │   │   ├── rule_evaluator.go  # Single rule CEL evaluation
│   │   │   ├── complete_evaluator.go # Full evaluation with scope matching
│   │   │   ├── get_active_rules.go # Active rules retrieval (DB or cache)
│   │   │   ├── limit_checker.go   # Limit enforcement with usage counters
│   │   │   ├── verify_audit_event.go # Hash chain verification
│   │   │   └── get_*, list_*      # Standard CRUD queries
│   │   ├── cache/                 # In-memory rule cache
│   │   │   ├── rule_cache.go      # Thread-safe cache (sync.RWMutex)
│   │   │   ├── adapter.go         # CacheAdapter → ActiveRulesRepository
│   │   │   ├── warmup.go          # Initial cache population from DB
│   │   │   └── cached_rule.go     # CachedRule struct (Rule + compiled program)
│   │   └── workers/               # Background workers
│   │       ├── rule_sync_worker.go      # Polls DB for rule changes
│   │       └── usage_cleanup_worker.go  # Cleans expired usage counters
│   ├── testhelper/                # Test helpers (e.g., time_of_day.go)
│   ├── testutil/                  # Shared test utilities
│   ├── testutil_dbsuite/          # DB test suite helpers
│   └── testutil_integration/      # Integration test helpers
├── pkg/
│   ├── model/                     # Domain entities (44 files)
│   │   ├── rule.go, limit.go, validation.go, scope.go
│   │   ├── audit_event.go, transaction_validation.go, transaction.go
│   │   ├── context.go, decision_maker.go, evaluation_result.go
│   │   ├── check_limits.go, scope_matcher.go, time_of_day.go
│   │   └── *_test.go             # Extensive domain model tests
│   ├── constant/                  # Error codes (TRC-XXXX), pagination, app constants
│   ├── clock/                     # Clock interface (Real + Fixed for MOCK_TIME)
│   ├── resilience/                # Circuit breaker wrapper (sony/gobreaker)
│   ├── validation/                # Date/time validation (RFC3339)
│   ├── hash/                      # SHA-256 hash chain for audit events
│   ├── logging/                   # WithTrace() — enriches logger with trace/span IDs
│   ├── sanitize/                  # Input sanitization helpers
│   ├── net/                       # HTTP cursor pagination, IP extraction
│   ├── contextutil/               # Context value extraction
│   └── shell/                     # Makefile color/utility includes
├── tests/
│   ├── integration/               # 45 testcontainers-based API test files
│   └── end2end/                   # BDD (Godog) with Gherkin features
├── migrations/                    # 17 migrations + seeds/
├── api/                           # Generated Swagger docs
└── .golangci.yml                  # Linter config (golangci-lint v2)
```

### Bounded Contexts

| Context | Responsibility |
|---------|----------------|
| **Validation** | Orchestrate validation requests, coordinate rule/limit evaluation, record audit |
| **Rules** | Manage rule lifecycle, compile/evaluate CEL expressions, in-memory cache |
| **Limits** | Manage spending limits, track usage counters, enforce thresholds |
| **Audit** | Immutable event log, hash chain verification, SOX/GLBA compliance |

### Key Interfaces

**Command layer** (`internal/services/command/`):

| Interface | Methods | Implementations |
|-----------|---------|-----------------|
| `RuleRepository` | Create, GetByID, GetByName, ListByStatus, Update, Delete, ListActiveByScopes, UpdateStatus | `postgres.Repository` |
| `LimitRepository` | Create, GetByID, Update, UpdateStatus | `postgres.LimitRepository` |
| `AuditWriter` | RecordValidationEvent, RecordRuleEvent, RecordLimitEvent | `command.RecordAuditEventCommand` |
| `ExpressionCompiler` | Compile(ctx, expr) → (any, error) | `celCompilerAdapter` (wraps `cel.Adapter`) |
| `RuleCacheWriter` | UpsertRule, RemoveRule | `cache.RuleCache` |

**Query layer** (`internal/services/query/`):

| Interface | Methods | Implementations |
|-----------|---------|-----------------|
| `ActiveRulesRepository` | GetActiveRulesForScopes | `cache.CacheAdapter`, `postgres.Repository` |
| `LimitRepository` | GetByID, List | `postgres.LimitRepository` |
| `UsageCounterRepository` | GetByLimitID, IncrementOrInsert, GetByScopeAndPeriod | `postgres.UsageCounterRepository` |

**Infrastructure** (`internal/adapters/postgres/db/`):

| Interface | Purpose |
|-----------|---------|
| `DB` | ExecContext, QueryContext, QueryRowContext |
| `Connection` | GetDB() → (DB, error) |
| `TxBeginner` | BeginTx(ctx, opts) → Tx |

## Code Patterns

### 1. Service Methods

Every service method starts with tracking + span:

```go
func (s *ActivateRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (*model.Rule, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "service.rule.activate")
    defer span.End()

    logger = logging.WithTrace(ctx, logger)
    // ... business logic ...
}
```

### 2. Error Handling

- Sentinel errors in `pkg/constant/errors.go` with TRC-XXXX codes
- Business errors: return directly, use `HandleSpanBusinessErrorEvent`
- Technical errors: wrap with `fmt.Errorf("context: %w", err)`, use `HandleSpanError`
- Use `%w` always, never `%v` for error wrapping (enforced by errorlint)
- Constructor validation: return `(*T, error)`, sentinel errors for nil deps

### 3. Domain Entities (pkg/model/)

- Constructor `New*()` validates invariants, returns `(*T, error)`
- Validate-before-mutate: no partial mutations on error
- Defensive copies for slices/maps
- Private fields with validated setters for state transitions
- `Validate()` method on every entity for persistence-boundary checks
- Amounts use `shopspring/decimal` (never float64)
- IDs use `uuid.UUID` (never string)
- Timestamps: always `time.Now().UTC()`

### 4. Repository Pattern

- Interfaces defined where used (command/ and query/ packages)
- PostgreSQL model structs separate from domain entities
- Squirrel SQL builder with `squirrel.Dollar` placeholder format
- Cursor-based pagination (no offset)
- `sortBy` validated against allowlist before cursor encoding

### 5. HTTP Handlers

- Fiber v2 framework
- Every handler: extract ctx, start span, parse input, call service, respond
- Response wrappers: `libHTTP.OK()`, `libHTTP.Created()`, `libHTTP.NoContent()`, `libHTTP.WithError()`
- Never use direct Fiber responses (`c.JSON()`, `c.Status().JSON()`)
- Swagger annotations required on all handlers
- Input validation in separate `*_validation.go` files

### 6. Testing Patterns

- TDD required: write test first, then implement
- Table-driven tests with descriptive case names
- `gomock` for interface mocks (never manual mocks)
- `sqlmock` for database query testing
- Deterministic data: `testutil.FixedTime()`, `testutil.MustDeterministicUUID(seed)`
- Never `uuid.New()` or `time.Now()` in tests
- `require.Len` before indexing slices
- `require.Equal` for ordered content, `require.ElementsMatch` for unordered
- No `defer ctrl.Finish()` — go.uber.org/mock v0.3.0+ auto-registers cleanup
- Build tags: `integration` for testcontainers, `e2e` for BDD

### 7. Structured Logging

- Always `WithFields` (never string interpolation)
- OpenTelemetry semantic conventions: `rule.id`, `error.message`, `operation`
- Enrich with trace context: `logger = logging.WithTrace(ctx, logger)`
- `operation` field must match span name
- Log messages: "Creating rule" (start), "Rule created successfully" (success), "Failed to create rule" (error)

### 8. Authentication

- API Key via `X-API-Key` header
- Constant-time comparison (`crypto/subtle`)
- Configurable: `API_KEY_ENABLED`, `API_KEY_ENABLED_ONLY_VALIDATION`
- Access Manager plugin: `PLUGIN_AUTH_ENABLED`, `PLUGIN_AUTH_ADDRESS`
- Public endpoints: `/health`, `/readyz`, `/metrics`, `/version`, `/swagger/*`

## Configuration

### Environment Variables

Most config loaded via `libCommons.SetConfigFromEnvVars(cfg)` from struct tags. Exception: `MOCK_TIME` is read once via `os.Getenv` at boot (not part of Config struct).

| Category | Variables |
|----------|-----------|
| **Application** | `SERVER_ADDRESS` (`:4020`), `LOG_LEVEL` |
| **Database** | `DB_HOST`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_PORT`, `DB_SSL_MODE`, `MIGRATIONS_PATH` |
| **Auth** | `API_KEY`, `API_KEY_ENABLED`, `API_KEY_ENABLED_ONLY_VALIDATION`, `PLUGIN_AUTH_ADDRESS`, `PLUGIN_AUTH_ENABLED` |
| **CORS** | `CORS_ALLOWED_ORIGINS` |
| **CEL** | `CEL_COST_LIMIT` (default: 10000) |
| **Evaluation** | `DEFAULT_DECISION_WHEN_NO_MATCH` (ALLOW\|DENY), `MAX_RULES_PER_REQUEST` (1000) |
| **Telemetry** | `ENABLE_TELEMETRY`, `OTEL_RESOURCE_SERVICE_NAME`, `OTEL_LIBRARY_NAME`, `OTEL_RESOURCE_SERVICE_VERSION`, `OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT`, `OTEL_EXPORTER_OTLP_ENDPOINT` |
| **Cleanup Worker** | `CLEANUP_WORKER_ENABLED` (false), `CLEANUP_INTERVAL_HOURS` (24) |
| **Rule Sync** | `RULE_SYNC_POLL_INTERVAL_SECONDS` (10), `RULE_SYNC_STALENESS_THRESHOLD_SECONDS` (50), `RULE_SYNC_OVERLAP_BUFFER_SECONDS` (2) |
| **Multi-Tenancy** | `MULTI_TENANT_ENABLED` (false), `MULTI_TENANT_URL`, `MULTI_TENANT_SERVICE_API_KEY`, `MULTI_TENANT_REDIS_HOST`, `MULTI_TENANT_ALLOW_INSECURE_HTTP` |
| **Testing** | `MOCK_TIME` (RFC3339 via `os.Getenv`, not in Config struct — read once at boot for deterministic integration tests) |

### Docker Compose Services

The component compose declares only the `tracer` app service (container `midaz-tracer`, built from `Dockerfile.dev` with build context `../../`, the monorepo root) and joins the shared external `infra-network`. PostgreSQL and other shared infra come from `components/infra/docker-compose.yml`, not from this compose.

| Service | Image | Port |
|---------|-------|------|
| tracer | built from `Dockerfile.dev` (context `../../`) | `${SERVER_PORT}` (4020) |

### Dockerfile (Production)

Multi-stage: `golang:1.26.3-alpine` (builder, `--platform=$BUILDPLATFORM`) → `gcr.io/distroless/static-debian12:nonroot` (runtime, stage `prod`). Both Dockerfiles COPY `go.mod`/`go.sum` from the repo-root build context and build `components/tracer/cmd/app/main.go`. Exposes 4020. Non-root user. No shell in production — use orchestrator probes.

## Linting

### golangci-lint v2

Configuration in `.golangci.yml`. Key enabled linters:

- `errorlint` — enforces `%w` usage, proper error assertions
- `contextcheck` — verifies correct context propagation
- `depguard` — blocks `io/ioutil` (deprecated)
- `gocyclo` — max complexity 20
- `revive` — import shadowing, empty blocks, use-any
- `wsl_v5` — whitespace conventions (empty lines before return, if, defer)
- `staticcheck`, `bodyclose`, `prealloc`, `nilerr`, `unconvert`, `wastedassign`

Shadow declarations of `err` and `ok` are excluded from govet shadow check.

### wsl_v5 Whitespace Rules

Empty line required before: `return`, `if`, `for`, `switch`, `defer`, assignments after different statement types. No empty lines at block boundaries.

## Required Libraries

### Lerian-Specific

- **lib-auth/v2** (`v2.8.0`): Auth middleware, auth client
  - `authMiddleware.NewAuthClient(address, enabled, &logger)`
- **lib-commons/v5** (`v5.4.1`): Common utilities, infrastructure
  - Tracking: `libCommons.NewTrackingFromContext(ctx)` → logger, tracer, headerID
  - Database: `libPostgres.New(config)` → Client with primary/replica
  - Config: `libCommons.SetConfigFromEnvVars(cfg)` — struct tag loading
  - HTTP: `libHTTP.OK()`, `libHTTP.Created()`, `libHTTP.WithError()`, `libHTTP.HandleFiberError()`
  - Launcher: `libCommons.NewLauncher(opts...).Run()` — graceful multi-service lifecycle
- **lib-observability** (`v1.0.1`): OpenTelemetry, logging (observability split out of lib-commons)
  - OpenTelemetry: `libOtel.HandleSpanError(span, "msg", err)`, `libOtel.HandleSpanBusinessErrorEvent(span, "msg", err)`
  - Logging: `libZap.New()`, `libLog.Logger` interface
  - Packages: `lib-observability/{log,metrics,runtime,tracing,zap}`

### Key Third-Party

- `gofiber/fiber/v2` — HTTP framework
- `google/cel-go` v0.28.1 — CEL expression engine
- `Masterminds/squirrel` — SQL query builder
- `shopspring/decimal` — Precise decimal arithmetic
- `google/uuid` — UUID generation
- `jackc/pgx/v5` — PostgreSQL driver
- `sony/gobreaker` — Circuit breaker
- `bxcodec/dbresolver/v2` — Primary/replica DB routing
- `golang-migrate/migrate/v4` — Database migrations
- `go-playground/validator/v10` — Struct tag validation for DTOs
- `DATA-DOG/go-sqlmock` — SQL mocking
- `testcontainers/testcontainers-go` — Container-based integration tests
- `cucumber/godog` — BDD test framework
- `go.uber.org/mock` — Interface mocking (gomock)

## CI/CD

All CI uses shared workflows from `LerianStudio/github-actions-shared-workflows`.

### Pre-Commit Checklist

1. `make lint` — linters pass
2. `make test-unit` — unit tests pass
3. `make sec` — no security issues
4. `make generate-docs` — Swagger updated (if API changed)
5. Commit message follows conventional commits

## Common Tasks

### Adding a New API Endpoint

1. Add handler method to `internal/adapters/http/in/{resource}_handler.go`
2. Add input validation in `{resource}_validation.go`
3. Register route in `routes.go`
4. Add Swagger annotations
5. Run `make generate-docs`
6. Write tests for handler

### Adding a New Command

1. Create file in `internal/services/command/{verb}_{resource}.go`
2. Define input struct in same file
3. Implement `Execute(ctx, input)` with tracking + span
4. Wire dependencies in `internal/bootstrap/config.go`
5. Write table-driven tests with gomock

### Adding a New Migration

1. Create `migrations/000NNN_descriptive_name.up.sql` and `.down.sql`
2. Test: `make migrate && make migrate-down && make migrate`
3. Add indexes for join/filter columns

## Debugging Tips

| Problem | Solution |
|---------|----------|
| Server won't start | Check `make up` ran, verify `.env` exists, check `make logs` |
| Migration stuck | `make migrate-version` to check state, `make migrate-force VERSION=N` |
| Integration tests fail | Ensure Docker running, try `LOW_RESOURCE=1` |
| E2E tests fail | `make test-e2e E2E_SKIP_RESET=1` to reuse DB |
| Rule not evaluated | Verify rule status is ACTIVE (not DRAFT) |
| Cache stale | Check `RULE_SYNC_POLL_INTERVAL_SECONDS`, readiness probe reports cache health |
| CEL expression error | Check `CEL_COST_LIMIT`, verify expression compiles at rule creation |
| Testcontainers hang | Set `TESTCONTAINERS_RYUK_DISABLED=true` (macOS/Docker Desktop) |

## Key Files

| File | Purpose |
|------|---------|
| [`AGENTS.md`](AGENTS.md) | Concise agent overview (read first) |
| [`docs/PROJECT_RULES.md`](docs/PROJECT_RULES.md) | Full architectural rules, domain model, testing standards |
| [`.env.example`](.env.example) | All environment variables with documentation |
| [`.golangci.yml`](.golangci.yml) | Linter configuration |
| [`internal/bootstrap/config.go`](internal/bootstrap/config.go) | Composition root — all DI wiring |
| [`pkg/constant/errors.go`](pkg/constant/errors.go) | All error codes (TRC-XXXX) |
| [`pkg/model/`](pkg/model/) | Domain entities (44 files) |
| [`api/swagger.json`](api/swagger.json) | OpenAPI specification (auto-generated by swaggo — do not edit) |
| [`api/swagger.yaml`](api/swagger.yaml) | OpenAPI spec YAML (auto-generated by swaggo — do not edit) |
| [`api/openapi.yaml`](api/openapi.yaml) | OpenAPI spec via openapi-generator (auto-generated — do not edit) |
| [`api/docs.go`](api/docs.go) | Swagger Go bindings (auto-generated by swaggo — do not edit) |

---

**Last Updated**: June 2026
**Go Version**: root `go.mod` `go 1.26.3` (toolchain `go1.26.4`), Docker builder image `golang:1.26.3-alpine`
**Migrations**: 17 (000001 through 000017)
