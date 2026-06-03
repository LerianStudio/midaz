# Dossier 06 — plugin-fees → EMBED into `components/ledger`, integrate with transactions endpoint

**Target repo:** `/Users/fredamaral/repos/lerianstudio/plugin-fees`
**Module:** `github.com/LerianStudio/plugins-fees/v3` (Go 1.26.3)
**End state:** fee calculation collapsed INTO `components/ledger`, invoked in-process and synchronously inside the transaction-create command path. No HTTP hop, no m2m, no separate service.
**Verdict:** This is the hardest of the five moves. It is NOT co-location — it is a *service collapse with a double-entry correctness surface*. The good news, discovered below, is that the integration seam is already obvious in both codebases.

---

## 0. TL;DR for the planner

1. **fees today is a pure compute service.** It receives a Midaz `Transaction` payload, calculates fee operations, and returns the *mutated transaction* (extra `from`/`to` legs + metadata). It does NOT persist anything to the ledger and does NOT create transactions. It owns its own MongoDB (fee packages + billing packages), an internal cache, m2m auth, and read-only outbound calls to ledger for account/segment resolution.
2. **Nobody in midaz calls fees today.** `grep` across the entire midaz repo for `plugins-fees`, `/v1/fees`, `FEES_URL` → **zero hits**. The orchestration is external: a client/gateway calls `POST /v1/fees` (fees mutates the transaction) and then submits the mutated transaction to ledger's `POST .../transactions`. Ledger has no knowledge of fees. The "integration" is currently a convention enforced by the caller, not by code.
3. **The integration seam is a gift.** Both fees and ledger call the SAME function: `mtransaction.ValidateSendSourceAndDistribute(ctx, transactionInput, status)`. In ledger it lives at `components/ledger/internal/adapters/http/in/transaction_create.go:1045`. fees calls it on the inbound payload, computes fees, then re-writes `Send.Source.From` / `Send.Distribute.To`. **The embed point is: between that `ValidateSendSourceAndDistribute` call and the downstream balance/operation creation, invoke the in-process fee engine to rewrite the legs.**
4. **The 18 midaz imports are the easy part.** All 18 import `github.com/LerianStudio/midaz/v3/pkg/transaction`. That path **no longer exists at midaz HEAD** — it was renamed to `pkg/mtransaction`. Once co-located, every import flips to the internal `mtransaction` package and the dependency vanishes. Symbols used: `Amount`, `FromTo`, `Transaction`, `Send`, `Source`, `Distribute`, `Responses`, `Share`, `ValidateSendSourceAndDistribute` — all present at HEAD with compatible (decimal-based) shapes.
5. **The hard part is double-entry correctness.** The fee engine adds operation legs (`fee_source` accounts, proportional splits, deductible vs non-deductible rules, rounding with Ceil/Floor on the max account to keep totals exact). Today this runs OUTSIDE the ledger's balance machinery and is re-validated when the caller re-submits. Embedded, fees' output legs must pass ledger's OWN `ValidateSendSourceAndDistribute` + balance/Lua machinery. The rounding-correction logic (`applyFeeCorrection`) exists precisely to keep `sum(legs) == fee total`; any mismatch becomes an unbalanced transaction = third-rail violation.

---

## 1. What fees DOES — the fee calculation domain

### Endpoints (`internal/http/in/routes.go`)
All under `/v1`, all behind `lib-auth` `Authorize(...)` + tenant middleware + header/path parsing + body tracing:

| Method | Path | Handler | Purpose |
|---|---|---|---|
| POST | `/packages` | `packHandler.CreatePackage` | CRUD on fee packages (MongoDB) |
| GET/GET/PATCH/DELETE | `/packages[/:id]` | `packHandler.*` | package management |
| **POST** | **`/fees`** | **`feeHandler.CalculateFee`** | **core: apply fees to a transaction** |
| **POST** | **`/estimates`** | **`feeHandler.EstimateFeeCalculation`** | dry-run fee calc against a named package |
| POST/GET/PATCH/DELETE | `/billing-packages[/:id]` | `billingPkgHandler.*` | billing-package CRUD |
| POST | `/billing/calculate` | `billingCalcHandler.CalculateBilling` | volume/usage billing rollups |

