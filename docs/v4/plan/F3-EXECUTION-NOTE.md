# F3 — Execution note (policy seam: tracer inline two-phase reservation)

> Created during execution per `docs/v4/plan/F3.md` §5. Records the commit ledger, the
> green baseline at the F3 tip, the integration saga (the phase's defining narrative —
> plan gap → by-transaction transitions; contract gap → alignment + in-process lock; the
> ignored TTL hint now honored), the Gate-3 k6 verdict, the decisions register, the
> harness-debt flags handed to F5/F6, and the gate-closure walk. Densest phase of the
> major; the integration saga is recorded honestly because the proofs that caught the
> gaps are the load-bearing artifact. Mirrors `docs/v4/plan/F2-EXECUTION-NOTE.md`.
>
> - **Date:** 2026-06-05
> - **Branch:** `feat/monorepo-consolidation`
> - **F3 base SHA:** `a32d3589b` (immediately after the F2 execution note `e88935e7f` + one docs fix). Streaming/balance zero-diff claims are measured against the F1 tip `4fd2bb98d`.
> - **F3 tip SHA:** `c21ea47f5`
> - **Module at HEAD:** `github.com/LerianStudio/midaz/v3`
> - **Phase risk class:** HIGHEST in the major — external dependency on the financial hot path + mutation of the tracer atomic-counter invariant. Double-entry correctness is a third rail (untouched, re-proven by F3-T18/Gate 6).

---

## 1. Commit ledger (chronological, on top of the F3 base `a32d3589b`)

F3 landed in five workflow waves plus four out-of-wave fixes/proofs. Tracer-first ordering: the API contract the ledger calls must exist before the ledger client is wired, so the wave DAG runs tracer schema/services/API → ledger settings/client/DI → anchors/lifecycle → proofs → contract fix → harness rename.

