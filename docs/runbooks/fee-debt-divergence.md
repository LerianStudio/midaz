# Fee-debt divergence

A deferrable fee that its payer cannot fund opens a debt. The debt lives in one Redis
list per debtor balance, and that list is the only live copy of the receivable: the
engine reads nothing else when it settles, cancels, reopens or refunds. The Fees
`fee_debt` MongoDB documents are a projection that completion writes after the engine
answers. This runbook compares the two, read-only, and says which differences an
operator must take over.

It never writes. A lost or stale list is not repaired from MongoDB: the projection lags
completion and rounds past 34 significant digits, so rebuilding a list from it would
turn a detected loss into a silent mis-charge.

## Keys and documents

| Side | Address | Open debt |
| --- | --- | --- |
| Redis | `<prefix>fee-debt:{transactions}:<org>:<ledger>:<alias#key>` | every element of `items` (`id`, `remaining`, `seq`) |
| MongoDB | collection `fee_debt`, fields `organization_id`, `ledger_id`, `debtor_balance_ref` | every document with `remaining > 0` (`_id`, `remaining`, `seq`) |

`<prefix>` is empty in single-tenant deployments and `tenant:<tenantId>:` in
multi-tenant ones. A document `_id` equals the Redis item `id`.

## Query

1. List the debtors on both sides for one ledger:

   ```bash
   redis-cli --scan --pattern '<prefix>fee-debt:{transactions}:<org>:<ledger>:*'
   ```

   ```js
   db.fee_debt.distinct("debtor_balance_ref",
     { organization_id: "<org>", ledger_id: "<ledger>", remaining: { $gt: NumberDecimal("0") } })
   ```

2. For each debtor in either list, print its open debts as `id remaining seq`, sorted:

   ```bash
   redis-cli --raw GET '<prefix>fee-debt:{transactions}:<org>:<ledger>:<alias#key>' \
     | jq -r '.items[] | "\(.id) \(.remaining) \(.seq)"' | sort > redis.txt
   ```

   ```js
   db.fee_debt.find(
     { organization_id: "<org>", ledger_id: "<ledger>", debtor_balance_ref: "<alias#key>",
       remaining: { $gt: NumberDecimal("0") } },
     { remaining: 1, seq: 1 }
   ).forEach(d => print(`${d._id} ${d.remaining.toString()} ${d.seq}`))
   ```

   Sort the MongoDB output into `mongo.txt` and run `diff redis.txt mongo.txt`.

Compare amounts as decimals, not as text: a Decimal128 may print another scale, or
a 34-digit rounding of a longer Redis value.

## Reading the result

| Difference | Meaning | Action |
| --- | --- | --- |
| None | converged | none |
| A line differs and the transaction that last moved the debt is still in `engine:{transactions}:recover` | completion has not projected it yet | wait for recovery; re-run the query |
| A Redis item has no MongoDB document | the opening is not projected yet | same as above |
| MongoDB shows a debt open that the Redis list lacks, with no pending recovery record | the list lost the debt (flush, eviction or failover) | park: record the debtor and debt ids and escalate to an operator |
| The Redis key is absent while MongoDB shows open debts | the list is lost | park; a revert of their origin already fails with `fee_debt_conflict` until an operator resolves it |
| The Redis list's `nextSeq` is not above a MongoDB `seq` | the list was restored from an older copy | park |

Parking means stopping at detection: do not `SET` the list, do not edit the documents,
and do not replay transactions. Settlement of a lost debt is a business decision the
operator makes with the client; the ledger's balances are not affected by the loss
itself, only the receivable is.
