# Balance Engine — Midaz Ledger (v4)

**Dashboard UID:** `midaz-v4-balance-engine`

**Audience:** ledger engineering and platform/SRE

**Method:** RED for adapter invocations and recovery; bounded cardinality/shape panels for the internal request path

This dashboard is a readiness surface for the internal accounting adapter. The execution
port remains unset, so an empty panel is expected until a local opt-in or an instrumented
environment invokes the adapter. It does not imply that the public API currently exercises
the engine.

All rates use Grafana's `$__rate_interval`. Labels are limited to the bounded `outcome`,
`code`, and posting `type` vocabularies. No panel groups by an identifier, alias, metadata,
amount, or request payload.

The reproducible measurement procedure and blank result template live in
[`docs/performance/balance-engine-report.md`](../../../performance/balance-engine-report.md).
