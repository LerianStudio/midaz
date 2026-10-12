# Ledger / System Facts — the `midaz.transaction_committed` producer

The ledger is a System Facts producer through `github.com/LerianStudio/lib-system-facts`,
pinned at the stable `v1.0.0`. It records one fact, `midaz.transaction_committed`, inside the
PostgreSQL transaction that commits an APPROVED transaction row, and a forwarder delivers it
from the kit's outbox table once the contract the ledger reconciles at boot is active at System
Facts.
Everything below is grounded in code under `components/ledger/`; the cited symbols are the
durable anchors.

## What is recorded

| Item | Value |
|---|---|
| Producer / namespace / team | `midaz` / `lerian.midaz.transactions` / `mazar` (`contracts/system-facts/manifest.yaml`) |
| Fact type / version / shape | `midaz.transaction_committed` / `1.0.0` / event-record, `TRANSITIVE` compatibility |
| Subject | `{type: "midaz.transaction", id: <transaction id>}` |
| `occurredAt` | the `updated_at` the completion writes on the row: equal to `created_at` for `direct`/`revert`, the commit time for a PENDING→APPROVED `commit` |
| `factId` | UUIDv5 over the fixed namespace `ee4ae017-0733-48aa-bf59-b31995b9b223` and `midaz.transaction_committed:<transaction id>` (`adapters/systemfacts/contract.go`). APPROVED is terminal and reached once per transaction, so a retried completion derives the same id and the kit deduplicates it |
| Reference | `lerian.midaz.transaction_id`, owner self, scope `[ledgerId, organizationId]`, sensitivity none, correlation exact, searchable false; cardinality `one`, required. It is the join Pix and Fees declare as external |

Payload (`contracts/system-facts/schemas/midaz.transaction_committed/1.0.0.json`; root and
items carry `additionalProperties: false`):

| Field | Class | Notes |
|---|---|---|
| `action` | none | `direct`, `commit` or `revert`: the ledger transition that reached APPROVED. The destination rows a cross-ledger v2 commit creates are new rows and carry `direct`; their `groupId` tells them apart from a singular create |
| `status` | none | always `APPROVED` |
| `direction` | none | derived from the legs by side: a leg is a source when `operation.direction` is debit and a destination when it is credit (`type` decides only when `direction` is empty), and fee legs are ignored. `INFLOW` when every source leg is external (`@external/`) and no destination leg is; `OUTFLOW` the mirror; `INTERNAL` when no leg is external; `MIXED` otherwise |
| `amount`, `assetCode` | none | the amount the sender authorized, as a plain decimal string with trailing zeros trimmed (`10.5`, never `10.50` or `1.05E1`); per-leg amounts in `operations` use the same form |
| `createdAt` | none | the transaction's effective date (`transactionDate`) as stored in `created_at`; it differs from `occurredAt` on a `commit` |
| `chartOfAccountsGroupName`, `scheme`, `parentTransactionId`, `groupId` | none | optional; `parentTransactionId` only on `revert`, `groupId` only on the parts of a cross-ledger group |
| `description` | **encrypted** (`x-sensitivity`) | free text; revealed only with `facts:reveal` |
| `operations[]` | none | `id`, `type`, `amount`, `assetCode`, `chartOfAccounts`, `accountId` (opaque system id, in clear by team decision), `external`, `feeLeg` |

Aliases, balances, metadata, routes, the control-skip flags and the organization/ledger ids are
not in the payload: the last two travel as the reference scope.

## Where it is recorded

The single seam is `completion.Store` (`adapters/postgres/completion/completion.postgresql.go`),
whose `PersistWithOutcome` and `PersistBulkWithOutcome` run one SQL transaction: `BeginTx` on the
primary pool (a `*sql.Tx`), `SELECT … FOR UPDATE`, the `transaction` row insert or the
PENDING→APPROVED CAS update, the bulk `operation` insert, verify, then `Commit`. Between the
verify and the `Commit`, for every row whose lifecycle phase is `created` or `updated` (never
`noop`, the verified replay), the Store calls the `command.TransactionFactRecorder` port
(`services/command/transaction_fact_recorder.go`). Its implementation,
`adapters/systemfacts.Recorder`, builds the payload from the write set and calls the kit's
`Emit` with that `*sql.Tx`, so the fact and the row commit or roll back together.

