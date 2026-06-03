# P4-T23 — Serialization decimal-precision boundary

**Status:** Resolved. **Finding: NO lossy boundary on any seam — all three round-trip full precision.**

This is the branch condition for P4-T11. Because no seam truncates, P4-T11 emits
fee legs **unrounded** (the ISO-4217 precision table is deleted with no
replacement rounding); the residual-to-max reconciliation in
`applyFeeCorrection` alone guarantees `sum(legs) == fee total` exactly.

## Scope

The ledger amount/balance columns are **unbounded `DECIMAL`** (migrations
`000005`/`000006`: `ALTER ... TYPE DECIMAL`, `DROP COLUMN *_scale`). A bare
`DECIMAL` column cannot truncate, so it is **OUT of scope** — there is no
boundary there to find. The real serialization seams a `decimal.Decimal` passes
through are the three below.

## Root cause: every codec routes decimal through its string form

`github.com/shopspring/decimal@v1.4.0` serializes via the **full decimal string**
on every codec path, with no scale limit:

| Interface | Implementation | Lossy? |
|---|---|---|
| `MarshalJSON` / `UnmarshalJSON` | `"\"" + d.String() + "\""` ↔ `NewFromString` | No |
| `MarshalText` / `UnmarshalText` | `d.String()` ↔ `NewFromString` | No |
| `MarshalBinary` / `UnmarshalBinary` | GOB(big.Int value) + 4-byte exp ↔ decode | No |

`d.String()` emits the full unbounded decimal. The only place precision is
bounded anywhere in the fee flow is `decimal.DivisionPrecision = 16` inside
`Div` — that is **fee-internal arithmetic**, not a serialization seam, and it is
exactly the sub-precision residual that `applyFeeCorrection` reconciles.

## Seam-by-seam findings (proven by round-trip tests, real codecs)

### Seam 1 — JSONB `body` column

- **Path:** `mtransaction.Transaction` marshalled via `encoding/json` into the
  `body JSONB` column; read back with `json.Unmarshal`
  (`transaction.postgresql.go`).
- **Finding:** full precision, no loss.
- **Test:** `TestDecimalBoundary_JSONBBody_RoundTripFullPrecision`
  (`components/ledger/internal/adapters/postgres/transaction/decimal_precision_boundary_test.go`).
  Round-trips a 30-place 1/3 residual, 18-place ETH dust, and a 24-integer +
  18-fraction value through `Send.Value`, the From leg, and the To leg —
  `decimal.Equal` holds on all.

### Seam 2 — msgpack `TransactionProcessingPayload` (async / crash-recovery)

- **Path:** `TransactionProcessingPayload` (`transaction.go:415` — formerly
  "TransactionQueue"): `Validate *mtransaction.Responses` (msgpack `Validate`),
  `Input *mtransaction.Transaction` (msgpack `ParseDSL`), `Balances
  []*mmodel.Balance` — encoded by `github.com/vmihailenco/msgpack/v5` to
  RabbitMQ / the crash-recovery backup seed.
- **Finding:** full precision, no loss. (This is the seam P4-T25 worries about:
  a worker reconstructing from the backup seed reads the exact same decimals.)
- **Test:** `TestDecimalBoundary_MsgpackPayload_RoundTripFullPrecision` (same
  file). Asserts `Validate.Total`, `Validate.From`/`To` legs, `Balance.Available`,
  and `Input.Send.Value` all survive `decimal.Equal`.

### Seam 3 — Mongo metadata mirror

- **Path:** `MetadataMongoDBModel.Data JSON` (`map[string]any`, bson tag
  `metadata`) via `go.mongodb.org/mongo-driver/v2` bson codec.
- **Finding:** full precision **when the decimal-derived value is stored as its
  `String()` form** — which is the only supported metadata contract (flat
  free-form key/value). The fee engine writes only string/bool exemption
  metadata, so no raw decimal reaches this seam in practice.
- **Negative boundary (documented):** a **raw `decimal.Decimal`** placed in
  `map[string]any` does **NOT** round-trip through bson — its fields are
  unexported, so it marshals to an empty sub-document. This is why the metadata
  contract requires string-form, not raw decimals.
- **Tests:** `TestDecimalBoundary_MongoMetadata_StringForm_RoundTrip` (positive)
  and `TestDecimalBoundary_MongoMetadata_RawDecimal_NotPreserved` (negative
  boundary) in
  `components/ledger/internal/adapters/mongodb/transaction/decimal_precision_boundary_test.go`.

## Conservation is INDEPENDENT of any precision rule

The third-rail balance guarantee does **not** hinge on the boundary. The
invariant `sum(fee legs) == fee total` is held EXACTLY by
`applyFeeCorrection`'s residual-to-max reconciliation:

```
delta = feeValue.Value.Sub(newFeeTotalPaying)   // full precision, no .Round(scale)
```

`delta` is dumped on the max account's leg (and, on the deductible path,
subtracted symmetrically from that account's reduced balance to keep internal
debit == credit). After reconciliation, the distributed legs sum to the fee
total under `decimal.Equal` with **zero tolerance**, regardless of how each leg
was quantized. Deleting the ISO-4217 table therefore **cannot break the third
rail** — it only changes fee quoting granularity, a product/presentation matter.

This is proven WITH THE TABLE DELETED by:

- `TestConservation_NonDeductible_LegSumEqualsFeeTotal`,
  `TestConservation_Deductible_LegSumEqualsFeeTotal`,
  `TestConservation_DeltaDropEdgeCase`
  (`conservation_test.go`) — the precision matrix {JPY/0, BRL/2, KWD/3, BTC/8,
  ETH/18} × account counts {1,2,3,5,7} × fee shapes, with assets carried as
  plain transaction labels, no longer precision-table keys.
- `FuzzConservation_LegSumEqualsFeeTotal_TableDeleted` (`conservation_fuzz_test.go`)
  — 600k+ randomized executions (random send value, 1..7-way splits, 0..100%
  fees, all five assets, deductible/non-deductible) with zero conservation
  failures.

## P4-T11 decision

- **Branch taken:** "no lossy boundary → emit unrounded."
- `components/ledger/pkg/fee/asset_precision.go` and `getAssetPrecision`
  **deleted** (no fallback table — a scoped fallback would itself be a shim).
- `distribute.go`: removed per-leg `RoundCeil`/`RoundFloor`/`Round`; the
  residual delta is computed at full precision. Dead `isRepeatingDecimal`
  (its only consumer) removed from `filter.go`.
- `calculate-fee.go`: removed the fee-total `Round(getAssetPrecision(...))`.
- Billing (`internal/services/fees/billing-calculate-service.go`): removed its
  `fee.GetAssetPrecision(...).Round(...)` calls (the only other table consumer)
  to honor "delete entirely"; billing amounts are emitted at full precision.

## P4-T24 decision (denomination)

Fee legs are denominated in the transaction's `Send.Asset`, never the global
default currency:

- `calculate-fee.go` resolves `feeAsset = Send.Asset` (falling back to the
  configured default only when the transaction omits an asset) and stamps it on
  the fee total.
- `distribute.go` stamps each emitted leg with `feeValue.Asset` (= `Send.Asset`)
  rather than the payer account's asset, so **no path can emit a fee leg in an
  asset other than `Send.Asset`** — the validator's per-asset aggregation always
  balances single-asset.
- Tests: `TestCalculateFee_LegsDenominatedInSendAsset_NotDefaultCurrency`
  (USD tx with BRL default → USD legs, non-deductible + deductible) and
  `TestCalculateFee_EmptySendAssetFallsBackToDefault`
  (`denomination_test.go`).
