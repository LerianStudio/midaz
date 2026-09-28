# Engine Lua fragments

The Go adapter embeds these fragments in the order declared in `script.go` and
concatenates them with `scripts/engine.lua`. Redis receives the resulting source
as one script, so all validations, calculations, and writes remain within one
atomic execution.

| Fragment | Responsibility |
| --- | --- |
| `decimal.lua` | Exact decimal comparison, addition, and subtraction without converting monetary values to Lua numbers. |
| `json.lua` | Exact JSON parsing and deterministic serialization, including preservation of large numeric tokens. |
| `protocol.lua` | Shared protocol primitives and validation of text, UUIDs, integers, references, and money. |
| `balance_cache.lua` | Compatibility decoding, validation, and encoding of live balance-cache records. |
| `request.lua` | Redis key-type checks and validation of the protocol-v3 execution request, including per-balance and per-transaction scope. |
| `receipt.lua` | Validation and replay of engine execution receipts; this closes the lost-response window independently of HTTP idempotency. |
| `posting_algebra.lua` | Monetary meaning of each supported posting type. |
| `execution.lua` | Protection checks, live-state loading, in-memory application, write preparation, and commit. |
| `../engine.lua` | Entrypoint and closed error-protocol translation. |

The fragments are not standalone Redis scripts and must not use `require`,
`dofile`, or filesystem access. Keep their dependency order explicit in
`script.go`. Apply the shared cache policy only after concatenation.

All predictable failures and serialization work must remain before
`commitPreparedExecution`. Redis writes belong only in that function. Once
`commitStarted` is true, any failure is indeterminate because earlier Redis
commands are not rolled back.

## Runtime call chain

The entrypoint tells one ordered story:

1. `main` validates the six ARGV values (payload, byte budgets, and trusted
   transaction/posting/balance limits), decodes the request, and calls
   `execute`.
2. `prepareExecutionProtection` calls `storedReceipt` first. A valid receipt
   returns the exact prior response without loading balances. A new execution
   validates shared key types, lifecycle guards, recovery conflicts, and bounded
   causal references against the current transaction-state index, then prepares
   protection coordinators in memory.
3. `loadBalancePool` reads live cache values. A valid Redis value is
   authoritative; a request snapshot is only an in-memory seed for a cache miss.
   Noncanonical legacy limits request a separate precommit repair.
4. `loadAccountProtection` reads the closing controls of every account of the
   pool, and `validateAccountClosingAvailability` refuses every requirement and
   posting whose account is closing or closed, and every used balance this
   execution would seed without a confirmed admission (`admissionConfirmed`). The
   check is unconditional: no skip, permission, cancellation, or account-block
   exception exempts it, and a companion that only sits in the pool is untouched.
   An unused marker pair is the normal state of an open account; a marker that
   exists but carries no value refuses technically.
   `loadSeedAdmission` reads the administrative ownership key by its type: absent
   holds nothing; a string is an exclusive owner (closing, balance creation or
   deletion) and never confirms a seed, even when it carries the request's token;
   a sorted set holds the shared seed admissions, and a seed is confirmed only
   when the request's token is a live member (`ZSCORE`). A blank string owner or
   any other type refuses with `account_protection_unreadable`. The engine only
   reads that key; the balance loads that took the admissions release them.
5. `validateAccountBlockExceptions` re-reads every presented single-use grant,
   compares its alias and amount with the transaction's bound primary outflow,
   and prepares a transaction-local exemption for that primary balance and its
   overdraft companion. Missing, expired, consumed, malformed, or mismatched
   grants refuse before any write.
6. `validateLiveBalanceAvailability` checks both deletion-marker namespaces
   and the live account-block control for declared requirements and postings.
   A valid grant bypasses only the block and sending controls for its bound
   account pair; deletion markers and every other balance remain enforced.
   Cancellation explicitly disables the block control; generated companions
   repeat both protections at their exact mutation site.
7. `applyTransactionsInMemory` validates live asset/permission requirements,
   runs the closed `postingAlgebra`, resolves real overdraft draws or repayments,
   and builds truthful movements and version chains without writing Redis.
8. `selectCompanionCacheWrites` picks, for every account whose balances this
   execution writes, the overdraft companion that must stay cached beside them:
   an unmoved cached companion gets its expiry refreshed, and an unmoved seeded
   companion is published when `admissionConfirmed` proves its account's
   admission, the account is neither closing nor closed, and neither deletion
   marker is set. Without that proof the companion stays uncached and the
   execution is not refused. A published companion keeps its seed's amounts and
   version but takes the account-level `blocked` flag from a balance of its
   account that the execution writes and read from Redis: an account PATCH
   rewrites that flag only on cached balances, so the seed may carry a block
   state the account no longer has. When every written balance of the account
   was seeded too, the companion keeps its seed's flag. A refreshed companion
   keeps its cached value, flag included.