Every writer converges there: create v1/v2 (json, inflow, outflow), commit of a pending
transaction, revert v1/v2, atomic batch and cross-ledger direct/commit/revert (a cross-ledger
commit approves the PENDING origins as `commit` and creates the destinations as new `direct`
rows), fee-debt collection, the asynchronous write-behind consumer
(`RABBITMQ_TRANSACTION_ASYNC=true`) and the engine recovery runner's replay.

Deliberately not a fact: `hold` (PENDING, money only reserved), `cancel` (CANCELED, nothing
moved), NOTED annotations (the legacy per-leg writer runs no SQL transaction), the legacy
`backup_queue:{transactions}` replays and legacy-format RabbitMQ messages, the
`transaction_group` row, and a transaction parked in `transaction_backup_quarantine` until an
operator resolves it, because it never reaches the Store. Nothing is ever emitted from a handler,
a use case, or outside the completion transaction.

## Failure policy

- **Validation refusal** (`emitter.IsValidationRefusal`): the row commits without its fact. The
  recorder logs a Warn with `transaction_id` and the kit counts
  `system_facts_emit_refused_total{reason,type}`. The fact is lost and rebuildable from the row.
- **Kit or driver error**: the recorder returns it, the Store rolls the transaction back and the
  engine recovery runner (`engine:{transactions}:recover`) replays the completion; the movement
  was already applied in Redis/Lua and the replay is idempotent.
- **`outbox.ErrFactAlreadyRecorded`**: wrapped as `systemfacts.ErrTransactionFactAlreadyRecorded`,
  the Store rolls the transaction back and the recovery runner replays the record. It is
  unreachable by construction: the fact and the row commit in one transaction, and a replay of a
  committed row is `noop` before the recorder runs. The branch exists so an unexpected duplicate is
  never committed a second time.

## Configuration

`components/ledger/.env.example` holds the block; `components/ledger/internal/bootstrap/config.go`
holds `validateSystemFactsConfig`.

| Variable | Default | Role |
|---|---|---|
| `SYSTEM_FACTS_ENABLED` | `false` | Records the fact from the first commit, reconciles the contract at boot, runs the forwarder once it is active, registers the readiness check; requires `PLUGIN_AUTH_ENABLED=true` |
| `SYSTEM_FACTS_URL` | — | Delivery target and contract registry; required when enabled |
| `SYSTEM_FACTS_M2M_CLIENT_ID` / `_SECRET` | — | Required in both modes: single-tenant the producer's own credential (delivery and registration); multi-tenant the tenantless service credential that registers the contract, delivery being per tenant |
| `SYSTEM_FACTS_INSECURE_HTTP` | `false` | Plain `http://` to a non-loopback host; accepted only with `DEPLOYMENT_MODE=local` set explicitly (an unset mode, which elsewhere defaults to local, refuses boot) |
| `SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS` | `60000` | Per-call bound on each `Preflight`/`Run` of the boot reconcile, `5000..600000` |
| `SYSTEM_FACTS_RETENTION_INVALID_HOURS` | `0` | Sweep of `INVALID` outbox records; `0` keeps them |
| `SYSTEM_FACTS_RETENTION_PUBLISHED_HOURS` | `0` | Sweep of `PUBLISHED` outbox records; `0` is the kit's 7 days |
| `SYSTEM_FACTS_FORWARDER_BATCH_SIZE` | `0` | Records a dispatch cycle claims; `0` is the kit's 10 |
| `SYSTEM_FACTS_FORWARDER_DISPATCH_INTERVAL_MS` | `0` | Cycle cadence; `0` is the kit's 500 ms |
| `SYSTEM_FACTS_FORWARDER_PROCESSING_TIMEOUT_MS` | `0` | Lifetime of a claim; `0` is the kit's 30 min. The kit requires it to exceed `BatchSize × PublishMaxAttempts (3) × DeliveryTimeout (45 s)`, else boot refuses and names the formula |
| `SYSTEM_FACTS_SOURCE_SERVICE_CHECK` | `warn` | `warn` logs a failing `sourceService` claim once per token source and still delivers; `require` fails the grant so facts wait in the outbox; anything else refuses boot |

A negative tuning or retention value refuses boot. Capacity: one replica delivers at most
`BatchSize / DispatchInterval` facts per second per dispatch scope (20/s with the kit defaults).
Above a sustained arrival rate the outbox grows; the kit's `system_facts_outbox_depth{status="pending"}`
gauge is the signal to raise the batch or shorten the interval, keeping the processing timeout
above the claim bound.

