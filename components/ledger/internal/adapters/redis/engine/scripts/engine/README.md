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
| `fee_debt.lua` | Fee-debt lists: loading, deferral, settlement, cancellation, reopening and refund, in memory only. |
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
   and builds truthful movements and version chains without writing Redis. It
   applies fee-debt changes to the lists `loadFeeDebts` read (`fee_debt.lua`).
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
    published companion balances, changed fee-debt lists, synchronization
    schedule members, refreshed companion expiries, recovery records, guards,
    protection coordinators, and index entries, deletes consumed grant keys, and
    finally writes the receipt.

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
`collect` or `refund` posting is byte-identical to today in request, response,
receipt and recovery record.

### Live state

One key per debtor balance: `utils.FeeDebtInternalKey(org, ledger, "alias#key")`,
i.e. `fee-debt:{transactions}:<org>:<ledger>:<alias#key>`, behind the same
tenant prefix as the balance keys. Absent means the debtor never had a debt. No
TTL and never deleted, so `seq` is never reused. Lua writes a changed value with
`SET` (no `EX`) only in `commitPreparedExecution`, after the balances and before
the receipt, charging it to the prepared-byte budget. Go's copy (`FeeDebtItem`,
only `id` and `creditRef`) is a seed; Lua always reads the live key.

```json
{"v":1,"nextSeq":"4","items":[
  {"id":"<originTx>:<debitPostingRef>","creditRef":"@fees#default","remaining":"40",
   "opened":"70","originTransactionId":"<uuid>","seq":"3","assetCode":"BRL",
   "debitRoute":{"id":"<routeId>","code":"<rubric>","description":"<rubric>",
                 "revertCode":"<rubric>","revertDescription":"<rubric>"},
   "creditRoute":{"id":"<routeId>","code":"<rubric>","description":"<rubric>",
                  "revertCode":"<rubric>","revertDescription":"<rubric>"}}]}
```

- `v` is the JSON number 1. A stored value that breaks any rule below is
  corrupt live state and fails the execution with `fee_debt_conflict`.
- `nextSeq` and `seq` are decimal integer strings; `nextSeq` starts at `"1"`.
- `items` (possibly `[]`) is in settlement order and `seq` strictly ascends
  along it. `id` is the identity and is unique in the list.
- `opened` is the amount the debt opened with and never changes;
  `0 < remaining <= opened`, both canonical decimals.
- `debitRoute` and `creditRoute`, each absent or `{id, code, description}` with a
  non-empty `id` and string rubric texts, are the operation routes of the fee's
  payer debit and fee-account credit, stored at opening and never changed. Go
  adds `revertCode` and `revertDescription` when the route's revert entry has a
  rubric for the opposite side; Lua stores them as given.
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
- Collect, refund and deferral debtors are always postings. A reopen debtor is
  in `balances` as a touch, with no movement, because a revert can fold its
  legs to zero; a debtor that only cancel reaches need not appear in
  `balances`. A revert declares the debtors of its parent's `feeDebtOpenings`
  and `feeDebtSettlements` and, on `/v2`, each debtor that gets a collect (see
  "Revert").

Per transaction, omitted when empty:

```json
"reopenFeeDebts":[{"debtId":"<O>:<debitPostingRef>","debtorRef":"@payer#default",
  "creditRef":"@fees#default","amount":"12.5","opened":"70","seq":"3",
  "debitRoute":{...},"creditRoute":{...}}]
```

Per posting, each omitted when false or empty: `deferShortfall` (bool, debit
only), `fundedByRef` (string, credit only), `debtRoute` (route object, on a
`deferShortfall` debit or its `fundedByRef` credit: the route that leg books
under), `items` (array of debt ids, collect only) and `refunds` (array, refund
only). Go appends a collect posting right
after each eligible credit, with the seed's item ids of the credited balance,
oldest first:

```json
{"ref":"<creditPostingRef>:collect","balanceRef":"<the credited balance>","type":"collect",
 "amount":"<the credit amount>","drawPolicy":"forbidden","overdraftAmount":"0",
 "items":["<O>:<debitPostingRef>"]}
```

On a revert of a transaction whose metadata carries `feeDebtOpenings` (see
"Revert"), Go appends after every other posting one refund posting per debtor
of that list, `n` counting them from 0, its entries in list order:

```json
{"ref":"fee-refund:<n>","balanceRef":"<the debtor>","type":"refund","amount":"<sum of opened>",
 "drawPolicy":"forbidden","overdraftAmount":"0",
 "refunds":[{"debtId":"<O>:<debitPostingRef>","creditRef":"@fees#default","opened":"70","seq":"3",
   "expectedRefund":"30"}]}
```

