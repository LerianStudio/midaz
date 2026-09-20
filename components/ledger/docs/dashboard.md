# The ledger `/dashboard` reads

Three aggregate reads over one ledger, for an operator console. Base
`/organizations/{organization_id}/ledgers/{ledger_id}/dashboard`, on `/v1` and
`/v2`. Tenant from the validated JWT, org and ledger from the path. All three
answer `Cache-Control: private, max-age=60` and authorize under a dedicated
`("dashboard","get")`; borrowing `transactions` would widen it to disclose
balance positions. **That pair is not yet in the caradhras seed**, so a
policy-enforcing environment answers 403 until it lands.

| Path | Answers |
|---|---|
| `/metrics` | `total`, `byStatus` (every status the enum knows, `0` when absent), `volumeByAsset` |
| `/volume` | one point per UTC calendar day the window touches |
| `/assets` | position per asset from `balance`; no window (params ignored) |

## Window

`period=7d\|30d\|90d` (default `30d`) **or** `start_date`/`end_date` (RFC3339,
together). Both forms named, an unknown period, half a pair, a bad date, an end
at or before the start, or a range over **90 days** is a 400, `0498`. Bounds
snap to the minute, UTC, half-open `[from, to)`. **Start rounds down, end rounds
up**: rounding it down hides up to 60 s of recent activity, which a console
reads as "nothing happened". `updatedAt` says when it was computed.

`/volume` returns one point per calendar day the window **touches**, so
`?period=7d` from mid-afternoon spans 8 days and returns **8 points, not 7** —
render the points given, never assume a count. A day with no transactions is
present with `transactions: 0, byAsset: []` (`generate_series` LEFT JOIN: a gap
is a zero, not a missing key).

## Money

Always per asset, never a cross-asset total — no field can hold one. `numeric`
to `shopspring/decimal` to a QUOTED JSON string, exact, scale not normative
(`41000.00` may render `"41000"`); never a float, never a bare JSON number.

`volumeByAsset.amount` sums `transaction.amount` for **settled** rows only.
Settled is `{APPROVED}`: `CREATED` is promoted to `APPROVED` on all three
persistence paths, so a row at `CREATED` is an in-flight write, not money that
moved; `PENDING`, `CANCELED`, `NOTED` never moved money. Non-settled rows still
count in `total` and `byStatus`, so those disagreeing is expected. Statuses come
from `constant.TransactionStatuses` and the settled set is a SQL parameter, so a
new status needs no change here.

**Volume is gross of reversals; `reversalsByAsset` is the reverted part; net =
volume − reversals is the operator's arithmetic, not a field.** Reverting writes
a NEW settled row carrying `parent_transaction_id` and leaves the original
`APPROVED`, so 1000 EUR posted then reverted reads 2000 EUR over 2 transactions
— the throughput the ledger carried. `/metrics` reports the reverted part per
asset from the same scan (one more FILTER pair, no second pass); `/volume` is
gross too and grows no field.

`/assets` sums `available` and `on_hold` **as stored**: migration
`000005_update_balance` made both `DECIMAL` and dropped `scale`, so there is no
per-row division (if the column returns, so does the division, pre-sum). It
counts only accounts that HOLD money. `@external/<asset>` is the contra side of
every issuance, so summing it roughly doubles the position; that mirror is the
ledger's **issued supply**, a legitimate future field but never a term in a
position. The exclusion matches `lower(account_type)` — rows before 3cd8a0423
(2026-07-13) can read `External`, and an exact match publishes the mirror (a
negative number) as the position.

## Cache

Read-through decorator on the repository port, TTL 60 s, expiry only. Keys as
written: `midaz:dashboard:{endpoint}:{org}:{ledger}:p={period}:{to}` for a
relative period, `...:{from}:{to}` for an explicit range,
`midaz:dashboard:assets:{org}:{ledger}` with no window segment;
`valkey.GetKeyContext` prefixes `tenant:{tenantID}:`. The org and ledger
segments are mandatory — without them two ledgers under one tenant read each
other's figures. `singleflight` on the full key, so N viewers arriving together
cost one aggregation. No Valkey, an unreachable one, or a bad entry falls
through to Postgres and logs, never fatal. Single-tenant mode has no tenant
segment (`lib-commons` `GetKey` passes an empty tenant through), so two
deployments sharing a Valkey collide only on identical org AND ledger UUIDs —
platform convention.

## Cost and the operating limit

Both window reads are one statement, and the measured property is that time is
**linear in rows inside the window**, not in table size. The plan is the
planner's call and moves with selectivity: on a 900k-row ledger beside a 2k
sibling, bitmap over `idx_transaction_created_at` at 7d/30d, **Seq Scan** at
90d, index scan over `idx_transaction_organization_ledger_id` for the small
ledger. `idx_transaction_date_range` was never chosen. Seeded 3.1M rows, 3
assets, 90 days, median of three, `/metrics`//`/volume`: 17/22 ms at 33k, 84/115
at 155k, 181/205 at 332k, 586/735 at 996k.

A covering `INCLUDE (status, asset_code, amount)` index was measured and **not
shipped**: Index Only plans, half the buffers, but only 1.25–1.33x where it
matters, 382 MB beside a 440 MB heap, ~+6% insert cost, moving only `/volume` at
30d over the line.

**Out of scope**: `/top-accounts`, operations-level counts, distinct accounts
touched — each needs a new index on `operation` or an outbox-fed rollup, plus a
backfill story.
