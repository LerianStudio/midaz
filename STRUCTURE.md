# Project Structure Overview

Welcome to the comprehensive guide on the structure of the Midaz project. This document reflects the current architecture of the codebase, designed with a focus on scalability, maintainability, and clear separation of concerns following Hexagonal Architecture and Command Query Responsibility Segregation (CQRS) patterns.

## Directory Layout

```
midaz/
├── bin/                          # Compiled binaries (gitignored)
├── components/                   # Service applications
│   ├── console/                  # Next.js frontend application
│   ├── crm/                      # Customer relationship management service
│   ├── infra/                    # Docker infrastructure definitions
│   ├── mdz/                      # CLI tooling component
│   ├── onboarding/               # Entity management (orgs, ledgers, accounts)
│   └── transaction/              # Transaction processing & balance calculations
├── docs/                         # Documentation
│   └── agents/                   # Agent-specific technical documentation
├── mk/                           # Makefile includes
│   └── tests.mk                  # Test-related make targets
├── pkg/                          # Shared libraries
│   ├── assert/                   # Domain invariant assertions (production-safe)
│   ├── constant/                 # Error codes and constants
│   ├── gold/                     # Transaction DSL parser (ANTLR-based)
│   ├── mgrpc/                    # gRPC protobuf definitions
│   ├── mlint/                    # Custom golangci-lint plugins
│   ├── mmodel/                   # Domain models shared across components
│   ├── mongo/                    # MongoDB database utilities
│   ├── mruntime/                 # Safe goroutine handling with panic recovery
│   ├── net/http/                 # HTTP helpers and middleware
│   ├── shell/                    # Shell utilities and ASCII art
│   ├── transaction/              # Transaction models and validations
│   ├── utils/                    # General utility functions
│   └── errors.go                 # Typed business error definitions
├── postman/                      # Postman collections for API testing
├── scripts/                      # Build and automation scripts
└── tests/                        # Test suites
    ├── chaos/                    # Chaos engineering tests
    ├── e2e/                      # End-to-end tests (Apidog CLI)
    ├── fixtures/                 # Test fixtures and data
    ├── fuzzy/                    # Fuzz testing
    ├── helpers/                  # Test helper utilities
    ├── integration/              # Integration tests
    └── property/                 # Property-based tests
```

## Components (`./components`)

### Console (`./components/console`)

**Purpose**: Next.js-based frontend application for the Midaz ledger system.

**Technology Stack**:
- Next.js (React framework)
- TypeScript
- Playwright (E2E testing)
- Jest (Unit testing)
- Internationalization (i18n) support

**Key Directories**:
- `src/app/` - Next.js app router pages
- `src/components/` - React UI components
- `src/core/` - Core business logic and domain models
- `src/hooks/` - Custom React hooks
- `src/lib/` - Library utilities and integrations
- `src/schema/` - Validation schemas
- `tests/e2e/` - Playwright end-to-end tests
- `locales/` - Internationalization files
- `public/` - Static assets (images, fonts, SVG)

**Configuration**:
- `next.config.mjs` - Next.js configuration
- `tsconfig.json` - TypeScript configuration
- `playwright.config.ts` - E2E test configuration
- `jest.config.ts` - Unit test configuration

### CRM (`./components/crm`)

**Purpose**: Customer Relationship Management service handling aliases, holders, and holder-link relationships.

**Architecture**: Hexagonal architecture with MongoDB-only persistence (no CQRS separation in services).

**Key Directories**:
- `api/` - OpenAPI/Swagger documentation
- `cmd/app/` - Application entry point
- `internal/`
  - `adapters/` - External integrations
    - `http/in/` - HTTP handlers (Fiber-based)
    - `mongodb/` - MongoDB repositories (alias, holder, holder-link)
  - `bootstrap/` - Application initialization and dependency injection
  - `services/` - Business logic (flat structure, no command/query separation)
- `scripts/` - API validation scripts (JavaScript-based)

**Database**: MongoDB only (no PostgreSQL)

**API Documentation**: Swagger/OpenAPI at `/api/swagger.yaml`

### Infrastructure (`./components/infra`)

**Purpose**: Docker Compose definitions and configuration for all infrastructure dependencies.

