---
_meta:
  schema_version: "1.0.0"
  service_name: midaz
  source_commit_sha: cfd912cf64d4886a2089e71364e2eb297bb24741
  lib_observability_version: v2.1.1
  lib_commons_version: v6.7.0
  midaz_major_version: v4
  environment_surveyed: internal dev single-tenant (namespace midaz-dev-st)
  generation: hand-curated
---

# Telemetry Dictionary — Midaz

Contract between the Go telemetry emission sites and the dashboards in this directory.

## Provenance — read this before trusting a row

This dictionary is **hand-curated, not machine-generated.** Saying otherwise would
misrepresent how it was produced, so the header above records `generation: hand-curated`
and there is no "do not edit by hand" banner.

Each row was established one of two ways:

| Source | What it establishes | Confidence |
|---|---|---|
| `pkg/utils/metrics.go` (the central registry) | Declared name, unit, description | Exact — read from source |
| Live Mimir series in the surveyed environment | Wire name, actual labels, whether it is emitted at all | Exact — queried directly |

Label sets come from Mimir rather than from code, because the wire is what a dashboard
query binds to. Where the two disagree, the wire wins and the disagreement is called out.

Not established: per-call-site `file:line` emission maps for counters and histograms, and
the span/cross-cutting inventory. The sweep that would have produced them returned results
for the wrong repository and was discarded rather than transcribed. Treat the
`emission_sites` absence as unknown, not as zero.

## Wire-name translation (OTLP → Prometheus)

The single most common source of a broken panel. The declared name in Go is **not** the
series name in Mimir. The OTLP-to-Prometheus translation appends a suffix derived from the
instrument type and unit:

| Declared in Go | Unit | Instrument | Series in Mimir |
|---|---|---|---|
| `balance_synced` | `1` | Counter | `balance_synced_total` |
| `domain_operations_total` | `1` | Counter | `domain_operations_total` |
| `domain_operation_duration_ms` | `ms` | Histogram | `domain_operation_duration_ms_milliseconds` |
| `readyz_check_duration_ms` | `ms` | Histogram | `readyz_check_duration_ms_milliseconds` |
| `bulk_recorder_bulk_duration_ms` | `ms` | Histogram | `bulk_recorder_bulk_duration_ms_milliseconds` |
| `redis_backup_queue_depth` | `1` | Gauge | `redis_backup_queue_depth_ratio` |
| `balance_sync_batch_failures_total` | `1` | Counter | `balance_sync_batch_failures_total` |
| `balance_sync_cleanup_failures_total` | `1` | Counter | `balance_sync_cleanup_failures_total` |
| `balance_sync_tenant_skip_total` | `1` | Counter | `balance_sync_tenant_skip_total` |
| `balance_sync_orphan_dropped_total` | `1` | Counter | `balance_sync_orphan_dropped_total` |
| `balance_sync_last_success_timestamp` | `s` | Gauge | `balance_sync_last_success_timestamp_seconds` |

Two consequences worth internalising:

- The `_ms_milliseconds` double suffix is expected, not a typo. A name already ending in
  `_ms` still receives the unit suffix.
- `redis_backup_queue_depth_ratio` is **not a ratio.** It is an absolute count of queued
  items; the `_ratio` suffix is an artefact of `Unit: "1"` on a gauge.
- A name that already ends in `_total` is **not** doubled, which is why the four
  `balance_sync_*_total` counters reach Mimir unchanged. The unit suffix has no such
  dedup, so `balance_sync_last_success_timestamp` is declared **without** `_seconds`:
  spelling it in Go would land the series as `..._seconds_seconds`.

## Environment and scope selectors

```promql
# Application metrics (emitted by the Go binaries)
{job="ledger", k8s_namespace_name="<namespace>"}

# Infrastructure metrics (cAdvisor, kube-state-metrics)
{namespace="<namespace>"}
```

`namespace` and `k8s_namespace_name` hold the same value but are **not interchangeable**:
the `namespace` label exists only on infrastructure series. Filtering an application metric
by `namespace` returns an empty result with no error.

Jobs: `ledger` (the unified binary), `midaz-crm`, `tracer`.
The `k8s_namespace_name` label enumerates the deployed midaz environments; the dashboards
expose it as the Namespace variable rather than pinning any one of them.

`job` is not optional on an application query. A namespace may also run the pre-unification
standalone `midaz-crm` deployment alongside the v4 binary, and it emits the same
`crm_protection_*` series. A dashboard's `pod` variable is populated `job="ledger"`-only,
but its `All` value expands to the regex `.*` and matches `midaz-crm` pods as well, so a
query that omits `job` sums two separate services.

