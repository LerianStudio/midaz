# Reporter → Embedded Fetcher Engine Migration

Status: PLAN (not started) · Author: pairing w/ Galadriel · Date: 2026-06-09
Anchored to: `github.com/LerianStudio/fetcher/pkg/engine@pkg/engine/v1.0.0`

## Goal

Stop the midaz reporter from calling fetcher over HTTP. Embed fetcher's extraction
engine (`pkg/engine`, stdlib-only standalone module) in-process. Collapse the remote
job/notification/storage roundtrip into a synchronous in-process extraction, and unify
reporter-manager + reporter-worker into a single binary deployed split in production.

## Locked decisions

1. **Embed the engine in-process.** Replace the `pkg/reporter/fetcher` HTTP client with
   in-process `engine.New(...)` + `PlanExtraction` + `ExecuteExtraction`.
2. **Single binary, two runnables** (`RUN_MODE=api|worker|all`), built on the lib-commons
   Launcher pattern. **Deploy SPLIT in production from day 1** (two Deployments, one
   image: `RUN_MODE=api`, `RUN_MODE=worker`); `all` is for local dev / minimal envs only.
   Rationale: `all` shares blast radius and can't autoscale cleanly — split delivers
   failure isolation + independent KEDA scaling without ever paying that tax.
3. **Keep RabbitMQ** as the durable generate-report job queue. A financial reporter
   cannot silently drop a requested report on a pod restart.
4. **Transport-S3 dies; delivery-S3 stays.** The fetcher-writes-encrypted-blob →
   worker-downloads → decrypt → HMAC-verify roundtrip is theater against our own process
   memory once in-process — delete it. The final rendered report (PDF) stored in S3 for
   user download is unaffected.
5. **Engine carries zero third-party deps.** Its go.mod require block is empty;
   `require .../pkg/engine v1.0.0` does NOT bump midaz's lib-commons (v5.5.0 stays). This
   is the payoff of the standalone-module split and is verified in Phase 0.

## Architecture: before → after

**Before** (remote, async two-hop):
```
manager(:4005) --REST--> creates report, enqueues generate-report (RabbitMQ)
worker(:4006) --consume generate-report--> CreateExtractionJob (HTTP) --> fetcher service
fetcher --extract from DBs--> encrypt+HMAC blob --> S3 --> publish job.completed (RabbitMQ)
worker --consume notification--> download S3 --> decrypt --> verify HMAC --> render --> deliver S3
worker --reconciler (Redis lock)--> polls fetcher for stuck jobs
```

**After** (in-process, single hop):
```
manager (RUN_MODE=api) --REST--> creates report, enqueues generate-report (RabbitMQ)
                                 template authoring: in-process engine DiscoverSchema/ValidateSchema
worker  (RUN_MODE=worker) --consume generate-report-->
        engine.PlanExtraction + ExecuteExtraction (Direct mode, in-process, tenant-scoped)
        --> result map --> render --> deliver S3 --> mark report finished
```
No CreateExtractionJob HTTP, no fetcher.job.events consumer, no transport-S3, no
decrypt/HMAC-verify, no reconciler, no job→report mapping. Extraction is synchronous
inside the generate-report job handler.

## Engine port → reporter infra mapping

Engine v1.0.0 ports (post-cleanup; EventSink/TenantResolver/ExecutionStore.FindExecution
are removed):

| Engine port | Need | Reporter provides | Notes |
|---|---|---|---|
| **ConnectorRegistry** (required) | YES | NEW adapter: `ConnectorFactory`/`Connector` with `QueryStream→RowCursor` over `pkg/reporter/postgres` + `pkg/reporter/mongodb` | The core work. Cursor = streaming pg/mongo query. **Tenant-aware** (see critical risk). |
| **ConnectionStore** | for Plan/validate | READ-MOSTLY adapter over `pkg/reporter/datasource-config.go` + `safe-datasources.go` | Internal datasources are env-configured (`DATASOURCE_*`), not user-CRUD. Implement `FindConnection`/`FindByID`/`List`; `Create/Update/Delete/UpdateByID/DeleteByID/ListPaged` → unsupported error. |
| **CredentialProtector** | NO | — skip `WithEncryptedPersistence` | Creds come from env/secret-manager straight to connectors; nothing encrypted-at-rest in our Mongo. Drop the whole port. |
| **ResultSink** | optional | Direct mode default (in-memory → render); `OpenResultStream→S3` for huge extractions | Streaming sink (Phase 5) is the bounded-memory answer for big reports. |
| **SchemaCache** | optional | adapter over `pkg/reporter/redis` | Perf for manager template-authoring schema discovery. |
| **ExecutionStore** | optional | adapter over Mongo (`SaveExecution` only) | Durable extraction-state if wanted; reporter already persists report status. Likely optional. |
| **ActiveExecutionChecker** | NO | skip | Reporter job dedup is its own concern. |
| **Observability** | optional | adapter over the lib-observability tracer the reporter already wires | Free tracing continuity — wire it. |

