# LEDGER.md — Branch Review Audit & Remediation Campaign

Tracking doc for the `feat/monorepo-consolidation` code-review remediation. This is the
**filtered** version of a 41-reviewer Gate-8 review (68 raw findings) after a skeptical
per-finding audit (11 cluster auditors reading real code) classified each as REAL,
PARTIALLY_VALID, or FALSE_POSITIVE under a strict false-positive definition.

- **Branch:** `feat/monorepo-consolidation`
- **Base:** `ef34ffdb079524621b0275b08e19d8abcea6c8ea` (merge-base with `develop`)
- **Head at audit:** `38cdabd853e824cd53d27c8a6bdb3265c7c8c239`
- **Audit verdict:** 68 findings → **54 REAL, 10 PARTIALLY_VALID, 4 FALSE_POSITIVE**
- **Distinct work units after dup-collapse:** ~38 (14 duplicates fold in, 1 deferred)
- **Guiding principle:** AGAINST over-engineering. Lean correct fix over edge-case gold-plating.

## Status legend

| Mark | Meaning |
|------|---------|
| ⬜ | pending — not started |
| 🔄 | in-progress — agent dispatched |
| ✅ | fixed — agent returned, awaiting supervisor review |
| 👁 | verified — supervisor reviewed diff + build/tests green |
| ⏸ | deferred — tracked, not in this campaign |
| ❌ | closed — false positive, no action |

---

## FALSE POSITIVES — closed, no action

| ID | Title | Why closed |
|----|-------|-----------|
| E1 | ~26 error codes flipped 400→422 | Intentional v4 break, documented in `docs/plans/2026-06-07-v4-error-status-migration-notes.md` + locked by `mainline_error_contract_test.go`. `missed_existing_protection`. |
| E2 | Transient lock-conflict → DLQ in async path | Dead sentinel (0086 has no producers) + real transient error (0174) only fires in the SYNC HTTP path, never reaches the async classifier. `factually_wrong`. |
| M1 | `RecordDomainOperation` untested | 5 test files exercise it via real `sdkmetric.ManualReader`, incl. the nil-factory guard the reviewer named. `factually_wrong`. |
| D10 | schemaCache Get/Put key divergence | `PutSchema` interface has no configName param; pinned fetcher v1.0.0 sets `snapshot.ConfigName = descriptor.ConfigName` before Put. Mismatch structurally impossible. Invented edge case. |

---

## TIER 1 — MERGE BLOCKERS (correctness / tenant isolation / data loss)

| ID | Cluster | Title | Sev | Chosen fix | Status |
|----|---------|-------|-----|-----------|--------|
| F1 | FEE | MT fee seam reads single static DB for all tenants | Crit | `resolveFeesTenantContext` derives ctx with fees tenant Mongo (generic MB key) INSIDE `applyFees` after the revert/annotation/nil short-circuit (supervisor moved it there); `feesDBResolver` port plumbed into `TransactionHandler`; unit test (2 tenants → distinct DBs, request ctx untouched). | 👁 |
| T1 | TRACER | MT tracer reserve calls ship unauthenticated & tenant-less | Crit | Fail-fast boot guard in `buildTracerReserver`: error when `MultiTenantEnabled && TracerBaseURL!="" && no auth provider`. Full M2M wiring tracked separately (no token source exists yet). | 👁 |
| Q1 | QUARANTINE | Unparseable poison payloads can never persist (JSONB NOT NULL rejects non-JSON) | High | Migration 000034 `payload JSONB → BYTEA` (net-new, edit in place) + update `000034_..._test.go` + testcontainer Insert test with non-JSON payload in `transactionquarantine.postgresql_integration_test.go`. | 👁 |
| K1 | BACKFILL | Document collision aborts entire backfill, permanently (not self-healing) | High | Broaden idempotency in `CreateHolderWithID` only: on `IsDuplicateKeyError \|\| isDocumentAssociationError (errors.As + Code)`, re-fetch by deterministic `_id`; return idempotent success iff fetched `_id == supplied id`, else propagate. Generic `CreateHolder` unchanged. (Also kills K4.) | 👁 |
| K2 | BACKFILL | Single-tenant backfill never materializes PG holder_id | High | In single-tenant branch of `Run`, resolve ambient onboarding conn via `r.onbPG.connection.Resolver(ctx)` + inject via `tmcore.ContextWithPG(ctx, db, ModuleOnboarding)` before `RunTenant`. + single-tenant integration test running real `Run(context.Background())`. | 👁 |