#### Emission continuity

<!-- Heading level is deliberate: verify-dashboard-primitives.sh parses `### ` headers as
     the registry of documented metric names, so prose sections must not use that level. -->

Series in this namespace are **intermittent, not continuous.** Every `live_observed: true`
row below means "this metric is emitted", never "this metric has a sample right now". Two
independent causes:

| Cause | Observation |
|---|---|
| Pod churn | Cumulative counters reset per pod generation, and a series only reappears once the new pod serves a request exercising that label combination. Continuous redeploys make this the dominant effect in non-production. |
| Burst traffic | HTTP series count sits at a health-probe-only baseline, spikes for the duration of a test run, then falls back. Business writes are concentrated in those bursts. |

Both are quantified for the surveyed environment in § Surveyed observations.

**Rule for panel authors: establish a metric's existence with `count_over_time` over a day,
never with an instant query.** An instant query carries a 5-minute lookback, so in this
namespace it tests whether a burst is running, not whether the metric exists. The difference
is not subtle — at an idle moment:

```promql
count(domain_operations_total{k8s_namespace_name="<namespace>"})                  # => no data
count(count_over_time(domain_operations_total{k8s_namespace_name="<namespace>"}[1d]))  # => hundreds
```

The same trap applies to range windows. `increase()` needs two samples inside its window,
and the Mimir datasource declares `timeInterval: 30s`, so `$__rate_interval` floors at 2m
while `$__interval` resolves to well under a minute at the 1h and 6h ranges. Measured against
a live environment: `increase(...[30s])` and `increase(...[1m])` return no series; `[2m]` and
`[4m]` return 10. Panels use `$__rate_interval` for this reason.

---

## Counters

### domain_operations_total

```yaml
declared_at: pkg/utils/metrics.go:21
description: Count of business operations by component, operation and result. Emitted from RecordDomainOperation at the single exit boundary of a use case.
emitter: pkg/utils/metrics.go:41 (RecordDomainOperation)
labels: [component, operation, result]
label_values_observed:
  operation: [calculate_fee, create_account, create_asset, create_holder_with_id, create_ledger, create_organization, get_account, get_organization, list_accounts, list_ledgers, list_organizations, create_transaction]
  result: [success, business_error, technical_error]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

This is the primary business-health metric and the **only** reliable error signal for the
domain layer — see the `calls_total` note on span status below.

### balance_synced_total

```yaml
declared_at: pkg/utils/metrics.go:87
declared_name: balance_synced
description: Number of balances synced by the balance sync worker.
labels: [organization_id, ledger_id, tenant_id, mode]
label_cardinality_estimate: unbounded
live_observed: true
unit: "1"
```

Carries UUID labels. Aggregate without them. `tenant_id` is empty in single-tenant, which
Prometheus treats as absent, so the pre-existing single-tenant series keep their identity.

### balance_sync_batch_failures_total

```yaml
declared_at: pkg/utils/metrics.go:95
declared_name: balance_sync_batch_failures_total
description: Total batch sync operation failures.
labels: [organization_id, ledger_id, tenant_id]
label_cardinality_estimate: unbounded
live_observed: unknown
unit: "1"
```

The primary failure signal for the sync pipeline, but blind to a stall in which nothing fails
because nothing runs — see `balance_sync_last_success_timestamp_seconds`.

### balance_sync_cleanup_failures_total

```yaml
declared_at: pkg/utils/metrics.go:105
declared_name: balance_sync_cleanup_failures_total
description: Total balance sync schedule cleanup failures.
labels: [organization_id, ledger_id, tenant_id]
label_cardinality_estimate: unbounded
live_observed: unknown
unit: "1"
```

Rising here means keys the flush had finished with were not removed from the schedule, so the
next cycle reprocesses them. Wasteful, not incorrect.

It does **not** imply the balances were persisted. Two paths emit it: after a successful
database write, and the all-orphans early return, which cleans up expired or unparseable keys
without writing anything. Read it alongside `balance_sync_orphan_dropped_total` to tell them
apart.

### balance_sync_tenant_skip_total

```yaml
declared_at: pkg/utils/metrics.go:113
declared_name: balance_sync_tenant_skip_total
description: Total tenants skipped by the balance sync worker due to connection resolution failure.
labels: [tenant_id]
label_cardinality_estimate: low
live_observed: unknown
unit: "1"
```

Bounded by the tenant set. A tenant appearing here is not syncing at all.

### balance_sync_orphan_dropped_total

```yaml
declared_at: pkg/utils/metrics.go:141
declared_name: balance_sync_orphan_dropped_total
description: Total scheduled balance sync keys dropped without persisting (value expired or unparseable).
labels: [organization_id, ledger_id, tenant_id, reason]
label_values_observed:
  reason: [expired, unparseable]