Engine facade used: `engine.New(WithConnectorRegistry, WithConnectionStore,
WithResultSink?, WithSchemaCache?, WithObservability?, WithLimits)` →
`PlanExtraction(ctx, tenant, ExtractionRequest)` → `ExecuteExtraction(ctx, plan)`.
Connector contract: `Build → {TestConnection, DiscoverSchema, QueryStream→RowCursor, Close}`.

## Critical risk (the meatiest part)

**Multi-tenant DB resolution moves in-process.** The remote fetcher owned per-tenant
connection pooling and database-per-tenant resolution. In-process, the reporter's
`ConnectorRegistry`/`ConnectionStore` adapter must resolve the correct per-tenant
datasource from `TenantContext.TenantID`, using midaz's lib-commons tenant managers (the
same MT machinery the rest of midaz uses). This is where the migration can balloon. It
also **collapses the dual-provider split**: `DirectProvider` (single-tenant, direct DB)
and `FetcherProvider` (multi-tenant, remote) existed only because remote-fetcher was the
MT path. The in-process engine unifies them — one tenant-aware path, `FETCHER_ENABLED`
gate dies. Treat the tenant-aware connector adapter (Phase 1) as the critical path; spike
it first.

## Deletions (after migration)

- `pkg/reporter/fetcher/` — entire HTTP client (`client.go`, `client_management.go`,
  `client_extract.go`, `types.go`).
- `pkg/reporter/datasource/fetcher_provider.go` + the `DataSourceProvider` dual-provider
  abstraction; `FETCHER_ENABLED` gate in `factory.go`.
- `components/reporter-worker/internal/adapters/rabbitmq/notification_consumer.go`
  (Consumer 2, `fetcher.job.events`).
- `components/reporter-worker/internal/services/process-notification.go` (claim +
  download/decrypt/HMAC-verify pipeline).
- Transport parts of `data-pipeline.go`: `downloadExtractedData`, `decryptExtractedData`,
  `verifyHMACOrReject`.
- `components/reporter-worker/internal/services/reconciler.go` (reconciles stuck fetcher
  jobs — extraction is synchronous now).
- `ExtractionMapping` persistence (jobID→reportID) and its repository.
- `pkg/reporter/storage/fetcher_adapter.go` (transport download adapter).
- Env: `FETCHER_URL`, `FETCHER_ENABLED`, fetcher M2M token provider, transport external
  HMAC key.

## Retentions

- RabbitMQ generate-report queue + its consumer (Consumer 1).
- Render pipeline (template → PDF).
- Delivery S3 + `storage/s3-client.go`, `storage/seaweedfs-adapter.go` (final artifact).
- `pkg/reporter/postgres`, `pkg/reporter/mongodb` — now back the engine connectors.
- `pkg/reporter/redis` — SchemaCache + any locks.
- lib-observability tracing.

## Phasing (rolling wave — Phase 1 detailed, rest epic-level)

### Phase 0 — De-risk gate (DETAILED)
The whole premise is "embedding inherits zero deps." Prove it before building anything.
- 0.1 Add `require github.com/LerianStudio/fetcher/pkg/engine v1.0.0` to midaz go.mod;
  `go mod download`.
- 0.2 Throwaway file importing `engine` + `engine/memory`; `go build ./...`.
- 0.3 `go mod graph` / `go mod why` diff: assert NO new third-party module entered, and
  `lib-commons/v5` stays at v5.5.0, mongo-driver unchanged.
- **GATE**: if anything bumps, STOP — investigate whether the engine module is truly
  clean or pruning leaked something; decide before proceeding.

