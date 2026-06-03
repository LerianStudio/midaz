# P9-T01 — Frozen Component / Topology Inventory

Single source of truth for the Phase 9 documentation rewrites (STRUCTURE.md, AGENTS.md,
CLAUDE.md, llms-full.txt, llms.txt). Every factual claim in those files MUST trace back to
this inventory. All values below were read verbatim from the live tree on branch
`phase/p9-cleanup` (consolidation code-complete).

> Do NOT hand-edit version strings here to "GA" or rounded values. They are read verbatim
> from `go.mod` and the component compose/env files. If a value changes, re-run the
> verification commands at the bottom and update this file first, then the dependent docs.

## 1. Module & toolchain (read from `go.mod`)

| Fact | Value |
|------|-------|
| Module path | `github.com/LerianStudio/midaz/v3` |
| Go directive | `1.26.3` |
| toolchain | `go1.26.4` |
| Workspace | single root `go.mod` — **no** `go.work`, **no** `replace` directives |
| License | Elastic License 2.0 |

## 2. Pinned dependency versions (GA pre-gate)

| Dependency | Version | Notes |
|------------|---------|-------|
| `github.com/LerianStudio/lib-commons/v5` | `v5.4.1` | GA pin — no `-beta`/`-rc` suffix |
| `github.com/LerianStudio/lib-observability` | `v1.0.1` | log / metrics / tracing / assert / panic-recovery (moved out of lib-commons) |
| `github.com/LerianStudio/lib-auth/v2` | `v2.8.0` | auth middleware |
| `go.mongodb.org/mongo-driver/v2` | `v2.6.0` | MongoDB driver (v2 line) |
| `github.com/gofiber/fiber/v2` | `v2.52.13` | HTTP framework |
| `github.com/jackc/pgx/v5` | `v5.9.2` | PostgreSQL driver |
| `github.com/rabbitmq/amqp091-go` | `v1.11.0` | RabbitMQ client |
| `github.com/redis/go-redis/v9` | `v9.20.0` | Valkey/Redis client |
| `github.com/shopspring/decimal` | `v1.4.0` | decimal arithmetic |
| `github.com/antlr4-go/antlr/v4` | `v4.13.1` | Gold DSL parser |
| `github.com/Masterminds/squirrel` | `v1.5.4` | SQL builder |
| `github.com/testcontainers/testcontainers-go` | `v0.42.0` | integration tests |
| `go.opentelemetry.io/otel` | `v1.44.0` | OpenTelemetry |
| `go.uber.org/mock` | `v0.6.0` | mock generation |
| golangci-lint | `v2.4.0` | pinned in `Makefile` (`GOLANGCI_LINT_VERSION`), run via `go run @version` |

## 3. Component directories (`ls -d components/*/`)

Six directories exist. Five are deploy units (4 Go services + infra). `components/crm` is
**not** a deploy unit — it is a collapsed package tree imported by `components/ledger` and
served by the unified ledger binary.

| Directory | Deploy unit? | Role | Port |
|-----------|--------------|------|------|
| `components/infra` | yes (infra) | Single unified `docker-compose.yml` backing stack | — |
| `components/ledger` | yes (Go service) | Unified ledger binary: onboarding + transaction + **CRM** (holder/alias) + **fees** | `:3002` |
| `components/tracer` | yes (Go service) | Real-time transaction validation / fraud-prevention API (CEL rules, hash-chained audit) | `:4020` |
| `components/reporter-manager` | yes (Go service) | REST API that accepts report-generation requests and publishes jobs to RabbitMQ | `:4005` |
| `components/reporter-worker` | yes (Go service) | Async consumer that renders PDFs (headless Chromium) and writes to S3-compatible storage | `:4006` |
| `components/crm` | **no** — package tree | Holder/alias domain (`adapters/`, `api/`, `services/`), imported by ledger; routes served on `:3002` | (served by ledger) |

Port sources: `components/ledger/.env.example` (`SERVER_PORT=3002`), `components/tracer/.env.example`
(`SERVER_PORT=4020`), `components/reporter-manager/.env.example` (`SERVER_PORT=4005`),
`components/reporter-worker/.env.example` (`HEALTH_PORT=4006`).

There is **no** `components/onboarding`, **no** `components/transaction`, and `components/crm`
has **no** `cmd/` and **no** `internal/` (it was lifted out of `internal/` so ledger can import
it across the component boundary).

## 4. CRM as a collapsed package tree

```
components/crm/
├── adapters/
│   ├── http/in/        # Fiber handlers + routes.go (registered into the ledger app)
│   └── mongodb/         # holder/ + alias/ repositories
├── api/                 # swagger.yaml / openapi.yaml / docs.go
└── services/            # command/query use cases
```