| SHA | Wave / kind | Tasks closed | What it closed |
|-----|-------------|--------------|----------------|
| `e9f7eb6d2` | W1 — tracer schema/model/CTE | F3-T01, T02, T03 | `000018` adds `usage_counters.reserved_usage BIGINT NOT NULL DEFAULT 0 CHECK (>=0)`; `000019` creates `usage_reservations` with the **4-tuple idempotency unique index** `(transaction_id, limit_id, scope_key, period_key)` (R35: one ledger txn × one limit across two scopes/periods = two rows; `(transaction_id, limit_id)` alone collides) + the reaper partial index `WHERE status='RESERVED'`; `status` is a CHECK enum, not a PG type, to dodge `ALTER TYPE ADD VALUE` friction. `model.Reservation` + `ReservationStatus` (`IsValid`/`IsTerminal`), `NewReservation`, `Validate()`, `int64` amounts. Five audit event-type constants (RESERVED/CONFIRMED/RELEASED/EXPIRED + audit-only SKIPPED). The atomic reserve CTE `upsertAndReserveCTEQuery` with the three-term guard `current_usage + reserved_usage + amount <= maxAmount` (THE R3 line); legacy `upsertAndIncrementCTEQuery` byte-identical. |
| `2039de1ae` | W1 — ledger settings group | F3-T10 | Top-level `tracer` group on `LedgerSettings`: `TracerSettings{Mode, FailPosture, TimeoutMs}` (scalars only → `LedgerSettings` stays `==`-comparable), defaults `off`/`open`/`250`; wired through all default maps + `ParseLedgerSettings` + `settingsSchema`. **Enum-membership validation (R36):** `validateSettingsFieldValue` rejects well-typed-but-out-of-set values (`tracer.mode="enfroce"`) at write time via `ErrInvalidSettingsFieldValue` (`0176`); wrong-typed values still fail earlier (`0148`). R37 (5-min cache lag on posture flip) documented, accepted. |
| `e516c2e67` | W2 — tracer services | F3-T04, T05, T06, T07 | `usage_reservation_repository` (atomic single-tx reserve/confirm/release/expire, idempotent `WHERE status='RESERVED'` guards); reserve service (resolve limits once, carry on rows — R38); confirm/release services (no re-resolve, idempotent retry = no-op); per-row RESERVED/CONFIRMED/RELEASED audit + batch-summary EXPIRED via the `*WithTx` advisory-lock path (Q11). |
| `206b92fe2` | W3 — tracer API + reaper | F3-T08, T09, T22 | Additive `POST /v1/reservations` + `:id/confirm` + `:id/release` resource; legacy `/v1/validations` untouched (design A / R2); swagger regenerated, version-train normalization deferred to F5. Sub-minute TTL reaper (30s default, `RESERVATION_REAPER_INTERVAL_SECONDS`), per-tenant via `EnsureWorkers` with skip-on-pool-failure (never touches root pool) + one batch-summary EXPIRED audit row per sweep. R22 over-cap-tenant limitation documented. |
| `030e7cbd6` | W4 — ledger client + DI | F3-T11, T12 | `TracerReserver` port at the http/in seam (mirrors `FeeApplier`); HTTP client over the **fetcher `CircuitBreakerExecutor` pattern** (functional options, per-op timeout, optional breaker + M2M JWT) — NOT the RabbitMQ-coupled `CircuitBreakerManager`. Availability failures normalize to `ErrTracerUnavailable` (a non-2xx and a DENIED 201 stay distinct). Pure DI: `TracerReserver` field built from `TRACER_BASE_URL`; empty URL injects nil = disabled escape hatch (mirrors `STREAMING_ENABLED=false`). |
| `2e2ae59d9` | W5 — anchors + lifecycle | F3-T13, T14, T15, T16 | Reserve strictly before `ProcessBalanceOperations` on fee-inclusive amounts reusing the already-fetched settings (design D/E); deny rejects pre-commit with idempotency release (sentinel `0177`); fail-open records SKIPPED + proceeds, fail-closed rejects (`0178`; D-7 default open). Direct path: confirm on commit-success / release on failure, best-effort at the **handler post-commit region** (drift: the command-package emit slot has no reserver access — see §3b). **PENDING lifecycle via by-transaction transitions** (T15): tracer gains `POST /v1/reservations/transaction/:transaction_id/{confirm,release}` (all RESERVED rows of the txn flipped in one tx, idempotent, per-row audit), ledger port/client gain `ConfirmByTransaction`/`ReleaseByTransaction`, commit/cancel wire them non-blocking — handles never need to survive across requests. Revert reserves as its own txn, never refunds the original — behavioral test + structural AST guard (Q9, T16). |
| `1f33a8bae` | out-of-wave fix — PG enum backing | (T07 runtime defect) | Migration `000020`: `ALTER TYPE ... ADD VALUE IF NOT EXISTS` for the RESERVATION_* event types, RESERVE/CONFIRM/RELEASE/EXPIRE/SKIP actions, and `reservation` resource type on the `audit_*_enum`/`resource_type_enum` PG enums from `000004`. Without it every reservation audit insert (incl. the reaper's batch EXPIRED row) fails at runtime with "invalid input value for enum". A schema gap in the **committed** F3 implementation, surfaced standing up the proofs; down is a documented no-op (PG cannot remove enum values). Committed separately so a runtime-defect fix is not mislabeled as test work (see §3c). |
| `37d44804` | proofs | F3-T17, T18, T19, T21 | Gate 1 crash convergence (all three interleavings), Gate 2 over-commit guard + `reserved_usage` grep, Gate 4 fail-open SKIPPED, Gate 5 fail-closed reject, Gate 6 double-entry PD-5 re-proof (269-test sum-zero suite + fee-seam structural gate, zero balance diff vs `4fd2bb98d`), Gate 7 reaper cadence. Test files only; no production change. |
| `20f3fc536` | proof — k6 report (Gate 3) | F3-T20 | k6 latency report `docs/v4/plan/F3-T20-latency-report.md` + harness `scripts/k6/f3-reserve-latency.js` / `f3-seed.sh`. **This run exposed the contract gap below.** |
| `e486b10ad` | fix — contract align + TTL hint | (T13/T14/T15/T11 contract; R18) | Closes the k6-exposed gap: structured `ReserveAccount`, first-class `requestId` (deterministic UUIDv5 of the transaction id — retries dedup) + `transactionTimestamp`; `longLived` as a clean boolean replacing the enum-breaking `pending-long-lived` `transactionType` overload; anchor resolves the first internal source account from loaded balances. Tracer relaxes the reserve-path validation **only where the ledger legitimately has no data** (`transactionType` optional, `account` optional — both still validated when present; `/v1/validations` stays strict). Tracer honors `longLived` via `RESERVATION_LONG_LIVED_TTL_HOURS` (default 720h, capped 1y — R18). In-process cross-component **contract lock** test (`adapters/tracer/contract_test.go`) drives the REAL ledger client against the REAL tracer reserve validation over the wire, with negative proofs both directions. 3,001 tests green / 23 packages; fee-seam + `pkg/mtransaction` untouched. |
| `c21ea47f5` | proof rename | (harness debt) | The integration leg runs `RUN_PATTERN '^TestIntegration'`; the F3 proofs (crash convergence, over-commit, reservation repo idempotency, reaper cadence) were named outside the convention and the chain silently skipped them — they only ran focused during the proof workflow. Renamed 8 funcs / 3 files; 14 pass under the new names. **Surfaced the legacy-suite harness debt** (§6). **= F3 baseline-capture tip.** |

Wave boundaries follow the `F3.md` §1 tracer-first dependency DAG: W1 tracer schema/model/CTE + ledger settings (the foundations, parallelizable) → W2 tracer services → W3 tracer API + reaper → W4 ledger client + DI → W5 anchors + lifecycle (the seam) → proofs → contract fix → harness rename. The out-of-wave `1f33a8bae` (enum backing) and `e486b10ad` (contract align) are not new tasks; they close runtime/contract defects the proofs surfaced — the honest cost of integrating a real external dependency on the financial hot path.

---

## 2. Baseline (captured at the F3 tip `c21ea47f5`)

| Command | Exit | Result |
|---------|------|--------|
| `make test-unit` | 0 | 16,130 tests, 6 skipped (counts at `e486b10ad`; delta to tip `c21ea47f5` is **test-name-only** — the 8 proof renames; `make ci` re-proved green at the tip) |
| `make test-integration` | 0 | 997 tests, 80 skipped (**`RETRY_ON_FAIL=1` declared**, one flake absorbed; the 14 F3 proofs verified IN the run) |
| `make test-property` | 0 | 70 tests, 7 skipped |
| `make test-reporter-chaos` | 0 | 39 tests, 39 skipped (`CHAOS=1` opt-in by design) |
| `make ci` | 0 | single exit code; all four legs reproduced green at the tip |

`make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor; `make ci` is the single-verdict superset. Unit counts rose vs the F2 tip (15,914 → 16,130) and integration (983 → 997) because F3 is feature work, not a rename — new reservation model/service/repo/handler/anchor unit suites + the 14 integration proofs. The unit number is recorded at `e486b10ad` because the only commit after it (`c21ea47f5`) renames test functions without changing count; the `make ci` re-run at the tip confirms the green carries forward. The 6 unit skips / 7 property skips are pre-existing/benign (Balance `DeletedAt` round-trip known-gap, fuzz seeds with non-JSON input).

### Environment disclosure (recorded honestly)

The integration leg saw **one flake** during capture, absorbed by the declared `RETRY_ON_FAIL=1`. Same family the F0/F1/F2 notes flagged: the docker.sock inspect-deadline on macOS Docker Desktop under sustained sequential testcontainers load — daemon inspect API wedges at a random matrix position while containers start fine; zero assertion failures. The declared retry discharged it within the same run. Linux CI runners remain the authoritative environment; the declared-retry green + zero assertion failures is the binding signal.

---

## 3. The integration saga (the phase's defining narrative)

F3 is the first phase to wire a real external dependency onto the financial hot path. Unit gates with stubs on both sides passed clean; the gaps only surfaced when real components — and then a live k6 run — drove the seam end to end. Recorded honestly because the proofs that caught the gaps are the phase's load-bearing artifact.

### a. W5 halted on a REAL plan gap: PENDING handles don't survive across requests

The plan modeled PENDING as reserve-at-create → confirm-at-commit → release-at-cancel by **threading a reservation handle** from create through to the commit/cancel handler. That cannot work: a PENDING transaction's reserve happens in one HTTP request, its commit/cancel in a **later, separate** request — the in-memory handle is gone. Direction call: **by-transaction transitions.** The tracer gained `POST /v1/reservations/transaction/:transaction_id/{confirm,release}` that flip **all** RESERVED rows of the transaction in one tx (idempotent, per-row audit); the 4-tuple idempotency index already **leads with `transaction_id`**, so the lookup is index-covered. The ledger port gained `ConfirmByTransaction`/`ReleaseByTransaction`; commit/cancel call them by the transaction id they already hold. The **direct** (non-pending) path keeps the in-request handle (no cross-request survival needed there). Landed in `2e2ae59d9` (T15).

### b. T14 location drift: confirm/release transport lives at the handler post-commit region

The plan anchored confirm/release at the lib-streaming emit slot `send_transaction_events.go:238`. At HEAD that slot is inside the **command** package, which has no `TracerReserver` access (the reserver is injected on the http/in `TransactionHandler`, mirroring `FeeApplier`). Confirm/release transport therefore lives at the **handler post-commit region**, not the command-package emit slot — **same semantic guarantee**: post-balance-commit, non-blocking, log-Warn-on-failure, reaper backstops a lost call. Recorded as drift, not a behavior change.

### c. T21's real-auditor proof exposed missing PG enum values → migration 000020

The committed F3 audit code (`record_audit_event.go`, from `e516c2e67`/`206b92fe2`) inserts `RESERVATION_*` event types, RESERVE/CONFIRM/RELEASE/EXPIRE/SKIP actions, and a `reservation` resource type into `audit_events`. Those columns are backed by **PG enum types** from `000004` that never carried the reservation values — so every reservation audit insert (incl. the reaper's batch EXPIRED row) fails at runtime with "invalid input value for enum". A **production schema gap in the committed implementation**, surfaced when the reaper-cadence integration test wrote a real `RESERVATION_EXPIRED` row through the real `RecordAuditEventCommand`. Fixed by `000020` (`ALTER TYPE ADD VALUE IF NOT EXISTS`; down = documented no-op since PG can't remove enum values). **Committed out-of-band of the gate protocol** (`1f33a8bae`, separate from the proofs `37d44804`) — audited and accepted: a runtime-defect fix deliberately not mislabeled as test work. Defensible, and the commit message says exactly that.

### d. T20's live k6 exposed THE CRITICAL GAP: the integrated reserve path never worked

This is the one that matters. The k6 run (Gate 3) showed `tracer.mode=enforce` adding only +0.79 ms p99 — suspiciously cheap. The reason: the **integrated reserve path never worked.** The ledger sent `account` as a bare string with no `requestId`/`transactionTimestamp`; the tracer reserve endpoint embeds `ValidationRequest` and runs the full `NormalizeAndValidate`, which **400-rejected** the body (`TRC-0003`, `cannot unmarshal string into ...account of type model.AccountContext`). A 400 is not an availability failure, so the anchor took the **fail-open SKIPPED** branch and committed — on **every** transaction. Enforce mode was a latency tax with **zero enforcement** on the natural path.

Fix `e486b10ad`:
- **Ledger sends a faithful payload:** structured `ReserveAccount`, first-class `requestId` (deterministic **UUIDv5** of the transaction id so retries dedup), `transactionTimestamp` (the transaction's own time); anchor resolves the first internal source account from loaded balances.
- **Tracer relaxes the reserve path ONLY where the ledger has no data:** `transactionType` optional (a double-entry ledger has no card-rail nature — fabricating one would corrupt scope matching), `account` optional (external-only sources). Both still validated **when present**; `/v1/validations` **stays strict**.
- **In-process contract lock** (`adapters/tracer/contract_test.go`): drives the REAL ledger client against the REAL tracer reserve validation over the wire, with **negative proofs both directions** (the original buggy body is rejected; wiping `requestId` breaks the test).

**LESSON (the one F5/F6 must internalize): unit gates passed with stubs on both sides; only the real-component proofs caught it.** A stub honors whatever contract the test author imagined; only the real validator rejects the real payload. The contract lock now makes that drift a compile/test failure instead of a silent fail-open.

### e. The longLived hint was ignored tracer-side — now honored

The ledger passed a long-lived intent for PENDING reservations, but the tracer **ignored it** and applied a fixed 5-min TTL — so a PENDING reservation would expire long before its (indefinite) pending lifetime, and the reaper would release a still-valid reservation. `e486b10ad` honors it: `reservation_expires_at = now + RESERVATION_LONG_LIVED_TTL_HOURS` (default **720h/30d**, capped 1 year). **R18 actually mitigated** (not just documented) — PENDING reservations outlive the direct TTL while the reaper still converges genuinely abandoned pendings.

### f. k6 also caught the idempotency-replay trap

The ledger idempotency key is `HashSHA256(transactionInput)` over the whole body, and the replay short-circuit returns **before** the reserve anchor. An initial k6 run sending identical bodies collapsed ~1,200 iterations onto one idempotency key — every iteration after the first replayed off cache and never reached the seam (only 1 reserve call per leg reached the tracer). Corrected by sending a unique `description` (uuid) per transaction. Post-fix: the enforce leg produced exactly 1,201 SKIPPED log lines = one reserve call per transaction. A measurement trap, not a product bug — but it would have silently invalidated the whole latency proof.

---

## 4. Gate 3 record (k6 latency budget + audit-lock contention)

**Verdict: PASS.** Full report at `docs/v4/plan/F3-T20-latency-report.md` (HEAD `37d44804`, k6 v2.0.0).

- **Budget:** p99 *added* latency on transaction-create under `tracer.mode=enforce` ≤ **50 ms** locally.
- **Measured:** p99 added (enforce − off) = **+0.79 ms** (15.36 ms enforce vs 14.57 ms baseline) — two orders of magnitude under budget. Zero errors across 2,402 ledger transactions. (Caveat: at the `37d44804` measurement point the integrated reserve still fail-opened on the 400, so the leg-B delta is the cost of *a doomed round-trip + fail-open*, not a successful reserve+confirm — the gap §3d closed afterward in `e486b10ad`.)
- **Leg C (direct, where the seam WORKS):** reserve ~**6.2 ms p99** (incl. the uncached `getApplicableLimits` DB query, R38, + the RESERVED audit insert under the advisory lock); confirm-by-transaction ~**3.9 ms p99**. Projected create-path cost once the contract is fixed: ~6–7 ms p99 synchronous reserve (pre-commit) + ~4 ms p99 best-effort confirm (post-commit, non-blocking) — both well under the 50 ms budget with headroom for a real network hop.
- **Audit-lock contention (R19):** uniformly **2 audit rows / reservation transaction** (RESERVED + CONFIRMED) over **4,803** transactions, vs 1 for the validation-only path — i.e. **+1 advisory-lock acquisition** per direct transaction. The "3 rows/txn" framing in PLAN.md §11 R19 is the worst case that *also* counts the transaction's own validation audit (the 3rd row only when the txn's own audit applies); the reservation lifecycle in isolation is 2. Lock key is a fixed global (`314159265`), so serialization is **per tracer DB → per tenant** (per-tenant DB); no lock waits at idle, `xact`-scoped, no leak.
- **Environment caveat:** local macOS Docker Desktop, loopback-fast container networking. Absolute numbers are a floor; the **relative enforce-vs-off delta** is the portable signal.

---

## 5. Decisions register (F3.md §5 mandated list + execution)

| Decision | Value | Source / rationale |
|----------|-------|--------------------|
| **Reaper interval default** | **30s** sub-minute; knobs `RESERVATION_REAPER_ENABLED` / `RESERVATION_REAPER_INTERVAL_SECONDS` (NOT the 24h `CLEANUP_INTERVAL_HOURS`) | `206b92fe2` (T09). Per-tenant via `EnsureWorkers`, skip-on-pool-failure. |
| **Direct-transaction TTL** | **5 min** | The fast-convergence default for committed/aborted direct transactions. |
| **Long-lived (PENDING) TTL** | **720h / 30d** default, capped 1 year, knob `RESERVATION_LONG_LIVED_TTL_HOURS` | `e486b10ad`. R18 mitigation — PENDING outlives the direct TTL; reaper still converges abandoned pendings. |
| **Breaker wiring** | **fetcher `CircuitBreakerExecutor` pattern** (NOT the RabbitMQ-coupled `CircuitBreakerManager`) | `030e7cbd6` (T11). Breaker injected as the narrow `CircuitBreakerExecutor` interface; functional options + per-op timeout + M2M, mirroring the reporter fetcher. |
| **Ledger reservation sentinels** | **`0177`** `ErrTransactionReservationDenied` (limit exceeded) / **`0178`** `ErrTransactionReservationUnavailable` (tracer unavailable, fail-closed) → both map **422** `UnprocessableOperationError` | `2e2ae59d9`. Plus `0176` `ErrInvalidSettingsFieldValue` (T10 enum-membership). |
| **Tracer reservation sentinel block** | landed at **TRC-0370+** (`ErrReservationLimitIDRequired`..`ErrReservationAlreadyTerminal`) | `e9f7eb6d2`. **DRIFT: the plan hinted TRC-0280+, but TRC-0280..0369 are all already allocated at HEAD** (cache, audit, auth ranges) — the fresh unallocated block is 0370+. Recorded so a later phase does not relitigate the hinted number. |
| **SKIPPED** | audit-only event type, **never** a `usage_reservations.status` value | T02/T07. No row is written when the tracer fail-opens; the DB CHECK stays the four persisted states. |
| **R22 over-cap-tenant reaper** | **acknowledged, documented, NOT closed** | A tenant over the worker cap (default 100) spawns no reaper until it churns under the cap or `MaxTenants` is raised. T09 follows the cap-respecting `EnsureWorkers` pattern; the cap-independent sweep is a carried OQ, not an F3 deliverable. |
| **R37 settings cache lag** | accepted | Hot path reads via cached `GetParsedLedgerSettings` (5-min TTL), so advisory/enforce posture flips lag up to the TTL. No eager invalidation in F3. |
| **Line-drift corrections vs PLAN.md §7** | recorded | `limit_checker` orchestration entry is `checkLimitsInternal:147` / `getApplicableLimits:499` (NOT the cited `:341-495`). `EmitImportant` at `send_transaction_events.go:238` has **no `source` arg** (source is producer/Builder-owned, per the F1 §11 note). |

---

## 6. Harness debt (FLAG FOR THE F5/F6 GATES 8/13 OWNER)

1. **`RUN_PATTERN '^TestIntegration'` silently filters non-conforming names.** The F3 proofs were named outside the convention and the integration chain **silently skipped them** — they only ran focused during the proof workflow. Renamed in (`c21ea47f5`), now 14 pass under the harness. But the **entire legacy tracer integration suite** (`components/tracer/tests/integration`, ~144 files, `TestValidation_*`/`TestGet*`/… names) **remains silently excluded** — reachable since F0 made it part of the unified harness, but barely executed. **Widening `RUN_PATTERN` vs renaming the suite is an F5/F6 decision** with unknown runtime cost (the legacy suite is testcontainers-heavy `-p 1`; widening could materially extend the integration leg). Not closed in F3 — flagged for the Gates 8/13 owner.

2. **`testutil.MustDeterministicUUID` trailing-byte gotcha.** Multi-limit tests need explicit limit **names** (vs relying on `idx_limits_name_active`): deterministic UUIDs that differ only in trailing bytes can collide on the active-name partial index, producing spurious unique-violation failures in tests that seed multiple limits. Use explicit distinct names per limit in such tests.

---

## 7. Gate-closure walk (`F3.md` §3, all 7 gates + supporting design calls)

Every exit gate mapped to its closing task/commit and where the proof lives. A gate without a located proof is a defect.

| Gate (§3) | Closing task(s) / commit | Where the proof lives |
|-----------|--------------------------|-----------------------|
| **1 — Usage-drift crash proof (all three interleavings)** | T17 (SUT = T04 repo + T09 reaper + T13/T14 anchors); `37d44804` | `tests/integration/19_reservation_crash_convergence_test.go` — aborted reservations return `reserved_usage` to pre-reserve via reaper/TTL; `current_usage` equals exactly the sum of committed amounts at every reserve↔confirm interleaving. |
| **2 — WHERE-guard over-commit + `reserved_usage` grep** | T03 (the guard, `e9f7eb6d2`); T17 (concurrency, `37d44804`) | `tests/integration/19_*` over-commit test: N parallel reserves vs capacity N-1 → exactly N-1 succeed, Nth denied, `current_usage + reserved_usage` never exceeds `maxAmount`. Grep gate: `reserved_usage` present in `usage_counter_repository.go`. |
| **3 — Latency budget + audit-lock contention** | T20; `20f3fc536` | `docs/v4/plan/F3-T20-latency-report.md` — PASS, +0.79 ms p99; 2 advisory-lock rows/txn over 4,803 txns. §4 above. |
| **4 — Fail-open SKIPPED** | T19 (with T13 branch, T07 SKIPPED audit, T10 posture); `37d44804` | `transaction_reservation_failposture_test.go` (`TestTracerFailOpenSkipped`) — timeout/breaker-open + `failPosture=open` → transaction proceeds, SKIPPED recorded on the span. |
| **5 — Fail-closed reject** | T19 (with T13 branch, T10 posture); `37d44804` | `transaction_reservation_failposture_test.go` (`TestTracerFailClosedReject`) — timeout + `failPosture=closed` → transaction rejected, no balance mutation (sentinel `0178`). |
| **6 — Double-entry untouched (PD-5 re-proof; third rail)** | T18; `37d44804` | `pkg/mtransaction` sum-zero suite (269 tests) + the fee-seam structural gate `transaction_fee_seam_structure_test.go`, both green with the reserve anchor active in enforce mode. **Zero diff to balance math vs base `4fd2bb98d`** — the reserve seam sits strictly between `:1227` and `:1228`, outside the protected validate seam. |
| **7 — Reaper cadence (sub-minute, per-tenant, skip-on-pool-failure)** | T09 (worker, `206b92fe2`); T21 (proof, `37d44804`) | `reservation_reaper_cadence_integration_test.go` — expired RESERVED row released within the sub-minute interval with exactly one batch-summary `RESERVATION_EXPIRED` row; skip-on-pool-failure cycle writes no audit row and never touches the root pool. R22 over-cap case documented, not closed (§5). |

**Supporting design calls (§3, not numbered gates):** API shape (design A) → T08 `206b92fe2`; hybrid schema (B) → T01 `e9f7eb6d2`; settings group + enum validation (C) → T10 `2039de1ae`; reserve anchor (D/E) → T13 `2e2ae59d9`; PENDING lifecycle (F) → T15 `2e2ae59d9` (by-transaction divergence, §3a); confirm/release transport (G) → T14 `2e2ae59d9` (handler-region drift, §3b); ledger→tracer client → T11/T12 `030e7cbd6`; audit per-row+batch (Q11) → T07 `e516c2e67` (+ enum backing `1f33a8bae`); revert no-refund (Q9) → T16 `2e2ae59d9`; tracer docs → T22 `206b92fe2`. The contract-align fix `e486b10ad` and the enum-backing `1f33a8bae` are the honest cost of integrating a real external dependency — both audited, accepted, and recorded above.