The transaction-relevant surface is `POST /fees` and `POST /estimates`. Everything else (packages CRUD, billing) is configuration/reporting and can be embedded as plain CRUD endpoints or — for billing — re-evaluated as a candidate to fold into the reporter/CRM consolidation, not the transaction hot path.

### The fee compute contract (`POST /fees`)

- **Input** (`pkg/model/fees.go` `FeeCalculate`):
  ```go
  type FeeCalculate struct {
      SegmentID   *uuid.UUID
      LedgerID    uuid.UUID
      Transaction transaction.Transaction   // a full Midaz transaction payload
  }
  ```
- **Output:** HTTP 200 with the **same `FeeCalculate` object, mutated in place.** Specifically `Transaction.Send.Source.From`, `Transaction.Send.Distribute.To`, `Transaction.Send.Value`, and `Transaction.Metadata` (`packageAppliedID`, `feeExemption`) are rewritten to include fee legs. It is a compute/RPC endpoint — 200, no persistence (handler comment explicitly cites this).

### Business logic (`internal/services/calculate-fee.go` + `pkg/fee/*`)

Flow per `CalculateFee`:
1. Load fee packages for `(organizationID, ledgerID)` from MongoDB (`packageRepo.FindByOrganizationIDAndLedgerID`). Zero packages → no fee, return unchanged.
2. `transaction.ValidateSendSourceAndDistribute(ctx, cf.Transaction, "")` → normalizes the payload into a `transaction.Responses{From, To map[string]Amount, ...}`.
3. Pick the applicable package: single package (amount-range check) or `feeUtils.FindPackageToCalculateFee(packages, route, segmentID, value)` for multiple.
4. `feeUtils.CalculateFee(...)` (`pkg/fee/calculate-fee.go`) — the engine:
   - Sorts fees by `Priority`.
   - Per fee, selects reference amount (`afterFees` vs original), computes value via one of three application rules: `maxBetweenTypes`, `flatFee`, `percentual`.
   - Rounds to asset precision (`getAssetPrecision`, default scale logic in `pkg/fee/asset_precision.go`).
   - `applyDeductibleAndReferenceAmountRules` → distributes the fee across `From`/`To` accounts proportionally, honoring **exemptions** (direct alias match OR segment-membership via a Midaz API lookup) and **deductible vs non-deductible** semantics.
   - `applyFeeCorrection` reconciles rounding drift onto the max-value account so `sum(distributed) == fee total` exactly.
   - Rewrites `f.Transaction.Send.Source.From` / `Distribute.To` from the mutated `Responses` maps via `updatedAmountsFromFee` (which decodes synthetic keys like `credit->fee_source0->payer->routeId`).
5. Sets `Metadata["packageAppliedID"]` when a fee was applied (or `feeExemption` when all accounts exempt).

`POST /estimates` (`estimate-fee-calculation.go`) is the same engine against a single named `PackageID`, returning the would-be result without claiming application.

**Inputs:** a transaction payload + org/ledger/segment scope. **Outputs:** the same payload with fee legs injected. The engine is deterministic and side-effect-free *except* for the segment-exemption lookup, which calls ledger.

---

## 2. How fees integrates with transactions TODAY (runtime call path)

### There is no code coupling. The orchestration is external.

- midaz repo: **zero** references to fees (`grep -rn "plugins-fees|/v1/fees|FEES_URL"` → empty). Ledger's `transaction_create.go` does its own `ValidateSendSourceAndDistribute` and balance creation with no fee awareness.
- fees never POSTs a transaction to ledger. The only outbound calls fees makes to ledger are **reads** (see §4 MidazClient interface): account existence/details + transaction count-by-route. There is no `CreateTransaction` / `POST /transactions` anywhere in `pkg/net/http/midaz-service.go`.

### Inferred runtime topology (today)
```
client / gateway
   │  1. POST  fees:/v1/fees          (transaction payload)
   ▼
plugin-fees ──reads──▶ ledger (GetAccount, ListAccounts, CountByRoute)   [m2m bearer token]
   │  2. returns mutated transaction (fee legs injected)
   ▼
client / gateway
   │  3. POST  ledger:/.../transactions   (the fee-mutated payload)
   ▼
ledger  →  balances / operations  (double-entry, the real ledger write)
```

