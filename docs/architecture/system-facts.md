# Ledger / System Facts — the `midaz.transaction_committed` producer

The ledger is a System Facts producer through `github.com/LerianStudio/lib-system-facts`.
It records one fact, `midaz.transaction_committed`, inside the PostgreSQL transaction that
commits an APPROVED transaction row, and a forwarder delivers it from the kit's outbox table.
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
| `action` | none | `direct`, `commit` or `revert`: the ledger transition that reached APPROVED |
| `status` | none | always `APPROVED` |
| `direction` | none | `INFLOW` (every source leg is `@external/`), `OUTFLOW` (every destination leg is), `INTERNAL` (no external leg), `MIXED` |
| `amount`, `assetCode` | none | the amount the sender authorized, as a plain decimal at the asset's scale; fee legs are listed in `operations` |
| `chartOfAccountsGroupName`, `scheme`, `parentTransactionId`, `groupId`, `createdAt` | none | optional except `createdAt`; `parentTransactionId` only on `revert` |
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
transaction, revert v1/v2, atomic batch and cross-ledger direct/commit/revert, fee-debt
collection, the asynchronous write-behind consumer (`RABBITMQ_TRANSACTION_ASYNC=true`) and the
engine recovery runner's replay.

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
  the Store rolls back and the completion is treated as already done.

## Configuration

`components/ledger/.env.example` holds the block; `components/ledger/internal/bootstrap/config.go`
holds `validateSystemFactsConfig`.

| Variable | Default | Role |
|---|---|---|
| `SYSTEM_FACTS_ENABLED` | `false` | Builds the producer, runs the forwarder, registers the readiness check; requires `PLUGIN_AUTH_ENABLED=true` |
| `SYSTEM_FACTS_URL` | — | Delivery target and preflight registry; required when enabled |
| `SYSTEM_FACTS_M2M_CLIENT_ID` / `_SECRET` | — | Single-tenant producer credential, both required when enabled |
| `SYSTEM_FACTS_INSECURE_HTTP` | `false` | Plain `http://` to a non-loopback host; `DEPLOYMENT_MODE=local` only |
| `SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS` | `10000` | Bound on the boot preflight, `1000..60000` |
| `SYSTEM_FACTS_RETENTION_INVALID_HOURS` | `0` | Sweep of `INVALID` outbox records; `0` keeps them |

Credentials: single-tenant uses the static pair. Multi-tenant reads each tenant's own credential at
`tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/system-facts/credentials` on the
`M2M_SECRETS_BACKEND` backend (`aws` or `vault`, `M2M_VAULT_MOUNT`), cached for five minutes so
a rotated secret is picked up without a restart; the static pair is never a fallback there,
because its token would ingest every tenant's facts into one tenant.

Access Manager prerequisites: `AUTH_M2M_INVERSION_ENABLED=true`; the producer application
`midaz` created with a `source` (otherwise the service refuses deliveries with SF-0028); every
credential issued or rotated after the `sourceService` claim exists (otherwise SF-0027); the
reconciler identity granted `catalog:reserved` (role `lerian-producer`) to register `lerian.*`
names. The kit's source-service check stays at its Warn default until every credential is rotated.

## Rollout order

1. System Facts is up and its `/readyz` is green in the target environment.
2. The ledger's reconciler runs as a pre-sync Job over `contracts/system-facts` and exits `0`
   (`1` rejected, `2` unavailable, `3` invalid; any non-zero exit keeps the previous release).
3. Migration `000046` (`system_facts_outbox`) is applied in every transaction database: the
   single-tenant database, or every tenant database through the tenant-manager migration image.
4. Multi-tenant: one M2M credential per tenant at the path above.
5. The ledger rolls out with `SYSTEM_FACTS_ENABLED=true`. Pix and Fees declare
   `lerian.midaz.transaction_id` as external and wait for the Midaz reconciler
   (`--external-type-wait`).

Helm values and the reconciler Job live in the downstream GitOps repo, not here.

## Operations

- **Boot**: `buildSystemFacts` (`bootstrap/system_facts.go`) compiles the embedded contracts,
  builds the token source, transport, producer and registry client, then runs
  `reconciler.Preflight` under `SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS`. Only `OK` wires the
  validating emitter; any other outcome wires the kit's no-op emitter, logs a Warn and counts
  `system_facts_preflight_total{outcome}`. Boot is never refused for a preflight outcome.
- **Forwarder**: the `System Facts Forwarder` Launcher app (`bootstrap/system_facts_forwarder.go`)
  runs `producer.Forwarder().Run` on a recovered goroutine whenever a producer exists, even with
  emission off, so a backlog recorded before a failed preflight still drains. On SIGINT/SIGTERM it
  calls `Shutdown` under a 30s budget, the server's graceful-drain budget.
- **Readiness**: `/readyz` carries `system_facts_forwarder` whenever `SYSTEM_FACTS_ENABLED=true`:
  `idle` and `running` are `up`, `stopped` and `failed` are `down` with the fixed text
  `forwarder <state>`; the forwarder's own error is logged at Warn and never written to the
  response. With emission off the check stays `up` and its `reason` reads
  `emission disabled by contract preflight`. The outbox depth is not probed: its error can carry
  tenant ids and hosts. `tls` is `true` when `SYSTEM_FACTS_URL` is `https://`, and
  `DEPLOYMENT_MODE=saas` refuses boot otherwise, as for every other dependency.
- **Metrics**: `system_facts_preflight_total{outcome}` (ledger) and the kit's
  `system_facts_emit_refused_total{reason,type}`, `system_facts_delivery_total{route,outcome}`,
  `system_facts_outbox_depth{status}`, `system_facts_outbox_oldest_age_seconds`,
  `system_facts_outbox_depth_sample_errors_total` and
  `system_facts_outbox_terminal_failures_total{reason,route}`. Aggregate the gauges across
  replicas with `max()`.
- **Alert**: `increase(system_facts_emit_refused_total[5m]) > 0` — every refusal is a producer
  defect and a lost fact. Pair any depth or age alert with one on
  `system_facts_outbox_depth_sample_errors_total`.

## Pending

- **Security**: the retention of `INVALID` outbox records (`SYSTEM_FACTS_RETENTION_INVALID_HOURS`,
  which hold `description` in clear until swept), the classes chosen for the payload
  (`description` encrypted, `accountId` in clear as an opaque system id) and catalog gate G05
  (the sensitive-reference review of the System Facts service).
- **Release**: the kit is pinned at `lib-system-facts v1.0.0-beta.8` (lib-auth is on the stable
  `v5.6.0`). The pre-release gate warns on `develop` and blocks PRs to `release-candidate` and
  `main` until a stable kit tag exists.