`expectedRefund` is what the Fees `fee_debt` projection says later credits
settled of that debt, net of reopens: the sum of its `settled` entries minus the
sum of its `reopened` entries, read from the exact text amounts of the entries,
never from the Decimal128 `remaining`. It stays out of the engine intent, so it
never changes a replay fingerprint. The refund debtor is an explicit balance, so
`LoadEngineSnapshotPool` already loads its overdraft companion; a debtor with
`overdraftUsed > 0` and no companion refuses with `overdraft_companion_missing`.

Collect and refund postings count toward the existing posting limit.
`request.lua` is the only owner of the deferral pairing (Go does not check it)
and refuses with `invalid_protocol` when:

- `deferShortfall` is on a non-debit or in a transaction whose `action` is not
  `direct`, or its payer `(scope, balanceRef)` is not declared in `feeDebts`;
- `fundedByRef` is on a non-credit or does not name an earlier `deferShortfall`
  debit of the same transaction with the same amount and asset; two credits name
  one debit; or a `deferShortfall` debit is named by no credit;
- `debtRoute` is on a posting with neither `deferShortfall` nor `fundedByRef`, or
  a route object (there, on an item or on a reopen) lacks a non-empty `id` or has
  a non-string rubric text;
- `items` is on a non-collect; a collect has no items or a duplicate, or its
  debtor key is not declared;
- `reopenFeeDebts` appears when `action` is not `revert`; a `debtId` repeats
  within its transaction; a reopen names an undeclared debtor key or a debtor
  absent from `balances`, a `debtId` whose first 36 characters are not a UUID
  followed by `:`, a non-positive amount, opened or seq, an amount above its
  opened, or a `creditRef` that no debit posting of the same transaction
  debits;
- `refunds` is on a non-refund; a refund posting appears when `action` is not
  `revert`, has no entries, an amount other than the sum of their `opened`, or
  an undeclared debtor key; an entry has a non-positive `opened` or seq, an
  `expectedRefund` below 0 or above its `opened`, a `creditRef` outside its
  scope's pool, or a `debtId` not starting with
  `<parentTransactionId>:`; a `debtId` repeats across the transaction's refund
  entries;
- `feeDebts` repeats an entry, or an entry's key index or key suffix is wrong.

### Execution

`take(available, owed) = min(max(available, 0), owed)`. Per transaction, before
its first posting:

1. Cancel (only when `action` is `revert`) runs over the declared lists of the
   transaction's scope: every item whose `originTransactionId` is its
   `parentTransactionId` is removed; one `canceled` change each, oldest first
   per list, amount = its `remaining`, remembered per debt for the refund.
2. Reopen, in array order: a live item with that `debtId` gains `amount` on its
   `remaining` (its `seq`, `creditRef` and `opened` must match, and `remaining`
   stays at most `opened`); otherwise the item is inserted where `seq` keeps
   ascending, with `remaining = amount`, the entry's `opened`, the UUID that
   leads `debtId` as `originTransactionId` and the `creditRef` balance's asset,
   and its `seq` must be below `nextSeq`. A mismatch is the technical error
   `fee_debt_conflict`. Each reopen first touches its debtor (posting index
   -1), so a deleted debtor refuses with `balance_deleted` and a closed or
   closing account refuses exactly as for any touched balance. One `reopened`
   change each.

Then the posting loop:

- `deferShortfall` debit on an internal balance whose direction is not debit:
  `paid = take(available, amount)`, `shortfall = amount - paid`. It moves `paid`
  and never draws overdraft, whatever `drawPolicy` says. Every other check
  (status, blocks, `allowSending`, asset) is unchanged. On any other balance it
  is an ordinary debit with shortfall 0.
- Its `fundedByRef` credit moves the debit's `paid`. When `shortfall > 0` it
  appends an item to the payer's list (`id = <txId>:<debitRef>`,
  `creditRef` = this credit's balance, `debitRoute` = the debit's `debtRoute`,
  `creditRoute` = this credit's) and emits one `opened` change whose
  `postingRef` is the debit's ref. A reopen inserting an item stores its entry's
  routes.
- Collect: `budget = take(debtor.available, amount)`, then it walks the live
  list from its head. It never calls `touch`, never refuses and ignores
  account-block exceptions. It stops softly, settling nothing more, when the
  debtor is external, debit-direction, deleted, live-blocked,
  `allowSending = false`, closing/closed or an unconfirmed seed; when the budget
  is 0; or at the FIRST live item whose id is not in `items`, whose position in
  `items` is not after the previous settled item's, or whose creditor is
  outside the pool, not credit-direction, the debtor itself, external, deleted,
  live-blocked, `allowReceiving = false`, closing/closed, an unconfirmed seed,
  `overdraftUsed > 0`, or of another asset. Each settled item moves
  `take(budget, remaining)` as one debit of the debtor and one credit of its
  creditor, is removed at 0, and emits one `settled` change.