The caller is responsible for sequencing fees-then-ledger. fees is a stateless transformer sitting beside ledger, sharing the same `transaction.*` wire types so its output drops straight into ledger's input. **That shared type is the entire reason this embed is tractable.**

### What "embedded + integrated with transactions endpoint" means concretely

Collapse steps 1–3 into a single ledger request. Inside ledger's transaction-create command path:

```
ledger: POST .../transactions
  → parse + ValidateSendSourceAndDistribute (transaction_create.go:1045)
  → [NEW] feeEngine.Apply(ctx, org, ledger, segment, &transactionInput, validateResult)
        - load packages (in-process Mongo or pg repo)
        - run pkg/fee engine, rewrite Send.Source.From / Distribute.To / Metadata
        - re-derive `validate` (Responses) from the now-mutated input
  → balance/operation creation as today
```

**The exact seam:** immediately after `transaction_create.go:1045` (and the matching call in `transaction_state_handlers.go:433`), before `GetParsedLedgerSettings` / route propagation / balance writes. The fee engine mutates the legs; ledger's existing machinery then validates and persists them. Because the fee engine already re-balances via `applyFeeCorrection`, the mutated payload should re-pass `ValidateSendSourceAndDistribute` — but that re-validation MUST be re-run after fee mutation (not reused from before), or unbalanced fee legs slip through.

---

## 3. The 18 `midaz/v3/pkg/transaction` imports — and how they flip

### The 7 production import sites (the other 11 are tests/mocks)
```
internal/services/calculate-fee.go
internal/services/estimate-fee-calculation.go
internal/services/payload_builder.go
pkg/net/http/body_validator.go          (alias: modelTransaction)
pkg/fee/distribute.go
pkg/fee/calculate-fee.go
pkg/model/fees.go
```

### Symbols consumed (production, frequency)
```
258  transaction.Amount
221  transaction.FromTo
 85  transaction.Transaction
 73  transaction.Send
 71  transaction.Source
 71  transaction.Distribute
 29  transaction.Responses
 15  transaction.Share
  2  transaction.ValidateSendSourceAndDistribute
```

### The critical fact: `pkg/transaction` no longer exists at midaz HEAD
fees is pinned to `midaz/v3 v3.5.2`. At HEAD, that package was **renamed to `pkg/mtransaction`** (`/Users/fredamaral/repos/lerianstudio/midaz/pkg/mtransaction/transaction.go`). The types are present and decimal-based:
- `Amount{Asset string; Value decimal.Decimal; ...internal fields}` ✅ matches fee engine's `.Value.Mul/.Add/.Round`.
- `FromTo{AccountAlias, BalanceKey, Amount *Amount, Share *Share, Metadata, Route (deprecated), RouteID *string, ...}` ✅.
- `Send{Asset, Value decimal.Decimal, Source, Distribute}`, `Source{Remaining, From []FromTo}`, `Distribute`, `Transaction`, `Responses{Total decimal.Decimal, From/To map[string]Amount, ...}` ✅.
- `ValidateSendSourceAndDistribute(ctx, Transaction, status) (*Responses, error)` ✅ — same signature both call sites use.

**Flip on embed:** every `"github.com/LerianStudio/midaz/v3/pkg/transaction"` import becomes the internal `mtransaction` package (or whatever the ledger calls it post-merge). The dependency on `midaz/v3` as an *external module* disappears entirely — that is the whole point of "liso e final, sem shims." Co-located, fees code imports ledger's own transaction types directly.

**The hazard:** the v3.5.2 → HEAD shape drift must be diffed field-by-field. HEAD `FromTo` carries `RouteID *string` + deprecated `Route` + `BalanceKey`; fee engine writes synthetic `Route` strings (`fromTo.Route = route` in `updatedAmountsFromFee`). At HEAD `Route` is deprecated in favor of `RouteID`. The fee engine's route handling MUST be re-pointed at `RouteID` or the routes silently stop validating. This is a real behavior change, not a mechanical rename — flag it.

---

## 4. fees' own infrastructure (what gets absorbed or dropped)

