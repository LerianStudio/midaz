# The ledger `/dashboard` reads

Three aggregate reads over one ledger, for an operator console. Base
`/organizations/{organization_id}/ledgers/{ledger_id}/dashboard`, on `/v1` and
`/v2`. Tenant from the validated JWT, organization and ledger from the path. All
three answer `Cache-Control: private, max-age=60` and authorize under a
dedicated `("dashboard","get")` — borrowing `transactions` would widen that
grant to disclose balance positions. **The pair is not yet in the caradhras
central seed**, so a policy-enforcing environment answers 403 until it lands.

| Path | Answers |
|---|---|
| `/metrics` | `total`, `byStatus` (every status the enum knows, `0` when absent), `volumeByAsset` |
| `/volume` | one point per UTC calendar day the window touches |
| `/assets` | position per asset from `balance`; no window (window params ignored) |

## Window

`period=7d\|30d\|90d` (default `30d`) **or** `start_date`/`end_date` (RFC3339,
together). Both forms named, an unknown period, half a pair, an unparseable
date, an end at or before the start, or a range over **90 days** is a 400,
`0498`. Bounds snap to the minute, UTC, half-open `[from, to)`. **Start rounds
down, end rounds up**: rounding the end down hides up to 60 s of the most recent
activity, which a console reads as "nothing happened". `updatedAt` says when it
was computed.

`/volume` returns one point per calendar day the window **touches**, so
`?period=7d` from mid-afternoon spans 8 days and returns **8 points, not 7** —
render the points given, never assume a count. A day with no transactions is
present with `transactions: 0, byAsset: []` (`generate_series` LEFT JOIN: a gap
is a zero, not a missing key).

## Money

Always per asset, never a cross-asset total — no field can hold one. Exact
decimal strings, scale not normative (`41000.00` may render `"41000"`).

`volumeByAsset.amount` sums `transaction.amount` for **settled** rows only.
Settled is `{APPROVED}`: `CREATED` is promoted to `APPROVED` on all three
persistence paths, so a row resting at `CREATED` is an in-flight write, not
money that moved; `PENDING`, `CANCELED`, `NOTED` never moved money. Non-settled
rows still count in `total` and `byStatus`, so those two disagreeing is
expected. Statuses come from `constant.TransactionStatuses` and the settled set
is a SQL parameter, so a new status needs no change here.

`/assets` sums `available` and `on_hold` **as stored** — no per-row scale
division: migration `000005_update_balance` made both `DECIMAL` and dropped
`scale`. If a scale column returns, so does the division, per row, pre-sum.

## Cache

Read-through decorator on the repository port, TTL 60 s, expiry only. Key
`midaz:dashboard:{endpoint}:{org}:{ledger}:{start}-{end}`, tenant-prefixed by
`valkey.GetKeyContext`; `/assets` carries no window segment. The org and ledger
segments are mandatory — without them two ledgers under one tenant read each
other's figures. `singleflight` on the full key, so N viewers arriving together
cost one aggregation. No Valkey, an unreachable one, or an undecodable entry all
fall through to Postgres and log; never fatal.

## Cost and the operating limit

Both window reads are one statement over `idx_transaction_date_range`; cost is
linear in **rows inside the window**, not table size. Seeded 3.1M rows, 3
assets, 90 days, median of three, `/metrics`//`/volume`: 17/22 ms at 33k rows,
84/115 ms at 155k, 181/205 ms at 332k, 586/735 ms at 996k. `/volume` binds,
crossing 200 ms at **~320k rows in the window**: about **46,000 transactions/day
at 7d, 10,700 at 30d, 3,600 at 90d** (`/metrics` at ~365k). Past that the 60 s
cache is the protection: one aggregation per ledger per minute at any viewer
count. `/assets` is bounded by account cardinality, not history — 63 ms at 50k
balances, 320 ms at 200k, 200 ms at **~125k**.

A covering `INCLUDE (status, asset_code, amount)` index was measured and **not
shipped**: Index Only plans and half the buffers, but only 1.25–1.33x where it
matters, 382 MB beside a 440 MB heap, ~+6% insert cost, and it moves only
`/volume` at 30d across the line.

## Out of scope, deliberately

`/top-accounts`, operations-level counts, distinct accounts touched — each needs
a new index on `operation` or an outbox-fed rollup, plus a backfill story.
