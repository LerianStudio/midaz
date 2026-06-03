# P4-T01 — `pkg/transaction@v3.5.2` vs `pkg/mtransaction@HEAD` field diff + import repoint

**Scope:** the fee engine (now embedded under `components/ledger`) was compiled against the published
`github.com/LerianStudio/midaz/v3 v3.5.2` package `pkg/transaction` (Go package name `transaction`).
At midaz HEAD that package no longer exists; it has been renamed and evolved into `pkg/mtransaction`
(Go package name `mtransaction`). This document enumerates every `transaction.*` symbol the fee code
consumes, its v3.5.2 shape vs the HEAD `mtransaction` shape, and the behavioral follow-ups that this
**pure-mechanical relocation chunk deliberately did not absorb**.

## Repoint summary (mechanical)

- Import path rewritten in all moved files: `github.com/LerianStudio/midaz/v3/pkg/transaction`
  → `github.com/LerianStudio/midaz/v3/pkg/mtransaction`.
- **Package-name drift handled with an alias, not a rename of call sites.** The HEAD package is named
  `mtransaction`, but every fee file qualifies it as `transaction.` (e.g. `transaction.Responses`).
  To keep this chunk zero-diff at the call sites, the import is aliased back to `transaction`:
  `transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"`. 14 moved files (the ones that use
  the `transaction.` qualifier) carry this alias; `feeshared/nethttp/body_validator.go` already carried
  its own alias `modelTransaction` and was left untouched.
- **Grep gate (T01 acceptance):** `grep -rn 'midaz/v3/pkg/transaction"' .` from the fees working-tree
  root is empty after repoint; equivalently `grep -rn 'midaz/v3/pkg/transaction"' components/ledger`
  in midaz returns nothing. Verified empty.

## Consumed symbols — field-by-field

Legend: `+` field added at HEAD, `~` field changed, unmarked = identical.

### `Amount`
| Field | v3.5.2 `pkg/transaction` | HEAD `pkg/mtransaction` |
|---|---|---|
| `Asset string` | yes (`validate:"required"`) | yes (identical tags) |
| `Value decimal.Decimal` | yes (`validate:"required"`) | yes (identical) |
| `Operation string` | yes | yes (`+ swaggerignore:"true"`) |
| `TransactionType string` | yes | yes (`+ swaggerignore:"true"`) |
| `Direction string` | — | `+` (internal, `swaggerignore`) |
| `RouteValidationEnabled bool` | — | `+` (internal, `swaggerignore`) |
| `OverdraftAmount decimal.Decimal` | — | `+` (internal, `swaggerignore`) |

Fee usage: constructed by named fields only (`distribute.go:186` → `&transaction.Amount{Asset, Value}`;
`distribute.go` ranges over `map[string]transaction.Amount`). The three new internal fields are purely
additive; **fees reads/writes none of them** (verified: zero matches for `.OverdraftAmount`,
`.Direction`, `.RouteValidationEnabled` in moved code). Compile-clean.

### `FromTo`
| Field | v3.5.2 | HEAD |
|---|---|---|
| `AccountAlias string` | yes | yes |
| `BalanceKey string` | yes | yes |
| `Amount *Amount` | yes | yes |
| `Share *Share` | yes | yes |
| `Remaining string` | yes | yes |
| `Rate *Rate` | yes | yes |
| `Description string` | yes | yes |
| `ChartOfAccounts string` | yes | yes |
| `Metadata map[string]any` | yes | yes |
| `IsFrom bool` | yes | yes |
| `Route string` | yes (active) | `~` **now deprecated/passive** (`validate:"omitempty,max=250"`, "Accepted from client and persisted, but not used in any validation or business logic. Use routeId instead.") |
| `RouteID *string` | — | `+` **canonical** (`validate:"omitempty,uuid"`) |

Fee usage: `pkg/fee/distribute.go:184-194` constructs `transaction.FromTo{...}` and writes
`fromTo.Route = route` (the synthetic `credit->fee_sourceN->payer->routeId` key tail). It does **NOT**
write `RouteID`. The `Route` field still exists at HEAD, so this **compiles unchanged** — but it now
writes only the passive/deprecated field. See FOLLOW-UP 1.

`FromTo.ConcatAlias` also changed implementation between versions (v3.5.2 used raw `ft.BalanceKey`; HEAD
defaults empty `BalanceKey` to `constant.DefaultBalanceKey`). Fees does not call `ConcatAlias` directly
(verified), so no fee-side compile impact; flagged for awareness only.

### `Responses`
| Field | v3.5.2 | HEAD |
|---|---|---|
| `Total decimal.Decimal` | yes | yes |
| `Asset string` | yes | yes |
| `From map[string]Amount` | yes | yes |
| `To map[string]Amount` | yes | yes |
| `Sources []string` | yes | yes |
| `Destinations []string` | yes | yes |
| `Aliases []string` | yes | yes |
| `Pending bool` | yes | yes |
| `TransactionRoute string` | yes | yes |
| `TransactionRouteID *string` | — | `+` |
| `OperationRoutesFrom map[string]string` | yes | yes |
| `OperationRoutesTo map[string]string` | yes | yes |

