# Project Structure Overview

This guide covers the project structure after the monorepo consolidation. The codebase is a
single Go module (`github.com/LerianStudio/midaz/v4`, Go 1.26.4, single root `go.mod` — no
`go.work`, no `replace`) following hexagonal architecture with Command Query Responsibility
Segregation (CQRS).

The repository ships **four deploy surfaces** (three Go binaries + infra). CRM and fees are not
deploy units: CRM is a collapsed package tree imported by the ledger binary, and fees are
embedded inside the ledger binary. Reporter is one unified codebase
(`components/reporter`) that ships as a single image deployed in two `RUN_MODE` surfaces
(api on `:4005`, worker on `:4006`).

#### Directory Layout

```
MIDAZ
 |   components
 |   |---   infra                  # backing-stack docker-compose (single source)
 |   |   |---   grafana
 |   |   |---   mongo
 |   |   |---   postgres
 |   |   |---   rabbitmq
 |   |   |---   seaweedfs
 |   |---   ledger                 # DEPLOY UNIT :3002 — onboarding + transaction + CRM + fees
 |   |   |---   api
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |   |---   adapters
 |   |   |   |   |---   http
 |   |   |   |   |   |---   in     # Fiber handlers + routes (incl. CRM holder/instrument + fees_routes.go)
 |   |   |   |   |---   mongodb    # metadata + fees repositories
 |   |   |   |   |---   postgres   # onboarding + transaction repositories
 |   |   |   |   |---   rabbitmq
 |   |   |   |   |---   redis
 |   |   |   |---   bootstrap      # composition root (initCRM, fee wiring)
 |   |   |   |---   services
 |   |   |   |   |---   command
 |   |   |   |   |---   query
 |   |   |   |   |---   fees       # embedded fee/billing use cases
 |   |   |---   migrations
 |   |   |---   pkg
 |   |   |   |---   fee            # embedded fee engine
 |   |   |   |---   feeshared      # embedded fee shared types/constants (plugin-fees)
 |   |---   crm                    # PACKAGE TREE (not a deploy unit) — imported by ledger
 |   |   |---   adapters
 |   |   |   |---   mongodb        # CRM persistence (only adapter in the package tree)
 |   |   |   |   |---   holder
 |   |   |   |   |---   instrument
 |   |   |   |---   services       # holder/instrument command/query use cases
 |   |---   tracer                 # DEPLOY UNIT :4020
 |   |   |---   api
 |   |   |---   cmd
 |   |   |   |---   app
 |   |   |---   internal
 |   |   |---   migrations
 |   |---   reporter               # UNIFIED CODEBASE — one image, RUN_MODE=api|worker|all
 |   |   |---   api                # api surface (:4005) Swagger spec
 |   |   |---   cmd
 |   |   |   |---   app            # single main.go; RUN_MODE selects the surface
 |   |   |---   internal
 |   |   |   |---   app            # shared bootstrap wiring (RUN_MODE dispatch)
 |   |   |   |---   manager        # REST API surface (:4005)
 |   |   |   |---   worker         # RabbitMQ consumer + health server surface (:4006)
 |   |---   reporter-manager       # Dockerfile stub (distroless image anchor, RUN_MODE=api)
 |   |---   reporter-worker        # Dockerfile stub (alpine+Chromium image anchor, RUN_MODE=worker)
 |   image
 |   pkg                           # shared libraries (root, non-exhaustive)
 |   |---   constant
 |   |---   gold
 |   |---   mbootstrap
 |   |---   mmodel
 |   |---   mongo
 |   |---   mtransaction
 |   |---   net
 |   |---   pagination
 |   |---   reporter               # reporter shared library (used by both RUN_MODE surfaces)
 |   |---   repository
 |   |---   shell
 |   |---   streaming
 |   |---   utils
 |   postman
 |   scripts
 |   tests                         # shared test trees (root)
 |   |---   chaos
 |   |---   helpers
 |   |---   reporter               # reporter shared suites (e2e/integration/property/fuzzy/chaos/utils)
 |   |---   utils
```

#### Deploy Units