label_cardinality_estimate: unbounded
live_observed: unknown
unit: "1"
```

`reason="expired"` is data loss: the pending delta is unrecoverable. `reason="unparseable"` is
a key-format regression. Alert on them separately — the rules and the response procedure live
in the runbooks repository, under `midaz/troubleshooting/balance-sync-alerting.md`.

### balance_sync_last_success_timestamp_seconds

```yaml
declared_at: pkg/utils/metrics.go:131
declared_name: balance_sync_last_success_timestamp
description: Unix timestamp of the last successful balance batch sync.
labels: [organization_id, ledger_id, tenant_id]
label_cardinality_estimate: unbounded
live_observed: true
unit: "s"
```

Absolute unix timestamp, not an age — query with `time() - metric` so no periodic re-emission
is needed. The series only exists for a scope that has completed a batch since the pod booted,
so a staleness alert cannot rest on an absent series.

### db_read_source_total

```yaml
declared_at: pkg/utils/metrics.go:149
description: Count of read-routing decisions by served source. Per-operation granularity lives on the span attribute db.read_source, not on a label.
labels: [source]
label_values_observed:
  source: [primary, replica]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

### readyz_requests_total

```yaml
declared_at: pkg/utils/metrics.go:275
description: Total number of readyz endpoint requests.
labels: []
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

### readyz_check_status_total

```yaml
declared_at: pkg/utils/metrics.go:268
description: Count of health check outcomes by checker and status.
labels: [checker, status]
label_values_observed:
  checker: [mongo, mongo_crm, mongo_fees, mongo_onboarding, mongo_transaction, postgres_onboarding, postgres_transaction, rabbitmq, redis, vault]
  status: [up, down, "n/a"]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

The label is `checker`, **not** `dep`. `vault` reports `n/a` while the KMS runs in legacy
mode; that is not a failure.

### bulk_recorder_transactions_attempted_total

```yaml
declared_at: pkg/utils/metrics.go:196
description: Total transactions sent to bulk INSERT.
labels: []
live_observed: true
unit: "1"
```

### bulk_recorder_transactions_inserted_total

```yaml
declared_at: pkg/utils/metrics.go:203
description: Total transactions actually inserted, excluding duplicates suppressed by ON CONFLICT DO NOTHING.
labels: []
live_observed: true
unit: "1"
```

`attempted` minus `inserted` must be zero. A positive gap is a transaction that was
accepted but never reached the database — the dashboard surfaces this as a red stat.

### bulk_recorder_operations_attempted_total / bulk_recorder_operations_inserted_total

```yaml
declared_at: pkg/utils/metrics.go:217, pkg/utils/metrics.go:224
description: Same attempted-versus-inserted contract at the accounting-operation level.
labels: []
live_observed: true
unit: "1"
```

### bulk_recorder_bulk_size_total

```yaml
declared_at: pkg/utils/metrics.go:272
declared_name: bulk_recorder_bulk_size
description: Number of messages per bulk processing batch.
labels: []
live_observed: true
unit: "1"
```

### crm_protection_status_total

```yaml
declared_at: pkg/utils/metrics.go:295
description: Total field-protection status outcomes per evaluated CRM record.
labels: [status]
label_values_observed:
  status: [none]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

### crm_protection_mode_resolution_total

```yaml
declared_at: pkg/utils/metrics.go:287
description: Total protection mode resolutions. legacy is lib-commons symmetric crypto; envelope is Vault Transit with Tink.
labels: [mode]
label_values_observed:
  mode: [legacy]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

### crm_protection_encrypt_decrypt_total

```yaml
declared_at: pkg/utils/metrics.go:304
description: Total encrypt/decrypt operations. Any outcome other than success is PII that failed to be read or written.
labels: [path, outcome, error_type]
label_values_observed:
  outcome: [success]
label_cardinality_estimate: low
live_observed: true
unit: "1"
```

Code documents `path` and `error_type`; only `outcome` was observed carrying a value in
the surveyed environment. Declaration is at `pkg/utils/metrics.go:301-303`.

