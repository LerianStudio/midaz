# Tenant Health Probe

In multi-tenant mode every tenant owns its own PostgreSQL databases (onboarding
and transaction) and its own MongoDB cluster, resolved per request from the
tenant-manager. The pod-level `/readyz` cannot tell whether those databases are
reachable, and must not try: one unreachable tenant would turn the probe into a
503 and take the pod out of rotation for every other tenant.

This document covers what `/readyz` reports in multi-tenant mode, the
background prober that watches each tenant's databases instead, the signals it
publishes, ready-made alert expressions, its configuration and its cost.

The failure mode it exists for: a tenant's database becomes unreachable while
the pod-level readiness stays green. Every request of that tenant answers 503,
and nothing says so until traffic arrives. A tenant with no traffic produces no
signal at all.

## What `/readyz` covers in multi-tenant

Single-tenant deployments are unchanged: `postgres_onboarding` and
`postgres_transaction` probe the configured databases.

With `MULTI_TENANT_ENABLED=true`:

| Check | Status | Why |
|---|---|---|
| `postgres_onboarding` | `n/a` | The static connection built from `DB_ONBOARDING_*` is a fallback that belongs to no tenant, so a probe of it says nothing about any tenant. |
| `postgres_transaction` | `n/a` | Same, for `DB_TRANSACTION_*`. |
| Per-tenant MongoDB | not present | No per-tenant check is added to `/readyz`. |
| `mongo_fees` | probed | The fees module keeps a real static connection. When it lives on the same cluster as the tenants' MongoDB, it doubles as a canary for that shared cluster. |

Both `n/a` checks carry this fixed reason, verbatim:

```text
multi-tenant: per-tenant databases are probed in the background; see tenant_dependency_up
```

`n/a` does not count against the overall status, so it never turns `/readyz`
into a 503. The HTTP status code of `/readyz` does not change; only the status
of those two checks moves from `up` to `n/a`, and only in multi-tenant mode.
`readyz_check_status_total{checker=~"postgres_.*",status="n/a"}` keeps being
emitted by the handler.

## Per-tenant signals

| Signal | Nature | What it tells you |
|---|---|---|
| `lerian.http.server.responses_5xx.by_tenant` | Reactive | 5xx responses per authenticated tenant. Needs traffic. |
| Middleware ERROR logs on tenant connection resolution | Reactive | A request could not resolve the tenant's database. Needs traffic. |
| `balance_sync_tenant_skip_total` | Partial | The balance sync worker skipped a tenant it could not connect to. Covers only tenants with pending balance work. |
| Balance reconciler metrics (when deployed) | Partial | Covers only the transaction PostgreSQL database, once per reconciler pass. |
| `tenant_dependency_up`, `tenant_dependency_probe_failures_total`, `tenant_health_probe_pass_failures_total`, `tenant_health_probe_leader` | Continuous | Published by the tenant health prober on every pass, with or without traffic. |

### The tenant health prober

The prober lives in `components/ledger/internal/bootstrap/tenant_health.prober.go`
and runs only in multi-tenant mode.

- **Leader only.** Every pod competes for a Redis lease on its own lock key
  (`lock:{transactions}:tenant-health-probe`), the same mechanism the balance
  reconciler uses. Only the pod holding the lease probes; the others skip the
  pass. A pod that is not the leader is idle by design, not failing.
- **Series are per pod.** Every pod, leader or not, publishes
  `tenant_health_probe_leader` on every tick: `1` when it held the lease, `0`
  otherwise. The gauges are cumulative, so a former leader's
  `tenant_dependency_up` series keep their last value until that pod restarts,
  `1` and `0` alike. Always join `tenant_dependency_up` with
  `tenant_health_probe_leader == 1` on the pod label to read the current
  leader's view. A new leader forgets the previous tenure's recovery state, so
  its first success is not logged as a recovery.
