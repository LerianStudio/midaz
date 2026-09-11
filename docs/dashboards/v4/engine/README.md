# Engine — Midaz Ledger (v4)

**Dashboard UID:** `midaz-v4-engine`

**Audience:** ledger engineering and platform/SRE

**Method:** RED for adapter invocations and recovery; bounded cardinality/shape panels for the internal request path

This dashboard is the observability surface for the ledger's default accounting engine.
An empty panel means that the selected environment has not emitted engine telemetry in the
current time range or that telemetry is not configured.

All rates use Grafana's `$__rate_interval`. Labels are limited to the bounded `outcome`,
`code`, and posting `type` vocabularies. No panel groups by an identifier, alias, metadata,
amount, or request payload.

The reproducible measurement procedure and blank result template live in
[`docs/performance/engine-report.md`](../../../performance/engine-report.md).
Metric semantics and the execution/recovery boundaries they observe are defined
in [`docs/architecture/engine.md`](../../../architecture/engine.md).