### Bootstrap / config (`internal/bootstrap/config.go`, 29KB)
Full standalone composition root: `Config` struct with ~60 env vars, `Validate()`, `ApplyDefaults()`, Mongo connection builder, Midaz outbound HTTP service builder, multi-tenant wiring (tenant-manager client/event/middleware/mongo/redis/cache), m2m credential provider, AWS Secrets Manager, license client, self-probe, readyz checkers, TLS enforcement/detection, graceful drain. On embed, almost all of this is **deleted** — ledger already owns logging, telemetry, license, server lifecycle, multi-tenant, readyz, drain. Only fee-specific config survives: `DEFAULT_CURRENCY`, package cache toggles/TTLs, and the fee Mongo connection (if kept separate).

### MongoDB (fees owns its own database)
- `internal/mongodb/pack/` — fee package repo (`Package` model: fees, calc models, waivedAccounts, min/max amounts, priority, routes, segments).
- `internal/mongodb/billing_package/` — billing package repo.
- `scripts/mongodb/` — **11 migration index files** (`000001`–`000011`) defining compound indexes on `(org, ledger, enable, deleted, route, segment, amounts)` plus billing-package indexes.
- **DB ownership decision:** fees stores fee/billing packages in its own MongoDB instance (`plugin-fees-mongodb` in docker-compose). Ledger uses PostgreSQL for transactional data and MongoDB for metadata. Embedded, fee packages could (a) live in ledger's existing MongoDB as new collections — simplest, preserves the document model and the 11 indexes; or (b) migrate to PostgreSQL tables — more work, but aligns config CRUD with ledger's pg-first posture. **Recommend (a)**: fee/billing packages are config documents, not ledger entries; the document model + indexes port cleanly and avoid a schema rewrite. The 11 index migrations must be folded into ledger's Mongo migration pipeline regardless.

### Cache (`internal/cache/`)
In-process + tenant-aware caches for packages, billing packages, accounts (TTL-configurable, tenant-keyed). Account cache exists to avoid re-hitting ledger for account resolution. **Once in-process, the account cache becomes pointless** — fees would resolve accounts via a direct ledger query/repository call, not an HTTP round-trip. The package cache may still earn its keep to avoid Mongo hits on the transaction hot path.

### m2m (`internal/m2m/`) — DELETE on embed
Machine-to-machine auth: `M2MCredentialProvider` pulls per-tenant credentials from AWS Secrets Manager (`tenants/{env}/{tenantOrgID}/{app}/m2m/{target}/credentials`), `TenantAwareAuthGetter` wraps the auth-token getter so outbound calls to ledger carry a per-tenant bearer token. **This entire package exists solely to authenticate fees→ledger HTTP calls. In-process, there are no outbound HTTP calls, so m2m, the AWS Secrets Manager dependency, `CLIENT_ID/CLIENT_SECRET`, `M2M_TARGET_SERVICE`, and the whole `MidazService` HTTP client are deleted.** This is the single biggest simplification of the embed.

### Outbound MidazClient interface (`pkg/net/http/midaz-service.go:48`) — REPLACE with in-process calls
```go
type MidazClient interface {
    GetAccountFromMidazByAlias(ctx, creditAccount, orgID, ledgerID) error
    GetAccountDetailsByAlias(ctx, orgID, ledgerID, alias) (*pkg.Account, error)
    CountTransactionsByRoute(ctx, params CountParams) (int64, error)
    ListAccounts(ctx, orgID, ledgerID, filters, page, limit) (*AccountPage, error)
}
```
All four are **reads against ledger** used for: account existence validation, segment-membership resolution (exemptions), and billing transaction counts. In-process these become direct calls into ledger's account/segment query use cases and transaction repository — no HTTP, no auth, no serialization. `internal/adapters/midaz/{account_resolver,transaction_counter}.go` wrap this client and would re-point at the internal query layer.

### Metrics (`internal/metrics/`)
m2m metrics, readyz metrics, tenant metrics. m2m + standalone readyz metrics die with the standalone service. Fee-specific business metrics (if any worth keeping) fold into ledger's observability.

### License
fees runs `lib-license-go` middleware independently (`LICENSE_KEY`, `APPLICATION_NAME=plugin-fees`, `ORGANIZATION_IDS`). Embedded, it inherits ledger's license enforcement. The `plugin-fees` application name / resource ACLs (`packages`, `fees`, `estimates`, `billing-packages`, `billing-calculate`) used in `auth.Authorize(...)` must be reconciled with ledger's auth resource model.

