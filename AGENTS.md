# AGENTS.md — Midaz Quick-Start for AI Agents

All `AGENTS.md` files must be written and maintained in English.

## Context and Sources

Read the guide for the area being changed before applying its local conventions:

| Context | Guide |
| --- | --- |
| Ledger transaction persistence, including sync/async, replay, and recovery | [Ledger persistence guide](components/ledger/internal/AGENTS.md) |
| Tracer service | [Tracer guide](components/tracer/AGENTS.md) |
| API versions, organization scoping, and auth | [Scoping](docs/api/SCOPING.md) and [RBAC namespaces](docs/auth/RBAC-NAMESPACES.md) |
| CRM encryption | [Encryption architecture](docs/architecture/crm-field-encryption.md) |

Apply local instructions only within their stated context. The ledger persistence
guide does not govern unrelated CRM or fee administration work. Preserve separate
v1/v2 and ST/MT contracts rather than generalizing one path to every caller.

Use `go.mod` for dependency versions and the relevant component configuration for
runtime settings. Binding standards and approved specs define intended behavior;
code and contract tests establish what currently executes. Historical examples in
`docs/AGENTS-REFERENCE.md`, `llms*.txt`, or `docs/PROJECT_RULES.md` may lag behind those contracts.
If they disagree, report the conflict and verify the affected implementation and
tests before changing behavior. Do not silently change a wire contract to match
a stale example. Resolve OpenSpec's configured root/store instead of assuming
planning artifacts live in this checkout.

## What Is This?

Midaz is a **source-available core banking platform** written in Go, built around a double-entry ledger. One Go monorepo ships three deploy surfaces: the unified ledger HTTP API (onboarding + transaction + CRM + fees), the Tracer real-time transaction-validation / fraud-prevention API, and the Infra backing stack. Licensed under the Elastic License 2.0 (source-available, not open-source).

## Quick Facts

| Aspect | Detail |
|--------|--------|
| Language | Go 1.27.0 (toolchain 1.27.2 in Docker builders and CI) |
| Module | `github.com/LerianStudio/midaz/v4` (single root `go.mod`, no `go.work`) |
| License | Elastic License 2.0 |
| Architecture | Hexagonal + CQRS |
| HTTP Framework | Huma v2 (OAS 3.1) over Fiber v3 — Fiber is the runtime router/auth chain; Huma generates the API contract and validates requests |
| Databases | PostgreSQL 17, MongoDB (also holds CRM keysets/registry in envelope mode), RabbitMQ 4.1, Valkey |
| Shared libraries | `github.com/LerianStudio/lib-commons/v7`, `lib-observability/v4`; resolve exact versions from `go.mod` |
| API contract library | `github.com/danielgtaylor/huma/v2`; resolve its version from `go.mod` |
| KMS / crypto | `hashicorp/vault/api` (CRM envelope KEK), `tink-crypto/tink-go/v2` (per-org DEKs) |
| Deploy surfaces | Ledger+CRM+Fees (:3002), Tracer (:4020), Infra (Docker Compose) |

> **CRM and fees are not deploy units.** CRM is a package tree at `components/ledger/internal/crm`, imported by
> the ledger binary (holder/instrument routes served on :3002). Fees are embedded in the ledger
> binary (`components/ledger/pkg/fee`, `components/ledger/internal/services/fees`, fee seam in
> `internal/services/command/create_transaction_v2.go`). Tracer is a separate co-located Go service.

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
  adapters/redis/     → Cache repos + the default accounting engine adapter
  adapters/rabbitmq/  → Message queue adapters
  bootstrap/          → Config, DI, server lifecycle
  domain/accounting/  → Storage-independent engine contract and monetary results
  services/command/   → Write use cases (one file per operation)
  services/query/     → Read use cases (one file per operation)

