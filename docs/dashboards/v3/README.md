# Midaz v3 dashboards — not yet authored

This directory is a placeholder. It holds no `.libsonnet` and no
`telemetry-dictionary.md` yet, and `make dashboards` skips it.

Nothing here is a stub to be filled in mechanically. The v3 dashboards are a separate
authoring job because **v3 and v4 do not share a telemetry contract**, and the panels
cannot be produced by copying the v4 ones and renaming a label.

## What differs from v4

| | v3 | v4 |
|---|---|---|
| Ledger, CRM, fees | three deployments: `ledger`, a standalone `midaz-crm`, and `plugin-fees` | one unified binary |
| Application selector | one `job` per service | `job="ledger"` covers all three |
| CRM protection metrics | emitted by the `midaz-crm` service | emitted by the ledger binary |
| Fees metrics | emitted by `plugin-fees` | emitted under the ledger binary's `fees` component |

The consequence for panel authoring: every v4 query that reads `job="ledger"` has to be
re-pointed at whichever v3 service owns that metric, and the "Operations by component"
panel — which exists to show the unified binary's internal split — has no v3 equivalent at
all. Some v4 metrics will be absent; some v3 metrics have no v4 counterpart.

## How to author these

Follow the same discipline the v4 tree was built with. In order:

1. **Survey before writing.** Establish which series each v3 service actually emits, with
   their real label sets, by querying live. Do not transcribe from Go source — the declared
   name in Go is frequently not the series name in Mimir, and the v4 dictionary's
   "Wire-name translation" section explains why.
2. **Write `v3/telemetry-dictionary.md` first.** The verifier
   (`make dashboards-verify`) checks every panel against the dictionary belonging to its
   own version, so a dashboard cannot land ahead of its contract. Carry over the v4
   dictionary's provenance discipline: record how each row was established, and keep
   environment-specific measurements in a clearly marked observations section rather than
   mixed into the contract.
3. **Then author the dashboards**, reusing `../lib/dashboard.libsonnet`. The helpers are
   version-independent — but `selector.app` and `selector.api` hardcode `job="ledger"`, so
   they need a v3-aware form before they can be shared. Parameterising the job is the
   cleaner change; forking the selector into the v3 tree is the smaller one.
4. Use dashboard UIDs `midaz-v3-*`, matching the `midaz-v4-*` convention, so both versions
   can live in one Grafana folder without colliding.

## Where to survey

*Environment-specific, and true only at the time of writing — verify before relying on it.*

A complete v3 deployment was observed in the `midaz-prd-st` namespace, running `ledger`
and `midaz-crm` together at version `3.8.2`. That is production, so keep the survey
read-only. Mixed-version namespaces also exist — a v4 ledger running beside a v3
`midaz-crm` — and they are a trap for a survey: filter by `service_version`, not just by
`job`, or the two contracts will be conflated.