---

## 5. Double-entry correctness implications (the third rail)

This is non-negotiable territory. The fee engine creates **additional operation legs**:
- Non-deductible fees: payer's source amount increases (`Send.Value` grows by the fee), a new `fee_source` credit leg is added on the `To` side.
- Deductible fees: the fee is carved OUT of the destination amounts (recipients receive less).
- Proportional distribution across multiple accounts with explicit rounding control: `RoundCeil` on the max account, `RoundFloor` on others for repeating decimals; `applyFeeCorrection` then adds the residual delta back to the max account so `sum(legs) == fee total` exactly.

Today this runs in fees and is **independently re-validated** when the caller resubmits to ledger (ledger runs its own `ValidateSendSourceAndDistribute`). Embedding removes that second validation as a free safety net — so:

1. **Re-run `ValidateSendSourceAndDistribute` AFTER fee mutation.** The mutated legs must balance under ledger's own validator, not just fees' internal arithmetic. Do not reuse the pre-fee `Responses`.
2. **Asset-precision/scale alignment.** fees has its own `getAssetPrecision` (`pkg/fee/asset_precision.go`). Ledger has its own scale handling (`Scale` on balances, Lua-driven splits). If the two disagree on rounding for any asset, fee legs round one way and ledger balances another → off-by-one cents → unbalanced transaction. **The precision logic must be unified, not duplicated.**
3. **Overdraft / Lua interaction.** HEAD `Amount` carries `OverdraftAmount` and the comment notes "Lua derives the split from live balance state." Fee legs injected before balance evaluation interact with overdraft enrichment (`transaction_overdraft_enrichment.go`). Order of operations matters: fees must run before overdraft enrichment, and the enrichment's assumption that "ValidateSendSourceAndDistribute already signed off on the totals" must still hold for the fee-augmented totals.
4. **Idempotency.** The idempotency hash (`transaction_create.go:1030`) is computed on the *user's* input payload BEFORE fee mutation. Good — fees are deterministic given the same input + same packages, so the hash stays stable. But if package config changes between two identical requests, the same idempotency key would now produce different fee legs. Decide: is the idempotency key over the raw request (current) sufficient, or must package-config version be part of the key? Flag for the planner.

---

## 6. Build / deploy / CI

- **main:** `cmd/app/main.go` → `bootstrap.InitServers().Run()` → single Fiber HTTP service on `:4002` (default). Deleted on embed; fees becomes a package inside ledger's binary.
- **docker-compose.yml:** `plugin-fees` + `plugin-fees-mongodb`, joined to `plugin-auth-network`, `onboarding-network`, `infra-network`. The fee service container disappears; only the Mongo (if kept separate) or new collections in ledger's Mongo remain.
- **Dockerfile:** standard 2-stage Go 1.26-alpine build with BuildKit GitHub-token secret for private modules — irrelevant once internal.
- **Makefile:** standard Lerian targets (`test`, `lint`, `sec`, `up`, `down`, `build-docker`, `generate-docs`). Tests fold into ledger's `make test-unit` / `test-integration`.
- **CI** (`.github/workflows/`): `build.yml`, `go-combined-analysis.yml`, `gptchangelog.yml`, `pr-security-scan.yml`, `pr-validation.yml`, `release.yml` — all dissolve into midaz's pipeline. The independent `release.yml` (separate versioning at `3.0.8`) is dropped; fees no longer ships independently.

---

## 7. Version / dependency skew (build-blocking)

| Dependency | plugin-fees (v3.5.2 era) | midaz HEAD | Impact |
|---|---|---|---|
| Go | 1.26.3 | 1.26.3 | ✅ aligned |
| `lib-commons/v5` | **v5.1.0** | **v5.2.0-beta.12** | minor skew, resolvable |
| `lib-observability` | **v1.1.0-beta.5** | **v1.0.1** | fees is AHEAD; midaz must bump or fees must pin down |
| `lib-auth/v2` | v2.7.0 | v2.8.0 | minor skew |
| `lib-license-go/v2` | v2.3.4 | (check) | likely removed on embed |
| `lib-streaming` | (none) | v1.4.0 | fees gains streaming on embed |
| `midaz/v3` | **v3.5.2** | self (HEAD) | **`pkg/transaction` → `pkg/mtransaction` rename; embed removes the dep entirely** |