- **Tenant source.** The authoritative list of active tenants comes from the
  tenant-manager. When that call fails, the pass logs a WARN, counts
  `tenant_health_probe_pass_failures_total{reason="tenant_list"}` and probes the
  tenants in the local tenant cache instead. Without a tenant-manager client
  only the cache is used. When the tenant-manager answers, its list is
  authoritative and the cache is not merged in.
- **Three dependencies per tenant**, the `dependency` label:

  | `dependency` | Probe |
  |---|---|
  | `postgres_onboarding` | `SELECT 1` on the tenant's onboarding database |
  | `postgres_transaction` | `SELECT 1` on the tenant's transaction database |
  | `mongo` | One primary ping on the tenant's MongoDB cluster, through the first configured module manager (onboarding, then transaction, then CRM) |

  A module whose manager is not configured is skipped. Fees is not probed: it
  uses a static connection already covered by `/readyz`.
- **One failure never aborts a pass.** Each probe has its own timeout; a
  timeout or a recovered panic counts as down. Up to four tenants are probed
  concurrently. When the pass budget runs out, the remaining tenants are probed
  first on the next pass, and the pass logs a WARN and counts
  `tenant_health_probe_pass_failures_total{reason="budget"}`.
- **`tenant_id`** is the tenant-manager ID, the same value as the `tenantId`
  JWT claim the request middleware uses to resolve the tenant's databases.

### Metrics

| Declared name | Series in Mimir | Type | Labels |
|---|---|---|---|
| `tenant_dependency_up` | `tenant_dependency_up_ratio` | Gauge | `tenant_id`, `dependency` |
| `tenant_dependency_probe_failures_total` | `tenant_dependency_probe_failures_total` | Counter | `tenant_id`, `dependency` |
| `tenant_health_probe_pass_failures_total` | `tenant_health_probe_pass_failures_total` | Counter | `reason` |
| `tenant_health_probe_leader` | `tenant_health_probe_leader_ratio` | Gauge | none (per pod) |

`tenant_dependency_up` is `1` when the last probe of that tenant dependency
succeeded and `0` when it failed. It is declared with `Unit: "1"`, so the
OTLP-to-Prometheus translation appends `_ratio` to the gauge; it is still a
plain `0`/`1` value. See the wire-name section of
[the telemetry dictionary](../dashboards/v4/telemetry-dictionary.md).

`tenant_health_probe_leader` is `1` when the pod held the lease on its last
tick and `0` when another pod held it or the lease could not be tried. It also
gets the `_ratio` suffix. It is the selector for the current leader's
`tenant_dependency_up` series: a former leader's series stay frozen at their
last value until that pod restarts.

`reason` values on `tenant_health_probe_pass_failures_total`:

| `reason` | Meaning |
|---|---|
| `tenant_list` | The tenant-manager call failed; the pass fell back to the tenant cache. |
| `lease` | Trying the Redis lease failed. Another pod holding the lease is NOT counted; that is the normal non-leader state. |
| `budget` | The pass budget ran out; the remaining tenants were deferred to the next pass. |

### Logs

| Level | Message | Fields |
|---|---|---|
| ERROR | `tenant dependency probe failed` | `tenant_id`, `dependency`, `error` |
| INFO | `tenant dependency recovered` | `tenant_id`, `dependency` |
| WARN | `tenant health probe could not list active tenants; probing cached tenants` | `error` |
| WARN | `tenant health probe pass budget exhausted` | `probed`, `remaining` |

The ERROR is logged on every pass that fails, not only on the transition, so a
tenant that stays down keeps producing one line per dependency per interval.
The INFO is logged once, on the first successful probe after a failure.

## Suggested alerts

The alert rules themselves are maintained outside this repository. These
expressions are ready to use; add your environment selectors
(`k8s_namespace_name`, `job="ledger"`). The pod label in this stack is
`k8s_pod_name`.

A tenant dependency is down for the whole window, as seen by the current
leader:

```promql
min_over_time(tenant_dependency_up_ratio[5m]) == 0
  and on (k8s_pod_name) tenant_health_probe_leader_ratio == 1
```

Any probe failure in the window (faster, noisier):

