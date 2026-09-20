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
| `/top-rules` | the ten busiest rules: matches, executions, detection rate, mean latency |

## Window

Three query params, snake_case like the rest of Tracer: `period` (`7d`/`30d`/
`90d`, default `30d`), or `start_date`/`end_date` (RFC3339, supplied together).
The two forms are mutually exclusive. Naming both, an unknown period, a
half-supplied pair, an unparseable date, an end at or before the start, or a
range over **90 days** is a 400 (`0498`).

Both bounds snap to the minute and normalise to UTC, half-open `[from, to)`.
The snapping is what makes a relative period name the same window for every
caller inside a minute; without it every cache key is unique and the cache
never hits. **The start rounds DOWN and the end rounds UP**, so the window is
always a superset of what was asked for — rounding the end down instead hid the
most recent 60 seconds of decisions, which is a fraud console reporting zero
fraud half a minute after blocking a transaction. Staleness is therefore
bounded by up to TWO minute steps plus one TTL; `updatedAt` reports when the
figures were computed.

`/volume` returns one point per UTC calendar day the window TOUCHES, so
`?period=7d` from mid-afternoon spans 8 calendar days and returns **8 points,
not 7**. A caller must render the points it is given rather than assume a count.

## Bodies

JSON, camelCase, amounts as decimal **strings**, rates as fractions in `[0,1]`,
every response carrying `windowStart`, `windowEnd`, `updatedAt`.

**Money is per asset, always.** `amountSavedByAsset` is the figure; a
cross-asset total would not be money. `amountSaved`/`asset` are populated only
when exactly one asset carried blocked volume, absent otherwise.

`volume` buckets by **day for every period** (`date` is `YYYY-MM-DD` on the
wire, and a day bucket caps the series at 91 points). `fraud-types` splits by
`transaction_type` only — `sub_type` is a free VARCHAR with no enum, so it
would give a chart an unbounded axis. `activeRules`/`activeLimits` are
point-in-time, not windowed.

`top-rules` reports at most ten rules, ordered by `matches` descending then
`name` ascending so a tie renders identically on every refresh. `executions`
counts the validations that EVALUATED the rule and `matches` those where it
fired, so a rule that guards nothing is reported with `matches: 0` rather than
being absent — the answer an operator most needs is the one a matched-only
query withholds. `productType` is read from the rule's own scope, never
inferred from the traffic it saw, and is absent when the rule scopes no
transaction type. `avgProcessingMs` is the mean end-to-end latency of the
validations the rule MATCHED, not the rule's own evaluation cost, which the
trail does not record per rule.

## Cache

Valkey read-through, TTL **60s**, key
`tenant:{tenantID}:tracer:dashboard:{endpoint}:{window}`; responses carry
`Cache-Control: private, max-age=60`. It reuses the tenant-manager Pub/Sub
client — Tracer's only Valkey connection — and never opens a second. Without
one (single-tenant mode) it passes through: slower, never wrong. An outage, an
undecodable entry and a miss are all handled as a miss; errors are never cached.

## Cost

Measured, not asserted. Fixture `scripts/seed_dashboard_benchmark.sql`:
1,000,000 validations over 365 days, PostgreSQL 17, `shared_buffers=256MB`.

