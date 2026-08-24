# Business & CRM — Midaz Ledger (v4)

**Dashboard UID:** `midaz-v4-business`
**Audience:** product, engineering working on ledger/CRM domain logic
**Method:** business-outcome counters plus a data-integrity check

## What this answers

- Are transactions and accounts actually being created?
- Which domain operations run, and which fail?
- Is the bulk recorder losing writes?
- Are reads being served by the replica or the primary?
- Is CRM field encryption running, in which mode, and is it failing?

## Panels that carry real weight

**Insertion gap (transactions)** — `bulk_recorder_transactions_attempted_total` minus
`bulk_recorder_transactions_inserted_total`. This must be zero. Any positive value means a
transaction was accepted and never reached the database. It is the one panel here that is
a correctness check rather than an activity chart, and it turns red at 1.

**Operations by result** — the only business error signal that exists. Span `status_code`
is always `STATUS_CODE_UNSET`, so no span-derived error rate is possible; see the
dictionary's `calls_total` entry.

**Mode resolution** — `legacy` versus `envelope` CRM encryption, selected by `KMS_VENDOR`.
Under migration to envelope mode, "Legacy-format reads" should trend to zero.

## Reading it correctly

**These panels use `increase()` per interval, not `rate()` per second.** Non-production write
volume is on the order of tens of events per week; a per-second rate rounds every one of
these series to zero and the dashboard looks dead when it is merely quiet. Sparse is the
correct appearance here — it reflects the environment, not a broken query.

**Transaction and account counts come from `domain_operations_total`**, filtered by
`operation="create_transaction"` / `operation="create_account"` with `result="success"` —
not from `transactions_processed_total` / `accounts_created_total`. Those two series still
exist in Mimir but appear nowhere in the Go source; they are legacy series inside
retention and would have read zero permanently. The dictionary records them under
"Legacy series — do not build panels on these", and CI rejects any panel that references
them again.

**`redis_backup_queue_depth_ratio` is a count, not a ratio.** The suffix comes from
`Unit: "1"` on the gauge declaration.

## No alerts attached

The insertion-gap panel is the first candidate for an alert once this dashboard is
promoted to production, since a nonzero value is unambiguous data loss.