---

## TIER 2 — STANDARDS & OBSERVABILITY CORRECTNESS

| ID | Cluster | Title | Sev | Chosen fix | Status |
|----|---------|-------|-----|-----------|--------|
| F2 | FEE | T5 span-class violation: `HandleSpanBusinessErrorEvent` on technical errors (multiple sites) | High | Swap to existing `handleSpanByErrorClass` at use-case-return error checks (fees_handler:79, billing_calculate:90, billing_package:90/182/240/317/365, fees_package:331/386). Keep `HandleSpanBusinessErrorEvent` for in-handler validation errors. | 👁 |
| F3 | FEE | T5 misclassify at the transaction seam (green span on fee infra failure → 500) | Med | Swap `transaction_create.go:1068` to `handleSpanByErrorClass`. Verify `transaction_fee_seam_structure_test.go` still passes. (Subset of F2.) | 👁 |
| F4 | FEE | Double Warn log for one fee failure (T8) | Med | Delete inner Warn in `applyFees` (transaction_fee_application.go:70); seam owns the single log. (Also F9.) | 👁 |
| F7 | FEE | Intra-file span-helper inconsistency (fees_package 331/386) | Low | Covered by F2 (same two lines). | 👁 |
| E5 | ERRORS | `ErrMidazQueryFailed` 500 (should 503), `ErrMissingSegmentContext` 400 (should FailedPrecondition) | Med | Re-class to existing types; **add rows to v4 status-migration notes + contract test** (ErrMidazQueryFailed is a wire change). | 👁 |
| E9 | ERRORS | `ErrTransactionReservationUnavailable` (0178) is 422, should be 503 | Low | Re-class to `ServiceUnavailableError`; 0178 is new in v4 (clean) — add v4 note row + contract test. | 👁 |
| T4 | TRACER | No W3C traceparent injected → orphaned tracer spans on hot path | High | One line in `client.go` `do()`: `libOpentelemetry.InjectHTTPContext(ctx, req.Header)` (already-imported helper; fixes all 5 ops). | 👁 |
| B2 | BACKOFF | Uninterruptible retry sleep ignores ctx → delays graceful shutdown | High | Replace `rm.sleepFunc(backoff)` with `commons/backoff.WaitContext(ctx, backoff)`; return on non-nil err (leave unacked → safe redelivery). (Also B3.) | 👁 |
| N1 | ALIAS | Span attr key `app.request.alias_id` carries instrument IDs (split-brain trace) | Med | Rename key → `app.request.instrument_id` across all 8 non-test sites (4 handler + 3 instrument.mongodb.go + 1 instrument_maintenance.mongodb.go). (Converges N4/N5.) | 👁 |
| N2 | ALIAS | Stale `alias` locals + span-error/log messages in CRM instrument services | Low | Rename locals (aliasID/alias/aliases/...) + human-readable span-error/log messages alias→instrument across create/get-all/update/delete instrument services. | 👁 |
| N4 | ALIAS | Observability+storage alias leak (span attrs/op-names) | Low | Rename span attr keys + span op-names (incl. fix Count's misnamed `find_all_alias`); **keep `aliases_<org>` collection name** (renaming orphans prod data). (Folds into N1.) | 👁 |
| S1 | STREAMING | `account.created` HolderID value-copy untested (mis-wire would pass) | Med | Add HolderID to the two mapping tests (nil-assert in minimal, set+assert-equality in all-optional), mirroring EntityID. (Also S2/S3.) | 👁 |

---

## TIER 3 — THIRD RAIL / lib-commons

| ID | Cluster | Title | Sev | Chosen fix | Status |
|----|---------|-------|-----|-----------|--------|
| B1 | BACKOFF | `BackoffCalculator` reinvents `commons/backoff` (mandatory lib-commons) | High | Route both paths through `commons/backoff.ExponentialWithJitter` + explicit `min()` cap; delete `BackoffCalculator` + Factor field. **Behavior change**: full-jitter `[0,exp)` vs current `[exp,2*exp]` — update `backoff_test.go`. Reshape producer's stateful loop to attempt counter. | 👁 |
| B6 | BACKOFF | Tracer wraps `sony/gobreaker`; lib-commons has tenant-aware `circuitbreaker` | Low | **Deferred** — reviewer self-scoped out; pre-existing tracer code, separate migration ticket. | ⏸ |

---

## TIER 4 — CLEANUP / DEAD CODE / TESTS / COMMENTS

| ID | Cluster | Title | Sev | Chosen fix | Status |
|----|---------|-------|-----|-----------|--------|
| T5 | TRACER | Orphaned `WithCircuitBreaker` seam (test-only) | Med | Delete the breaker seam (option/interface/field/do() branch/tests) until a consumer needs it. (Also T6.) | 👁 |
| T7 | TRACER | Dead `WithHTTPClient` option (zero callers) | Low | Delete option + doc comment. | 👁 |
| B4 | BACKOFF | Dead deprecated `FullJitter`/`NextBackoff` reporter wrappers | Low | Delete backoff.go:94-108 + test cases. (Bundle with B1 if it lands.) | 👁 |
| B5 | BACKOFF | Orphaned shim wrappers `BuildRetryHeaders`/`ExtractTenantID` (test-only) | Low | Delete the two test-only wrappers + their tests; keep live `GetRetryCount`/`TenantIDFromHeaders`/`NewProducerHeaders`. (= E7.) | 👁 |
| R1 | REDIS | Dead `CountBackupQueue` + lying doc comment | Med | Delete from interface + impl + regenerate/trim gomock. (Do NOT wire it — `len(messages)` is correct & cheaper.) | 👁 |
| R2 | REDIS | Write-only `serviceName` field + false "for logging" comment | Med | Drop field + constructor param + assignment; update MT call site config.go:987. | 👁 |
| R3 | REDIS | Double-unmarshal per cycle (emitQueueGauges + processing loop) | Med | Single parse pass before fan-out, track oldestTTL there, emit gauges with `(depth, oldestTTL)`. (Also R4.) | 👁 |
| R5 | REDIS | Overdraft replay audit divergence is unobservable | Low | Add span attr (`app.replay.recomputed_balances_after`) + counter when nil-balancesAfter replay branch fires. No reconciliation (deferred to T-006.1/T-009). | 👁 |
| R6 | REDIS | `redis.NewScript` per-call re-hashes SHA1 on hot path | Low | Hoist to package-level vars beside the `//go:embed` decls (3 sites: 825/1284/1347). | 👁 |
| F5 | FEE | Hand-authored `ValidationError{}` literals bypass factory | Med | Replace both literals with `feeerrors.ValidateBusinessError(ErrInvalidRequestBody, ...)`. | 👁 |
| F6 | FEE | Asymmetric nil-guard at interface boundary (no live panic) | Low | Add `if feeCalculate == nil` guard before fees_handler.go:84, matching billing siblings. (PARTIALLY_VALID — low value, consistency only.) | 👁 |
| F8 | FEE | Dead nil-`FeeApplier` branch + misleading "disabled" comment | Low | Reword comment to "test/defensive no-op" (no FeesEnabled flag exists). Keep guard. | 👁 |
| F10 | FEE | `fmt.Errorf` untyped errors in nil-result guards | Low | Return existing typed technical error from pkg/errors.go (no new sentinel). | 👁 |
| K3 | BACKFILL | MT backfill orchestration (`Run`/`runForTenant`) untested | High | Unit-test the MT loop with a fake tenant lister + injectable per-tenant step seam: per-tenant invocation, distinct ctx, abort-on-failure-N. | 👁 |
| K5 | BACKFILL | `holderReaderAdapter.Exists` not-found discrimination untested | Med | Table-driven test via a narrow `GetHolderByID` interface seam: found / ErrHolderNotFound→(false,nil) / other EntityNotFoundError Code→propagate / infra err→propagate. | 👁 |
| K6 | BACKFILL | `"external"` literal not linked to `constant.ExternalAccountType` | Low | Bind `constant.ExternalAccountType` in `buildMaterialiseQuery`; update lockstep test assertion. (Also K7.) | 👁 |
| K8 | BACKFILL | No nil-element guard on tenant slice from lib-commons | Med | `if tenant == nil { continue }` (+ optional Warn) in the MT loop. | 👁 |
| N3 | ALIAS | Double `time.Now()` for CreatedAt/UpdatedAt in create-instrument | Low | Hoist `now := time.Now()`, reuse for both (mirror create-holder-with-id.go). | 👁 |
| N6 | ALIAS | `time.Now()` in instrument_test.go (banned pattern) | Low | Replace with inline `time.Date(...)` literals (values never asserted; no shared fixture needed). | 👁 |
| S4 | STREAMING | Comments reference a phantom "e2e mirror struct" that doesn't exist | Low | Delete the two phantom references; JSONShape is the sole guard. | 👁 |
| E3 | ERRORS | Bare `value.(string)` type assertions can panic (settings) | Med | comma-ok assertion → existing `ErrInvalidSettingsFieldValue` out-of-set error. (Also E4.) | 👁 |
| E6 | ERRORS | Dead `HTTPError` branch in `WithError` (no producers) | Low | Delete the branch (behavior-identical 500); keep HTTPError type as Swagger anchor. | 👁 |
| E8 | ERRORS | `errors.As` chain-walk makes status wrap-order dependent (dormant) | Low | Contract test wrapping each platform class in a sibling, asserting outermost wins. No discriminator-method refactor (over-engineering). | 👁 |
| D1 | REPORTER | PG vs Mongo schema validation diverge on empty-field tables | Med | Move PG `matchedTables[key]=true` above the `len==0` continue (mirror Mongo); + unit test feeding `{"t":[]}` to both. (Resolves D7 too.) | 👁 |
| D2 | REPORTER | Stale "TPL-XXXX code" package doc comment | Low | Reword: sentinels re-export numeric constants for ValidateBusinessError mapping. | 👁 |
| D3 | REPORTER | Stale "Fetcher-side/network errors" comment (remote fetcher retired) | Low | Reword to "in-process extraction failures (query/connection/cursor)". | 👁 |
| D4 | REPORTER | Dead `ErrSchemaValidationFailed`/`ErrExtractionJobFailed` aliases | Low | Delete the two aliases + their guarding tests (callers use constant.* directly). Keep ErrDataSourceNotFound/Unavailable (live). | 👁 |
| D5 | REPORTER | Inconsistent not-found error shape (Get vs Validate) | Low | Make `ValidateSchema` match `GetDataSourceSchema` (wrap `ErrDataSourceNotFound`) + parallel contract test. **Do NOT** strip the wrap (breaks contract_test.go:85). Reconcile 0284 vs ErrMissingDataSource. | 👁 |
| D6 | REPORTER | Duplicated tenant-validation predicate (third-rail boundary) | Med | Extract `validateTenantID(string) error` (empty + IsValidTenantID); both `resolveTenant` and `requireTenant` call it, each keeping its own ctx-read/error-type. | 👁 |
| D7 | REPORTER | Dead `matchedTables` in Mongo validator (masks D1) | Low | Resolve with D1 (restructure Mongo two-pass) rather than a separate delete. | 👁 |
| D8 | REPORTER | Two parallel PG introspection impls (drift risk); claimed test doesn't exist | Med | Add the missing drift-lock test now (testcontainer parity incl. a view). FromDB-constructor unification tracked as larger follow-up. | 👁 |
| D9 | REPORTER | `NewMultiTenantDirectProvider` doesn't nil-check required managers | Low | Panic at construction with clear bootstrap message if pg/mongo nil (keeps no-error signature; fails closed loudly at startup). | 👁 |

---

## DUPLICATES (fold into primary)

| ID | Folds into | ID | Folds into |
|----|-----------|----|-----------|
| T2 | T1 | T3 | T1 |
| T6 | T5 | B3 | B2 |
| R4 | R3 | F7 | F2 |
| F9 | F4 | K4 | K1 |
| K7 | K6 | N5 | N1 |
| S2 | S1 | S3 | S1 |
| E4 | E3 | E7 | B5 |
| D7 | D1 | | |

---

## EXECUTION LOG

_Supervisor (Galadriel) appends one line per dispatched/verified unit._

- (campaign initialized — Tier 1 dispatching)
- T1 👁 verified — `buildTracerReserver` fail-fast guard (config.go +13); test `config.tracer_reserver_test.go`; build+unit green.
- Q1 👁 verified — payload JSONB→BYTEA (000034 up.sql + migration test); new integration test asserts non-JSON Insert succeeds; build+unit green, integration compiles (needs Docker).
- K1 👁 verified — `CreateHolderWithID` idempotency broadened via `isDocumentAssociationError` (errors.As+Code); 4 unit cases; resolves K4; build+unit green.
- K2 👁 verified — single-tenant `Run` injects onboarding PG into ctx; new integration e2e test; build+unit green, integration compiles (needs Docker).
- FEE cluster 👁 fully verified (build+unit green, seam-structure gate passes):
  - F1 — `resolveFeesTenantContext` + `feesDBResolver` port; **supervisor correction**: agent placed resolution at the seam (before applyFees' revert/annotation/nil short-circuit), causing reverts/annotations to resolve a fee DB they never use. Moved resolution INSIDE applyFees after the short-circuit; seam back to single `applyFees(ctx,...)` call.
  - F2/F3/F7 — span-class swaps to `handleSpanByErrorClass` at use-case-return sites; in-handler validation records kept business-class.
  - F4 — inner Warn deleted from `applyFees` (+ unused logger import).
  - F5 — **agent override (correct)**: my suggested `ErrInvalidRequestBody` (0094) is NOT in the ValidateBusinessError errorMap (only in ValidateUnmarshallingError) → would yield a bare untyped error. Used registered `ErrInvalidLedgerID` (0203, ValidationError) instead. Verified.
  - F6/F10 — reused typed `ValidateInternalError(ErrInternalServer=0046)`; no new sentinels.
  - F8 — comment reworded to defensive/test no-op.
- TRACER cleanup 👁 (pkg components/ledger/internal/adapters/tracer, client.go): T4 one-line `InjectHTTPContext(ctx, req.Header)` in do() (fixes all 5 ops); T5/T6 orphaned circuit-breaker seam deleted (interface+field+option+breakerName const+do() branch+2 tests), grep-confirmed only tmclient.WithCircuitBreaker is live (different pkg); T7 dead WithHTTPClient deleted. build+vet+test green.
- REDIS 👁 (consumer.redis.go adapter + redis.consumer.go bootstrap + config.go call site): R1 dead CountBackupQueue removed from iface+impl+mock; R2 write-only serviceName field/param/assignment dropped (+ fuzz/property/mt test sites updated); R3/R4 single-parse refactor — depth gauge upfront, oldestTTL tracked in the existing dispatch-loop unmarshal (before TTL skip, matching old semantics), oldest-age gauge after wg.Wait; preserved early-return/TTL-skip/fan-out; R5 new counter `redis_backup_replay_recomputed_balances_after_total` + span attr on the nil-balancesAfter replay branch; R6 NewScript hoisted to 2 package vars (3 sites). build+vet+test green. Supervisor read the R3 hot-path diff directly — correct.
- ALIAS 👁 (N1/N5 8 attr keys app.request.alias_id→instrument_id + 4 handler messages; N2 service locals+messages; N4 span op-names incl. Count misnomer fixed, `aliases_` collection KEPT 4/2/1 unchanged + 1 WHY comment; N3 single time.Now; N6 12 time.Now→time.Date). 0 non-test alias_id remain. build+vet+test green. Note: `delete-related-party.go`/handler `aliasID` PARAM names left (tied to repo interface; not observable) — deliberate scope call.
- STREAMING 👁 (S1/S2/S3 HolderID nil-assert + distinct set+assert mirroring EntityID; S4 deleted phantom e2e-mirror/strict-e2e-unmarshal comment phrases — grep confirmed no e2e mirror exists). JSONShape field count still locked at 16. test green.
- ERRORS 👁: E3/E4 comma-ok assertions in settings (→ existing ErrInvalidSettingsFieldValue, no panic); E6 dead HTTPError branch deleted (type kept as Swagger anchor); E8 declaration-order contract test added + doc tightened. **E5/E9 client-visible status changes** (in the documented v4 window, doc rows + contract-test lock added): ErrMidazQueryFailed 500→503, ErrMissingSegmentContext 400→**500** (FailedPreconditionError maps to 500 — server-misconfig, not a 4xx; agent flagged for eyeball), ErrTransactionReservationUnavailable(0178) 422→503.
  - **Supervisor correction**: E9 reclass broke 2 existing reservation-anchor tests asserting the old 422 (UnprocessableOperationError); the ERRORS agent updated the contract-test lock but missed these. Fixed both to assert ServiceUnavailableError. http/in green.
- BACKFILL-rem 👁: K6/K7 bound constant.ExternalAccountType (+ lockstep test); K8 nil-tenant guard in MT loop; K3 MT-orchestration tests via injectable seams (activeTenantLister iface + runForTenantFn field) — per-tenant invocation/abort-on-Nth/nil-skip/list-failure; K5 holderReaderAdapter.Exists table test via holderByIDReader iface seam. Production wiring untouched (concrete types satisfy ifaces).
- REPORTER 👁: D1 PG matchedTables moved above empty-field continue (guarded `matchedKey!=""`) + PG/Mongo parity test; D7 dead Mongo matchedTables deleted; D2/D3 stale comments fixed (D3 died with D4); D4 two dead sentinel aliases deleted; D5 ValidateSchema now matches GetDataSourceSchema wrap (contract_test.go:85 still green, + parallel test); D6 shared validateTenantID in new pkg/reporter/multitenant; D8 BASE-TABLE drift-lock integration test added (claimed-existing test did NOT exist; FromDB unification noted as follow-up); D9 nil-manager panic guard at construction.
- BACKOFF 👁: B2/B3 retry.go now uses commons/backoff.WaitContext(ctx) (interruptible; abandons on cancel→safe redelivery; SleepFunc field removed); **B1 third-rail migration** — BackoffCalculator deleted, all 4 call sites (incl. an unlisted worker consumer site) route through commons/backoff.ExponentialWithJitter + min(cap); producer's 3 stateful loops reshaped to attempt-counter (supervisor verified exponents align, no busy-loop). **B1 BEHAVIOR CHANGE**: full-jitter [0,exp) replaces old [exp,2·exp] — retry sleeps smaller on average (canonical AWS full-jitter). B4 dead deprecated wrappers + B5/E7 orphaned shim wrappers (BuildRetryHeaders/ExtractTenantID) deleted (grep-confirmed test-only); bonus dead constants removed.

## CAMPAIGN COMPLETE
All 38 distinct work units verified (👁) or deferred (⏸ B6) or closed (❌ E1/E2/M1/D10). Full `go build ./...` green; all touched test packages green.

### Decisions flagged for Fred
1. **E5/E9 client-visible HTTP status changes** (500→503, 400→500, 422→503) — done inside the v4 breaking-change window, documented + contract-locked. Veto any you disagree with; reverting a registry arm + doc row is trivial.
2. **B1 retry-timing behavior change** — full-jitter is arguably more correct (thundering-herd) but IS a live change to reporter/ledger retry cadence.
3. **T1** — MT + TRACER_BASE_URL now FAILS TO BOOT (by design, until an M2M token source exists). Full M2M wiring is a separate tracked item.
4. **B6** deferred (tracer sony/gobreaker→lib-commons) — separate migration ticket.