**Services Managed**:
- **PostgreSQL 17**: Primary database with read replica support
- **MongoDB 8**: Metadata storage with replica set configuration
- **Valkey 8**: Redis-compatible cache (fork of Redis)
- **RabbitMQ 4.1.3**: Message queue for async processing
- **Grafana Stack**: Observability (LGTM - Loki, Grafana, Tempo, Mimir)

**Key Files**:
- `docker-compose.yml` - Infrastructure orchestration
- `postgres/init.sql` - PostgreSQL initialization
- `mongo/mongo.sh` - MongoDB replica set setup
- `grafana/otelcol-config.yaml` - OpenTelemetry collector configuration

### MDZ (`./components/mdz`)

**Purpose**: Command-line interface (CLI) tooling for Midaz operations.

**Status**: Under development (artifacts directory present)

### Onboarding (`./components/onboarding`)

**Purpose**: Entity management service handling Organizations, Ledgers, Assets, Portfolios, Segments, Accounts, and Account Types.

**Architecture**: Hexagonal architecture with CQRS pattern.

**Key Directories**:
- `api/` - OpenAPI/Swagger documentation
- `cmd/app/` - Application entry point
- `internal/`
  - `adapters/` - External integrations
    - `grpc/` - gRPC adapters (in: service endpoints, out: client calls)
    - `http/` - HTTP adapters (in: handlers, out: client calls)
    - `mongodb/` - MongoDB metadata repository
    - `postgres/` - PostgreSQL repositories by entity:
      - `account/` - Account repository
      - `accounttype/` - Account Type repository
      - `asset/` - Asset repository
      - `ledger/` - Ledger repository
      - `organization/` - Organization repository
      - `portfolio/` - Portfolio repository
      - `segment/` - Segment repository
    - `redis/` - Redis cache consumer
  - `bootstrap/` - Application initialization, dependency injection, config
  - `services/` - Business logic with CQRS separation
    - `command/` - Write operations (Create, Update, Delete)
    - `query/` - Read operations (Get, GetAll, Count)
- `migrations/` - PostgreSQL database migrations (golang-migrate)
- `scripts/` - API validation scripts

**Database**: PostgreSQL (primary) + MongoDB (metadata) + Redis (cache)

**API Documentation**: Swagger/OpenAPI at `/api/swagger.yaml`

**Domain Entities**:
- Organization - Top-level tenant
- Ledger - Accounting ledger within organization
- Asset - Currency/asset type
- Portfolio - Grouping of accounts
- Segment - Account segmentation
- Account - Ledger account
- AccountType - Account classification

### Transaction (`./components/transaction`)

**Purpose**: Transaction processing service handling Transactions, Operations, Balances, Asset Rates, and routing.

**Architecture**: Hexagonal architecture with CQRS pattern + async messaging.

**Key Directories**:
- `api/` - OpenAPI/Swagger documentation
- `cmd/app/` - Application entry point
- `internal/`
  - `adapters/` - External integrations
    - `grpc/` - gRPC adapters (in: service endpoints, out: client calls)
    - `http/` - HTTP adapters (in: handlers, out: client calls)
    - `mongodb/` - MongoDB metadata repository
    - `postgres/` - PostgreSQL repositories by entity:
      - `assetrate/` - Asset rate repository
      - `balance/` - Balance repository
      - `operation/` - Operation repository
      - `operationroute/` - Operation routing repository
      - `transaction/` - Transaction repository
      - `transactionroute/` - Transaction routing repository
    - `rabbitmq/` - RabbitMQ producer/consumer for async events
    - `redis/` - Redis cache with Lua scripts
  - `bootstrap/` - Application initialization, dependency injection, config
  - `services/` - Business logic with CQRS separation
    - `command/` - Write operations, async event handling
    - `query/` - Read operations, balance calculations
- `migrations/` - PostgreSQL database migrations (golang-migrate)
- `scripts/` - API validation scripts

**Database**: PostgreSQL (primary) + MongoDB (metadata) + Redis (cache) + RabbitMQ (events)

**API Documentation**: Swagger/OpenAPI at `/api/swagger.yaml`