components/ledger/internal/crm/         → CRM package tree (holders/instruments), imported by ledger — NOT a deploy unit
  adapters/mongodb/               → CRM persistence (only adapter; no http/ or api/ tree here)
  adapters/mongodb/encryption/    → Keyset + registry + audit-event Mongo repos (envelope mode)
  services/                       → Holder/instrument use cases
  services/encryption/            → FieldEncryptor seam + EncryptionService (encrypt/decrypt PII, search tokens)
  (CRM HTTP handlers + routes live in components/ledger/internal/adapters/http/in/:
   routes:   holder_routes.go, instrument_routes.go, encryption_routes.go, audit_routes.go,
             holder_accounts_routes.go, composition_routes.go — midaz namespace.
   handlers: holder_handler.go, instrument_handler.go, encryption_handler.go, audit_handler.go,
             composition_handler.go; cores in the matching *_core.go.
   Encryption/protection routes (envelope mode only, midaz ns): POST .../encryption/provision,
   GET .../encryption/status, GET .../protection/audit)

pkg/crypto/            → CRM crypto primitives: kms/vault/ (Vault Transit KEK), tink/ (Tink DEKs), mode + resolver

components/ledger/pkg/  → Embedded fees: fee/ (engine), feeshared/ (plugin-fees types)
  (fee use cases at components/ledger/internal/services/fees; fee seam in internal/services/command/create_transaction_v2.go)

components/tracer/     → Separate Go service deploy unit

pkg/
  mmodel/             → Domain models (Organization, Account, Transaction, etc.)
  constant/errors.go  → Error codes (ledger numeric sentinels (0001+), 30 CRM-00xx (CRM-0006..CRM-0043))
  errors.go           → Typed error structs
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
7. **IDs**: Prefer `uuid.UUID` in new internal contracts. Preserve existing string/pointer representations in DTOs, persisted models, and versioned envelopes; convert and validate at boundaries rather than changing their wire format as cleanup.
8. **HTTP methods**: Use `http.MethodGet` constants, never string literals
9. **Engine boundary**: Go composes postings and completion context; live balance
   approval, overdraft arithmetic, and versions stay inside the atomic Lua execution.
   Never retry accounting after a timeout/unknown outcome. Recovery completes
   already-applied projections and must not invoke the engine.
10. **Consumer failures**: Classify errors; use bounded retry/backoff for transient failures and DLQ/quarantine for permanent or exhausted failures. Historical `nack+requeue on error` examples are not permission for unbounded requeue. Follow the persistence guide for engine evidence retention and distinct broker/recovery ACKs.

## Per-Call Control Skips