9. `prepareExecutionWrites` serializes the response, including the execution's
   single `appliedAtUnixMicro` value, changed balance blobs, published companion
   blobs, versioned write-behind evidence, transaction-state index entries,
   recovery records, receipt, guards, and protection data while enforcing the
   total prepared-byte ceiling. An execution without movements prepares none of
   them, so refusals and no-ops publish no companion.
10. `commitPreparedExecution` is the only publication phase. It receives the
    score derived from the same Redis `TIME` read and writes changed and
    published companion balances, synchronization schedule members, refreshed
    companion expiries, recovery records, guards, protection coordinators, and
    index entries, deletes consumed grant keys, and finally writes the receipt.

A published companion carries no movement and no version increment and appears
in neither the response nor the recovery evidence. Its synchronization is
version-guarded: it leaves PostgreSQL unchanged for a seed read from the current
row, and updates the row only when the seed was rebuilt ahead of it.

The receipt is written last deliberately: its presence means the complete
prepared command sequence returned through the final write. Recovery records are
written before it so a failure after a monetary write retains as much completion
evidence as possible. A recovery consumer may complete SQL/MongoDB projection and
acknowledge that evidence; it must never call this script to reapply accounting.

`execute` reads Redis `TIME` once after the replay short-circuit. The decimal
microsecond timestamp is assembled as a string before `numberToken` emits it,
avoiding floating-point precision loss; only the scheduling score uses numeric
seconds plus fractional microseconds.

## Declared key layout

`KEYS[1..7]` are the schedule, recovery, receipt, guard, protection,
transaction-index, and evidence keys for the execution's primary scope. The
receipt remains in that scope. Each transaction's guard, protection, index,
and evidence live in the scope of that transaction. Its index records the
scope of the receipt that wrote it. A dependency is accepted only while the
index still names the referenced execution and both evidence and receipt
remain present.
Each balance then contributes one ordered triplet: live balance, dedicated
deletion marker, and compatibility deletion marker. After all balance triplets,
each transaction that presents an account-block exception contributes exactly
one grant key, in transaction order. The request carries the corresponding
one-based key index; Lua verifies the tail position and exception-ID suffix.
Balance triplets and grant keys are derived from their balance or transaction scope;
logical balance references are indexed by organization, ledger, and reference
inside Lua so equal aliases in different ledgers cannot collide.

The account block has one triplet per account of the balance pool, in
ascending account order: the account-closing marker, the account-closed marker,
and the administrative ownership key. Every one of them carries the complete
organization, ledger, and account scope, and Lua verifies that each ends in its
own account identifier. The request declares those accounts in the same order,
each with the administrative admission token its caller holds — empty when the
caller owns none. The token is private to that boundary and reaches no receipt,
recovery record, or public contract.

For multi-scope executions, five keys per additional transaction scope follow
the account block: receipt, guard, protection, transaction index, and evidence.
They are ordered by organization and ledger ID. The receipt key for an
additional scope is available to validate a dependency on an earlier execution
whose primary scope was that ledger; the new execution still writes one receipt
at `KEYS[3]`. A single-scope request has no added keys or `scopeKeys` field.

A valid stored receipt is checked before the live grant. Replaying the same
execution therefore returns its prior result after the grant has been consumed;
a new execution cannot reuse that now-missing grant.

## Maintenance rules

- Keep every live balance-dependent decision inside this one Redis execution.
  Go may supply seeds and declarative postings but must not pre-approve funds,
  approve a cached account-block exception, calculate the authoritative
  overdraft split, or retry on a snapshot version.
- Add route/version behavior by changing Go posting composition when the monetary
  algebra is unchanged. Do not fork the Lua engine merely to mirror API versions.
- Perform predictable validation, calculation, JSON encoding, and size checks
  before `commitPreparedExecution`. No business refusal belongs after writes
  begin.
- Monetary values remain canonical decimal strings. Do not use Lua `tonumber`
  for amounts; it is reserved for small bounded protocol values and Redis time.
- Keep fragment dependencies explicit in `script.go`. A fragment may call only
  functions defined by an earlier fragment or by the final entrypoint.
- Treat a timeout, transport failure, invalid response, or post-commit runtime
  failure as potentially applied. Only a validated receipt replay, confirmed
  NOSCRIPT fallback, precommit normalization pass, or projection recovery may be
  repeated automatically.

