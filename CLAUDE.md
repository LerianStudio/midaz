# Midaz

Enterprise-grade open-source ledger system for financial infrastructure implementing double-entry accounting with complex n:n transactions.

## Issue Tracking

This project uses **bd (beads)** for issue tracking.
Run `bd prime` for workflow context, or install hooks (`bd hooks install`) for auto-injection.

**Quick reference:**
- `bd ready` - Find unblocked work
- `bd create "Title" --type task --priority 2` - Create issue
- `bd close <id>` - Complete work
- `bd sync` - Sync with git (run at session end)

For full workflow details: `bd prime`

## Project Overview (WHY)

- **Purpose**: Core banking ledger infrastructure for fintech and banking solutions with double-entry accounting, multi-currency/multi-asset support
- **Domain**: Financial services - handles Organizations, Ledgers, Assets, Portfolios, Segments, Accounts, Transactions, and Balances
- **Users**: Fintech companies, banking systems, financial service providers integrating ledger capabilities

## Tech Stack (WHAT)

| Layer | Technology |
|-------|------------|
| Language | Go 1.24.2+ (toolchain 1.25.1) |
| Framework | GoFiber v2, gRPC |
| Database | PostgreSQL 17 (primary + replica), MongoDB 8 (metadata) |
| Cache | Valkey 8 (Redis-compatible fork) |
| Queue | RabbitMQ 4.1.3 |
| Observability | Grafana + OpenTelemetry (LGTM stack) |
| Testing | testify, uber/mock, sqlmock |
| API Docs | Swagger/OpenAPI (swaggo) |
| DSL | ANTLR4 (Gold transaction language) |

## Project Structure (WHAT)

```
midaz/
├── components/           # Service applications
│   ├── onboarding/       # Entity management (orgs, ledgers, accounts)
│   ├── transaction/      # Transaction processing, balance calculations
│   ├── crm/              # Customer relationship management
│   ├── console/          # Frontend (Next.js)
│   └── infra/            # Docker infrastructure definitions
├── pkg/                  # Shared libraries
│   ├── assert/           # Domain invariant assertions (CRITICAL)
│   ├── constant/         # Error codes, constants
│   ├── gold/             # Transaction DSL parser (ANTLR)
│   ├── mgrpc/            # gRPC protobuf definitions
│   ├── mlint/            # Custom golangci-lint plugins
│   ├── mruntime/         # Safe goroutine handling (REQUIRED)
│   └── mmodel/           # Domain models
├── tests/                # Test suites
│   ├── integration/      # API integration tests
│   ├── chaos/            # Chaos engineering tests
│   ├── fuzzy/            # Fuzz testing
│   └── property/         # Property-based tests
└── mk/                   # Makefile includes
```

## Essential Commands (HOW)

```bash
# Development Setup
make dev-setup                    # Full dev environment + git hooks
make set-env                      # Copy .env.example to all components

# Running Services
make up                           # Start all services (infra + components)
make down                         # Stop all services
make logs                         # View logs

# Testing
make test                         # All unit tests
make test-integration             # Integration tests (requires services)
make test-e2e                     # E2E tests with Apidog CLI

# Code Quality
make lint                         # Custom linter + golangci-lint
make format                       # Format all code
make sec                          # Security checks (gosec)

# Documentation
make generate-docs                # Generate Swagger docs

# Verification (run before completing tasks)
make lint && make test            # Lint + unit tests (minimum check)
```

## Architecture Patterns

- **Hexagonal Architecture (Ports & Adapters)**: `internal/adapters/{http,grpc,postgres,mongodb,rabbitmq}` separate from business logic in `internal/services`
- **CQRS Pattern**: Commands (`services/command/`) for writes, Queries (`services/query/`) for reads - strict separation
- **Repository Pattern**: Interfaces in domain, implementations in `adapters/postgres/*.postgresql.go`, mocks for testing
- **Safe Concurrency**: Always use `mruntime.SafeGoWithContextAndComponent()`, never bare `go` keyword (enforced by custom linter)

## Critical Rules

1. **Never use panic()**: Production code must return errors, not panic. Only initialization-time panics allowed (repository constructors).
2. **Always wrap errors**: Use `fmt.Errorf("context: %w", err)` for error wrapping with context.
3. **Typed business errors**: Use `pkg/errors.go` types (`EntityNotFoundError`, `ValidationError`, etc.) validated via `pkg.ValidateBusinessError()`.
4. **Safe goroutines only**: Use `mruntime.SafeGoWithContextAndComponent()` with panic recovery policy - bare `go` is forbidden (caught by panicguard linter).
5. **Domain invariants**: Use `assert.That()`, `assert.NotNil()`, `assert.NotEmpty()` from `pkg/assert` for validation - these include context and truncate values.
6. **Context propagation**: Always pass `context.Context` through call chain - required for tracing, logging, cancellation.
7. **OpenTelemetry tracing**: Create spans with `tracer.Start(ctx, "handler.{operation}")` for all significant operations.