**Domain Entities**:
- Transaction - Financial transaction (parent entity)
- Operation - Individual operation within transaction (debit/credit)
- Balance - Account balance (versioned, with key support)
- AssetRate - Exchange rate for asset conversions
- OperationRoute - Routing rules for operations
- TransactionRoute - Routing rules for transactions

**Key Features**:
- Double-entry accounting validation
- Async balance calculation via RabbitMQ
- Idempotency key support
- Transaction reversal support
- Gold DSL for complex transaction definitions

## Common Utilities (`./pkg`)

### `assert/` - Domain Invariant Assertions

**Purpose**: Production-safe runtime assertions for detecting programming bugs.

**Key Features**:
- Always-on assertions (remain enabled in production)
- Rich context with key-value pairs
- Automatic value truncation for logs
- Domain predicates (Positive, NonNegative, ValidUUID, ValidAmount)
- Integrates with mruntime panic recovery

**Core Functions**:
- `assert.That(condition, msg, kv...)` - General assertion
- `assert.NotNil(value, msg, kv...)` - Nil check
- `assert.NotEmpty(str, msg, kv...)` - Empty string check
- `assert.NoError(err, msg, kv...)` - Error check
- `assert.Never(msg, kv...)` - Unreachable code marker

**Use Cases**: Pre-conditions, post-conditions, invariant checks, unreachable code detection.

**Documentation**: `pkg/assert/doc.go`, `docs/agents/pkg-libraries/assert.md`

### `constant/` - Error Codes and Constants

**Purpose**: Centralized constants for error messages, HTTP status codes, pagination, and entity-specific constants.

**Key Files**:
- `errors.go` - Error message constants
- `http.go` - HTTP header and status constants
- `pagination.go` - Pagination constants
- `account.go`, `balance.go`, `operation.go`, `transaction.go` - Domain-specific constants

### `errors.go` - Typed Business Errors

**Purpose**: Root-level typed error definitions for business logic validation.

**Error Types**:
- `EntityNotFoundError` - Entity lookup failures
- `ValidationError` - Business rule violations
- `EntityConflictError` - Uniqueness/conflict violations

**Integration**: Validated via `pkg.ValidateBusinessError()` for consistent error handling across services.

**Documentation**: `docs/agents/pkg-libraries/errors.md`

### `gold/` - Transaction DSL Parser

**Purpose**: ANTLR4-based parser for the Gold transaction language, enabling complex n:n transaction definitions via DSL.

**Key Components**:
- `Transaction.g4` - ANTLR grammar definition
- `parser/` - Generated ANTLR parser
- `transaction/` - Transaction model and validation

**Use Case**: Define complex multi-leg transactions using a domain-specific language instead of verbose JSON.

**Documentation**: `docs/agents/transaction-dsl.md`

### `mgrpc/` - gRPC Protobuf Definitions

**Purpose**: Shared gRPC service definitions and error utilities for inter-service communication.

**Key Files**:
- `grpc.go` - gRPC utilities and helpers
- `errors.go` - gRPC error mapping
- `balance/` - Balance service protobuf definitions

**Usage**: Used by onboarding and transaction services for synchronous inter-service calls.

### `mlint/` - Custom golangci-lint Plugins

**Purpose**: Custom linter plugins enforcing project-specific coding standards.

**Plugins**:
- `panicguard/` - Enforces use of `mruntime.SafeGo` instead of bare `go` keyword
- `panicguardwarn/` - Warning variant of panicguard

**Integration**: Configured in `.golangci.yml` and enforced by `make lint`.

**Rationale**: Prevents unhandled panics in goroutines by mandating safe concurrency patterns.

### `mmodel/` - Domain Models

**Purpose**: Shared domain models used across components (onboarding, transaction, CRM).

**Key Models**:
- `organization.go`, `ledger.go`, `asset.go` - Onboarding entities
- `account.go`, `account-type.go`, `portfolio.go`, `segment.go` - Account structures
- `balance.go` - Balance entity
- `operation-route.go`, `transaction-route.go` - Routing models
- `holder.go`, `holder-link.go`, `alias.go` - CRM entities
- `queue.go` - Message queue models
- `status.go` - Entity status enums
- `error.go` - Domain error types