| Unit | Port | Role |
|------|------|------|
| **infra** | — | Single `components/infra/docker-compose.yml`: PostgreSQL 17 (primary/replica), MongoDB, Valkey, RabbitMQ, SeaweedFS, KEDA, OTEL-LGTM. All units join `infra-network`. |
| **ledger** | `:3002` | Unified binary: onboarding + transaction, **CRM** (holders/instruments), and **fees** (fee engine + billing). |
| **tracer** | `:4020` | Real-time transaction validation / fraud-prevention API. Hexagonal + CQRS, CEL rule engine, hash-chained audit log. Ships its own migrations under `components/tracer/migrations`. |
| **reporter** | `:4005` / `:4006` | One unified codebase (`components/reporter`), one binary, deployed split via `RUN_MODE`: **api** (`:4005`) is the REST API that accepts report-generation requests and publishes jobs to RabbitMQ (`reporter.generate-report.{exchange,queue,key}`), shipped as a distroless image; **worker** (`:4006`) is the async consumer that runs extraction in-process and renders PDFs via headless Chromium (chromedp), writing output to S3-compatible object storage (SeaweedFS), shipped as a fat alpine image with the Chromium userland (cannot be distroless — R20). `RUN_MODE=all` runs both surfaces in one process (dev only). |

#### Components (`./components`)

##### Ledger (`./components/ledger`) — deploy unit, `:3002`

The unified ledger binary folds four domains into one process:

* **Onboarding + Transaction**: the original midaz ledger (organizations, ledgers, assets,
  portfolios, segments, accounts, transactions, operations, balances; routing via
  account-types / operation-routes / transaction-routes).
* **CRM (folded)**: holder/instrument routes registered from the `components/ledger/internal/crm` package tree.
  See below.
* **Fees (embedded)**: fee engine at `components/ledger/pkg/fee`, shared types at
  `components/ledger/pkg/feeshared`, use cases at `components/ledger/internal/services/fees`,
  Mongo repos at `components/ledger/internal/adapters/mongodb/fees`, routes at
  `components/ledger/internal/adapters/http/in/fees_routes.go`. The fee seam runs inside the
  `transaction_create.go` HTTP handler (not the command layer) after
  `mtransaction.ApplyDefaultBalanceKeys(...)` and the idempotency claim, mutating the send legs
  before the post-fee re-validation; `applyFees` itself lives in `transaction_fee_application.go`.

Composition root: `components/ledger/internal/bootstrap/config.go` (wires onboarding,
transaction, `initCRM`, and fees).

##### CRM (`./components/ledger/internal/crm`) — package tree, NOT a deploy unit

CRM was lifted out of `internal/` so the ledger binary can import it across the component
boundary. It has **no** `cmd/`, **no** standalone binary, and **no** HTTP or API tree of its
own — the package tree holds only persistence and use cases:

* **Adapters** (`./components/ledger/internal/crm/adapters/mongodb/{holder,instrument}`): CRM persistence (the
  only adapter in the package tree).
* **Services** (`./components/ledger/internal/crm/services`): holder/instrument command/query use cases.

The entire CRM HTTP surface lives in the ledger tree under
`components/ledger/internal/adapters/http/in/`: `crm_routes.go` (holder/instrument registration,
`midaz` namespace), `composition_routes.go` (holder↔account composition), and the
`holder.go`, `holder_accounts.go`, and `instrument.go` handlers. CRM endpoints are folded into
the ledger Swagger spec (`components/ledger/api`); there is no separate CRM OpenAPI spec.

CRM scopes requests by the `:organization_id` URL path parameter (path-based org hierarchy; R22
reversed pre-GA) — see `docs/api/SCOPING.md`. CRM error responses now carry canonical midaz codes; the legacy
`CRM-00xx` transform shim was removed (PD-2). The 16 surviving CRM domain sentinels live in
`pkg/constant/errors.go`.

##### Tracer (`./components/tracer`) — deploy unit, `:4020`

Real-time transaction validation and fraud-prevention API. Hexagonal + CQRS, CEL rule engine,
hash-chained audit log. Ships its own migrations under `./components/tracer/migrations`.

