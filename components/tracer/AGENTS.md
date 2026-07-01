# AGENTS.md — AI Agent Quick Reference

Universal entry point for any AI coding agent working on the Tracer codebase.

## Project Identity

**Tracer** is a real-time transaction validation and fraud prevention API built by Lerian Studio. It provides instant ALLOW/DENY/REVIEW decisions for financial transactions using CEL rule expressions, multi-scope spending limits, and an immutable audit trail with hash chain verification for SOX/GLBA compliance.

- **Language**: Go 1.26.4 (single root `go.mod` for the midaz monorepo, module `github.com/LerianStudio/midaz/v4` — tracer has no own go.mod)
- **Architecture**: Hexagonal Architecture (Ports & Adapters) + CQRS
- **Database**: PostgreSQL 17
- **Rule Engine**: Google CEL (cel-go v0.28.1) with in-memory cache
- **Auth**: lib-auth/v2 (v2.8.0) (API Key + Access Manager plugin)
- **License**: Elastic License 2.0

## Quick Start

```bash
cp .env.example .env   # Create environment file
make up                # Start Tracer via Docker Compose (joins the shared infra-network; PostgreSQL comes from components/infra)
make test              # Run all tests
make lint              # golangci-lint v2
```

Health: `GET http://localhost:4020/health`

## Architecture Overview

Single service with four bounded contexts, all under `internal/`:

| Context | Role |
|---------|------|
| **Validation** | Orchestrate validation requests, coordinate rules + limits, record audit |
| **Rules** | Manage rule lifecycle (DRAFT→ACTIVE→INACTIVE→DELETED), compile/evaluate CEL |
| **Limits** | Spending limits (DAILY/MONTHLY/WEEKLY/PER_TRANSACTION/CUSTOM), usage counters |
| **Audit** | Immutable event log, hash chain verification, SOX/GLBA compliance |

### Layer Structure

```
internal/
├── bootstrap/         # Composition root: config, DI, server, workers
├── adapters/
│   ├── http/in/       # Fiber handlers, routes, middleware, validation
│   ├── postgres/      # Repository implementations (squirrel SQL builder)
│   └── cel/           # CEL expression engine adapter
├── services/          # Business logic
│   ├── command/       # Write operations (create, update, activate, deactivate, draft, delete)
│   ├── query/         # Read operations (get, list, evaluate, check limits, verify audit)
│   ├── cache/         # In-memory rule cache with warmup + background sync
│   └── workers/       # RuleSyncWorker, UsageCleanupWorker
├── testutil/          # Shared test helpers
pkg/
├── model/             # Domain entities (Rule, Limit, Validation, Scope, AuditEvent)
├── constant/          # pagination/app constants (error codes live in root pkg/constant)
├── clock/             # Clock interface (Real + Fixed for MOCK_TIME)
└── resilience/        # Circuit breaker (sony/gobreaker wrapper)
```

## Essential Commands

| Command | Purpose |
|---------|---------|
| `make build` | Build binary to .bin/tracer |
| `make run` | Run locally with .env config |
| `make test` | All tests |
| `make test-unit` | Unit tests with race detector |
| `make test-integration` | Integration tests (testcontainers, -p=1) |
| `make test-e2e` | E2E BDD tests (Godog/Gherkin) |
| `make lint` | golangci-lint v2 with auto-fix |
| `make sec` | gosec + govulncheck |
| `make generate` | go generate (mocks) |
| `make generate-docs` (repo root) | Regenerate Swagger docs for all three REST services (ledger, tracer, reporter) |
| `make migrate` | Apply database migrations |
| `make up` / `make down` | Docker Compose lifecycle |

## Code Conventions

### Entity Constructors
```go
func NewRule(name, expression string, action Decision) (*Rule, error)
```
Validate all invariants. Return `(*T, error)`. Never panic. Defensive copies for slices.

### Service Methods
```go
func (s *ActivateRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (*model.Rule, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "service.rule.activate")
    defer span.End()
    logger = logging.WithTrace(ctx, logger)
    // ...
}
```
Always start with tracking + span. Enrich logger with trace context.

### Repository Pattern
- Interfaces defined where used (command/ and query/ packages)
- Separate PostgreSQL model structs from domain entities
- Squirrel SQL builder with `squirrel.Dollar` placeholder format
- Cursor-based pagination (no offset)

### Error Handling
- Sentinels live in the canonical registry `github.com/LerianStudio/midaz/v4/pkg/constant` (e.g. `constant.ErrRuleNotFound`); render via `pkg.ValidateBusinessError(constant.ErrX, constant.EntityY)` + `http.WithError`. The former `TRC-xxxx` fork was folded into the numeric registry.
- Wrapping: `fmt.Errorf("context: %w", err)` — always `%w`, never `%v`
- Business: `libOtel.HandleSpanBusinessErrorEvent(span, "msg", err)` — span stays OK
- Technical: `libOtel.HandleSpanError(span, "msg", err)` — span marked ERROR

### HTTP Responses
- Always use lib-commons wrappers: `libHTTP.OK()`, `libHTTP.Created()`, `libHTTP.WithError()`
- Never use direct Fiber responses (`c.JSON()`, `c.Status().JSON()`)

## Testing Requirements

| Tag | Scope | Run with |
|-----|-------|----------|
| (none) | Unit tests | `make test-unit` |
| `//go:build integration` | Testcontainers | `make test-integration` |
| `//go:build e2e` | Full stack BDD | `make test-e2e` |

- TDD required: write test first, then implement
- Table-driven tests with gomock (never manual mocks)
- Deterministic data: `testutil.FixedTime()`, `testutil.MustDeterministicUUID(seed)`
- Never use `uuid.New()` or `time.Now()` in tests
- `require.Len` before indexing slices
- No `defer ctrl.Finish()` — go.uber.org/mock auto-registers cleanup

## PR Standards

- Conventional commit format in PR titles
- Run `make lint && make test-unit && make sec` before pushing
- Run `make generate-docs` from the repo root if the API changed (regenerates ledger, tracer, and reporter together)
- All code, comments, and docs in English

## Key Files to Read

| Priority | File | Why |
|----------|------|-----|
| 1 | `AGENTS.md` (this file) | Quick orientation |
| 2 | `CLAUDE.md` | Deep patterns, interfaces, commands, debugging |
| 3 | `../../docs/PROJECT_RULES.md` | Monorepo-wide architectural rules and testing standards |
| 3 | `../../docs/tracer/INVARIANTS.md` | Tracer-specific invariants (CEL, hash-chained audit, migration renumbering, latency budget) |
| 4 | `.env.example` | All configuration variables |
| 5 | `.golangci.yml` | Linter rules |
| 6 | `internal/bootstrap/config.go` | Composition root — how everything is wired |
| 7 | `api/swagger.json` | Full API specification |

## What NOT to Do

1. **Never use `%v` for error wrapping** — always `%w` (enforced by errorlint)
2. **Never use `time.Now()` without `.UTC()`**
3. **Never panic in production code** — return errors
4. **Never use direct Fiber responses** — use `libHTTP.*` wrappers
5. **Never put business logic in repositories** — repositories are data access only
6. **Never use `uuid.New()` or `time.Now()` in tests** — use testutil deterministic helpers
7. **Never use `float64` for monetary amounts** — use `shopspring/decimal`
8. **Never update or delete audit log records** — append-only
9. **Never hardcode configuration** — use environment variables
10. **Never reference task/ticket IDs in code** — code must be self-explanatory