Credentials: the static `SYSTEM_FACTS_M2M_*` pair is the service credential and is required in
both modes. Single-tenant it is the producer's own credential and authenticates both delivery and
the contract registration, because the contract owner is the verified subject of the token that
delivers. Multi-tenant it is a tenantless credential that only registers the contract: it is
exchanged through the same `accessmanager.NewTokenSource` with the same options and must be issued
with the same `source` as every tenant credential, so the owner is the same `svc:<sourceService>`.
Multi-tenant delivery reads each tenant's own credential at
`tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/system-facts/credentials` on the
`M2M_SECRETS_BACKEND` backend (`aws` or `vault`, `M2M_VAULT_MOUNT`); the static pair never
authenticates a tenant's delivery, because its token would ingest every tenant's facts into one
tenant. Each tenant's pair is cached in-process for five minutes (`tracer.WithCredentialTTL`): an
entry older than that is read again on the next lookup, and a read that fails serves the cached
pair and logs once at Warn until a read succeeds (stale-while-revalidate), so a rotated secret
reaches the token grant without a restart and a secret-store outage never stops delivery. This is
a documented deviation from the multi-tenant standard's two-level M2M credential cache (L1
in-process plus L2 in Valkey): the kit asks for the pair only when it mints a token, so a per-pod
cache is enough and no shared L2 is read or invalidated.

Access Manager prerequisites: `AUTH_M2M_INVERSION_ENABLED=true`; the producer application
`midaz` created with a `source` (otherwise the service refuses deliveries with SF-0028); every
credential issued or rotated after the `sourceService` claim exists (otherwise SF-0027); the
registration credential (the `SYSTEM_FACTS_M2M_*` pair) granted `catalog:reserved` (role
`lerian-producer`) to register `lerian.*` names. Keep `SYSTEM_FACTS_SOURCE_SERVICE_CHECK=warn`
until every credential (every tenant's, in multi-tenant mode) was issued after the claim existed,
then switch to `require`: a bad credential then fails the grant and its facts wait in the outbox
instead of being refused by the service.

## Rollout order

1. System Facts is up and its `/readyz` is green in the target environment.
2. Migration `000046` (`system_facts_outbox`) is applied in every transaction database: the
   single-tenant database, or every tenant database through the tenant-manager migration image.
3. Credentials: the `SYSTEM_FACTS_M2M_*` pair in both modes (granted `catalog:reserved`);
   multi-tenant also one M2M credential per tenant at the path above.
4. The ledger rolls out with `SYSTEM_FACTS_ENABLED=true`, which can be on from the first
   rollout: the app records from the first commit and delivers once its boot reconcile reaches
   `OK`. Pix and Fees declare `lerian.midaz.transaction_id` as external and wait for it
   (`--external-type-wait`), so they keep retrying until the ledger has registered it.

There is no pre-sync Job, no init container and no deploy step: the app reconciles its own
contract at boot. The kit's `reconciler` command remains an operator tool, to register the
contract ahead of a rollout or to diagnose a refusal. Helm values live in the downstream GitOps
repo, not here.

## Operations

- **Boot**: `buildSystemFacts` (`bootstrap/system_facts.go`) compiles the embedded contracts,
  builds the token sources (delivery and registration), transport, producer, registry client and
  the contract gate (`bootstrap/system_facts_reconcile.go`). With `SYSTEM_FACTS_ENABLED=true` the
  recorder wraps the kit's recording emitter from the first commit; nothing gates the emitter, and
  the flag off keeps the kit's no-op emitter. Boot is never refused for a registry outcome.
- **Contract reconcile**: the `System Facts Forwarder` Launcher app
  (`bootstrap/system_facts_forwarder.go`) first reconciles the ledger's own contract on its
  recovered goroutine, in the background and fail-open. One attempt is `reconciler.Preflight`
  (read-only): `OK` releases the forwarder; a gap (`Rejected`) runs `reconciler.Run` (PUT and
  read-back), whose `OK` releases. `Unavailable` from either call, and a `Run` refused with a 404
  (an ingress default backend mid-rollout, or a `SYSTEM_FACTS_URL` naming something else), waits
  `FullJitter(min(Exponential(5s, attempt-1), 5min))` and retries without limit while the process
  lives. `Rejected` or `Invalid` from `Run`, and `Invalid` from `Preflight`, are permanent: one
  Error log, the forwarder stays stopped and only a redeploy retries, because the inputs (manifest,
  credential, URL) are static under a running process. Each call is bounded by
  `SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS`. The gate moves `pending` → `active` | `rejected`.
  No cross-replica lock is taken: the kit's `Run` is idempotent (a rerun on an unchanged manifest
  answers `Replayed`) and the service serialises concurrent PUTs. Records stay `PENDING` in the
  outbox until the forwarder starts, and none turns `INVALID` while waiting, because a record
  turns `INVALID` only through dispatch attempts. A shutdown while the loop waits ends it quietly
  with the gate still pending.