- Refund: per entry, `refund = opened - canceled`, where `canceled` is what
  step 1 of this transaction removed of that debt (0 when it was no longer
  live, i.e. fully settled). The execution refunds nothing and fails with
  `fee_debt_conflict` when the debtor's list is missing or its `nextSeq` is not
  above an entry's `seq`, and with `fee_debt_record_pending` when a refund
  differs from the entry's `expectedRefund`, which a retry clears once the
  record catches up. A lost list that a later deferral recreated past the
  entry's `seq` cancels nothing, so only `expectedRefund` stops it from
  refunding the whole `opened` of a debt still open; that revert holds for an
  operator instead. Each positive
  refund then debits the entry's `creditRef` as an ordinary debit (touch, no
  overdraft draw) and refuses the execution exactly as that debit would; an
  external or non-credit-direction creditor refuses with `insufficient_funds`.
  It is never partial. Each positive refund, in entry order, credits the debtor
  through the ordinary credit algebra: it repays `overdraftUsed` first, mirrored
  on the debtor's companion, and the rest raises `available`; then it debits
  its creditor and emits one `refunded` change. So the payer always gets back
  what later credits settled and never owes what was still open, whatever the
  order of the settlements.

No fee-debt movement of amount 0 is recorded, except a refund's debtor credit
that went entirely to overdraft: its `overdraftDelta` carries the repayment.

### Movements

Every fee-debt movement is per item, its ordinal the item id's 0-based index in
`items` or the entry's in `refunds`. A collect posting yields, per settled item,
one `fee_debt_debit` on its debtor (type `debit`) then one `fee_debt_credit` on
the item's `creditRef` (type `credit`), both of its settlement and with
`overdraftDelta = "0"`. Several credits may land on one creditor. A refund
posting mirrors it per positive refund: one `fee_debt_refund_credit` on its
debtor (type `credit`, the part that reached `available`, `overdraftDelta` =
minus the overdraft it repaid), then, when it repaid overdraft, one
`overdraft_companion` credit of that amount on the debtor's companion at the
entry's ordinal, then one `fee_debt_refund_debit` on the entry's `creditRef`
(type `debit`, `overdraftDelta = "0"`). Every ref keeps the form
`<txId>:<len(postingRef)>:<postingRef>:<role>:<ordinal>`; primary and an
ordinary companion use ordinal 0. Movements follow posting order and, within a
posting, sub 0 (primary), sub 1 (companion); a fee-debt item k takes sub 3k
(debtor), 3k+1 (companion) and 3k+2 (creditor). Readers order by
`(postingIndex, sub)`.

### Response, receipt and recovery

The response envelope gains `"feeDebt":[...]`, omitted when empty; the receipt
stores that response verbatim, so replay returns it, and receipt validation
accepts the four new roles and their ordinals, and a companion movement after a
refund credit that repaid overdraft. Each recovery record's
`record.result` gains the same array holding only that transaction's changes,
omitted when empty. Changes keep execution order. One change:

```json
{"transactionId":"<uuid>","postingRef":"fee-debit","kind":"opened",
 "debtId":"<originTx>:fee-debit","debtorRef":"@payer#default","creditRef":"@fees#default",
 "originTransactionId":"<uuid>","seq":"3","assetCode":"BRL","amount":"70","opened":"70"}
```

`kind` is `opened|settled|canceled|reopened|refunded`; `postingRef` is the
deferrable debit (opened), the collect posting (settled), the refund posting
(refunded) or `""` (canceled, reopened); `opened` is the debt's opened amount
and `0 < amount <= opened`. A result with fee-debt changes and no movement is
invalid in this release (`fee_debt_conflict` before any write): an execution
without movements stays a no-op, and the early return and the receipt's "no
movements" refusal are unchanged.

The adapter checks, per `deferShortfall` debit, that its primary amount (0 when
absent) plus its opened amount equals the posting amount and that the opened
`creditRef` is its credit's balance; per collect or refund posting, that each
change pairs with the debtor movement and the creditor movement at its entry's
ordinal: the debtor movement on the posting's balance, the creditor movement on
the change's `creditRef` and for its amount, and the debtor amount plus the
overdraft it repaid (which its companion mirrors) equal to that amount; and that
a collect's total does not exceed the posting amount.

### Completion (Go)

Collect movements persist as `FEE_SETTLEMENT` rows and refund movements as
`FEE_REFUND` rows, a refund's companion included. Their completion contexts have
an empty `OriginRef` and match a movement by `(PostingRef, Role, ordinal)` with
`BalanceRef` checked; every ordinal other than a primary's is read from the
movement's ref. Rows carry no fee-debt metadata. Each row books under the fee's
route: a settlement under the item's stored route and rubric, a refund under the
opening's route with the revert rubric stored with the debt (its companion with
the route's live overdraft rubric). The chart of accounts is empty on every fee leg, so the rubric is
what classifies these rows.

