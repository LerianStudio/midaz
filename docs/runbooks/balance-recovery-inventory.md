# Balance recovery inventory

This inventory is read-only. It counts persisted transaction and recovery
artifacts by record family, format, and lifecycle action. It does not delete,
rewrite, replay, migrate, or activate anything.

## Record families

| Family | Storage contract | Persisted format |
| --- | --- | --- |
| Legacy pending and backup records | tenant-scoped `backup_queue:{transactions}` hash | unversioned `TransactionRedisQueue`; historical version-2 engine envelopes remain readable |
| Engine recovery records | tenant-scoped `engine:{transactions}:recover:v2` hash | envelope and nested payload `formatVersion=2` only |
| Execution receipts | `engine:{transactions}:receipts:{organization}:{ledger}` hash | receipt v1, classified separately with or without protection v1 |
| Transaction guards | `engine:{transactions}:guards:{organization}:{ledger}` hash | opaque lifecycle token |
| Protection coordinators | `engine:{transactions}:protection:{organization}:{ledger}` hash | coordinator v1 |
| Cleanup schedule | tenant-scoped `engine:{transactions}:recovery-cleanup` sorted set | canonical organization, ledger, and execution UUID tuple |
| Quarantine | tenant transaction PostgreSQL `transaction_backup_quarantine` | original backup payload retained verbatim |

The Go seam `command.BuildRecoveryInventoryPage` accepts at most 1,000 records
from one tenant and one externally managed page. The external reader must also
enforce a byte budget before passing raw values to the classifier. It reports
counts only, maps explicit `direct` and `hold` actions to `create`, preserves
`revert`, `commit`, and `cancel`, and reports `unknown` when an artifact does not
persist an action. Malformed issue keys are represented by a truncated SHA-256
digest. Payloads, balances, aliases, amounts, and metadata are never returned.

## Safe enumeration

Resolve one authenticated tenant connection before reading any Redis or
PostgreSQL family. Never combine records from different tenant contexts in one
report page.

- Read hashes with `HSCAN` and a positive `COUNT` no greater than 1,000. Redis
  treats `COUNT` as a hint, so split an oversized returned batch locally without
  dropping entries and enforce both record-count and byte budgets. Continue from
  the returned cursor; do not use `HGETALL` for inventory.
- Retain independent cursors for `backup_queue:{transactions}` and
  `engine:{transactions}:recover:v2`. Do not merge records by field: the same
  `transactionUUID:executionUUID` field can exist independently in both hashes.
- Read the exact tenant cleanup sorted-set key with `ZSCAN` under the same bound.
- Enumerate receipt, guard, and protection hashes only from a bounded,
  authoritative organization/ledger scope list. Use their exact scoped keys;
  do not discover them with `KEYS` or an unbounded wildcard scan.
- Read quarantine with stable PostgreSQL keyset pagination ordered by
  `(quarantined_at, id)` and `LIMIT 1000`. Select the raw payload only into the
  classifier process; never print or log it.
- Retain each source cursor independently. A page is complete only when that
  source returns its terminal cursor or keyset page. A report marked complete
  must not carry a next cursor.

The classifier does not mutate its input, so legacy `overdraftAmount` values and
all fallback-bearing payload bytes remain unchanged.

## Evidence boundary

An empty or terminal page is not drain evidence by itself. A complete inventory
requires terminal cursors for every exact key family, every authoritative
organization/ledger scope, and every tenant, plus a terminal quarantine page.
Concurrent writes can appear after a cursor passes a bucket, so operational
drain evidence also requires an independently controlled observation window or
quiescence procedure.

Balance-cache TTL does not expire recovery envelopes, receipts, guards,
coordinators, cleanup members, or quarantine rows. Never use balance TTL or an
incomplete-scan absence to declare these families drained.

During rollback, an older binary cannot see `engine:{transactions}:recover:v2`.
Keep a compatible recovery consumer running until that hash is drained, or roll
forward to a compatible version. Do not copy records into the legacy hash.