- **Forwarder**: once the contract is active the same app runs `producer.Forwarder().Run` on that
  goroutine. On SIGINT/SIGTERM it calls `Shutdown` under a 30s budget, the server's graceful-drain
  budget, whether or not the contract was released. A `Run` that returns before that signal, with
  or without an error, is logged at Error and restarted with backoff (1s, doubling, capped at 30s,
  reset after a run longer than a minute); each restart counts
  `system_facts_forwarder_restarts_total`. The kit allows the restart: `beginRun` refuses a new run
  only after `Shutdown`.
- **Readiness**: `/readyz` carries `system_facts_forwarder` whenever `SYSTEM_FACTS_ENABLED=true`
  and never reports it down: the money path does not depend on delivery, so the forwarder state
  never takes the pod out of rotation. While the gate is pending the reason is `contract not
  active`; after a permanent refusal it is `contract rejected`. Once the contract is active,
  `idle` and `running` are `up` with no reason; `stopped` and `failed` are `up` with the fixed
  reason `forwarder <state>; facts are not draining on this pod`, and the forwarder's own error
  is logged at Warn, never written to the response. The outbox depth is not probed: its error can
  carry tenant ids and hosts.
  `tls` is `true` when `SYSTEM_FACTS_URL` is `https://`, and `DEPLOYMENT_MODE=saas` refuses boot
  otherwise, as for every other dependency.
- **Logs**: the reconcile logs Info `System Facts contract active` (`source` `preflight` or
  `reconcile`, `created`/`replayed`/`verified` step counts), Warn `System Facts contract reconcile
  deferred` (`attempt`, `outcome`, `next_retry_in`, `step_kind`, `step_subject`) per retry, and
  once Error `System Facts contract rejected; the forwarder stays stopped` (`outcome`,
  `step_kind`, `step_subject`, `problem_code`, `problem_detail` sanitized). The step detail is
  never logged: a registry client error embeds the target URL.
- **Metrics**: `system_facts_contract_reconcile_total{outcome}` (`ok`, `unavailable`,
  `rejected`, `invalid`; one sample per attempt) and
  `system_facts_forwarder_restarts_total` (ledger) and the kit's
  `system_facts_emit_refused_total{reason,type}`, `system_facts_delivery_total{route,outcome}`,
  `system_facts_outbox_depth{status}`, `system_facts_outbox_oldest_age_seconds`,
  `system_facts_outbox_depth_sample_errors_total` and
  `system_facts_outbox_terminal_failures_total{reason,route}`. Aggregate the gauges across
  replicas with `max()`.
- **Alerts**: `increase(system_facts_contract_reconcile_total{outcome=~"rejected|invalid"}[10m]) > 0`
  — the contract was refused and nothing delivers from that pod until a redeploy fixes the
  manifest, the credential or the URL; a sustained `unavailable` rate means the registry is
  unreachable while facts accumulate `PENDING`.
  `increase(system_facts_emit_refused_total[5m]) > 0` — every refusal is a producer
  defect and a lost fact. `increase(system_facts_forwarder_restarts_total[10m]) > 0` — the
  forwarder keeps stopping and nothing drains on that pod while readiness stays green.
  `max(system_facts_outbox_depth{status="pending"})` sustained above the capacity ceiling — the
  forwarder is below the arrival rate: raise `SYSTEM_FACTS_FORWARDER_BATCH_SIZE` or lower
  `SYSTEM_FACTS_FORWARDER_DISPATCH_INTERVAL_MS`. Pair any depth or age alert with one on
  `system_facts_outbox_depth_sample_errors_total`.

## Pending

- **Security**: the retention of `INVALID` outbox records (`SYSTEM_FACTS_RETENTION_INVALID_HOURS`,
  which hold `description` in clear until swept), the classes chosen for the payload
  (`description` encrypted, `accountId` in clear as an opaque system id) and catalog gate G05
  (the sensitive-reference review of the System Facts service).
- **Outside this repo**: lib-system-facts resolves the kit-table schema inside `Record` once per
  record; resolving it once per `*sql.Tx` would drop one query per fact. lib-commons'
  `ManagerPoolResolver.ListTenants` is called on every dispatch cycle and has no TTL cache. The
  toolchain pin `go1.27.2` carries advisories to review at the next bump.
