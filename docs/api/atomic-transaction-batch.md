# Atomic transaction batch (v2)

`POST /v2/transactions/direct/batch` creates several direct-v2 transactions as one
ordered, all-or-none monetary decision. It is a dedicated contract: the singular
`POST /v2/transactions/direct` continues to accept one transaction object, and each
route rejects the other route's request shape.

## Contract

The request is a wrapper whose `transactions` member contains the unchanged
direct-v2 request model:

```json
{
  "transactions": [
    {
      "description": "credit first",
      "asset": "BRL",
      "amount": "10",
      "debits": [{"alias": "@cash", "organizationId": "00000000-0000-0000-0000-000000000001", "ledgerId": "00000000-0000-0000-0000-000000000002", "amount": "10"}],
      "credits": [{"alias": "@customer", "organizationId": "00000000-0000-0000-0000-000000000001", "ledgerId": "00000000-0000-0000-0000-000000000002", "amount": "10"}]
    }
  ]
}
```

Every item must resolve to the same organization and ledger. The endpoint has no
path or query parameters; scope is taken from the validated transaction legs. It
uses the same `midaz/transactions/post` authorization chain and per-item direct-v2
fee, Tracer, route, overdraft, skip, and account-block-exception rules.

Success is HTTP 201 with a wrapper containing `batchId` and the created
`TransactionV2` objects. The response `transactions` array has the exact
request-array order. `batchId` is
an ephemeral idempotency and recovery correlation value. It is not stored on the
transaction rows or events, and there is no batch resource or query endpoint;
query created transactions through their individual transaction IDs.

## Ordering and atomicity

Array position is execution order. An earlier transaction may fund a later one;
reversing those items can therefore produce a different, valid refusal. The
application MUST preserve the decoded slice and MUST NOT sort it or reconstruct it
from a map.

This follows the JSON data model: [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259.html#section-1)
defines an array as ordered and an object as unordered, while its
[array grammar](https://www.rfc-editor.org/rfc/rfc8259.html#section-5) retains element
sequence. Go's [`encoding/json.Unmarshal`](https://pkg.go.dev/encoding/json#Unmarshal)
decodes array elements into their corresponding slice positions. Object-property
order is not part of request identity; array order at every nesting level is.

All prepared transactions enter the accounting engine in one Lua invocation. A
confirmed business refusal applies none of their monetary effects, and no caller
can observe a committed prefix. Once Lua applies the execution, every balance and
the version-two recovery evidence are published atomically. SQL/MongoDB projections
can finish item by item afterward, but recovery only completes that already-applied
execution and never invokes accounting again.

## Idempotency and recovery

`X-Idempotency` is the client key. If omitted, the server uses the canonical request
fingerprint as the effective key. Canonicalization ignores insignificant whitespace
and object-property order but preserves all array order. An identical terminal retry
returns the original byte-stable ordered response and sets
`X-Idempotency-Replayed`; reusing an explicit key for a different canonical request,
or retrying while an applied execution is still recovering, returns `0084`.

The replay retention is frozen when the request executes (`X-TTL`, default 300
seconds). An applied batch writes one version-two engine recovery record per
transaction plus collective receipt/protection evidence. It does not write the
legacy transaction write-behind cache or `backup_queue:{transactions}`. Recovery
projects each item idempotently, seals the original ordered response after all items
are durable, and never recalculates balances.

## Validation and limits

Structural validation examines all items before fees, persistent reads, Tracer, or
accounting. Its RFC 9457 `errors[]` details are ordered by zero-based transaction
index and then field location, capped at 100 entries with an explicit truncation
marker. State-dependent preparation and accounting stop at the first failing item
in array order.

| Boundary | Effective limit | Failure |
| --- | ---: | --- |
| Decoded request body | smaller than 1 MiB | `0143`, HTTP 413 |
| Transactions | 1 to `TRANSACTION_BATCH_MAX_SIZE` (absolute maximum 50) | `0513`, HTTP 400 |
| Aggregate input debit/credit legs | 1,000 | `0514`, HTTP 400 |
| Expanded postings after fees | 100 | `0515`, HTTP 422 |
| Execution balances, including overdraft companions | 150 | `0515`, HTTP 422 |
| Encoded completion plans | 256 KiB | `0515`, HTTP 422 |
| Serialized accounting request | 256 KiB | `0515`, HTTP 422 |
| Prepared execution representation | 1 MiB | `0515`, HTTP 422 |
| Estimated recovery representation | 512 KiB | `0515`, HTTP 422 |
| Cached terminal response representation | 1 MiB | `0515`, HTTP 422 |

The cardinality environment setting may only lower the accepted number of items;
the work and byte ceilings are fixed code-owned safety boundaries. A request that
passes the body and input-leg checks can still exceed a post-fee derived limit.
Derived-budget rejection reports the first crossing transaction and does not consume
the idempotency claim.

The release benchmark and rationale for the 100-posting/150-balance ceilings are
recorded in [the engine performance report](../performance/engine-report.md#atomic-direct-v2-batch-release-gate).
