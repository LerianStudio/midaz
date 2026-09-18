# Account closing protection

`account.closed_at` in the onboarding PostgreSQL database is the only source of truth
about whether an account is closed. The cache carries exceptional controls that make
the transition safe to decide and cheap to enforce; none of them is a state, and none
of them may be read as one.

This runbook covers what those controls are, which writers honor them, how an
interrupted closing is reconciled, and the order a rollout and a rollback must follow.

## Controls

All three keys are addressed by the full scope — tenant prefix, organization, ledger
and account — and live in their own namespace, apart from the balance blobs. They share
the `{transactions}` hash tag.

| Control | Key | Value | Expiry | Meaning |
| --- | --- | --- | --- | --- |
| Closing | `<tenant>account-closing:{transactions}:<org>:<ledger>:<account>` | the attempt's token, plus whether its conditional write was already issued | none | one closing attempt owns this account; movements and balance admissions are refused |
| Closed | `<tenant>account-closed:{transactions}:<org>:<ledger>:<account>` | the confirmed `closed_at` instant | 300 s | negative cache: the account is closed, answered without a query |
| Ownership | `<tenant>account-admin-ownership:{transactions}:<org>:<ledger>:<account>` | the owning operation's admission token | none | administrative ownership shared by closing, balance creation, balance deletion and cache-miss admission |

There is **no `open` key**. An account that was never closed owns nothing at all, which
is what keeps a warm cache hit free of both a cache lookup for state and a database
read. Consequently there is nothing to backfill for existing accounts, in either
direction.

Two absences must never be confused:

- **Both controls absent** is the normal state of an open account and refuses nothing.
- **A control that cannot be read** — unreadable value, or a cache that does not answer
  — refuses with `0520`. Reading a protection failure as an absence would turn it into
  an authorization.

The closing marker deliberately carries **no releasing expiry**. An attempt whose
outcome could not be established keeps its protection until the outcome is resolved;
no amount of elapsed time turns an unresolved write into a resolved one.

The closed marker's 300 s is a cache horizon, not a lifetime. When it expires the next
operation reads the authoritative row, refuses the closed account and recomposes the
marker. Expiry never reopens an account and never admits a balance.

## Writers that honor the controls

The protection is only as strong as the least current writer. Every path that admits a
balance into the transaction cache, creates one, or removes one takes the same
administrative ownership over the account first:

| Writer | Path | What it does |
| --- | --- | --- |
| Cache-miss load and seed admission | `services/query/get_balances.go` | takes the ownership, reads `closed_at` from the PRIMARY, rebuilds the seed and only then releases; a closed account refuses with `0519` and recomposes the negative cache |
| Seed admission inside the execution | `adapters/redis/engine/scripts/engine/execution.lua` | re-reads the controls and the ownership token inside the atomic execution; an unconfirmed admission refuses with `admission_not_confirmed` before any write |
| Additional balance creation | `services/command/create_balance_additional.go` | ownership before the account is inspected; the overdraft companion is the same account and takes no ownership of its own |
| Default balance creation | `services/command/create_balance.go` | same ownership and the same `closed_at` check; the external account of an asset is exempt by type, since `0074` makes it ineligible for closing |
| Balance deletion | `services/command/delete_all_balances_by_account_id.go` | same ownership per account, serializing deletion against a closing; the delete markers keep their own keys and semantics |
| Settings, allow-flags and blocked propagation | `services/command/update_balance.go`, `services/command/update_account.go` | rewrite in place only; an absent key stays absent and is never recreated from an old snapshot |
| Limit repair | `adapters/redis/transaction/consumer.redis.go` | normalizes only a blob it observed; an absent key is ignored |
| Closing | `services/command/close_account.go` | ownership, then the closing marker, then the verification, the conditional write and the finalization |

Cache **hits** take no ownership and read no database row: the hot path is unchanged.

An operation whose write outcome is unknown — a cancelled context, an expired deadline,
a lost connection — keeps its ownership instead of releasing it. A SQLSTATE the server
answered with, and a refusal decided before any write, both release immediately.

## Order of a successful closing

1. Read the account from the PRIMARY: it must exist in the scope, not be external
   (`0074`), and not already be closed (`0514`).
2. Take the administrative ownership, then install the closing marker under the same
   token. A second attempt is refused with `0515`.
