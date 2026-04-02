# AGENTS.md — Midaz Quick-Start for AI Agents

## What Is This?

Midaz is an **open-source double-entry ledger** written in Go. It provides HTTP APIs for managing organizations, ledgers, accounts, and financial transactions with full double-entry accounting.

## Quick Facts

| Aspect | Detail |
|--------|--------|
| Language | Go 1.25+ |
| Module | `github.com/LerianStudio/midaz/v3` |
| License | Elastic License 2.0 |
| Architecture | Hexagonal + CQRS |
| HTTP Framework | Fiber v2 |
| Databases | PostgreSQL 17, MongoDB 8, RabbitMQ 4.1, Valkey 8 |
| Components | Ledger (:3002), CRM (:4003), Infra (Docker Compose) |

## Get Running

```bash
make set-env     # Create .env files
make up          # Start everything (infra → ledger → CRM)
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

pkg/
  mmodel/             → Domain models (Organization, Account, Transaction, etc.)
  constant/errors.go  → Error codes (0001–0161)
  errors.go           → Typed error structs
  gold/               → Transaction DSL parser (ANTLR4)
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
| `docs/PROJECT_RULES.md` | 1130 lines of coding standards (DO NOT overwrite) |

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
- **[docs/PROJECT_RULES.md](docs/PROJECT_RULES.md)** — Coding standards and conventions (1130 lines)
