# Ledger

> The unified Midaz binary: double-entry ledger, onboarding, CRM, and fees on a single port

[![Go Version](https://img.shields.io/badge/Go-1.26.4+-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-336791?style=flat&logo=postgresql)](https://www.postgresql.org)
[![License](https://img.shields.io/badge/license-Elastic%20License%202.0-4c1.svg)](../../LICENSE)

Ledger is the primary deploy unit in the [Midaz monorepo](https://github.com/LerianStudio/midaz)
(module `github.com/LerianStudio/midaz/v4`, single root `go.mod`), released on the single
unified Midaz version.

---

## 📋 Table of Contents

- [Overview](#overview)
- [Domains](#domains)
- [Architecture](#architecture)
- [Project Structure](#project-structure)
- [Tech Stack](#tech-stack)
- [Quick Start](#quick-start)
- [Development](#development)
- [API Surface](#api-surface)
- [CRM Field Encryption / KMS](#crm-field-encryption--kms)
- [Testing](#testing)
- [Deployment](#deployment)
- [Documentation](#documentation)
- [License](#license)

---

## Overview

**Ledger** is a single Go binary listening on **:3002** that folds four domains into one
process, with no gRPC between them — all four register their routes onto one Fiber app:

- **Onboarding** — the resource hierarchy: Organization → Ledger → Assets/Portfolios/Segments → Accounts.
- **Transaction** — double-entry postings, balances, and the transaction lifecycle (commit/cancel/revert).
- **CRM** — holders and instruments, a package tree at `internal/crm` (no `cmd/`, no image; imported by the ledger binary).
- **Fees** — the fee engine at `pkg/fee`, shared types at `pkg/feeshared`, use cases at `internal/services/fees`.

Composition and ports are wired in [`internal/bootstrap/config.go`](internal/bootstrap/config.go)
and the unified server in [`internal/bootstrap/unified-server.go`](internal/bootstrap/unified-server.go);
route registration lives in [`internal/adapters/http/in/routes.go`](internal/adapters/http/in/routes.go).

### Why unified?

- **One deploy unit** — onboarding, transaction, CRM, and fees ship and scale together on :3002.
- **In-process calls** — the fee seam and CRM referential checks run inside the request, not over the wire.
- **Double-entry correctness** — postings and balances are the non-negotiable money path.
- **Multi-tenant capable** — tenant-isolated DB resolution via lib-commons tenant managers, gated by `MULTI_TENANT_ENABLED` (default off).

---

## Domains

| Domain | Responsibility | Home |
|--------|----------------|------|
| **Onboarding** | Organization/Ledger/Asset/Portfolio/Segment/Account CRUD + metadata | `internal/services/{command,query}`, `internal/adapters/postgres` |
| **Transaction** | Double-entry postings, balances, transaction lifecycle, async processing | `internal/services/{command,query}`, `pkg/mtransaction` |
| **CRM** | Holders + instruments, PII field encryption, search tokens | `internal/crm` (package tree) |
| **Fees** | Fee calculation applied at the transaction-create seam | `pkg/fee`, `pkg/feeshared`, `internal/services/fees` |

Transaction creation modes: JSON, DSL, inflow, outflow, annotation. Pending transactions can be
committed or cancelled; revert creates a reverse transaction. Async processing is controlled by
`RABBITMQ_TRANSACTION_ASYNC`. The fee seam sits in `transaction_create.go` after default balance-key
application and the idempotency claim, before post-fee re-validation.

---

## Architecture

### Pattern: Hexagonal + CQRS

Flow: **HTTP handlers → command/query use cases → repository interfaces → adapters.** Dependencies
flow inward; inner layers never import outer layers. Domain logic stays out of handlers and
repositories. Domain models live in `pkg/mmodel`.

- **HTTP layer:** [Huma v2](https://github.com/danielgtaylor/huma) (OAS 3.1) mounted over
  **Fiber v2**. Fiber remains the runtime router, auth chain, and middleware host; Huma sits on
  top to generate the API contract and validate typed request/response structs.
- **Errors:** RFC 9457 `application/problem+json`. Typed errors from `pkg/errors.go`, numeric
  sentinels from `pkg/constant/errors.go`. Not found → 404, business-rule violations → 422.
- **Handlers:** `internal/adapters/http/in` (the `*_handler_huma.go` files).
- **Write use cases:** `internal/services/command`. **Read use cases:** `internal/services/query`.

### Stores

| Store | Purpose |
|-------|---------|
| **PostgreSQL 17** | Onboarding and transaction data (primary + replica; separate `onboarding` and `transaction` databases) |
| **MongoDB 8** | Metadata, CRM holders/instruments, fee configuration, CRM keysets/registry |
| **Valkey/Redis 8** | Cache and balance-sync (the balance-sync collector/worker and Redis consumer) |
| **RabbitMQ 4.1.x** | Async transaction processing (gated by `RABBITMQ_TRANSACTION_ASYNC`) |

TLS is enforced per connection by the security tier derived from `DEPLOYMENT_MODE` /`ENV_NAME`; the
postgres/mongo/redis/rabbitmq constructors refuse plaintext dependencies unless `ALLOW_INSECURE_TLS=true`
(local development only). Optionally reserves against the co-located **tracer** service (:4020) over
gRPC/mTLS — see [`docs/architecture/ledger-tracer-topology.md`](../../docs/architecture/ledger-tracer-topology.md).

---

## Project Structure

```text
ledger/
├── cmd/
│   ├── app/main.go             # Entry point: loads .env, calls bootstrap
│   └── backfill/main.go        # Holder backfill utility
├── internal/
│   ├── bootstrap/              # Composition root (config, unified server, workers, readyz)
│   ├── adapters/
│   │   ├── http/in/            # Huma-over-Fiber handlers, routes, middleware
│   │   ├── postgres/           # Onboarding + transaction repositories
│   │   └── mongodb/            # Metadata, CRM, and fees repositories
│   ├── crm/                    # CRM package tree (holders, instruments, encryption)
│   │   ├── adapters/
│   │   └── services/
│   └── services/
│       ├── command/            # Write use cases
│       ├── query/              # Read use cases
│       └── fees/               # Fee use cases
├── pkg/
│   ├── fee/                    # Fee engine
│   └── feeshared/              # Shared fee types
├── docker-compose.yml          # App service (joins shared infra-network)
├── Dockerfile
├── Makefile
└── .env.example
```

Shared code lives at the repo root: `pkg/mmodel` (domain models), `pkg/mtransaction`, `pkg/errors.go`,
`pkg/constant/`.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Language** | Go 1.26.4 |
| **HTTP framework** | Fiber v2.52.13 (runtime) + Huma v2.38.0 (OAS 3.1 contract) |
| **Relational store** | PostgreSQL 17 (`jackc/pgx`, `Masterminds/squirrel`) |
| **Document store** | MongoDB 8 (`go.mongodb.org/mongo-driver/v2`) |
| **Cache / balance-sync** | Valkey/Redis 8 |
| **Messaging** | RabbitMQ 4.1.x |
| **Decimals** | `shopspring/decimal` (never float64) |
| **Auth** | lib-auth v2.8.1 (Access Manager plugin) |
| **Shared platform** | lib-commons v5.8.0, lib-observability v1.1.0, lib-streaming v1.6.2 |
| **Observability** | OpenTelemetry via lib-observability; otel-lgtm / Grafana stack from `components/infra` |

---

## Quick Start

### Prerequisites

- Docker 20+ & Docker Compose 2+
- Go 1.26.4+ (for local development)
- Make

### 1. Setup

```bash
git clone https://github.com/LerianStudio/midaz.git
cd midaz

# Generate .env files for every component from their .env.example templates
make set-env
```

### 2. Start the stack

```bash
# Starts shared infra (components/infra) then the Go components
make up

# Stop everything
make down
```

`make up` at the repo root brings up `components/infra` first (PostgreSQL, MongoDB, Valkey,
RabbitMQ, otel-lgtm), waits for readiness, then starts the ledger app. To drive the ledger's own
Makefile directly, use the delegation target: `make ledger COMMAND=<target>`.

### 3. Verify health

```bash
curl http://localhost:3002/health   # liveness
curl http://localhost:3002/readyz   # readiness (dependency checks)
```

---

## Development

```bash
# From the repo root, delegate to the ledger component Makefile:
make ledger COMMAND=build      # Compile the ledger binary
make ledger COMMAND=test       # Run the component test suite
make ledger COMMAND=up         # Start just the ledger app container
make ledger COMMAND=down       # Stop it

# Or run the monorepo-wide targets from the root:
make test-unit                 # Unit tests
make test-integration          # Integration tests (testcontainers)
make lint                      # golangci-lint v2.12.2
make format                    # gofmt
make sec                       # gosec + govulncheck
```

Environment is configured via [`.env.example`](.env.example). `DEPLOYMENT_MODE` (`local` | `byoc` |
`saas`) selects the TLS enforcement tier; `SERVER_PORT` defaults to `3002`.

---

## API Surface

Base URL: `http://localhost:3002/v1`. Organization is **path-scoped** — there are no
`X-Organization-Id` / `X-Ledger-Id` headers. Route families:

| Family | Prefix | Notes |
|--------|--------|-------|
| Onboarding | `/v1/organizations`, `.../ledgers`, `.../assets`, `.../portfolios`, `.../segments`, `.../accounts` | Resource hierarchy CRUD |
| Transaction | `.../transactions`, `.../operations`, `.../balances` | Double-entry postings + lifecycle |
| CRM | `.../holders`, `.../instruments` | Holders + instruments (PII encrypted at rest) |
| Fees | `/v1/fees`, fee routes | `plugin-fees` authz namespace |
| CRM encryption | `.../encryption/provision`, `.../encryption/status`, `.../protection/audit` | Envelope mode only (see below) |

RBAC namespaces in the unified binary: `midaz` (onboarding + transaction + CRM), `routing`,
`plugin-fees`. The canonical OpenAPI spec is generated at `components/ledger/api/openapi.huma.yaml`
and merged into `postman/specs/midaz.openapi.{yaml,json}`; treat the spec as the source of truth for
request/response shapes rather than duplicating them here.

---

## CRM Field Encryption / KMS

CRM encrypts holder/instrument PII at rest. Mode is selected by `KMS_VENDOR`: unset/`none` →
**legacy** (lib-commons symmetric crypto, no KMS); `hashicorp-vault` → **envelope** (HashiCorp Vault
Transit KEK wrapping per-organization Tink DEKs). The seam is the `FieldEncryptor` interface
([`internal/crm/services/encryption`](internal/crm/services/encryption)), which the holder/instrument
Mongo adapters call to encrypt/decrypt fields and to generate deterministic HMAC search tokens for
equality lookups over ciphertext. Key material is per-organization, but a single **shared,
mode-derived** Vault Transit engine (`transit-st` single-tenant / `transit-mt` multi-tenant) holds all
KEKs — tenant isolation lives in the key **name** (`{tenant}_org-{id}`), not in per-tenant mounts.

Envelope mode adds these env vars: `KMS_VENDOR`, `KMS_VAULT_ADDR`, `KMS_VAULT_ROLE_ID`,
`KMS_VAULT_SECRET_ID`, `KMS_VAULT_AUTH_METHOD` (`approle` | `token`), and `DEPLOYMENT_MODE` (which also
gates the dev root token to `local` only). The optional Vault container lives in `components/infra`
(`make ledger COMMAND=...` does not start it — use the infra Vault targets). Wiring is in
[`internal/bootstrap/config.crm.encryption.go`](internal/bootstrap/config.crm.encryption.go).

Full design — the fail-closed matrix, search-token write-one/read-all asymmetry, lazy
legacy→envelope migration, and key rotation posture — is in
[`docs/architecture/crm-field-encryption.md`](../../docs/architecture/crm-field-encryption.md).

---

## Testing

```bash
make ledger COMMAND=test         # Component test suite
make test-unit                   # Unit tests (repo-wide)
make test-integration            # Integration tests with testcontainers
```

- TDD is the norm; write the test before the behavior.
- Repository/adapter code is covered by integration tests against real dependencies (testcontainers);
  unit tests cover pure helpers and business branches.
- Never use `time.Now()` or `uuid.New()` in tests — use fixed times and deterministic UUIDs.

---

## Deployment

The component ships a multi-stage `Dockerfile` (distroless nonroot runtime) built from the repo-root
context. The app container is defined in `docker-compose.yml` and joins the shared external
`infra-network`; PostgreSQL, MongoDB, Valkey, RabbitMQ, and the OTel collector come from
`components/infra`, not from this compose.

All configuration is environment-driven — see [`.env.example`](.env.example) for the full set.
Key variables: `SERVER_PORT` (`3002`), `DEPLOYMENT_MODE`, `MULTI_TENANT_ENABLED`, the `DB_ONBOARDING_*`
/ `DB_TRANSACTION_*` groups, `MONGO_*`, `REDIS_*`, `RABBITMQ_*`, `PLUGIN_AUTH_*`, and the `KMS_*` group
for CRM envelope encryption.

---

## Documentation

This README is an orientation layer. The authoritative, deeper references live at the repo root — do
not look for a component `CLAUDE.md` or `AGENTS.md` here; the root ones cover the ledger:

| Document | Purpose |
|----------|---------|
| [`../../CLAUDE.md`](../../CLAUDE.md) | Agent reference: architecture, coding rules, streaming, multi-tenancy |
| [`../../AGENTS.md`](../../AGENTS.md) | Concise agent overview |
| [`../../docs/PROJECT_RULES.md`](../../docs/PROJECT_RULES.md) | Architecture patterns, domain model, testing standards |
| [`../../docs/standards/`](../../docs/standards/) | Binding telemetry (T1–T13) and error-handling (E1–E14) standards |
| [`../../docs/auth/RBAC-NAMESPACES.md`](../../docs/auth/RBAC-NAMESPACES.md) | RBAC namespace map |
| [`../../docs/architecture/ledger-tracer-topology.md`](../../docs/architecture/ledger-tracer-topology.md) | Ledger ↔ tracer reservation seam |
| [`../../llms-full.txt`](../../llms-full.txt) | Full API and environment reference |

---

## License

This project is licensed under the Elastic License 2.0. It is source-available: you are free to use,
modify, and distribute this software, but you may not provide it to third parties as a hosted or
managed service. See the [LICENSE](../../LICENSE) file for details.

---

Built with ❤️ by LerianStudio Engineering Team