The `lib-commons/v5 v2.9.1` indirect (a v2 of the v5 module path) in fees' go.mod is a transitive oddity to watch during `go mod tidy` post-merge. The `lib-observability` direction is notable: fees is on a *newer beta* than midaz HEAD's stable — merging forces a decision on which line midaz tracks.

---

## 8. Conflicts that block a clean merge

1. **Package path collision / rename.** `midaz/v3/pkg/transaction` (what fees imports) does not exist at HEAD; it is `pkg/mtransaction`. Mechanical for imports, but `FromTo.Route` is deprecated in favor of `RouteID` — fees' route-string handling is a behavioral conflict, not a rename.
2. **No existing integration code.** Unlike a refactor of existing coupling, there is NO seam in ledger to modify — it must be *created* at `transaction_create.go:1045` and `transaction_state_handlers.go:433`. Two call sites, must stay consistent.
3. **Asset precision duplication.** fees' `getAssetPrecision` vs ledger's scale handling — two sources of truth for rounding on a double-entry path. Must unify or risk unbalanced transactions.
4. **DB model mismatch.** fees is MongoDB-document-based for packages; ledger is pg-first with Mongo only for metadata. Where fee packages live is an unresolved ownership question.
5. **Auth resource model.** fees' `Authorize("plugin-fees", "fees", ...)` ACL names vs ledger's auth model.
6. **lib-observability beta-ahead.** fees on v1.1.0-beta.5, midaz on v1.0.1.
7. **Idempotency semantics** over fee-mutated vs raw payloads (see §5.4).

---

## 9. Effort

**Large.** This is the only one of the five that touches the double-entry write path. Breakdown:
- Mechanical (small): import rename `transaction`→`mtransaction`, delete main/bootstrap/docker/CI, fold tests. ~1–2 days.
- Infra teardown (medium): delete m2m + MidazService HTTP client, replace 4 outbound reads with in-process query calls, fold Mongo migrations, reconcile config/cache. ~3–5 days.
- **Integration + correctness (large/risky):** create the seam in both transaction-create call sites, re-run validation post-mutation, unify asset precision, verify rounding/balance under ledger's own machinery, overdraft/Lua ordering, idempotency decision, and write integration tests that prove `sum(fee legs) == fee total` AND the augmented transaction balances. This is where the time goes, and it cannot be rushed — it is the third rail. ~1.5–2.5 weeks with thorough testing.
- DB ownership decision + auth reconciliation: ~2–3 days.

Total: ~3–4 weeks of careful work, dominated by correctness validation, not code volume.

---

## 10. Integration surface (the load-bearing files)

**In ledger (where the seam goes):**
- `components/ledger/internal/adapters/http/in/transaction_create.go:1045` — primary embed point (after `ValidateSendSourceAndDistribute`).
- `components/ledger/internal/adapters/http/in/transaction_state_handlers.go:433` — second call site, must stay consistent.
- `components/ledger/internal/adapters/http/in/transaction_overdraft_enrichment.go` — ordering dependency (fees before overdraft).
- `pkg/mtransaction/transaction.go` + `pkg/mtransaction/validations.go` — the types/validator fees binds to.

**In fees (what moves):**
- `pkg/fee/calculate-fee.go`, `pkg/fee/distribute.go`, `pkg/fee/asset_precision.go`, `pkg/fee/filter.go`, `pkg/fee/segment-resolution.go` — the engine.
- `internal/services/calculate-fee.go`, `estimate-fee-calculation.go`, `payload_builder.go`, `service.go` (UseCase: `packageRepo`, `midazClient`, `defaultCurrency`) — the use-case layer.
- `pkg/model/fees.go`, `internal/mongodb/pack/`, `internal/mongodb/billing_package/`, `scripts/mongodb/` (11 index migrations) — model + persistence.
- `internal/adapters/midaz/{account_resolver,transaction_counter}.go` — re-point at internal queries.

**Deleted entirely:** `cmd/app/main.go`, `internal/bootstrap/*`, `internal/m2m/*`, `pkg/net/http/midaz-service.go` (HTTP client), `internal/cache/account_cache.go`, Dockerfile, docker-compose, CI workflows.