##### Reporter (`./components/reporter`) — one codebase, two `RUN_MODE` deploy surfaces

Reporter is a single unified codebase and binary (`components/reporter`) deployed split via
`RUN_MODE` (Option C split: shared library extracted to `pkg/reporter`, shared suites to
`tests/reporter`). Data extraction runs in-process — there is no remote fetcher hop.

* **API surface** (`RUN_MODE=api`, `:4005`): REST API that accepts report-generation requests
  and publishes jobs to RabbitMQ. Shipped as a distroless image (the
  `components/reporter-manager` Dockerfile stub is the image-name anchor).
* **Worker surface** (`RUN_MODE=worker`, `:4006`): async consumer that runs extraction
  in-process and renders PDFs via headless Chromium (chromedp), writing to S3-compatible object
  storage. Shipped as a fat alpine image with the Chromium userland (cannot be distroless —
  R20); the `components/reporter-worker` Dockerfile stub is that image-name anchor.
* **`RUN_MODE=all`**: both surfaces in one process — dev / minimal envs only.

Both surfaces ship from the same image, attach to `infra-network`, and use the shared Mongo /
Valkey / RabbitMQ backing services. Production deploys two Deployments (one per surface) from
that single image; `OTEL_RESOURCE_SERVICE_NAME` and `RUN_MODE` are set per-Deployment.

* **Shared library** (`./pkg/reporter`): datasource resolution, PDF/pongo rendering, template
  builder, S3 (SeaweedFS) and storage adapters, multi-tenant helpers — imported by both surfaces.
* **Shared suites** (`./tests/reporter`): `e2e`, `integration`, `property`, `fuzzy`, `chaos`,
  and `utils` test trees for the reporter component.

##### Infra (`./components/infra`) — deploy unit (backing stack)

`components/infra/docker-compose.yml` is the single source for the backing stack: PostgreSQL 17
(primary + replica), MongoDB, Valkey, RabbitMQ, SeaweedFS, KEDA autoscaling, and OTEL-LGTM. The
SeaweedFS S3 config (`s3.json`, `init-bucket.sh`) lives under `./components/infra/seaweedfs/`.

#### Shared Packages (`./pkg`)

Cross-component Go libraries (root module; non-exhaustive — additional packages such as
`buildinfo`, `proto`, and `rabbitmq` also live here):

| Package | Purpose |
|---------|---------|
| `pkg/mmodel` | Domain models (Organization, Ledger, Account, Asset, Transaction, Balance, Holder, Instrument, etc.) |
| `pkg/constant` | Error codes (`errors.go`, ledger numeric sentinels (`0001`+) + 16 `CRM-00xx`), entity/action/module constants |
| `pkg/gold` | ANTLR4 Gold DSL grammar + parser for transactions |
| `pkg/mtransaction` | Transaction processing utilities (formerly `pkg/transaction`) |
| `pkg/net` | HTTP middleware, pagination, protected-route helpers |
| `pkg/streaming` | lib-streaming event modeling (`pkg/streaming/events`) |
| `pkg/reporter` | Reporter shared library (datasource, rendering, storage) used by both reporter `RUN_MODE` surfaces |
| `pkg/mbootstrap` | Bootstrap helpers |
| `pkg/mongo` | MongoDB utilities |
| `pkg/pagination` | Pagination helpers |
| `pkg/repository` | Repository interfaces |
| `pkg/shell` | Shell/scripting utilities |
| `pkg/utils` | General utilities |

> Logging, telemetry, tracing, panic recovery, HTTP toolkit, and tenant-manager symbols
> (`libLog`, `libHTTP`, etc.) come from the external libraries
> `github.com/LerianStudio/lib-commons/v5` (v5.5.0) and
> `github.com/LerianStudio/lib-observability` (v1.0.1) — they are **not** subpackages of `./pkg`.

#### Miscellaneous

* **Images** (`./image`): project images and README assets.
* **Postman** (`./postman`): API collections for manual testing.
* **Scripts** (`./scripts`): coverage, docs generation, environment checks.
* **Makefile includes** (`./mk`): coverage, tests, quality targets.