`BuildTransactionWriteSet` derives two reserved transaction metadata keys from
the result alone, so a recovery replay writes the same bytes. Each holds a JSON
array in result order and is absent when the result has no change of its kind:

- `feeDebtOpenings`: one `command.FeeDebtOpening` (`debtId`, `debtorRef`,
  `creditRef`, `opened`, `seq`, and the item's `debitRoute` and `creditRoute`
  when it has them) per `opened` change.
- `feeDebtSettlements`: one `command.FeeDebtSettlement` (`debtId`, `debtorRef`,
  `creditRef`, `amount`, `opened`, `seq`, and the routes) per `settled` change,
  so a debt that two collects settled appears twice.

A pending commit's completion merges them into the transaction's existing
metadata and never replaces it. `TransactionRevert` copies the parent's metadata
onto the reversal, so completion of a revert R removes both keys as inherited
and writes only the values computed from R's own result; an inherited
`feeDeferPair` on a reversal leg stays, like `feeLeg`, and translation ignores it
outside `direct`. Completion is the only writer of every reserved fee-debt key,
and each survives any client update of transaction or operation metadata,
including one that sends no metadata.

Completion projects the result's changes through `FeeDebtRecorder` after the
metadata is confirmed and before any event. Production always wires it; a nil
recorder exists only in tests and skips the projection.

### Revert

A revert R of transaction P reads P's `feeDebtOpenings` and `feeDebtSettlements`
from the reversal payload's metadata, which `TransactionRevert` copies from P.
That holds on both routes that resolve P and for a grouped revert:
`prepareRevertTransaction`, which the cross-ledger group revert calls per
member, resolves P through `loadLifecycleResolution`, from engine evidence (a
lookup built by `BuildTransactionWriteSet` from plan and result) or from the
primary with P's Mongo metadata, and completion confirms that metadata before
the evidence is reaped. Rows are not the source: the primary route loads
operations without metadata, and an unpaid fee writes none. The Fees `fee_debt`
documents are not either: they lag completion and round `remaining` past 34
digits. They source only each refund's `expectedRefund`, which the revert reads
through `FeeDebtRecorder.Settled` from the exact entries, so a lag makes the
revert refuse with `fee_debt_record_pending`, never refund the wrong amount.

- Declared lists: the debtors of P's `feeDebtOpenings` (refund and cancel) and
  `feeDebtSettlements` (reopen) and, on `/v2`, each debtor that gets a collect.
  An item whose origin is P lives only in the list of a debtor P opened it for,
  since a reopen restores it to the same `debtorRef`, so the openings debtors
  cover every item cancel can find, a payer that is a source of the reversal
  included. A revert with nothing owed declares no key and stays byte-identical.
- `/v1`: a `/v1` revert composes refund, reopen and cancel but never a collect,
  so reverting a `/v2` transaction through `/v1` still reverses its fee fully;
  a parent without fee-debt metadata reverts on `/v1` byte-identically.
- P opened debts: the payer gets the whole fee back. The row reversal returns
  what P paid, one refund posting per debtor of `feeDebtOpenings` returns what
  later credits settled, and step 1 cancels what is still open.
- P settled debts: Go sums the `feeDebtSettlements` entries of each `debtId`
  into one `reopenFeeDebts` entry (`amount` = the sum; `debtorRef`,
  `creditRef`, `opened` and `seq` are the same on every entry), the reopens
  ordered by `seq`, and skips a debt whose origin is already reverted. Go
  learns that with `GetParentByTransactionID` on the origin (the UUID leading
  `debtId`) in the debtor's scope, the same read the revert gate uses.
- C, a credit that settled an O debt, reverted before O: C's revert reopened
  the debt, so O's revert cancels `opened` and refunds 0.
- C reverted after O: Go reopens no debt whose origin is already reverted, and
  C's revert takes that settlement back from the debtor, not from the fee
  account that O's refund already charged, reading its `debtId`, `creditRef`
  and routes from `feeDebtSettlements`.
- The reversal folds C's `FEE_SETTLEMENT` rows per balance and route: a fee
  account's net settlement is taken back under the route its row carries, so a
  fee account that C also credited under another route keeps both legs. The
  take-back books to the debit rubric of the fee route's revert entry, stored
  with the debt, and route validation holds it to none of C's transaction
  routes, which never list the fee's.
- Both reverts in flight at once is an accepted ceiling: when O's executes
  first but C's read O as not reverted, C reopens the debt and charges the fee
  account again; the reopened debt converges when a later collect settles it.
- Ceiling: one entry per debt the transaction opened or settled, about 170
  characters with typical aliases and no routes, about 520 when both routes
  carry direct and revert rubrics. The
  2000-character metadata limit is a request-body validator and stored values
  are never re-validated, so a longer value is stored and read unchanged.