### `mongo/` - MongoDB Utilities

**Purpose**: MongoDB connection pooling and client management.

**Key Features**:
- Connection pool management
- Replica set support
- Error handling utilities

**Usage**: Used by all components for metadata storage in MongoDB.

### `mruntime/` - Safe Goroutine Handling

**Purpose**: Panic recovery utilities with observability integration (mandatory for all goroutines).

**Key Features**:
- Policy-based panic recovery (KeepRunning, CrashProcess)
- Full observability (metrics, tracing, error reporting)
- Context-aware recovery
- Stack trace capture

**Core Functions**:
- `mruntime.SafeGoWithContextAndComponent(ctx, logger, component, name, policy, fn)` - Launch safe goroutine
- `mruntime.RecoverAndLogWithContext(ctx, logger, component, name)` - Deferred recovery
- `mruntime.RecoverWithPolicy(ctx, logger, component, name, policy)` - Policy-based recovery

**Integration**: OpenTelemetry metrics, Sentry error reporting (optional).

**Documentation**: `pkg/mruntime/doc.go`, `docs/agents/pkg-libraries/mruntime.md`, `docs/agents/concurrency.md`

**CRITICAL**: Use of bare `go` keyword is forbidden (enforced by mlint/panicguard linter).

### `net/http/` - HTTP Helpers

**Purpose**: Fiber HTTP utilities for request/response handling.

**Key Utilities**:
- `LocalUUID()` - Extract UUID from request locals
- `Payload()` - Parse request body
- `WithError()` - Error response helpers
- `WithSuccess()` - Success response helpers

**Documentation**: `docs/agents/pkg-libraries/http.md`

### `shell/` - Shell Utilities

**Purpose**: Shell scripting utilities and ASCII art.

**Key Files**:
- `colors.sh` - ANSI color codes for terminal output
- `ascii.sh` - ASCII art functions
- `logo.txt` - Midaz ASCII logo

**Usage**: Used by Makefile targets and scripts for enhanced terminal output.

### `transaction/` - Transaction Models

**Purpose**: Core transaction models, validations, and business rules.

**Key Features**:
- Transaction validation logic
- Operation validation
- Balance calculation utilities
- DSL integration

**Key Files**:
- `transaction.go` - Transaction model
- `validations.go` - Validation functions

**Usage**: Used by transaction service and Gold DSL parser.

### `utils/` - General Utilities

**Purpose**: General-purpose utility functions used across components.

**Key Utilities**:
- `cache.go` - Caching helpers
- `jitter.go` - Exponential backoff with jitter
- `metrics.go` - Metrics helpers
- `ptr.go` - Pointer utilities
- `utils.go` - Miscellaneous utilities

## Internal Architecture Pattern

All Go service components (onboarding, transaction, crm) follow a consistent internal structure based on Hexagonal Architecture:

```
{component}/internal/
├── adapters/             # External integrations (ports & adapters)
│   ├── grpc/             # gRPC adapters
│   │   ├── in/           # Inbound gRPC service implementations
│   │   └── out/          # Outbound gRPC client calls
│   ├── http/             # HTTP adapters
│   │   ├── in/           # Inbound HTTP handlers (Fiber)
│   │   └── out/          # Outbound HTTP client calls
│   ├── mongodb/          # MongoDB repositories
│   ├── postgres/         # PostgreSQL repositories (by entity)
│   │   └── {entity}/     # Entity-specific repository
│   ├── rabbitmq/         # RabbitMQ producer/consumer (transaction only)
│   └── redis/            # Redis cache
├── bootstrap/            # Application initialization
│   ├── app.go            # Main application setup
│   ├── config.go         # Configuration loading
│   ├── dependencies.go   # Dependency injection
│   └── services.go       # Service initialization
└── services/             # Business logic (hexagon core)
    ├── command/          # Write operations (CQRS)
    │   └── {operation}.go
    └── query/            # Read operations (CQRS)
        └── {operation}.go
```

**Key Architectural Decisions**:

1. **Hexagonal Architecture**: Business logic (`services/`) is independent of external integrations (`adapters/`).
2. **CQRS Separation**: Commands (writes) and Queries (reads) are strictly separated in onboarding and transaction services.
3. **CRM Exception**: CRM service uses flat `services/` structure (no command/query separation) due to simpler domain.
4. **Repository Pattern**: Each entity has its own repository with interface, implementation, and mock.
5. **Dependency Injection**: All dependencies are injected via `bootstrap/` package.
6. **Adapter Direction**: `in/` for inbound requests, `out/` for outbound calls.

## Testing Structure (`./tests`)

### Chaos Tests (`./tests/chaos`)

**Purpose**: Chaos engineering tests validating system behavior under infrastructure failures.

**Scenarios**:
- Database restarts (PostgreSQL, MongoDB)
- Cache eviction (Redis/Valkey)
- Message queue disruptions (RabbitMQ)
- Network partitions
- Resource pressure
- Rolling restarts under load
- Replica flapping

**Key Files**: `chaos_test.go`, `postgres_restart_writes_test.go`, `mongodb_restart_graceful_test.go`

### End-to-End Tests (`./tests/e2e`)

**Purpose**: E2E tests using Apidog CLI for full API workflow validation.

**Configuration**: `local.apidog-cli.json` - Apidog CLI test suite

**Execution**: `make test-e2e`

### Fixtures (`./tests/fixtures`)

**Purpose**: Test data fixtures organized by component.

**Structure**:
- `common/` - Shared fixtures
- `onboarding/` - Onboarding entity fixtures
- `transaction/` - Transaction fixtures

### Fuzzy Tests (`./tests/fuzzy`)

**Purpose**: Fuzz testing for robustness against malformed inputs.

**Scenarios**:
- HTTP payload fuzzing
- Header fuzzing
- Protocol timing fuzzing
- Structural fuzzing

**Key Files**: `fuzz_test.go`, `http_payload_fuzz_test.go`, `headers_fuzz_test.go`

### Helpers (`./tests/helpers`)

**Purpose**: Shared test utilities and helper functions.

**Key Helpers**:
- `setup.go` - Test environment setup
- `http.go` - HTTP test utilities
- `docker.go` - Docker container management
- `payloads.go` - Test payload generation
- `balances.go` - Balance verification utilities
- `auth.go` - Authentication helpers

### Integration Tests (`./tests/integration`)

**Purpose**: Integration tests validating API behavior with real database and infrastructure.

**Coverage**:
- Core entity flows (organizations, ledgers, accounts, transactions)
- Balance consistency validation
- Cache consistency
- Event-driven async flows
- Pagination and filtering
- Idempotency
- Concurrency handling
- Negative test cases

**Requirements**: All services must be running (`make up`).

**Execution**: `make test-integration`

**Key Files**: `core_flow_test.go`, `transaction_lifecycle_flow_test.go`, `balance_consistency_test.go`

### Property Tests (`./tests/property`)

**Purpose**: Property-based tests validating mathematical invariants.

**Properties Tested**:
- Balance consistency (sum of operations = balance)
- Conservation of value (debits = credits)
- Non-negative balances
- Operation sum integrity

**Key Files**: `balance_consistency_test.go`, `conservation_test.go`, `nonnegative_test.go`

## Documentation (`./docs`)

### Agent Documentation (`./docs/agents`)

**Purpose**: Technical documentation for AI agents and developers.

**Key Documents**:
- `architecture.md` - Hexagonal architecture, CQRS, system design
- `testing.md` - Testing patterns and strategies
- `database.md` - Schema design, migrations, PostgreSQL/MongoDB patterns
- `api-design.md` - REST/gRPC API conventions, Swagger annotations
- `error-handling.md` - Typed errors, error wrapping, logging
- `observability.md` - Metrics, traces, structured logging
- `transaction-dsl.md` - Gold language documentation
- `concurrency.md` - Goroutine patterns, mruntime usage
- `libcommons.md` - lib-commons library usage (observability, tracking)

**Progressive Disclosure**: Developers should consult these documents when working on specific features.

### Package Library Documentation (`./docs/agents/pkg-libraries/`)

**Purpose**: Detailed documentation for internal pkg/ libraries.