## Progressive Disclosure

**Before starting work**, check if any of these are relevant to your task:

| Document | When to Read |
|----------|--------------|
| `docs/agents/architecture.md` | Understanding hexagonal architecture, CQRS, adding features |
| `docs/agents/testing.md` | Writing tests, understanding test patterns (unit/integration/chaos/fuzz) |
| `docs/agents/database.md` | Schema changes, migrations, PostgreSQL/MongoDB patterns |
| `docs/agents/api-design.md` | Creating/modifying REST/gRPC endpoints, Swagger annotations |
| `docs/agents/error-handling.md` | Working with typed errors, error wrapping, logging patterns |
| `docs/agents/observability.md` | Adding metrics, traces, structured logging |
| `docs/agents/transaction-dsl.md` | Working with Gold language, ANTLR grammar |
| `docs/agents/concurrency.md` | Goroutines, mruntime patterns, panic recovery |
| `docs/agents/pkg-libraries/` | **Index** for all internal pkg/ libraries (assert, errors, mruntime, http, utils, models) |
| `docs/agents/pkg-libraries/assert.md` | Domain invariant assertions (preconditions, postconditions, unreachable code) |
| `docs/agents/pkg-libraries/errors.md` | Typed business errors (ValidationError, EntityNotFoundError, etc.) |
| `docs/agents/pkg-libraries/mruntime.md` | Safe goroutine handling with panic recovery policies |
| `docs/agents/pkg-libraries/http.md` | Fiber HTTP utilities (LocalUUID, Payload, response helpers, error mapping) |
| `docs/agents/libcommons.md` | Using lib-commons (observability, tracking, PostgreSQL, OpenTelemetry) |

**Note**: These files should be created as detailed guides - this CLAUDE.md keeps only universal instructions.

## Code Conventions

- **File naming**: kebab-case (e.g., `create-account.go`, `get-all-accounts.go`)
- **Test files**: `{name}_test.go` in same directory as source
- **Handlers**: `{Entity}Handler` struct with methods like `Create{Entity}`, `Get{Entity}ByID`
- **Errors**: Wrap with context using `fmt.Errorf()`, log at boundary (handlers), return from use cases
- **Logging**: Structured logging with `logger.Infof()`, `logger.Errorf()` - always include context
- **Comments**: Only comment WHY, not WHAT - code should be self-documenting

## Verification Checklist

Before completing any task, run:
```bash
make lint && make test
```

This runs: golangci-lint (with custom panicguard plugin), unit tests with coverage, formatting checks.

For comprehensive verification before PR:
```bash
make lint && make test && make test-integration
```

## Key Patterns & Examples

### Handler Pattern
- Location: `components/onboarding/internal/adapters/http/in/account.go:1`
- Swagger annotations for API docs
- Extract context: `libCommons.NewTrackingFromContext(ctx)`
- Start tracing span: `tracer.Start(ctx, "handler.{operation}")`
- Error handling: `http.WithError(c, err)`

### UseCase Pattern
- Location: `components/onboarding/internal/services/command/create-account.go:1`
- Command pattern in `services/command/`
- Query pattern in `services/query/`
- Always validate with `assert` package
- Return typed errors from `pkg/errors.go`

### Repository Pattern
- Interface: `components/onboarding/internal/adapters/postgres/account/account.go`
- Implementation: `components/onboarding/internal/adapters/postgres/account/account.postgresql.go`
- Mock: `components/onboarding/internal/adapters/postgres/account/account.postgresql_mock.go`

### Safe Goroutine Pattern
- Location: `pkg/mruntime/goroutine.go:1`
- Always: `mruntime.SafeGoWithContextAndComponent(ctx, component, policy, fn)`
- Never: bare `go` keyword (caught by panicguard linter)
- Policies: `mruntime.KeepRunning` or `mruntime.CrashProcess`

### Domain Assertions
- Location: `pkg/assert/assert.go:1`
- Use: `assert.That(condition, "message", key, value)`
- Use: `assert.NotNil(value, "message")`
- Use: `assert.NotEmpty(value, "message")`
- Values automatically truncated to 200 chars for logs

## Testing Strategy

- **Unit tests**: `*_test.go` next to source, use `t.Parallel()`
- **Integration tests**: `tests/integration/` (requires running services)
- **Chaos tests**: `tests/chaos/` (infrastructure failure scenarios)
- **Fuzz tests**: `tests/fuzzy/` (robustness testing)
- **Property tests**: `tests/property/` (property-based testing)
- **Mocking**: Use `uber/mock` with `gomock.Controller`
- **Table-driven**: Preferred pattern with named test cases

## Commit Message Format

Follow Conventional Commits:
```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`

Example: `feat(onboarding): add portfolio balance calculation`