Fee usage: fees reads `validationResult` (a `*Responses`) from `ValidateSendSourceAndDistribute` and
its `From`/`To` maps. The new `Responses.TransactionRouteID` is **not** read by fees (every
`TransactionRoute*` match in fee code is on fees' OWN model types — `pack.Package`,
`feeshared/model.*`, `billing_package` — never on `transaction.Responses`). Additive, compile-clean.
See FOLLOW-UP 2.

### `Transaction`
| Field | v3.5.2 | HEAD |
|---|---|---|
| `ChartOfAccountsGroupName string` | yes | yes |
| `Description string` | yes | yes |
| `Code string` | yes | yes |
| `Pending bool` | yes | yes |
| `Metadata map[string]any` | yes | yes |
| `Route string` | yes (active) | `~` **deprecated/passive** ("Use routeId instead.") |
| `RouteID *string` | — | `+` canonical (`validate:"omitempty,uuid"`) |
| `TransactionDate *TransactionDate` | yes | yes (same named type) |
| `Send Send` | yes (`validate:"required"`) | yes (identical) |

Fee usage: fees passes `cf.Transaction` / `feeModel.Transaction` into
`ValidateSendSourceAndDistribute`. It does not set `Transaction.Route`/`RouteID` directly. Additive,
compile-clean.

### `Send`
Identical between versions: `Asset string`, `Value decimal.Decimal`, `Source Source`,
`Distribute Distribute` with the same `validate` tags. No drift.

### `Source`
Identical: `Remaining string`, `From []FromTo` (`validate:"singletransactiontype,required,dive"`). No
drift.

### `Distribute`
Identical: `Remaining string`, `To []FromTo` (`validate:"singletransactiontype,required,dive"`). No
drift.

### `Share`
Identical: `Percentage int64`, `PercentageOfPercentage int64`. No drift.

### `ValidateSendSourceAndDistribute` (function)
| | v3.5.2 | HEAD |
|---|---|---|
| Signature | `func(ctx context.Context, transaction Transaction, transactionType string) (*Responses, error)` | **identical** |

Call-compatible. Fee call sites unchanged: `internal/services/fees/calculate-fee.go:63` and
`internal/services/fees/estimate-fee-calculation.go:76`, both invoked as
`transaction.ValidateSendSourceAndDistribute(ctx, <tx>, "")`. The only delta reaches fees indirectly
via the additive `Responses.TransactionRouteID` field, which fees does not consume.

## Net compile impact of the drift

**Zero.** All HEAD changes to the consumed symbols are additive (new struct fields, new `swaggerignore`
tags) or rename-only (`Route` → deprecated, `RouteID` added alongside). No field that fees reads or
writes was removed or retyped, and no consumed function signature changed. The relocation compiles
green against in-tree `mtransaction` with only the import-path rewrite + the `transaction` alias.

## Behavioral follow-ups (NOT absorbed in this chunk — owned by P4-T02)

1. **`FromTo.Route` synthetic write is now writing the passive field only.** `pkg/fee/distribute.go`
   `updatedAmountsFromFee` writes `fromTo.Route = route` for the synthetic
   `credit->fee_sourceN->payer->routeId` legs. At HEAD `RouteID` is canonical and `Route` is passive.
   The ledger's own op-builder writes **both** `Route` and `RouteID` (the SS2-accepted ledger-wide
   dual-write convention). **P4-T02** must mirror that: write `fromTo.RouteID = &route` AND keep
   `fromTo.Route = route` as the passive fallback. **Open risk:** `RouteID` carries
   `validate:"omitempty,uuid"`; the synthetic route values come from
   `feeModel.GetRouteFrom()/GetRouteTo()`. If those configured values are not UUID-shaped, writing them
   to `RouteID` fails uuid validation on any route-validation-enabled path — a real behavioral conflict
   to be resolved by the P2a-T17 spike before P4-T02 starts (may require a `name→ID` resolution step at
   the seam). Not touched here.

2. **`Responses.TransactionRouteID` is available but unconsumed.** HEAD surfaces the resolved
   transaction-route UUID on `Responses`. Fees currently filters packages off its own
   `TransactionRoute` string (`pkg/fee/filter.go::filterByTransactionRoute`) and never reads it from the
   validator result. If later chunks repoint fee route resolution to the canonical validator output,
   `Responses.TransactionRouteID` is the field to read. Noted; no change in this chunk.

3. **`Amount.OverdraftAmount` / `Amount.Direction` / `Amount.RouteValidationEnabled` are new internal
   processing fields.** Fees neither sets nor reads them. They are populated by the ledger transaction
   pipeline. No fee follow-up required for the relocation; recorded so that any future fee logic that
   constructs `Amount` for the ledger pipeline knows these exist and may need to be carried through.