**Expected Documents**:
- `assert.md` - Domain invariant assertions
- `errors.md` - Typed business errors
- `mruntime.md` - Safe goroutine handling
- `http.md` - Fiber HTTP utilities

## Build and Automation (`./mk`, `./scripts`)

### Makefile Includes (`./mk`)

**Purpose**: Modular Makefile includes for build automation.

**Key Files**:
- `tests.mk` - Test-related targets (test, test-integration, test-e2e, test-chaos)

**Usage**: Included by root `Makefile` for organized build logic.

### Scripts (`./scripts`)

**Purpose**: Build automation and utility scripts.

**Key Scripts**:
- `setup-deps.sh` - Install development dependencies
- `setup-git-hooks.sh` - Install Git hooks for pre-commit checks
- `generate-docs.sh` - Generate Swagger/OpenAPI documentation
- `coverage.sh` - Generate test coverage reports
- `check-envs.sh` - Validate environment configuration
- `check-tests.sh` - Validate test suite completeness
- `clean-artifacts.sh` - Clean build artifacts
- `ensure-crypto-keys.sh` - Generate cryptographic keys for testing

**Postman Collection Generation** (`scripts/postman-coll-generation/`):
- `convert-openapi.js` - Convert OpenAPI specs to Postman collections
- `enhance-tests.js` - Add test assertions to Postman collections
- `sync-postman.sh` - Sync collections to Postman cloud

## API Documentation (`./postman`)

**Purpose**: Postman collections for API testing and documentation.

**Usage**: Import collections into Postman for interactive API exploration and testing.

**Generation**: Automatically generated from OpenAPI specs via `scripts/postman-coll-generation/`.

## Configuration and Environment

**Environment Files**:
- `.env.example` files in each component (copy to `.env` via `make set-env`)
- Components have independent `.env` configurations

**Docker Compose**:
- `components/infra/docker-compose.yml` - Infrastructure services
- Each component has its own `docker-compose.yml` for standalone execution

**Makefile Targets**:
- `make dev-setup` - Complete development environment setup
- `make up` - Start all services
- `make down` - Stop all services
- `make logs` - View service logs

## Key Project Conventions

### File Naming

- Go files: `kebab-case.go` (e.g., `create-account.go`, `get-all-accounts.go`)
- Test files: `{name}_test.go` in the same directory as source
- Config files: `kebab-case.yaml` or `camelCase.json` depending on tooling

### Code Organization

- **One use case per file**: Each command/query operation is a separate file
- **Handler methods**: Named `{Operation}{Entity}` (e.g., `CreateAccount`, `GetAccountByID`)
- **Repository pattern**: Interface + implementation + mock
- **Swagger annotations**: Required for all HTTP handlers

### Error Handling

- **Wrap errors**: Use `fmt.Errorf("context: %w", err)` for error wrapping
- **Typed errors**: Use `pkg/errors.go` types (`EntityNotFoundError`, `ValidationError`)
- **Log at boundary**: Log errors in HTTP handlers, not in use cases
- **No panic in production**: Return errors, don't panic (except initialization)

### Concurrency

- **SafeGo mandatory**: Use `mruntime.SafeGoWithContextAndComponent()` for all goroutines
- **No bare `go`**: Enforced by `mlint/panicguard` linter
- **Context propagation**: Always pass `context.Context` through call chain

### Observability

- **OpenTelemetry tracing**: Create spans with `tracer.Start(ctx, "handler.{operation}")`
- **Structured logging**: Use `logger.Infof()`, `logger.Errorf()` with key-value pairs
- **Metrics**: Record key metrics (panics, API calls, database operations)

## Related Resources

- **Project Overview**: `README.md` at repository root
- **Coding Standards**: `CLAUDE.md` - AI agent instructions and coding conventions
- **API Documentation**: OpenAPI specs in `components/{component}/api/openapi.yaml`
- **Contributing Guide**: `CONTRIBUTING.md` (if exists)
- **License**: `LICENSE` (if exists)

---

**Last Updated**: 2025-12-14 (Generated from codebase analysis)

**Maintained By**: Lerian Studio

**For Questions**: Consult `CLAUDE.md` for progressive disclosure and refer to `docs/agents/` for detailed technical documentation.