## Fee-debt protocol (frozen contract)

This section is the frozen wire contract of pending fees: an unfunded deferrable
fee opens a debt, and later credits to the payer settle it oldest first. The Go
names live in `internal/domain/accounting`; `contract_test.go` locks their JSON.
Lua emits object keys sorted, so no reader depends on key order. An execution
that declares no fee-debt key, sets none of the fields below and has no
`collect` posting is byte-identical to today in request, response, receipt and
recovery record.

### Live state

One key per debtor balance: `utils.FeeDebtInternalKey(org, ledger, "alias#key")`,
i.e. `fee-debt:{transactions}:<org>:<ledger>:<alias#key>`, behind the same
tenant prefix as the balance keys. Absent means the debtor never had a debt. No
TTL and never deleted, so `seq` is never reused. Lua writes a changed value with
`SET` (no `EX`) only in `commitPreparedExecution`, after the balances and before
the receipt, charging it to the prepared-byte budget. Go's copy (`FeeDebtItem`)
is a seed only; Lua always reads the live key.

```json
{"v":1,"nextSeq":"4","items":[
  {"id":"<originTx>:<debitPostingRef>","creditRef":"@fees#default","remaining":"70",
   "originTransactionId":"<uuid>","seq":"3","assetCode":"BRL"}]}
```

- `v` is the JSON number 1; anything else is `invalid_protocol`.
- `nextSeq` and `seq` are decimal integer strings; `nextSeq` starts at `"1"`.
- `items` (possibly `[]`) is in settlement order and `seq` strictly ascends
  along it. `remaining` is a positive canonical decimal. `id` is the identity
  and is unique in the list.
- An opened item is appended with `seq = nextSeq`, then `nextSeq` advances.
- A deferrable debit with a shortfall on a payer whose list already holds 256
  items is refused with `insufficient_funds`. Reopening ignores the cap.

### Request

Top level, omitted when empty:

```json
"feeDebts":[{"organizationId":"<uuid>","ledgerId":"<uuid>","balanceRef":"@payer#default","keyIndex":57}]
```

- It is the union of every transaction's `feeDebtRefs`, once per
  `(organizationId, ledgerId, balanceRef)`, in first-appearance order over the
  transactions and then their refs.
- Its keys are the LAST block of `KEYS`, one per entry in the same order, so
  `#KEYS = 7 + 3*balances + grants + 3*accounts + 5*extraScopes + #feeDebts`
  and no existing index moves. `keyIndex` is the 1-based `KEYS` index, as for
  balances. Each key ends with the unprefixed `FeeDebtInternalKey` of its entry.
- A declared debtor need not appear in `balances`. On a revert Go declares
  every revert destination.

Per transaction, omitted when empty:

```json
"reopenFeeDebts":[{"debtId":"<O>:<debitPostingRef>","originTransactionId":"<O>",
  "debtorRef":"@payer#default","creditRef":"@fees#default","amount":"12.5","seq":"3"}]
```

Per posting, each omitted when false or empty: `deferShortfall` (bool, debit
only), `fundedByRef` (string, credit only) and `items` (array of debt ids,
collect only). Go appends a collect posting right after each eligible credit,
with the seed's item ids of the credited balance, oldest first:

```json
{"ref":"<creditPostingRef>:collect","balanceRef":"<the credited balance>","type":"collect",
 "amount":"<the credit amount>","drawPolicy":"forbidden","overdraftAmount":"0",
 "items":["<O>:<debitPostingRef>"]}
```

Collect postings count toward the existing posting limit. `request.lua` is the
only owner of the deferral pairing (Go does not check it) and refuses with
`invalid_protocol` when:

- `deferShortfall` is on a non-debit, or its payer `(scope, balanceRef)` is not
  declared in `feeDebts`;
- `fundedByRef` is on a non-credit or does not name an earlier `deferShortfall`
  debit of the same transaction with the same amount and asset; two credits name
  one debit; or a `deferShortfall` debit is named by no credit;
- `items` is on a non-collect; a collect has no items or a duplicate, or its
  debtor key is not declared;
- `reopenFeeDebts` appears when `action` is not `revert`; a `debtId` repeats
  within its transaction; a reopen names an undeclared debtor key, a
  non-positive amount or seq, or a `creditRef` that no debit posting of the same
  transaction debits;
- `feeDebts` repeats an entry, or an entry's key index or key suffix is wrong.

### Execution

`take(available, owed) = min(max(available, 0), owed)`. Per transaction, before
its first posting:

1. Cancel (only when `action` is `revert`): every item whose
   `originTransactionId` is the transaction's `parentTransactionId`, in any
   declared list of its scope, is removed; one `canceled` change each, oldest
   first per list, amount = its `remaining`.
2. Reopen, in array order: a live item with that `debtId` gains `amount` on its
   `remaining` (its `seq` and `creditRef` must match); otherwise the item is
   inserted where `seq` keeps ascending, with `remaining = amount` and the
   `creditRef` balance's asset, and its `seq` must be below `nextSeq`. A
   mismatch is a technical error. One `reopened` change each.

Then the posting loop:

- `deferShortfall` debit on an internal balance whose direction is not debit:
  `paid = take(available, amount)`, `shortfall = amount - paid`. It moves `paid`
  and never draws overdraft, whatever `drawPolicy` says. Every other check
  (status, blocks, `allowSending`, asset) is unchanged. On any other balance it
  is an ordinary debit with shortfall 0.
- Its `fundedByRef` credit moves the debit's `paid`. When `shortfall > 0` it
  appends an item to the payer's list (`id = <txId>:<debitRef>`,
  `creditRef` = this credit's balance) and emits one `opened` change whose
  `postingRef` is the debit's ref.
- Collect: `budget = take(debtor.available, amount)`, then it walks the live
  list from its head. It never calls `touch`, never refuses and ignores
  account-block exceptions. It stops softly, settling nothing more, when the
  debtor is external, debit-direction, deleted, live-blocked,
  `allowSending = false`, closing/closed or an unconfirmed seed; when the budget
  is 0; or at the FIRST live item whose id is not in `items` or whose creditor is
  outside the pool, not credit-direction, the debtor itself, external, deleted,
  live-blocked, `allowReceiving = false`, closing/closed, an unconfirmed seed,
  `overdraftUsed > 0`, or of another asset. Each settled item moves
  `take(budget, remaining)`, is removed at 0, and emits one `settled` change.

No movement of amount 0 is ever recorded.

### Movements

A collect posting yields one `fee_debt_debit` movement on its debtor (type
`debit`, ordinal 0, the total settled) followed by one `fee_debt_credit` per
settled item on that item's `creditRef` (type `credit`, ordinal = the item id's
0-based index in `items`, amount = its settlement), all with
`overdraftDelta = "0"`. Several credits may land on one creditor. Every ref
keeps the form `<txId>:<len(postingRef)>:<postingRef>:<role>:<ordinal>`;
primary, companion and `fee_debt_debit` use ordinal 0. Movements follow posting
order and, within a posting, sub 0 (primary or `fee_debt_debit`), then sub 1
(companion) or the `fee_debt_credit` movements in ascending ordinal; readers
order by `(postingIndex, sub)`.

### Response, receipt and recovery

The response envelope gains `"feeDebt":[...]`, omitted when empty; the receipt
stores that response verbatim, so replay returns it, and receipt validation
accepts the two new roles and their ordinals. Each recovery record's
`record.result` gains the same array holding only that transaction's changes,
omitted when empty. Changes keep execution order. One change:

```json
{"transactionId":"<uuid>","postingRef":"fee-debit","kind":"opened",
 "debtId":"<originTx>:fee-debit","debtorRef":"@payer#default","creditRef":"@fees#default",
 "originTransactionId":"<uuid>","seq":"3","assetCode":"BRL","amount":"70"}
```

`kind` is `opened|settled|canceled|reopened`; `postingRef` is the deferrable
debit (opened), the collect posting (settled) or `""` (canceled, reopened);
`amount` is positive. A result with fee-debt changes and no movement is invalid
in this release: an execution without movements stays a no-op, and the early
return and the receipt's "no movements" refusal are unchanged.

The adapter checks, per `deferShortfall` debit, that its primary amount (0 when
absent) plus its opened amount equals the posting amount and that the opened
`creditRef` is its credit's balance; per collect posting, that `fee_debt_debit`
is on the posting's balance, each `fee_debt_credit` pairs with one `settled`
change of the item at its ordinal, on that change's `creditRef` and for its
amount, and the total equals the debit and does not exceed the posting amount.

### Completion rows (Go)

Collect movements persist as `FEE_SETTLEMENT` rows. Their completion contexts
have an empty `OriginRef`, match a movement by `(PostingRef, Role, ordinal)`
with `BalanceRef` checked, and a `fee_debt_credit` context carries the flat
metadata `feeDebtDebtor`, `feeDebtId` and `feeDebtSeq` of its item.
