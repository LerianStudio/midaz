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

**`volumeByAsset` is the GROSS settled amount**: every settled leg inside the
window, reversal legs included. Reverting does not unwind the original —
`revert_transaction.go` writes the reversal as a NEW settled row carrying
`parent_transaction_id` and leaves the original `APPROVED` — so 1000 EUR posted
then reverted inside one window reads 2000 EUR over 2 transactions, the
throughput the ledger carried. **`reversalsByAsset` is the part of that made of
reversal legs**, same window, same settled filter, from the same scan (one more
FILTER pair, no second pass). `/volume` is gross for the same reason and grows
no field.

**A true net is NOT derivable from these two figures alone.** Subtracting gives
the settled amount excluding reversal legs, which is not net, because whether
the reversed original also falls inside the window changes the answer. Measured
on a 1000 EUR post-and-revert: a window holding BOTH legs reads volume 2000 /
reversals 1000, and nothing net moved; a window holding ONLY the reversal reads
volume 1000 / reversals 1000; a window holding ONLY the original reads volume
1000 / reversals 0, for money that was later reverted. A consumer that needs net
reads the transactions.

Three rules make that figure readable, all enforced upstream: a reversal can
never itself be reverted (`revert_transaction.go:172`,
`ErrTransactionIDIsAlreadyARevert`), so the count is never a chain depth; a
`PENDING` original cannot be reverted at all (`revert_transaction.go:179`
requires `APPROVED`); and a reversal carries the original's amount exactly
(`transaction.go:296` builds the reversal from `*t.Amount`, and the revert path
never calls `applyFees` — its only two call sites are
`create_transaction_v2.go:188` and `create_atomic_transaction_batch_v2.go:374`).
`parent_transaction_id` is originated by one function, the revert one
(`revert_transaction.go:239`), so the figure counts reversals and nothing else.

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
sibling, an index scan over `idx_transaction_created_at` (bitmap or plain, by
correlation) at 7d/30d, a **Seq Scan** at 90d, and
`idx_transaction_organization_ledger_id` for the small ledger.
`idx_transaction_date_range` was never chosen, so this states the plans observed
rather than the index expected. Seeded 3.1M rows, 3 assets, 90 days, median of
three, `/metrics`//`/volume`: 17/22 ms at 33k, 84/115 at 155k, 181/205 at 332k,
586/735 at 996k.

A covering `INCLUDE (status, asset_code, amount)` index was measured and **not
shipped**: Index Only plans, half the buffers, but only 1.25–1.33x where it
matters, 382 MB beside a 440 MB heap, ~+6% insert cost, moving only `/volume` at
30d over the line.

**Out of scope**: `/top-accounts`, operations-level counts, distinct accounts
touched — each needs a new index on `operation` or an outbox-fed rollup, plus a
backfill story.