```promql
increase(tenant_dependency_probe_failures_total[10m]) > 0
```

The tenant-manager could not list tenants, so the prober is running on the
cache:

```promql
increase(tenant_health_probe_pass_failures_total{reason="tenant_list"}[15m]) > 0
```

No pod is probing (no pod holds the lease, for example because Redis is
unavailable), sustained for 10 minutes (`for: 10m`):

```promql
max(tenant_health_probe_leader_ratio) < 1
```

With the prober disabled or every pod down the leader gauge has no series at
all; pair this alert with your usual pod-availability alert.

Reactive complement, 5xx per authenticated tenant (requires traffic):

```promql
sum by (tenant_id) (rate(lerian_http_server_responses_5xx_by_tenant_total[5m])) > 0
```

Probe failures in Loki:

```logql
{k8s_namespace_name="<namespace>", service_name="ledger", level="ERROR"} |= "tenant dependency probe failed"
```

Use `min_over_time` rather than an instant query on the gauge, and always
select the leader through `tenant_health_probe_leader_ratio == 1`: every pod
that ever led keeps exporting its last `tenant_dependency_up` values, so
without the join an ex-leader's stale `1` hides an outage and its stale `0`
fires forever. Right after a leadership change the join may briefly cover two
pods or none, until both have ticked.

## Configuration

All knobs are optional and only read in multi-tenant mode; single-tenant
deployments ignore them. A zero or negative value takes the default
and a positive value below the minimum is raised to the minimum, each with a
WARN at boot.

| Variable | Default | Minimum | Meaning |
|---|---|---|---|
| `TENANT_HEALTH_PROBE_ENABLED` | `true` (unset or empty) | — | Set `false` to disable the prober. |
| `TENANT_HEALTH_PROBE_INTERVAL_MS` | `60000` | `5000` | Time between passes. This is the detection ceiling. |
| `TENANT_HEALTH_PROBE_TIMEOUT_MS` | `2000` | `100` | Timeout of a single probe; a probe past it counts as down. |
| `TENANT_HEALTH_PROBE_PASS_BUDGET_MS` | `30000` | `1000` | Wall-clock budget of one pass; tenants not reached are probed first on the next pass. |

The lease expiry is `2 × interval + pass budget + 3 × timeout + 30 s`: a pass
can run its whole budget, then wait for the last tenant's three sequential
probes, plus the tenant-manager listing call (30 s, the tenant-manager client's
default HTTP timeout). A leader that dies is replaced within that span.

## Cost on the leader

For N active tenants, the leader pod keeps:

- 2N PostgreSQL pools open (onboarding and transaction), each holding at least
  one idle server connection.
- N MongoDB clients open, each with its own replica-set heartbeat.
- One tenant-manager listing call per pass, plus about 3N configuration
  lookups per probe interval: each probe touches a connection whose settings
  revalidation is due, and revalidation bypasses the tenant-manager client
  cache.

Every probe touches the cached connection, so the leader never evicts an idle
tenant connection. That cost is accepted on one pod; the other pods pay
nothing.

## Limits

- **Detection is bounded by the interval.** With the default, a database that
  goes down is reported within one minute, plus scrape and alert evaluation
  delay.
- **The fees MongoDB is outside the prober.** It is a static connection,
  covered by the `mongo_fees` check of `/readyz`.
- **One MongoDB ping per tenant.** It covers the tenant's cluster, not each
  module's database on it.
- **Removed tenants keep their last value.** A tenant that leaves the active
  list stops being probed, but its `tenant_dependency_up` series keeps its last
  value on the leader until the pod restarts. Filter dashboards by the active
  tenant set when this matters.
- **Former leaders keep their series.** A pod that loses the lease keeps
  exporting its last `tenant_dependency_up` values until it restarts. Join with
  `tenant_health_probe_leader_ratio == 1`, as the alerts above do.
- **Partial view on tenant-manager failure.** While the tenant-manager is
  unavailable, only tenants in the leader's local cache are probed.