Callers can opt out of individual controls per request under a **two-key gate**: a skip is
honored only when the request asks (a `skip` object on the create body) AND the ledger opts
in via `settings.Overrides.Allow{Fee,Tracer,Holder}Skip` (all default `false`). An
unauthorized skip returns **HTTP 422** (`ErrSkipNotPermitted`, `0490`). Resolver:
`pkg/skip.ResolveSkipFor`. Controls: `fees`/`tracer` on transaction create, `holder` on
account create. Honored skips persist to audit columns (`transaction.fees_skipped`,
`transaction.tracer_skipped`, `account.holder_check_skipped`). Invariant: an honored skip
adds **zero** downstream work (short-circuits before the control's lookup).
Reverts follow version-specific policy: v1 has no Tracer reservation; eligible v2
reverts use the v2 Tracer controls and must not inherit the origin's skip as a
permanent bypass. Idempotency replay returns the first outcome.

The transaction and holder skip seams are **`/v2` contracts**: a `/v1` transaction create never reaches
the fee engine and a `/v1` account create never reaches the holder seam, so a `skip` object
in a `/v1` body is inert and can never raise the 422. `scheme` is the other `/v2`-only create field (free-form, normalized by `pkg/scheme.Normalize` to trimmed upper-case `^[A-Z0-9_-]{1,50}$`; `CARD`, `PIX` are examples, not a closed set): `reserveTransaction` — the only translation point — forwards it as the reserve's `transactionType`, empty when none was declared; a `/v2` revert inherits it from the original transaction row, and commit/cancel never send it. Upgrade order: deploy the tracer first — its migrations `000028`–`000032` and image — then the ledger: a ledger that sends a scheme outside `CARD`/`WIRE`/`PIX`/`CRYPTO` to a tracer that still enforces the closed set is refused as invalid (`0532`, whatever `failPosture`); during the tracer rollout, pods of the previous build may fail reads of rows written with a free-form scheme until every pod runs the new build. A `/v1` account create links no holder
(`holder_id` stays NULL) and its response withholds `holderId` + `holderCheckSkipped`.
Outside the seam on both contracts: organization create (neither contract writes a CRM
self-holder — the idempotent backfill runner is the only provisioning path), the
asset-created external account (bypasses `CreateAccount`, no holder) and account update
(`holderId` is immutable). See `docs/api/SCOPING.md`.

## CRM Field Encryption (KMS)

CRM encrypts holder/instrument PII at rest. Mode is chosen by `KMS_VENDOR`: unset/`none` → **legacy**
(lib-commons symmetric crypto, no KMS); `hashicorp-vault` → **envelope** (Vault Transit KEK wraps per-org
Tink DEKs). The seam is the `FieldEncryptor` interface (`internal/crm/services/encryption`), which the
holder/instrument Mongo adapters call to encrypt/decrypt fields and mint deterministic HMAC search tokens
for equality lookups over ciphertext. One **shared, mode-derived** Transit engine (`transit-st`/`transit-mt`)
holds all KEKs — scope lives in the key **name** (`{tenant}_org-{id}` in MT,
`org-{id}` in ST), not per-tenant mounts.
Envelope routes (provision/status/audit) exist only in envelope mode. Key rotation is scaffolded, not yet
active. New env: `KMS_VENDOR`, `KMS_VAULT_ADDR`, `KMS_VAULT_ROLE_ID`, `KMS_VAULT_SECRET_ID`,
`KMS_VAULT_AUTH_METHOD` (`approle`|`token`), `DEPLOYMENT_MODE` (gates the dev root token to `local`). Full
design: [docs/architecture/crm-field-encryption.md](docs/architecture/crm-field-encryption.md).

## Key Files to Read First

| File | Why |
|------|-----|
| `components/ledger/internal/bootstrap/config.go` | Ledger composition root, configuration, init sequence, and `buildHumaMountDeps` |
| `components/ledger/internal/adapters/http/in/*_routes.go` | Entity-specific Huma registrations and Fiber auth chains; `routes.go` provides shared helpers |
| `docs/architecture/engine.md` | Accounting engine boundary, execution, recovery, compatibility, and rollout invariants |
| `pkg/mmodel/account.go` | Account model (representative of all models) |
| `pkg/constant/errors.go` | All error codes |
| `pkg/errors.go` | Error types + ValidateBusinessError factory |
| `components/ledger/.env.example` | Ledger environment settings; consult each other component's own configuration when working there |
| `docs/PROJECT_RULES.md` | Coding standards (DO NOT overwrite) |

## What NOT To Do

- Do NOT overwrite `docs/PROJECT_RULES.md`
- Do NOT use `interface{}` — use `any`
- Do NOT panic — return errors
- Do NOT put use-case policy in HTTP handlers or CRUD repositories. The Redis accounting engine adapter implements the domain accounting contract: live balance decisions must remain inside its atomic Lua execution, not be extracted into Go services as an architectural cleanup.
- Do NOT nest metadata values
- Do NOT use `time.Now()` in tests

## Product Console Impact (cross-repo, mandatory)

`product-console` (`LerianStudio/product-console`) is the single admin UI that unifies the product
frontends. It owns no business data: it consumes this service through a BFF adapter and renders
it. Ledger (onboarding, transactions, CRM, fees) is served through `src/core/infrastructure/midaz/`, `crm/` and `fees/` (`MIDAZ_BASE_PATH`, `MIDAZ_TRANSACTION_BASE_PATH`; screens under `src/app/(routes)/midaz/`, lane `develop-midaz`). Tracer is served through `src/core/infrastructure/tracer/` (`TRACER_BASE_PATH`; screens under `src/app/(routes)/tracer/`, lane `develop-tracer`). Changes under `components/ledger` or `components/tracer` are both in scope.

**Rule: every backend change in this repo must be evaluated for Product Console impact before the
PR is opened. When impact exists, the backend change ships together with the matching
`product-console` change, and the PRs are merged together, never one alone.**

1. **Evaluate.** There is impact when the change alters anything the console consumes or shows:
   routes, request or response fields, enums and status values, validation rules,
   pagination/filter/sort, error codes and messages, authorization resources and permissions,
   the OpenAPI contract, event payloads the UI renders, and config, feature flags or readiness the
   UI reflects. There is no impact for internal refactors, performance work, tests, CI, infra,
   observability and docs-only changes, but only when they leave what the console consumes or
   shows untouched: a docs change that edits the OpenAPI contract, or an infra change that alters
   a readiness signal the UI reflects, is still impact.
2. **Record the verdict in every backend PR description**, under a `Product Console impact`
   heading: `None - <one-line reason>` or `Required - LerianStudio/product-console#<n>`. A backend
   PR without a verdict is incomplete.
3. **When the verdict is `Required`, open the console PR in the same working session.** Branch `<type>/<slug>` from the product
   lane (`develop-<product>` when it exists, `develop-core` otherwise) and target that same lane.
   `product-console` AGENTS.md section 8 is authoritative for lanes, PR title scopes and promotion.
   Cross-link both PRs (`Paired with <repo>#<n>`) in both descriptions.
4. **When the verdict is `Required`, merge together.** Neither PR merges until both are green and ready; then merge them
   back-to-back in the same window. Rollout is not atomic, so keep the contract additive and
   backward compatible (new fields optional, no removal or rename without a compatibility
   window) so each side keeps working against the other's previously released version. If a
   breaking change is unavoidable, say so in both PR descriptions and coordinate the release
   order before merging.
5. **When the verdict is `Required`, validate integrated before merge.** Run the console BFF against this backend on a live
   stack and drive the affected flow with Playwright (`product-console` AGENTS.md section 6). A
   green run against a mocked backend does not count.
6. **Never defer the console side** to a follow-up ticket when the verdict is `Required`.

## Deeper References

- **[docs/AGENTS-REFERENCE.md](docs/AGENTS-REFERENCE.md)** — Deep technical reference (architecture, bootstrap, multi-tenancy, transaction processing). Historical `CLAUDE.md` references in code comments, migrations and plans mean this reference. `CLAUDE.md` is a symlink to this file.
- **[llms-full.txt](llms-full.txt)** — Complete reference with all env vars, API endpoints, error codes, models
- **[llms.txt](llms.txt)** — Concise overview following llmstxt.org spec
- **[docs/PROJECT_RULES.md](docs/PROJECT_RULES.md)** — Coding standards and conventions
- **[docs/standards/telemetry.md](docs/standards/telemetry.md)** — Binding telemetry standard (T1–T13: traces, logs, metrics)
- **[docs/standards/error-handling.md](docs/standards/error-handling.md)** — Binding error-handling standard (E1–E14: one error platform, canonical numeric registry)
- **[docs/auth/RBAC-NAMESPACES.md](docs/auth/RBAC-NAMESPACES.md)** — The three authz namespaces in the unified binary (R9)
- **[docs/api/SCOPING.md](docs/api/SCOPING.md)** — Path vs `X-Organization-Id` header scoping (R22)
- **[docs/architecture/crm-field-encryption.md](docs/architecture/crm-field-encryption.md)** — CRM PII field encryption + Vault/Tink KMS subsystem (legacy vs envelope modes, key management, provisioning)
- **[docs/architecture/engine.md](docs/architecture/engine.md)** — Default accounting engine, live-state Lua boundary, receipts, completion, recovery, and rollout compatibility