3. Verify, under that protection: every balance zero on `Available`, `OnHold` and
   `OverdraftUsed` (`0516`); no recovery record of this account still pending
   (`0518`); the live state proven persisted (`0518`, or `0520` when the evidence is
   inconclusive); no PENDING transaction holding the account's funds (`0517`). A
   dependency that cannot answer at all — the balance store, the cache, the operation
   trail, the pending query — is `0520` as well: the driver's own message stays on the
   span and in the log, never in the response.
4. Record the write intent on the marker, then issue the conditional
   `UPDATE … WHERE closed_at IS NULL AND deleted_at IS NULL … RETURNING closed_at`.
5. Finalize, in this order and each step confirmed before the next: evict every cached
   balance of the account, install the closed marker with its 300 s expiry, then remove
   the closing marker and the ownership by token.
6. Emit `account.closed` best-effort. A failure here is logged and does not change the
   204.

A pending transaction that names the account only as DESTINATION is not an impediment:
a hold reserves funds on the source alone and projects no operation row for the
destination. The inbound side is answered later — the commit of that pending is refused
because the destination is closed, while the cancel, which posts nothing on the
destination, still concludes.

## Reconciliation

`UseCase.ReconcileAccountClosings` runs on the ledger's lifecycle and is what resolves
the protection an interrupted attempt left behind, including across a restart. It
discovers work by a paginated scan of the protection namespace — not from anything held
in memory — and for each marker the authoritative row decides:

| Observed | Action |
| --- | --- |
| `closed_at` recorded | finish the same finalization: evict the balances, install the closed marker, remove the closing marker and the ownership by token. The instant is never rewritten |
| `closed_at` NULL and the attempt never recorded its write intent | release the closing marker and the ownership, conditionally on that exact phase |
| `closed_at` NULL and the write intent was recorded | leave everything in place. The write may still land, so a NULL column proves nothing |

Ownerships that remain after the closings of a pass are resolved belong to a balance
creation, a deletion or a cache-miss admission whose outcome this pass cannot establish.
They are counted as backlog and left where they are.

What reconciliation never does: reopen an account, rewrite an instant, reapply a
movement, or release a protection because time passed.

Operational signals, all without per-account labels: the pass counts scanned,
completed, released, retained, unreadable and the ownership backlog, and it reports the
age of the last COMPLETE pass. A pass that aborted on a scan does not advance that age,
because it reached an unknown part of the namespace.

### When a closing seems stuck

1. Read the account row. A recorded `closed_at` means the transition is durable and only
   the finalization is outstanding; the account is already refusing movements.
2. Check whether the cache is answering. While it is not, the protection stays by
   design, and removing a marker by hand would let a movement through.
3. Let a reconciliation pass run once the dependency recovers. It resumes the exact
   finalization that was interrupted.
4. Never delete a closing marker manually to "unblock" an account. The only safe removal
   is the conditional one reconciliation performs, which is tied to the attempt's token
   and to the phase that attempt had reached.

## Rollout

The close route may be exposed only after every writer above is running a version that
honors the controls. The order is:

1. Apply the nullable migration that adds `account.closed_at`. Existing accounts stay
   `NULL`; nothing infers a closing from history.
2. Deploy the reading, the immutability guard, the engine check and the coordinated
   admission to **every** binary that touches the balance cache — including background
   workers and any older replica still serving traffic. A single writer without the
   admission can readmit a balance of an account being closed.
3. Do not create any key for the accounts that stay open. There is no backfill, in
   either direction.
4. If the environment already carries closings from an earlier version, reconcile them
   before opening the route: for each such account, confirm the eviction of its cached
   balances under protection and let the negative cache be installed. A cache TTL is not
   a safe migration of stale blobs — a blob whose expiry has not elapsed is still
   servable.
5. Only then expose `POST /v2/.../accounts/{account_id}/close`.

## Rollback

Withdraw the route; keep the column and keep the protection in the writers.

- **Never clear `closed_at`.** The column is the record of a transition that already
  happened, and clearing it would readmit money to an account that was proven settled.
- **Never delete the controls to "free" an account.** A closed account must keep
  refusing movements whether or not its negative cache is present.
- Do not reintroduce a binary that lacks the coordinated admission while closed accounts
  exist: it would admit their balances back into the cache on the next miss.
- The down migration is for an environment with no closings and only compatible
  consumers. Verify both before running it.

## Related

- `docs/architecture/engine.md` — the preflight step that reads these controls and the
  key inventory that carries them.
- `docs/runbooks/transaction-recovery-inventory.md` — the recovery families a closing
  walks before it may conclude.
- `docs/performance/account-closing-report.md` — what the controls cost on the hot path
  and where the cost of one closing goes.