**It writes `created_at` in ascending order, and that is load-bearing.** The
trail is append-only by construction (`created_at DEFAULT now()` plus migration
`000003`'s `DO INSTEAD NOTHING` rules), so rows never move: correlation is 1.0
and any window is one contiguous heap stretch. An earlier fixture scattered
`created_at` (correlation -0.002) and measured a table this can never be —
which is how `/top-rules` was first, wrongly, reported as over its budget.

End-to-end **HTTP**, three runs, all inside the 200ms bound:

| Endpoint | 7d | 30d | 90d | Plan at 90d |
|---|---|---|---|---|
| `/metrics` | 7ms | 23ms | 67ms | Index Only Scan, `idx_transaction_validations_dashboard` |
| `/volume` | 5ms | 18ms | 53ms | Index Only Scan, `idx_transaction_validations_created` |
| `/fraud-types` | 5ms | 15ms | 25ms | Parallel Index Only Scan, `idx_transaction_validations_dashboard` |
| `/top-rules` | 29ms | 46ms | **106ms** | Parallel Index Scan, `idx_transaction_validations_created`, + heap |

`/top-rules` is the expensive one and always will be: `CROSS JOIN LATERAL
unnest(evaluated_rule_ids)` turns each validation into one row per rule it
evaluated (three in the fixture), and the arrays force a heap read — 11,823
buffers at 90 days, ~92MB, against 1,775 for `/metrics`.

### The generic-plan trap

pgx caches a server-side prepared statement per connection. PostgreSQL plans the
first five executions against the real parameters, then under the default
`plan_cache_mode=auto` switches to a **generic** plan built with no knowledge of
them — which cannot know a 90-day window holds 25% of the table rather than the
~0.3% its default selectivity assumes, so it drops the parallel scan and the
Memoize node. One pooled connection, eight consecutive `/top-rules` executions:

```
default (cache statement): 105  95  95  95  94 | 323 323 321  ms
pgx.QueryExecModeExec:     104  95  94  93  95 |  94  95  96  ms
```

So every windowed read passes `windowPlanMode` (`pgx.QueryExecModeExec`) first,
which uses an unnamed statement PostgreSQL always plans against the given
parameters. `dashboard_plan_mode_test.go` asserts the argument is present:
dropping it breaks no test and no row, it just makes the service quietly
several times slower after five minutes of uptime.

### Operating limit

Cost is linear in **rows inside the window**, not rows in the table;
`/top-rules` binds. Measured by widening the window until it crossed:

| rows in window | `/top-rules` |
|---|---|
| 246,575 (90d) | 146ms |
| 328,765 | 191ms |
| 356,162 | **214ms** |

The bound runs out near **340,000 rows in the window**: about **3,800
validations/day** on the 90-day view, 11,000/day on the default 30-day view,
48,000/day on 7 days. (The fixture is 2,740/day.) Above that rate the other
three endpoints stay well inside budget — they are 3-6x cheaper per row — and
`/top-rules` needs either a shorter maximum window or the rollup below.

### Index

Migration `000024` adds `idx_transaction_validations_dashboard`, a covering
index on `created_at` INCLUDE (decision, transaction_type, asset, amount,
processing_time_ms). Measured on this fixture:

- **56MB** beside a 300MB table, and 56MB either way — built by the migration
  against existing rows, or grown by a million inserts. They match because
  `created_at` is monotonic, so every insert lands on the rightmost page and
  packs like a fresh build. A table whose physical order had been destroyed
  would grow a larger index from page splits.
- **+0.6 to +0.7us per validation insert** (medians of seven 50,000-row batches
  with the index, against seven without). The table already carries nine other
  indexes costing ~15us of maintenance per row, so this is about 4% more.

**Index-only depends on the visibility map.** Pages autovacuum has not marked
all-visible still cost a heap fetch, so a trail under heavy write with starved
autovacuum degrades toward the pre-index timings. `VACUUM FULL` resets the map
entirely and every read reverts to a bitmap-heap plan until a plain `VACUUM`
runs.

The rule arrays are **not** indexed and must not be. A btree over `UUID[]`
carries one entry per array, and ~167 active rule ids overflow the tuple limit
(`index row size 2712 exceeds btree version 4 maximum 2704`), which fails the
INSERT — a validation that cannot be recorded against an append-only trail. A
GIN index answers containment, not the per-rule aggregation `/top-rules` needs.

If the limit is reached, a rollup table (per rule per day) is the next option,
trading write amplification for a bounded read. Not built: nothing measured
needs it yet, and the write cost is a product decision.

## Adding `/dashboard` to another product

1. **One base, `GET` only.** A dashboard owns no state.
2. **One window contract.** A closed period set with a documented default, or
   an explicit RFC3339 pair, mutually exclusive, hard-capped. Snap both bounds
   to the cache TTL's granularity, normalise to UTC, keep it half-open — and
   snap the END UP. Rounding it down hides the newest decisions, which on a
   fraud console is indistinguishable from "nothing is wrong".
3. **The error message must name the parameters the API registers.** Huma
   silently drops an unregistered query param, so a caller obeying a message
   that says `startDate` gets HTTP 200 for the default window and reads the
   wrong number. Assert the names against the generated spec, not a second copy
   in the message.
4. **One bounded aggregation per endpoint.** One statement on an indexed
   timestamp, no unbounded scan, no N+1. Publish a total and its breakdown from
   the SAME statement (`GROUPING SETS`) — two statements are two instants, and
   a cached pair that disagrees with itself lies for the whole TTL.
5. **Prove the cost on a fixture with production's physical layout.** For an
   append-only trail that means a monotonic timestamp and correlation 1.0. A
   fixture that scatters the key measures a table you will never have, in
   either direction: it made one query here look impossible and would make
   another look free.
6. **Force a custom plan on every parameterised window query.** A cached
   prepared statement goes generic after five executions per connection, and a
   generic plan cannot know the window's selectivity. Assert the exec-mode
   argument in a test: removing it breaks no behaviour, only speed, and only
   after a few minutes of uptime.
7. **Record plan, milliseconds and the operating limit.** Cost is linear in
   rows IN THE WINDOW, so the useful number is validations/day at the widest
   window, not rows in the table. Justify each new index with its measured
   write cost. A query that cannot meet the bound is reported, not quietly
   shipped.
8. **Read-through cache, TTL = the window granularity, with single-flight.**
   Key by tenant, endpoint, window. Every key rotates on the same wall-clock
   tick, so without coalescing every viewer misses at once and each runs the
   full aggregation. Run the shared computation on a context detached from the
   request that opened it, or the first viewer to close a tab fails all the
   others. Reuse the existing connection, pass through when there is none,
   treat every cache failure as a miss, never cache an error.
9. **Tenant from the validated JWT**, never a parameter. It belongs in the key
   and the connection, nowhere on the wire.
10. **Money per asset.** Never one headline figure summed across assets.
11. **`updatedAt` on every body**, and report a zero rather than omitting a row:
    "this rule guards nothing" is the answer an operator most needs.
