# P2a Execution Note + Frozen P4 Correctness Artifact (2026-06-03)

> **⚠️ BASELINE CORRECTION (2026-06-03, verified via `git show HEAD:go.mod` + import grep):** plugin-fees HEAD
> is on **lib-commons v4.6.3** (lib-auth v2.5.0, mongo-driver **v1 only**, lib-observability **ABSENT**, 78
> files on `lib-commons/v4`, 48 files on mongo-driver v1) — NOT the v5.1.0 the first analysis claimed (that
> baseline was confabulated). Fees is therefore a **TWO-MAJOR-JUMP migration (v4→v5.1.3→v5.4.1), like tracer
> P2c** — NOT a single-jump obs-first. **Jump 1:** v4→v5.1.3 + lib-auth v2.5.0→v2.7.0; obs STAYS in
> `commons/v5/log` (NO lib-observability yet), mongo stays v1 — pure import-path sweep, no logger-type change.
> **Jump 2:** v5.1.3→v5.4.1 + obs split to lib-observability + lib-auth v2.8.0 + mongo v1→v2 (48 files, use
> `P1-mongo-v2-migration-map.md`) + the `bsondecimal` codec (highest risk). The `commons/log/opentelemetry/zap`
> re-homing in Part A below is correct in SHAPE but happens at **Jump 2**, and the source paths are v4 (Jump 1)
> then v5 — not v5.1.0. Part B (fee correctness) is unaffected and stands as the frozen P4 gate.

Corrections over `plan/P2a.md` (written against stale lib-commons v5.2.x) and the frozen fee-correctness
artifact P4 consumes. Verified live against `/Users/fredamaral/repos/lerianstudio/plugin-fees` HEAD +
midaz `v3.5.2` transaction model.

## PART A — deps migration corrections

**Target = the corrected frozen pin** (`plan/P1.md#p1-frozen-target-pin`): lib-commons/v5 **v5.4.1**,
lib-observability **v1.0.1**, lib-auth/v2 **v2.8.0**, **mongo-driver/v2 v2.6.0**. plugin-fees current:
lib-commons v5.1.0, lib-observability v1.1.0-beta.5 (0 source imports), lib-auth v2.7.0, lib-license-go
v2.3.4, **mongo-driver v1.17.9 (direct, ~25 files)**, midaz v3.5.2, module `github.com/LerianStudio/plugins-fees/v3`, go 1.26.3.

**PLAN DEFECT (material): P2a.md has ZERO mongo-driver tasks.** At v5.4.1, lib-commons `commons/mongo` is on
mongo-driver **v2** (cutover at v5.3.0). Fees imports the v1 driver directly. New tasks required, parallel to
the observability chain:
- **T18 — mongo-driver v1→v2 path + API rewrite** (~25 files: `internal/mongodb/pack/*`, `billing_package/*`,
  `internal/cache/*`, `internal/services/*`, `pkg/model/update_package_input.go`, all `tests/integration/*`,
  `tests/chaos/*`). Deltas: import paths `/v2/`; options builder change (`options.Find/Update/Index` →
  builders; the raw `options.FindOptions{}` struct literal at `find.go:83` breaks hardest); `mongo.Connect`
  ctx-drop; `bson.D` decode-default trap. **NOTE: zero `primitive.*` sites in fees** — that rename is a no-op here.
- **T19 — `pkg/bsondecimal/decimal.go` v2 codec rewrite (HIGHEST RISK).** Custom `decimal.Decimal` BSON codec:
  v1 `MarshalBSONValue() (bsontype.Type, []byte, error)` / `UnmarshalBSONValue(bsontype.Type, []byte)` →
  v2 `ValueMarshaler`/`ValueUnmarshaler` use `byte`/`bson.Type` (the `bson/bsontype` package is gone in v2).
  The 280-line `decimal_test.go` is all v1 signatures. **This is the serialization boundary for STORED FEE
  MONEY** — its Gate-0 slice MUST include a marshal→unmarshal round-trip test asserting `decimal.Equal`
  (zero tolerance). Couples directly to fee correctness.

**Observability re-homing (P2a.md T03–T10 accurate, reproduced):** `commons/log`→`lib-observability/log` (43),
`commons/opentelemetry`→`lib-observability/tracing` (36), `commons/zap`→`lib-observability/zap` (7),
`commons/net/http` SPLIT (3 telemetry-mw symbols move to `lib-observability/middleware`, helpers stay — R13
trap), root `NewTrackingFromContext` (67) + `NewLoggerFromContext` (3) → root `lib-observability`.
`withRecover.go:51` is the canonical libCommons-vs-libObservability triple-split file. STAYS in lib-commons:
root `commons`, `tenant-manager/*`, `secretsmanager`, `server`, `mongo`, `license`.

**lib-license-go: bootstrap/routes ONLY** (`bootstrap/{config,server,selfprobe}.go`, `selfprobe_test.go`,
`http/in/routes.go`) — zero business-logic usage. **P4 drop is clean.** One nuance: `selfprobe.go` wires a
license `TestValidate` into `/readyz` (fail-open) — P4 must decide whether ledger readyz keeps a license probe.

**No Postgres in fees** → v5.4.1 Postgres-TLS default is a no-op. Mongo TLS path exists (`connection.go:62`
TLSCACert + a `requireTLS` testcontainer at `tests/integration/tls/`) — the v2 driver rewrite must keep it
working (TLS config moved under the v2 options builder). Orphan `lib-commons/v2 v2.9.1 indirect` evicts on tidy.

