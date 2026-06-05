# AGENTS.md — Midaz Quick-Start for AI Agents

## What Is This?

Midaz is an **open-source double-entry ledger** written in Go. It provides HTTP APIs for managing organizations, ledgers, accounts, and financial transactions with full double-entry accounting.

## Quick Facts

| Aspect | Detail |
|--------|--------|
| Language | Go 1.26.3 (toolchain go1.26.4) |
| Module | `github.com/LerianStudio/midaz/v3` (single root `go.mod`, no `go.work`) |
| License | Elastic License 2.0 |
| Architecture | Hexagonal + CQRS |
| HTTP Framework | Fiber v2 |
| Databases | PostgreSQL 17, MongoDB, RabbitMQ 4.1, Valkey |
| lib-commons | `github.com/LerianStudio/lib-commons/v5` v5.4.1 (+ `lib-observability` v1.0.1) |
| Deploy units | Ledger+CRM+Fees (:3002), Tracer (:4020), Reporter Manager (:4005), Reporter Worker (:4006), Infra (Docker Compose) |

> **CRM and fees are not deploy units.** CRM is a package tree at `components/crm`, imported by
> the ledger binary (holder/instrument routes served on :3002). Fees are embedded in the ledger
> binary (`components/ledger/pkg/fee`, `components/ledger/internal/services/fees`, fee seam in
> `transaction_create.go`). Tracer and the two reporter components are separate Go services.

## Get Running

```bash
make set-env     # Create .env files
make up          # Start everything (infra → ledger → tracer → reporter-manager → reporter-worker)
make test-unit   # Run unit tests
make lint        # Lint all code
```

## Project Structure (What Goes Where)

```
components/ledger/internal/
  adapters/http/in/   → HTTP handlers (one per entity)
  adapters/postgres/  → PostgreSQL repositories
  adapters/mongodb/   → MongoDB metadata repos
  adapters/redis/     → Cache repos
  adapters/rabbitmq/  → Message queue adapters
  bootstrap/          → Config, DI, server lifecycle
  services/command/   → Write use cases (one file per operation)
  services/query/     → Read use cases (one file per operation)

components/crm/         → CRM package tree (holders/instruments), imported by ledger — NOT a deploy unit
  adapters/http/in/     → Holder/instrument handlers + routes (midaz namespace)
  adapters/mongodb/     → CRM persistence
  services/             → Holder/instrument use cases

components/ledger/pkg/  → Embedded fees: fee/ (engine), feeshared/ (plugin-fees types)
  (fee use cases at components/ledger/internal/services/fees; fee seam in transaction_create.go)

components/{tracer,reporter-manager,reporter-worker}/  → Separate Go service deploy units

pkg/
  mmodel/             → Domain models (Organization, Account, Transaction, etc.)
  constant/errors.go  → Error codes (ledger 0001–0175, 16 CRM-00xx)
  errors.go           → Typed error structs
  gold/               → Transaction DSL parser (ANTLR4)
  mtransaction/       → Transaction processing utilities (formerly pkg/transaction)
  reporter/           → Reporter shared library (used by both reporter deploy units)
  net/http/           → Middleware, pagination, route helpers
```

## Key Conventions

1. **Error handling**: Business errors return directly; technical errors wrap with `%w`
2. **Validation order**: Normalize → Defaults → Validate → Execute
3. **Metadata**: Flat key-value only (no nesting), key max 100, value max 2000
4. **File naming**: `snake_case.go`, one handler or operation per file
5. **Imports**: stdlib → external → internal (blank-line separated)
6. **Context**: Always first param; check `ctx.Err()` before expensive work
7. **IDs**: `uuid.UUID` type, not strings
8. **HTTP methods**: Use `http.MethodGet` constants, never string literals

## Key Files to Read First

| File | Why |
|------|-----|
| `components/ledger/internal/bootstrap/config.go` | Composition root, all env vars, init sequence |
| `components/ledger/internal/adapters/http/in/routes.go` | All API routes registered here |
| `pkg/mmodel/account.go` | Account model (representative of all models) |
| `pkg/constant/errors.go` | All error codes |
| `pkg/errors.go` | Error types + ValidateBusinessError factory |
| `components/ledger/.env.example` | All environment variables |
| `docs/PROJECT_RULES.md` | 1159 lines of coding standards (DO NOT overwrite) |

## What NOT To Do

- Do NOT overwrite `docs/PROJECT_RULES.md`
- Do NOT use `interface{}` — use `any`
- Do NOT panic — return errors
- Do NOT put domain logic in handlers or repositories
- Do NOT nest metadata values
- Do NOT use `time.Now()` in tests

## Deeper References

- **[CLAUDE.md](CLAUDE.md)** — Deep technical reference (architecture, bootstrap, multi-tenancy, transaction processing)
- **[llms-full.txt](llms-full.txt)** — Complete reference with all env vars, API endpoints, error codes, models
- **[llms.txt](llms.txt)** — Concise overview following llmstxt.org spec
- **[docs/PROJECT_RULES.md](docs/PROJECT_RULES.md)** — Coding standards and conventions (1159 lines)
- **[docs/auth/RBAC-NAMESPACES.md](docs/auth/RBAC-NAMESPACES.md)** — The four authz namespaces in the unified binary (R9)
- **[docs/api/SCOPING.md](docs/api/SCOPING.md)** — Path vs `X-Organization-Id` header scoping (R22)