### crm_protection_legacy_read_total

```yaml
declared_at: pkg/utils/metrics.go:342
description: Reads still served from the legacy on-disk format. Should trend to zero under migration to envelope mode.
labels: []
live_observed: true
unit: "1"
```

---

## Histograms

### domain_operation_duration_ms_milliseconds

```yaml
declared_at: pkg/utils/metrics.go:29
declared_name: domain_operation_duration_ms
boundaries_source: default
description: Business operation duration by component and operation, recorded at the use-case exit boundary.
labels: [component, operation]
live_observed: true
unit: ms
```

### readyz_check_duration_ms_milliseconds

```yaml
declared_at: pkg/utils/metrics.go:295
declared_name: readyz_check_duration_ms
boundaries_source: default
description: Duration of individual health check probes. A dependency slowing here precedes timeouts on real requests.
labels: [checker, status]
live_observed: true
unit: ms
```

### bulk_recorder_bulk_duration_ms_milliseconds

```yaml
declared_at: pkg/utils/metrics.go:279
declared_name: bulk_recorder_bulk_duration_ms
boundaries_source: default
description: Time taken for one bulk processing batch.
labels: []
live_observed: true
unit: ms
```

Milliseconds rather than seconds throughout: the factory exposes `Int64Histogram`, so
sub-second latencies would truncate to zero in seconds. Reasoning recorded at
`pkg/utils/metrics.go:312-314`.

---

## Gauges

### redis_backup_queue_depth_ratio

```yaml
declared_at: pkg/utils/metrics.go:150
declared_name: redis_backup_queue_depth
description: Number of records currently in the Redis transaction backup queue. An absolute count despite the _ratio suffix. Sustained growth means a stalled consumer.
instrument_type: Int64Gauge (synchronous, MetricsFactory.Gauge().Set)
labels: []
live_observed: true
synchronous: true
unit: "1"
```

### system_cpu_usage_percentage / system_mem_usage_percentage

```yaml
description: Process CPU and memory utilisation sampled by the lib-observability runtime package, not by midaz code.
emitter: github.com/LerianStudio/lib-observability/v4/runtime
labels: [k8s_pod_name, k8s_deployment_name, service_name]
live_observed: true
unit: percent
```

Not declared in this repository. Compare against `container_memory_working_set_bytes`
from cAdvisor when diagnosing memory: the runtime percentage is not what triggers an
OOMKill.

---

## Framework-emitted metrics

Not declared in midaz. Documented so dashboards do not double-count them against manual
instrumentation.

### http_server_request_duration_seconds

```yaml
description: OTel HTTP semantic-convention server histogram covering every Fiber/Huma route.
labels: [http_request_method, http_route, http_response_status_code, error_type, client_id]
label_values_observed:
  http_response_status_code: [200, 201, 400, 401, 404, 500, 503]
live_observed: true
unit: s
```

The canonical RED source. `error_type` is populated only on 5xx.

### calls_total / duration_milliseconds

```yaml
description: RED metrics derived from spans by the OTel spanmetrics connector, configured collector-side and not in this repository.
labels: [span_name, span_kind, status_code, service_name]
label_values_observed:
  span_kind: [SPAN_KIND_INTERNAL, SPAN_KIND_SERVER]
  status_code: [STATUS_CODE_UNSET]
live_observed: true
```

**`status_code` is always `STATUS_CODE_UNSET`.** An error rate derived from span status is
therefore permanently zero. Use `http_response_status_code` for transport errors and
`domain_operations_total{result=...}` for business errors. This trap is the reason both
dashboards avoid span status entirely.

Useful `span_name` families for dependency latency: `postgres.*`, `mongo.*`, `mongodb.*`,
`redis.*`, plus `lib_auth.authorize` and `handler.*` / `query.*`.

---

## Infrastructure metrics referenced by dashboards

Emitted by cAdvisor and kube-state-metrics, not by midaz. Listed here so the primitive
verifier can stay strict about everything a dashboard query touches. These use the
`namespace` label, not `k8s_namespace_name`.

### container_memory_working_set_bytes

```yaml
description: Real container memory from cAdvisor. This is the value that triggers an OOMKill.
labels: [namespace, pod, container]
unit: bytes
```

### container_cpu_cfs_throttled_periods_total

```yaml
description: CFS periods in which the container was throttled. Divide by container_cpu_cfs_periods_total for the throttled fraction.
labels: [namespace, pod, container]
unit: "1"
```