## PART B — FROZEN P4 CORRECTNESS ARTIFACT (the danger list P4 must guard)

> **P2a-T17 SPIKE RESULT (2026-06-03, commit `fc20cce` on `chore/p2a-libcommons-v5.4.1`):** Conservation
> invariant `sum(legs)==feeTotal` under `decimal.Equal` — **HOLDS for NON-DEDUCTIBLE fees (100/100 combos,
> both debit + credit sides; frozen as the P4 gate in `pkg/fee/conservation_test.go`)**, but **BROKEN for
> DEDUCTIBLE fees (34/100 combos; quarantined as a `t.Skip` characterization test).** Root cause:
> `applyFeeCorrection`'s residual reconciliation (`distribute.go:358-384`) is DEAD CODE for the deductible
> path — its `maxAccountFromStructFeeKey`/`maxAccountToStructFeeKey` guard never matches the deductible key
> shape (deductible keys start with the credit account; `updateAmountToStruct` is never populated for
> deductible), so the rounding residual is silently dropped. Both under- and over-distribution occur (worst:
> JPY n7 −5.3%, JPY n5 +6%). **DEFAULT behavior for every deductible fee with uneven splits — not an edge
> case. Likely a pre-existing LIVE bug in the plugin-fees product. BLOCKS P4 deductible-fee embedding (item 1
> below).** Sub-findings: (i) 18-decimal (ETH) residuals may not be exactly representable → conservation may
> be unhittable at 18dp regardless of fix; (ii) the engine's `strings.Contains(key, "fee_source")` key-match
> is an injection surface for user route labels containing `fee`/`fee_source`. Fix approach pending Fred's
> decision — NOT patched.

Fee math is `shopspring/decimal` end-to-end (no float). Entry `CalculateFee` (`pkg/fee/calculate-fee.go:63`);
distribution `pkg/fee/distribute.go`; precision `pkg/fee/asset_precision.go` (ISO-4217 table). Ordered by severity:

1. **(THIRD RAIL) Leg-sum conservation must be proven before legs reach `ProcessBalanceOperations`.**
   `applyFeeCorrection` (`distribute.go:350`) computes `delta = feeValue - sum(distributed)` (`:374`) and adds it
   to the max account's leg — **but only if BOTH `maxAccountFromStructFeeKey != "" AND maxAccountToStructFeeKey
   != ""`** (`:375`), found via string `Contains`/`HasPrefix` matching (`:360-372`). **If the match fails, the
   delta is silently dropped → legs do NOT balance.** No existing test asserts `sum(legs)==feeTotal` under
   `decimal.Equal`. P2a-T17 freezes this invariant; P4 must (a) consume it as a gate and (b) RE-assert on the
   mutated payload (fees mutates in place).
2. **Inject fee legs BEFORE `ValidateSendSourceAndDistribute` re-runs.** `CalculateFee` mutates
   `Send.Source.From`, `Distribute.To` (`calculate-fee.go:140-141`) and `Send.Value` (`distribute.go:72`).
   The ledger send/distribute balance validation must run on the POST-fee payload (P4-T12 single-`validate`-var
   reassignment), else on-chain double-entry won't match.
3. **`Send.Value` is the SOLE top-level amount mutation** (`distribute.go:72`, guarded — only when a
   non-deductible fee adds a destination leg). Deductible fees redistribute within existing legs, no Send.Value
   change. `tran.Amount` derives from it for revert balancing.
4. **Route labels stay on `FromTo.Route` (free string, max=250), NEVER `RouteID`.** Routes are labels
   (`package.go:31-32` `taxa_débito`/`taxa_crédito`), not UUIDs; fees writes zero `RouteID`. Routing a label
   into a uuid-validated field breaks the funnel.
5. **`feeExemption` nested-metadata EMBED-BREAKER.** `distribute.go:138-163` writes a nested
   `map[string]any{exempt,reason,message}` into `Transaction.Metadata`; midaz metadata is **flat-only**
   (`nonested`, key≤100, value≤2000). **P4 fails validation on any fee-exempt transaction unless flattened.**
   Needs a P4 representation decision (flatten the exemption fields).
6. **Decimal must survive the BSON boundary** — couples to T19 `bsondecimal` v2 codec; a bad codec corrupts
   stored package fee values.
7. **transaction_lifecycle / streaming event must be fee-inclusive** — emit post-commit, pre-metadata-write,
   AFTER fee injection, observing post-fee `Send.Value` + injected legs.

8. **Revert/cancel is NET-NEW for P4 (PD-5).** Fees has NO reversal/refund path (verified). P2a-T17 only
   freezes the FORWARD conservation invariant; P4-T14/T16 builds the refund (reversal balances to sum==0 iff the
   forward distribution summed exactly to the fee total).

9. **Precision×balance:** per-leg rounding (`distribute.go:296`) creates residual reconciled onto max account;
   Ceil-max/Floor-others (`:298-303`) keeps residual non-negative/absorbable. The conservation property test
   MUST fuzz asset precision (0/2/3/8/18) × account count × repeating-decimal proportions — a 0-decimal asset
   (JPY) across 3 accounts can leave a full-unit residual that the correction must absorb.