### Phase 1 — Tenant-aware connector adapter (DETAILED — critical path)
Location: `components/reporter-worker/internal/adapters/engine/` (or `pkg/reporter/engine/`).
- 1.1 `ConnectorRegistry`: map `datasourceType` (postgres/mongodb/…) → `ConnectorFactory`.
- 1.2 `ConnectorFactory.Build(ctx, descriptor)`: map `ConnectionDescriptor` (host/port/db/
  type + creds) to a connection over the existing `pkg/reporter/postgres` / `mongodb`
  pools + circuit breakers.
- 1.3 `Connector.QueryStream(ctx, ExtractionRequest) → RowCursor`: translate
  `request.MappedFields` to SELECT/projection; return a cursor. pg: pgx rows; mongo:
  driver cursor. Respect `ctx` cancellation.
- 1.4 `RowCursor`: `Next/Row(table,row)/Err/Close` — one row at a time, bounded memory.
- 1.5 `DiscoverSchema → SchemaSnapshot` reusing existing schema introspection.
- 1.6 `TestConnection`, `Close`.
- 1.7 **Tenant resolution**: factory resolves the per-tenant DB connection from
  `TenantContext.TenantID` via lib-commons tenant managers. Single-tenant path = stable
  tenant. This is the spike — do it first and validate isolation.
- 1.8 Integration tests (testcontainers pg + mongo): stream rows, projection, filters,
  tenant isolation, ctx-cancel mid-stream, and a large-result test asserting bounded
  memory (cursor, not load-all).

### Phase 2 — Wire engine into the worker
- ConnectionStore (read-mostly over datasource-config) + Observability adapter; optional
  SchemaCache (Redis), ExecutionStore (Mongo). `engine.New(...)` in worker bootstrap with
  `Validate()` fail-fast. No CredentialProtector / WithEncryptedPersistence.

### Phase 3 — Swap the worker job handler
- generate-report handler: in-process `PlanExtraction` + `ExecuteExtraction` (Direct mode)
  → result map → render → deliver S3 → mark finished. Delete notification consumer,
  process-notification, reconciler, ExtractionMapping.

### Phase 4 — Manager schema/validate in-process
- Replace `FetcherProvider` `ListDataSources`/`ValidateSchema` with an in-process
  schema-scoped engine (DiscoverSchema/ValidateSchema), mirroring fetcher's
  connection_engine/schema_engine host pattern.

### Phase 5 — Store-mode streaming sink (bounded memory for big reports)
- Implement `ResultSink.OpenResultStream → ResultStreamWriter` over delivery S3 (or a
  staging area) so huge extractions stream to disk/object-store with constant memory
  instead of materializing in RAM. Choose Direct vs Store per report size threshold.

### Phase 6 — Single binary + split deploy
- `RUN_MODE=api|worker|all` runnable gating on the Launcher. **Mode-aware config
  validation** (api-only must not require S3/RabbitMQ; worker-only must not require HTTP
  TLS) and **mode-aware /readyz** probe sets. Helm: two Deployments, one image, split in
  prod; KEDA on queue depth for worker. Delete the fetcher HTTP client package, `FETCHER_*`
  env, transport-S3/HMAC. Collapse the separate reporter-worker deploy unit.

### Phase 7 — Cleanup
- Remove DirectProvider/FetcherProvider abstraction, dead env, update docs / llms-full.txt
  / Helm values / postman.

## Sequencing decision

Recommended: **embed first (Phases 0–5 into the existing reporter-worker), consolidate the
binary after (Phase 6).** Embedding is the substance and is isolated/lower-risk; the
binary consolidation is orthogonal plumbing. Doing it second means you prove the engine
works in-process before restructuring the process model. The alternative (consolidate
binary first) front-loads the deploy-topology change onto code that still calls HTTP — more
churn for no early validation.

## Verification per phase
- Phase 0: `go mod graph` diff clean, lib-commons unchanged.
- Phase 1: testcontainers integration green, bounded-memory + tenant-isolation tests.
- Phase 3: end-to-end generate-report produces the same rendered artifact as the HTTP path
  (golden-file compare against a pre-migration report).
- Phase 6: api-only and worker-only pods each boot with only their config; `/readyz`
  correct per mode; split Deployments scale independently.