### container_cpu_cfs_periods_total

```yaml
description: Total CFS scheduling periods elapsed for the container. Denominator for the throttling ratio.
labels: [namespace, pod, container]
unit: "1"
```

### kube_pod_container_status_restarts_total

```yaml
description: Cumulative container restarts. The *-migrate-* jobs appear here and are expected.
labels: [namespace, pod, container]
unit: "1"
```

---

## Log fields

Loki ingests via OTLP. Stream labels: `client_id`, `exporter`, `job`,
`k8s_deployment_name`, `k8s_namespace_name`, `k8s_pod_name`, `level`, `service_name`.

```yaml
level: [DEBUG, INFO, WARN, ERROR]
service_name: [ledger, midaz-crm, unknown_service]
```

`level` is a stream label, so filtering on it is cheap and does not require a parser:

```logql
{k8s_namespace_name="<namespace>", service_name=~"ledger|midaz-crm", level=~"ERROR|WARN"}
```

Findings from the log sweep (angle 5), which did complete successfully:

- 1385 `.Log()` sites, 324 unique field names, 71.8% inside an active span (so
  `trace_id`/`span_id` are auto-injected on those).
- **No `error_code` or `error_class` field exists anywhere.** The only error carrier is
  `error`, free-text from `err.Error()`. Error panels can group by `msg` only, unless a
  bounded code is introduced at the `pkg/errors.go` seam.
- `operation` is the one well-bounded dimension (123 dotted `layer.resource.action`
  values) but is effectively tracer-only: 295 of 296 uses are in `components/tracer`.
  Ledger's ~1008 log sites have no action dimension.
- `libLog.String("key", ...)` at 21 sites emits the literal `[REDACTED]`, because
  lib-observability auto-redacts the field name `key`. Those lines carry no information.
- 9 field names log monetary values (`amount`, `transaction_amount`, `alias_balance`,
  `value` and similar), contrary to the logging rules in `CLAUDE.md`.
- `pkg/mongo/mongo.go:113` and `:129` use `fmt.Sprintf` as the log *message*, which
  destroys `msg` aggregation.

---

## Legacy series — do not build panels on these

Present in Mimir under `job="ledger"`, absent from the Go source and from the Lerian
module cache. Each has a single sample in the last 14 days.

| Series | Status |
|---|---|
| `transactions_processed_total` | Not emitted by current code. Use `domain_operations_total{operation="create_transaction"}`. |
| `accounts_created_total` | Not emitted by current code. Use `domain_operations_total{operation="create_account"}`. |

Both carry `organization_id` and `ledger_id` (UUIDs) — a second reason to avoid them.

Distinct from the above: `tenant_connections_total`, `tenant_consumers_active_ratio` and
`tenant_messages_processed_total` are declared in `pkg/utils/metrics.go:166-191` and are
real, but are multi-tenant-only and therefore absent from a single-tenant deployment.

---

## Known gaps

- **No bounded error code on logs or metrics.** Error panels cannot group by cause.
- **Span status is never set**, so span-derived error rate is unavailable.
- **No DB client spans with statement-level detail** — dependency latency is available
  only at the granularity of the hand-instrumented span names above.
- **The `tracer` service emits no domain metrics**, only `calls_total`,
  `duration_milliseconds`, `http_server_request_duration_seconds` and the runtime gauges.

---

## Surveyed observations

Everything above is the version contract and holds wherever midaz v4 runs. This section is
the opposite: measurements taken in one environment at one moment, recorded so the claims
elsewhere in this document stay checkable. They are **not** invariants — re-measure before
relying on them.

Environment: `midaz-dev-st`. Measured 2026-08-21 by direct Mimir query.

| Observation | Value |
|---|---|
| Ledger pod generations in 24h | 9 distinct `midaz-ledger-*` pods |
| HTTP series count, idle versus burst | 3 (health probes only) → 511 during a burst → 3 |
| `domain_operations_total` series present in 1d | 237, while an instant query returns none |
| Write traffic over 7 days | 759k readyz checks and 342k HTTP requests, against 240 domain operations and 10 balance syncs |
| `$__rate_interval` floor | 2m — the Mimir datasource declares `timeInterval: 30s`, and `increase()` returns no series at `[30s]` or `[1m]` |
| Coexisting legacy service | a standalone `midaz-crm` pod exports `crm_protection_*` alongside the v4 binary, which is why `job="ledger"` is mandatory |

Business panels here are correct but sparse; that is a property of the environment, not of
the queries.
