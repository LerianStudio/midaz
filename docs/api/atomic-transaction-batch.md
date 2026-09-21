# Atomic transaction batch (v2)

`POST /v2/transactions/batch` creates several direct or hold v2 transactions as one
ordered, all-or-none monetary decision. It is a dedicated contract: the singular
`POST /v2/transactions/direct` and `POST /v2/transactions/hold` continue to accept
one transaction object, and each route rejects the other route's request shape.

## Contract

The request is a wrapper whose `transactions` member contains the unchanged
v2 request model together with the action and explicit logical order:

```json
{
  "transactions": [
    {
      "action": "direct",
      "order": 1,
      "description": "credit first",
      "asset": "BRL",
      "amount": "10",
      "debits": [{"alias": "@cash", "organizationId": "00000000-0000-0000-0000-000000000001", "ledgerId": "00000000-0000-0000-0000-000000000002", "balanceKey": "available", "amount": "10"}],
      "credits": [{"alias": "@customer", "organizationId": "00000000-0000-0000-0000-000000000001", "ledgerId": "00000000-0000-0000-0000-000000000002", "amount": "10"}]
    }
  ]
}
```

Every item must resolve internally to one organization and ledger, but distinct
`direct` items may name distinct ledgers when each participant enables
`settings.crossLedger.enabled`. A batch containing `hold` items remains
single-ledger; mixed scopes return `0499`. The endpoint has no path or query
parameters; scope is taken from the validated transaction legs. It accepts an
optional `balanceKey` on every debit or credit leg; an omitted key uses the
account's `default` balance. A supplied key selects that named balance and is
returned in the resulting operation. The key cannot contain whitespace and is
limited to 100 characters. The endpoint uses the same `midaz/transactions/post`
authorization chain and per-item v2 fee, Tracer, route, overdraft, skip, and
account-block-exception rules. `action` is either `direct` or `hold`; a hold is
returned initially as `PENDING` and is later committed or cancelled through the
existing individual transaction routes.

Success is HTTP 201 with a wrapper containing the created `TransactionV2` objects
plus their non-persisted `order`. The response `transactions` array is in
increasing logical order, rather than physical request array order. For this
endpoint, the internal idempotency and recovery batch identifier is not exposed,
is not stored as `group_id` on transaction rows or events, and has no batch
resource or query endpoint; query created transactions through their individual
transaction IDs. That differs from a decomposed cross-ledger direct request,
whose public `groupId` is deliberately persisted and returned.

## Ordering and atomicity

`order` is one-based, required, unique, and consecutive from 1 through the number
of items. It alone determines execution and response order. An earlier transaction
may fund a later one; changing their orders can therefore produce a different valid
refusal. Physical array placement is retained only for structural-error locations.
The application MUST preserve internal debit/credit array order, MUST NOT rebuild
items from maps, and may only stably arrange the outer items by validated `order`.

This follows the JSON data model: [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259.html#section-1)
defines an array as ordered and an object as unordered, while its
[array grammar](https://www.rfc-editor.org/rfc/rfc8259.html#section-5) retains element
sequence. Go's [`encoding/json.Unmarshal`](https://pkg.go.dev/encoding/json#Unmarshal)
decodes array elements into their corresponding slice positions. Object-property
order is not part of request identity. The canonical batch identity sorts only the
outer items by logical `order`; it preserves action, content, and every nested array
order. Thus a physical outer-array permutation with the same items/orders replays,
while changing an item's action or logical order conflicts for a reused explicit key.

All prepared transactions enter the accounting engine in one Lua invocation. A
confirmed business refusal applies none of their monetary effects, and no caller
can observe a committed prefix. Once Lua applies the execution, every balance and
the version-two recovery evidence are published atomically. SQL/MongoDB projections
can finish item by item afterward, but recovery only completes that already-applied
execution and never invokes accounting again.

## Idempotency and recovery

`X-Idempotency` is the client key. If omitted, the server uses the canonical request
fingerprint as the effective key. Canonicalization ignores insignificant whitespace
and object-property order as described above. An identical terminal retry
returns the original byte-stable ordered response and sets
`X-Idempotency-Replayed`; reusing an explicit key for a different canonical request,
or retrying while an applied execution is still recovering, returns `0084`.

The replay retention is frozen when the request executes (`X-TTL`, default 300
seconds). An applied batch writes one version-two engine recovery record per
transaction plus collective receipt/protection evidence. It does not write the
legacy transaction write-behind cache or `backup_queue:{transactions}`. Recovery
projects each item idempotently, seals the original ordered response after all items
are durable, and never recalculates balances.

## Rollout and rollback

The endpoint must be rolled out only with the version-two batch idempotency record
and engine recovery consumer enabled. Version-one records remain readable for their
retention window and replay through their compatible projection path; new records
capture the initial per-item representation before acknowledgement so a later hold
commit or cancel cannot alter the create replay. A rollback must keep the recovery
consumer and Redis record readers compatible until outstanding version-two records,
receipts, protections, and replay TTLs have drained. It must never replay accounting
to repair a projection: recovery completes only evidence already atomically applied
by Lua.

## Validation and limits

Structural validation examines all items before fees, persistent reads, Tracer, or
accounting. Its batch-specific primary error is `0517` / HTTP 400, `Invalid
Transaction Batch`. Its RFC 9457 `errors[]` details are ordered by physical,
zero-based transaction index and then field location, each naming its logical order
when that order is valid; they are capped at 100 entries with an explicit truncation
marker. State-dependent preparation and accounting stop at the first failing item in
logical order.

Account-closing protection applies to the batch as one execution. If any item
touches a closed account, the whole batch is refused with `0519` / HTTP 422; an
account whose closing is in progress refuses the whole batch with `0522` / HTTP
409. Neither refusal applies any item or exposes a committed prefix.

| Boundary | Effective limit | Failure |
| --- | ---: | --- |
| Decoded request body | smaller than 1 MiB | `0143`, HTTP 413 |
| Transactions | 1 to `TRANSACTION_BATCH_MAX_SIZE` (default 10; absolute maximum 50) | `0514`, HTTP 400 |
| Aggregate input debit/credit legs | 1,000 | `0515`, HTTP 400 |
| Expanded postings after fees | 100 | `0516`, HTTP 422 |
| Execution balances, including overdraft companions | 150 | `0516`, HTTP 422 |
| Encoded completion plans | 256 KiB | `0516`, HTTP 422 |
| Serialized accounting request | 256 KiB | `0516`, HTTP 422 |
| Prepared execution representation | 1 MiB | `0516`, HTTP 422 |
| Estimated recovery representation | 512 KiB | `0516`, HTTP 422 |
| Cached terminal response representation | 1 MiB | `0516`, HTTP 422 |

The cardinality environment setting may only lower the accepted number of items;
the work and byte ceilings are fixed code-owned safety boundaries. A request that
passes the body and input-leg checks can still exceed a post-fee derived limit.
Derived-budget rejection reports the first crossing transaction and does not consume
the idempotency claim.

The release benchmark and rationale for the 100-posting/150-balance ceilings are
recorded in [the engine performance report](../performance/engine-report.md#atomic-transaction-batch-release-gate).
