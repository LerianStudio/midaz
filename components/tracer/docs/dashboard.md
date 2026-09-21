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
Snapping is what makes a relative period name the same window for every caller
inside a minute; without it every cache key is unique and the cache never hits.
**The start rounds DOWN and the end rounds UP**, so the window is always a
superset of what was asked for — rounding the end down instead hid the most
recent 60 seconds of decisions, a fraud console reporting zero fraud half a
minute after blocking a transaction. Staleness is bounded by up to TWO minute
steps plus one TTL; `updatedAt` says when the figures were computed.

`/volume` returns one point per UTC calendar day the window TOUCHES, so
`?period=7d` from mid-afternoon spans 8 calendar days and returns **8 points,
not 7**. Render the points given; never assume a count.

## Bodies

JSON, camelCase, rates as fractions in `[0,1]`, every response carrying
`windowStart`, `windowEnd`, `updatedAt`. `activeRules`/`activeLimits` are
point-in-time, not windowed.

`amountSavedByAsset` is the money figure (convention rule 8);
`amountSaved`/`asset` are populated only when exactly one asset carried blocked
volume, absent otherwise. Amounts are decimal **strings** whose VALUE is exact
end to end — decimal from the database to the wire, never through a float — but
whose SCALE is not: decimal normalises it, so a stored `41000.00` renders
`"41000"` and `10352080.80` renders `"10352080.8"`. Format to the asset's
exponent rather than echoing the string.

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
one (single-tenant mode) it passes through: slower, never wrong. Failure
semantics are convention rule 7.

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
~0.3% its default selectivity assumes. One pooled connection, eight consecutive
`/top-rules` executions:

```
default (cache statement): 105  95  95  95  94 | 323 323 321  ms
pgx.QueryExecModeExec:     104  95  94  93  95 |  94  95  96  ms
```

What the generic plan discards is the **LATERAL fan-out plan** — the parallel
scan and the Memoize over the unnested rule ids — not anything about the shared
`created_at` predicate: `/metrics` and `/volume` do not regress at all over the
same eight executions, and `/fraud-types` loses 1.7x. All four reads carry
`windowPlanMode` (`pgx.QueryExecModeExec`) anyway, because its cost on the two
that do not regress is zero within noise and one rule is easier to keep true
than two exceptions.

Two guards, because the obvious one is not enough. `dashboard_plan_mode_test.go`
asserts the argument is passed; the integration suite counts
`pg_prepared_statements.generic_plans` after eight executions on one connection
and requires zero. A timing assertion was tried and rejected — on a small test
fixture a generic plan costs nothing measurable, so it passed against code with
the mode removed.

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
`/top-rules` needs either a shorter maximum window or a rollup table.

**Read those rows-per-day figures with the fan-out attached.** `/top-rules`
aggregates one row per rule EVALUATED, and the fixture evaluates three rules per
validation. A tenant running 50-150 active rules fans out proportionally further
and reaches the limit far sooner — the number that binds is evaluated rules per
day, not validations per day, and 340,000 is roughly 1,000,000 of those.

### Index

Migration `000024` adds `idx_transaction_validations_dashboard`, a covering
index on `created_at` INCLUDE (decision, transaction_type, asset, amount,
processing_time_ms). Measured on this fixture:

It costs **56MB** beside a 300MB table and **+0.6 to +0.7us** per validation
insert, about 4% of the write's existing index maintenance. It buys **1.4-1.7x**
on `/metrics` and `/fraud-types` — not feasibility: those reads meet the 200ms
bound without it at a million rows (worst case 115ms). What it buys is headroom.
The migration carries the measurements and how they were taken.

**Index-only depends on the visibility map.** Pages autovacuum has not marked
all-visible still cost a heap fetch, so a trail under heavy write with starved
autovacuum degrades toward the pre-index timings. `VACUUM FULL` resets the map
entirely and every read reverts to a heap-reading plan until a plain `VACUUM`
runs.

The rule arrays are **not** indexed and must not be. A btree over `UUID[]`
carries one entry per array, and ~167 active rule ids overflow the tuple limit
(`index row size 2712 exceeds btree version 4 maximum 2704`), which fails the
INSERT — a validation that cannot be recorded against an append-only trail. A
GIN index answers containment, not the per-rule aggregation `/top-rules` needs.

## Adding `/dashboard` to another product

Assume the hygiene any Lerian service already has: one `GET`-only base that owns
no state, one bounded aggregation per endpoint with no unbounded scan and no
N+1, the tenant from the validated JWT rather than a parameter, `updatedAt` on
every body. The eight rules below are the ones this lane paid for in defects.

1. **Snap the window to the cache TTL's granularity, and snap the END UP.** A
   closed period set with a default, or an explicit RFC3339 pair, mutually
   exclusive and hard-capped; UTC, half-open. Rounding the end down hides the
   newest decisions — on a fraud console, indistinguishable from "all clear".
2. **The error message must name the parameters the API registers.** Huma drops
   an unregistered query param silently, so a caller obeying a message that says
   `startDate` gets HTTP 200 for the default window and reads the wrong number.
   Assert the names against the generated spec, never a second copy.
3. **Publish a total and its breakdown from the SAME statement** (`GROUPING
   SETS`). Two statements are two instants, and a cached pair that disagrees
   with itself lies for the whole TTL.
4. **Prove the cost on a fixture with production's physical layout** — for an
   append-only trail, a monotonic timestamp and correlation 1.0. A scattered
   fixture measures a table you will never have, in both directions: here it made
   one query look impossible and one index look 7x more valuable than it is.
5. **Force a custom plan on every parameterised window query.** A cached
   prepared statement goes generic after five executions per connection, and a
   generic plan cannot know the window's selectivity. Pin it against
   `pg_prepared_statements.generic_plans`, not a stopwatch: on a small fixture a
   generic plan costs nothing measurable, so a timing assertion passes against
   code with the fix removed.
6. **Record plan, milliseconds and the operating limit.** Cost is linear in rows
   IN THE WINDOW, so the useful number is throughput at the widest window plus
   the fan-out it assumes. Justify each index with its measured write cost, and
   say whether it buys feasibility or only headroom.
7. **Read-through cache, TTL = the window granularity, with single-flight.** Key
   by tenant, endpoint, window. Every key rotates on the same wall-clock tick, so
   without coalescing every viewer misses at once and each runs the aggregation.
   Detach the shared computation from the request that opened it, and build that
   context INSIDE the shared call — a `defer cancel()` in the caller's frame
   hands the flight's lifetime straight back to whoever opened it. Reuse the
   existing connection, pass through when there is none, treat every cache
   failure as a miss, never cache an error.
8. **Money per asset, and a zero is an answer.** Never one figure summed across
   assets; keep the value exact end to end and let consumers format the scale.
   Report a zero rather than omitting a row — "this rule guards nothing" is what
   an operator most needs to see.