- ledger composition root wires CRM via `initCRM` in `components/ledger/internal/bootstrap/config.go`
  (Mongo config in `config.mongo.crm.go`, integration test `crm_collapse_integration_test.go`).
- CRM routes register under the auth namespace `plugin-crm` (`components/crm/adapters/http/in/routes.go`,
  `const ApplicationName = "plugin-crm"`).
- CRM scopes by the `X-Organization-Id` HTTP header (NOT path-based org hierarchy). See `docs/api/SCOPING.md` (R22).
- The legacy CRM `ErrorCodeTransformer` shim has been removed (PD-2): CRM 4xx/5xx now carry
  canonical midaz codes (e.g. `0009`, `0046`, `0047`, `0094`), not `CRM-00xx` rewrites.

## 5. Fees embedded in ledger

Fees are not a component dir; they live inside `components/ledger`:

| Path | Contents |
|------|----------|
| `components/ledger/pkg/fee` | fee engine |
| `components/ledger/pkg/feeshared` | shared fee types/constants (`ApplicationName = "plugin-fees"`) |
| `components/ledger/internal/services/fees` | fee + billing-package use cases (calculate, estimate, packages, billing-calculate) |
| `components/ledger/internal/adapters/mongodb/fees` | fee/package Mongo repositories |
| `components/ledger/internal/adapters/http/in/fees_routes.go` | fee/billing routes under the `plugin-fees` namespace |

Fee seam: `components/ledger/internal/adapters/http/in/transaction_create.go` —
fees run **after** `mtransaction.ApplyDefaultBalanceKeys(...)` (~L1006) and **before** the
idempotency claim (~L1009). The idempotency hash is computed over the raw pre-fee payload.

## 6. Auth / RBAC namespaces (R9 — preserved per locked decision)

The unified ledger binary authorizes under four namespace literals (no rename in P9):

| Namespace | Owner | Resources |
|-----------|-------|-----------|
| `midaz` | ledger (`midazName`) | organizations, ledgers, assets, asset-rates, portfolios, segments, accounts, balances, transactions, operations, settings |
| `routing` | ledger (`routingName`) | account-types, operation-routes, transaction-routes |
| `plugin-crm` | crm | holders, aliases |
| `plugin-fees` | fees (`feesApplicationName` / `constant.ModuleFees`) | packages, estimates, billing-packages, billing-calculate |

See `docs/auth/RBAC-NAMESPACES.md` (R9).

## 7. Shared trees

- `pkg/` (root): `constant`, `gold`, `mbootstrap`, `mmodel`, `mongo`, `mtransaction`, `net`,
  `pagination`, `reporter`, `repository`, `shell`, `streaming`, `utils`.
  `pkg/transaction` was renamed to `pkg/mtransaction`. `pkg/reporter` is the reporter shared
  library imported by both reporter deploy units.
- `tests/` (root): `chaos`, `helpers`, `reporter`, `utils`. `tests/reporter` holds the reporter
  shared test suites (e2e, integration, property, fuzzy, chaos, utils).

## 8. Unified infra (single source)

`components/infra/docker-compose.yml` is the single backing-stack source. Services:
`midaz-mongodb` (+ init), `midaz-valkey`, `midaz-postgres-primary`, `midaz-postgres-replica`,
`midaz-otel-lgtm`, `midaz-rabbitmq`, `midaz-seaweedfs`, `midaz-keda`. All deploy units attach
to `infra-network`.

## 9. Error codes

- Ledger numeric codes: `0001`–`0175` (includes overdraft `0167`–`0175`).
- CRM codes: **16** live domain sentinels (CRM-0018 pruned). Clients receive canonical midaz
  codes on the formerly-transformed paths — there is no transform shim.

CRM-0006, CRM-0008, CRM-0010, CRM-0013, CRM-0017, CRM-0019, CRM-0020, CRM-0021, CRM-0022,
CRM-0023, CRM-0024, CRM-0025, CRM-0026, CRM-0027, CRM-0028, CRM-0029.

## 10. Verification commands

```bash
grep -E '^go |^toolchain ' go.mod
grep -E 'lib-commons/v5 |lib-observability |mongo-driver/v2 ' go.mod
grep -E 'lib-commons/v5 v5\.[0-9]+\.[0-9]+$' go.mod   # GA pre-gate: matches, no -beta/-rc
ls -d components/*/
grep -E 'SERVER_PORT|HEALTH_PORT' components/{ledger,tracer,reporter-manager,reporter-worker}/.env.example
grep -c 'CRM-00' pkg/constant/errors.go               # 16
go build ./...                                         # green
```
