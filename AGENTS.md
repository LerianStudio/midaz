# AGENTS.md — Midaz Quick-Start for AI Agents

## What Is This?

Midaz is a **source-available core banking platform** written in Go, built around a double-entry ledger. One Go monorepo ships three deploy surfaces: the unified ledger HTTP API (onboarding + transaction + CRM + fees), the Tracer real-time transaction-validation / fraud-prevention API, and the Infra backing stack. Licensed under the Elastic License 2.0 (source-available, not open-source).

## Quick Facts

| Aspect | Detail |
|--------|--------|
| Language | Go 1.26.4 |
| Module | `github.com/LerianStudio/midaz/v4` (single root `go.mod`, no `go.work`) |
| License | Elastic License 2.0 |
| Architecture | Hexagonal + CQRS |
| HTTP Framework | Huma v2 (OAS 3.1) over Fiber v2 — Fiber is the runtime router/auth chain; Huma generates the API contract and validates requests |
| Databases | PostgreSQL 17, MongoDB (also holds CRM keysets/registry in envelope mode), RabbitMQ 4.1, Valkey |
| lib-commons | `github.com/LerianStudio/lib-commons/v5` v5.8.0 (+ `lib-observability` v1.1.0) |
| KMS / crypto | `danielgtaylor/huma/v2` v2.38.0, `hashicorp/vault/api` (CRM envelope KEK), `tink-crypto/tink-go/v2` (per-org DEKs) |
| Deploy surfaces | Ledger+CRM+Fees (:3002), Tracer (:4020), Infra (Docker Compose) |

> **CRM and fees are not deploy units.** CRM is a package tree at `components/ledger/internal/crm`, imported by
> the ledger binary (holder/instrument routes served on :3002). Fees are embedded in the ledger
> binary (`components/ledger/pkg/fee`, `components/ledger/internal/services/fees`, fee seam in
> `transaction_create.go`). Tracer is a separate co-located Go service.

## Get Running

```bash
make set-env     # Create .env files
make up          # Start everything (infra → ledger → tracer)
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

components/ledger/internal/crm/         → CRM package tree (holders/instruments), imported by ledger — NOT a deploy unit
  adapters/mongodb/               → CRM persistence (only adapter; no http/ or api/ tree here)
  adapters/mongodb/encryption/    → Keyset + registry + audit-event Mongo repos (envelope mode)
  services/                       → Holder/instrument use cases
  services/encryption/            → FieldEncryptor seam + EncryptionService (encrypt/decrypt PII, search tokens)
  (CRM HTTP handlers + routes live in components/ledger/internal/adapters/http/in/:
   crm_routes.go, composition_routes.go, holder.go, holder_accounts.go, instrument.go — midaz namespace.
   Encryption/protection routes (envelope mode only, midaz ns): POST .../encryption/provision,
   GET .../encryption/status, GET .../protection/audit)

pkg/crypto/            → CRM crypto primitives: kms/vault/ (Vault Transit KEK), tink/ (Tink DEKs), mode + resolver

components/ledger/pkg/  → Embedded fees: fee/ (engine), feeshared/ (plugin-fees types)
  (fee use cases at components/ledger/internal/services/fees; fee seam in transaction_create.go)

components/tracer/     → Separate Go service deploy unit

pkg/
  mmodel/             → Domain models (Organization, Account, Transaction, etc.)
  constant/errors.go  → Error codes (ledger numeric sentinels (0001+), 28 CRM-00xx (CRM-0006..CRM-0041))
  errors.go           → Typed error structs
  gold/               → Transaction DSL parser (ANTLR4)
  mtransaction/       → Transaction processing utilities (formerly pkg/transaction)
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

## Per-Call Control Skips

Callers can opt out of individual controls per request under a **two-key gate**: a skip is
honored only when the request asks (a `skip` object on the create body) AND the ledger opts
in via `settings.Overrides.Allow{Fee,Tracer,Holder}Skip` (all default `false`). An
unauthorized skip returns **HTTP 422** (`ErrSkipNotPermitted`, `0490`). Resolver:
`pkg/skip.ResolveSkipFor`. Controls: `fees`/`tracer` on transaction create, `holder` on
account create. Honored skips persist to audit columns (`transaction.fees_skipped`,
`transaction.tracer_skipped`, `account.holder_check_skipped`). Invariant: an honored skip
adds **zero** downstream work (short-circuits before the control's lookup); reverts always
re-run the tracer; DSL carries no skip; idempotency replay returns the first outcome.

## CRM Field Encryption (KMS)

CRM encrypts holder/instrument PII at rest. Mode is chosen by `KMS_VENDOR`: unset/`none` → **legacy**
(lib-commons symmetric crypto, no KMS); `hashicorp-vault` → **envelope** (Vault Transit KEK wraps per-org
Tink DEKs). The seam is the `FieldEncryptor` interface (`internal/crm/services/encryption`), which the
holder/instrument Mongo adapters call to encrypt/decrypt fields and mint deterministic HMAC search tokens
for equality lookups over ciphertext. One **shared, mode-derived** Transit engine (`transit-st`/`transit-mt`)
holds all KEKs — tenant isolation lives in the key **name** (`{tenant}_org-{id}`), not per-tenant mounts.
Envelope routes (provision/status/audit) exist only in envelope mode. Key rotation is scaffolded, not yet
active. New env: `KMS_VENDOR`, `KMS_VAULT_ADDR`, `KMS_VAULT_ROLE_ID`, `KMS_VAULT_SECRET_ID`,
`KMS_VAULT_AUTH_METHOD` (`approle`|`token`), `DEPLOYMENT_MODE` (gates the dev root token to `local`). Full
design: [docs/architecture/crm-field-encryption.md](docs/architecture/crm-field-encryption.md).

## Key Files to Read First

| File | Why |
|------|-----|
| `components/ledger/internal/bootstrap/config.go` | Composition root, all env vars, init sequence |
| `components/ledger/internal/adapters/http/in/routes.go` | All API routes registered here |
| `pkg/mmodel/account.go` | Account model (representative of all models) |
| `pkg/constant/errors.go` | All error codes |
| `pkg/errors.go` | Error types + ValidateBusinessError factory |
| `components/ledger/.env.example` | All environment variables |
| `docs/PROJECT_RULES.md` | Coding standards (DO NOT overwrite) |

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
- **[docs/PROJECT_RULES.md](docs/PROJECT_RULES.md)** — Coding standards and conventions
- **[docs/standards/telemetry.md](docs/standards/telemetry.md)** — Binding telemetry standard (T1–T13: traces, logs, metrics)
- **[docs/standards/error-handling.md](docs/standards/error-handling.md)** — Binding error-handling standard (E1–E14: one error platform, canonical numeric registry)
- **[docs/auth/RBAC-NAMESPACES.md](docs/auth/RBAC-NAMESPACES.md)** — The three authz namespaces in the unified binary (R9)
- **[docs/api/SCOPING.md](docs/api/SCOPING.md)** — Path vs `X-Organization-Id` header scoping (R22)
- **[docs/architecture/crm-field-encryption.md](docs/architecture/crm-field-encryption.md)** — CRM PII field encryption + Vault/Tink KMS subsystem (legacy vs envelope modes, key management, provisioning)
