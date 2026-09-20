# The `/dashboard` convention

Tracer's operator dashboard reads, and the pattern other Lerian products follow
when they add a `/dashboard` surface.

## Surface

Base `/v1/dashboard`, all `GET`, tenant-scoped, guarded by `("dashboard","get")`
— the tiers that already read `validations` (`editor`, `validator`, `viewer`)
plus `audit-viewer`, declared in `permissions.yaml`.

| Path | Answers |
|---|---|
| `/metrics` | counts, decision rates, blocked volume per asset, mean latency, live active rule/limit counts |
| `/volume` | validations per UTC day |
| `/fraud-types` | DENY + REVIEW split by transaction type |

`/top-rules` is **not implemented** — see "What is missing".

## Window

Three query params, snake_case like the rest of Tracer: `period` (`7d`/`30d`/
`90d`, default `30d`), or `start_date`/`end_date` (RFC3339, supplied together).
The two forms are mutually exclusive. Naming both, an unknown period, a
half-supplied pair, an unparseable date, an end at or before the start, or a
range over **90 days** is a 400 (`0498`).

Both bounds are truncated to the minute, normalised to UTC, half-open
`[from, to)`. The truncation is what makes a relative period name the same
window for every caller inside a minute; without it every cache key is unique
and the cache never hits. Staleness is bounded by one truncation (<= 60s) plus
one TTL (<= 60s); `updatedAt` reports when the figures were computed.

## Bodies

JSON, camelCase, amounts as decimal **strings**, rates as fractions in `[0,1]`,
every response carrying `windowStart`, `windowEnd`, `updatedAt`.

**Money is per asset, always.** `amountSavedByAsset` is the figure; a
cross-asset total would not be money. `amountSaved`/`asset` are populated only
when exactly one asset carried blocked volume, absent otherwise.

`volume` buckets by **day for every period** (`date` is `YYYY-MM-DD` on the
wire, and a day bucket caps the series at 90 points). `fraud-types` splits by
`transaction_type` only — `sub_type` is a free VARCHAR with no enum, so it
would give a chart an unbounded axis. `activeRules`/`activeLimits` are
point-in-time, not windowed.

## Cache

Valkey read-through, TTL **60s**, key
`tenant:{tenantID}:tracer:dashboard:{endpoint}:{window}`; responses carry
`Cache-Control: private, max-age=60`. It reuses the tenant-manager Pub/Sub
client — Tracer's only Valkey connection — and never opens a second. Without
one (single-tenant mode) it passes through: slower, never wrong. An outage, an
undecodable entry and a miss are all handled as a miss; errors are never cached.

## Cost

Seeded 1,000,000-row trail over 365 days, PostgreSQL 17, median of three warm
runs (`scripts/seed_dashboard_benchmark.sql`):

| Endpoint | 7d | 30d | 90d | Plan |
|---|---|---|---|---|
| `/metrics` | 7ms | 30ms | 92ms | Index Only Scan, `idx_transaction_validations_dashboard` |
| `/volume` | 8ms | 33ms | 98ms | Index Only Scan, `idx_transaction_validations_created` |
| `/fraud-types` | 5ms | 22ms | 40ms | Index Only Scan, `idx_transaction_validations_dashboard` |

Migration `000024` adds that covering index (30d was 224ms and 105ms before it).
It costs 56MB beside a 300MB table and +9.3us per validation insert, against a
29ms per-request budget. **Index-only depends on the visibility map**: pages
autovacuum has not marked all-visible still cost a heap fetch, so a trail under
heavy write with autovacuum starved degrades toward the older timings.

## What is missing

`/top-rules` needs `matched_rule_ids` and `evaluated_rule_ids`, two `UUID[]`
columns of unbounded width. They cannot go in a covering index — a row wide
enough to overflow the B-tree tuple limit would fail its INSERT, which here
means a validation that cannot be recorded. So it must fetch heap rows, and
measured **60/280/583ms** at 7d/30d/90d: past the bound at the default window.
A GIN-overlap variant reached 102ms at 30d but drops `executions`, so it cannot
report a detection rate. Next option is a rollup table, trading write
amplification for a bounded read. Not built: that is a product decision.

## Adding `/dashboard` to another product

1. **One base, `GET` only.** A dashboard owns no state.
2. **One window contract.** A closed period set with a documented default, or
   an explicit RFC3339 pair, mutually exclusive, hard-capped. Truncate both
   bounds to the cache TTL's granularity, normalise to UTC, keep it half-open.
3. **One bounded aggregation per endpoint.** One statement on an indexed
   timestamp, no unbounded scan, no N+1. Publish a total and its breakdown from
   the SAME statement (`GROUPING SETS`) — two statements are two instants, and
   a cached pair that disagrees with itself lies for the whole TTL.
4. **Prove the cost, then ship.** `EXPLAIN (ANALYZE, BUFFERS)` at a realistic
   row count; record plan and milliseconds in the migration and the doc;
   justify each new index with its write cost. A query that cannot meet the
   bound is reported, not quietly shipped.
5. **Read-through cache, TTL = the window granularity.** Key by tenant,
   endpoint, window. Reuse the existing connection, pass through when there is
   none, treat every cache failure as a miss, never cache an error.
6. **Tenant from the validated JWT**, never a parameter. It belongs in the key
   and the connection, nowhere on the wire.
7. **Money per asset.** Never one headline figure summed across assets.
8. **`updatedAt` on every body.**
