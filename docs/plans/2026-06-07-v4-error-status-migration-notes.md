# v4 Error Status Changes — Mainline Reclassification

**Task 3.6.2** of the four-family error consolidation. This note records every mainline error code whose **HTTP wire status changed** in v4. The classification source of truth is `docs/plans/2026-06-07-error-code-migration.md` ("Mainline 400 reclassification (Task 3.6.1)"); the binding standard is E3/D2 in `docs/standards/error-handling.md`. The typed-struct flips live in `pkg/errors.go`'s `ValidateBusinessError` errorMap and are locked by `components/ledger/internal/adapters/http/in/mainline_error_contract_test.go`.

## D2 rationale (breaking-change window)

D2 designates v4 as the single breaking-change window in which error wire statuses are corrected to their E3-binding class, so that consumers absorb all status drift in one major version rather than across a trickle of patch releases. Each code below was previously mis-typed (business-rule violations served as 400, an input-format failure served as 409, and a client-supplied bad value served as 500); v4 moves them to the status the E3 standard mandates, and no further status changes for these codes are planned within the v4 line.

## Status changes (26 codes)

The `code`, `title`, and `message` of every arm are unchanged — only the typed struct (hence the HTTP status) moved.

| code | old status | new status | title | meaning (one line) |
|---|---|---|---|---|
| 0008 | 400 | 422 | Action Not Permitted | action disallowed in the current environment/state |
| 0010 | 400 | 422 | Account Type Immutable | account type cannot be modified |
| 0011 | 400 | 422 | Inactive Account Type Error | account type cannot be set to INACTIVE |
| 0012 | 400 | 422 | Account Balance Deletion Error | cannot delete an account with a remaining balance |
| 0013 | 400 | 422 | Resource Already Deleted | resource is already in a deleted terminal state |
| 0014 | 400 | 422 | Segment ID Inactive | referenced segment is inactive |
| 0024 | 400 | 422 | Account Status Transaction Restriction | account status does not permit the transaction |
| 0025 | 400 | 422 | Insufficient Account Balance Error | account lacks sufficient balance for the operation |
| 0026 | 400 | 422 | Transaction Method Restriction | method not permitted for the given accounts |
| 0030 | 400 | 422 | Mismatched Asset Code | parent account asset code conflicts with the request |
| 0073 | 400 | 422 | Transaction Value Mismatch | source/destination sums do not reconcile to the amount |
| 0074 | 400 | 422 | External Account Modification Prohibited | external accounts cannot be modified or deleted |
| 0086 | 400 | 422 | Race condition detected | optimistic-lock/version conflict on a balance |
| 0087 | 400 | 422 | Transaction Revert already exist | a revert for this transaction already exists |
| 0088 | 400 | 422 | Transaction is already a reversal | a reversal transaction cannot itself be reverted |
| 0089 | 400 | 422 | Transaction can't be reverted | transaction is not in a revertable state |
| 0090 | 400 | 422 | Transaction ambiguous account | same account used in sources and destinations |
| 0091 | 400 | 422 | ID cannot be used as the parent ID | self-referential parent relationship rejected |
| 0093 | 400 | 422 | Balance cannot be deleted | cannot delete a balance that still holds funds |
| 0124 | 400 | 422 | Additional Balance Creation Not Allowed | additional balances disallowed for external accounts |
| 0135 | 400 | 422 | Metadata Index Limit Exceeded | maximum metadata indexes reached for the entity |
| 0137 | 400 | 422 | Metadata Index Deletion Forbidden | system metadata indexes cannot be deleted |
| 0170 | 400 | 422 | Reserved Balance Key Error | the balance key is reserved for system use |
| 0156 | 400 | 409 | Duplicate Action Route | operation route already assigned to the action |
| 0017 | 409 | 400 | Invalid Script Format Error | DSL script is malformed (reverse fix: parse error is not a conflict) |
| 0096 | 500 | 400 | Invalid Account Alias | alias contains invalid characters (reverse fix: bad input is not a server fault) |

**Totals:** 23 codes 400→422, 1 code 400→409, 2 reverse fixes →400. **26 rows.**
