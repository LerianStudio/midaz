# Integration Test Efficiency Plan

**Goal:** Make the Midaz integration signal complete and trustworthy, then reduce wall-clock time without weakening double-entry, money-path, tenant-isolation, or failure-recovery coverage.

**Architecture:** Integration build tags define what belongs to each lane. Required gates fail closed when discovery or prerequisites are missing. Datastore processes are eventually reused at package or shard scope, while every test keeps an isolated database, schema, namespace, or vhost. Parallelism is introduced only after isolation is explicit and measured.

**Status (corrected 2026-08-28): delivery abandoned after layer 1.** P0-P3 and every finding from the final independent review were implemented together on the source branch, and delivery was split into four sequential review layers. Only layer 1 (reusable integration gate platform, PR #2341) merged. Layers 2-4 (PRs #2342/#2348, #2343, #2344) and the parent PR #2337 were **closed without merge**, their branches have not moved since 19-20/08, and no open PR replaces them. The test-infrastructure side of P0-P3 did land inside layer 1; **the money-path product work reopened in P0.6 below did not, so both money-path defects remain open on `develop`** — see "Money-path defects OPEN" below. The measurements in this document were taken on the source branch, so any figure that depends on the money-path layers describes code that is not in the product. The current signal contains 1,735 exact integration tests; P1 reduces datastore starts by 92.9%, P2 reduces the like-for-like base critical path by 70.4%, and P3 removes redundant restarts, waits, cleanup, and history scans. A required capability lane additionally executes all 76 chaos scenarios that the base matrix classifies as skips, making the complete required critical path 478 seconds. Repository ruleset enforcement remains deliberately last.

## Phase overview

| Phase | Outcome | Status |
|---|---|---|
| P0 | Every required gate executes the coverage it claims and emits usable timing evidence | Gates and discovery complete and on `develop` via PR #2341; **the P0.6 product items reopened below are OPEN** — their layers were closed without merge |
| P1 | Ledger datastore startup and migrations are reused without sharing mutable test state | Complete — consolidated randomized and serial gates green |
| P2 | Independent families run concurrently within an explicit resource budget | Complete — five base shards plus the required chaos capability green within budget |
| P3 | Tracer restarts, fixed waits, polling, cleanup, and streaming history scans are reduced | Complete — implemented, measured, and revalidated |

Evidence for the split: PR #2341 carried 100 files, including the shard contract in
`ci/integration-shards.tsv`, the skip allowlist, and the reusable PostgreSQL/MongoDB fixtures under
`tests/utils/` — all present on `develop`. The P0.6 product work reopened below is not.

### Execution order — 2026-08-17

1. Close revert idempotency with a durable origin claim and a rolling-deploy-safe bridge from the legacy Redis slot.
2. Persist the Ledger transaction outcome in the same atomic balance mutation and deliver it idempotently to Tracer; unknown outcomes retain capacity rather than undercounting usage.
3. Complete P1 infrastructure reuse and prove at least a 90% reduction in datastore container starts without shared mutable test state.
4. Complete P2 bounded sharding and package/test parallelism under measured CPU, memory, container, and flake budgets.
5. Complete P3 restart, wait, polling, cleanup, streaming-history, and E2E-worker reductions. Apply GitHub ruleset enforcement only after the code-side gates and all four phases are complete.

## P0 — Trustworthy signal

### P0.1 Integration selection and discovery

- [x] Make the integration build tag the default source of truth; remove the implicit `^TestIntegration` naming convention.
- [x] Preserve explicit `RUN=<pattern>` filtering for focused local runs.
- [x] Propagate package-discovery errors instead of suppressing `go list` failures.
- [x] Fail required integration gates when discovery returns zero packages or zero tests.
- [x] Keep root and Tracer component entry points semantically consistent.
- [x] Add regression tests that prove representative Ledger and Tracer tests are selected.

### P0.2 Property-test lane

- [x] Inventory the actual property-based tests under `components` and `pkg` (16 files, 11 packages, 90 top-level tests).
- [x] Give property tests one explicit, mechanically discoverable contract.
- [x] Stop the property lane from running unrelated packages or becoming a vacuous pass.
- [x] Keep property-only tests out of the ordinary unit lane instead of duplicating 5,871 unrelated executions.
- [x] Add regression tests for discovery and focused execution.

### P0.3 Required E2E mode

- [x] Preserve opt-in local behavior where an unavailable stack may skip the Ledger E2E suite.
- [x] Add a required mode where a missing Ledger, Tracer, broker, or explicitly selected lane fails immediately.
- [x] Prevent a required E2E command from succeeding after all tests were skipped.
- [x] Add a process-level timeout and structured test report.
- [x] Require the Tracer wiring probe to observe HTTP 422 with code 0177; treat 5xx as a technical failure.
- [x] Generate one ephemeral CI credential for Ledger and Tracer without logging it, and reject empty, mismatched, or disabled REST authentication before starting the runner.
- [x] Require the seven Tracer financial REST operations to authenticate independently of the global API-key mode; keep non-financial routes under plugin authentication and gRPC under mTLS.

### P0.4 CI contract and measurement

- [x] Enable a real integration gate for pull requests, using isolated jobs when the shared workflow cannot express the monorepo contract.
- [x] Record selected-test counts so naming or tag drift cannot silently shrink coverage.
- [x] Emit per-package and per-test durations in a machine-readable artifact.
- [x] Establish wall-clock, container-start, and restart baselines for every lane.
- [x] Fail closed when the Docker event observer cannot start or dies before the lane finishes.
- [x] Keep test-result caching disabled in CI; compilation and module caches remain enabled.
- [x] Record terminal pass, skip, fail, and missing outcomes; reject unclassified, stale, or changed skip declarations.
- [x] Prove every classified chaos skip by exact `package#test` identity in a required capability artifact; declarations without an executed pass fail closed.

#### P0 baseline — Mordor, 2026-08-17

| Lane | Selected / executed | Wall clock | Infrastructure baseline |
|---|---:|---:|---|
| Unit | 15,454 cases, 6 existing skips | 21.0s | No containers |
| Property | 90 top-level tests in 11 packages | 16s | No containers |
| Tracer integration + race | 650 top-level tests, 1,165 cases/subcases | 230s | 16 container starts, no restart |
| Root integration, low resource | 1,620 top-level tests in 42 packages; 1,540 passed and 80 skipped | 2,972s | 1,320 container starts, no restart; RabbitMQ remains the dominant startup cost |
| Required Ledger E2E | 110 tests, 13 skips for capabilities not selected by this lane | 6s after readiness | 16 Compose services/jobs; cached-image startup 15s; no restart |

Root skip classification: 76 chaos-only scenarios, 2 streaming smokes covered by the required E2E lane, and 2 real gaps. The obsolete transaction-idempotency skip now executes and proves first-outcome replay when the retry changes the amount; the remaining gaps are the origin-collision defect on revert and a migration-harness gap for transaction routes without operations.

### P0.5 `lib-commons` race

- [x] Reproduce the historical tenant-manager lazy-client race against the predecessor of its upstream fix.
- [x] Confirm Midaz already consumes a corrected `lib-commons` release; no dependency release or bump is needed.
- [x] Add an upstream regression test covering all three lazy-client paths in a separate local `lib-commons` commit.
- [x] Re-enable `-race` for Tracer integration tests and remove the stale test-only telemetry bypass.
- [x] Verify the focused reproducer and the complete Tracer integration lane under the race detector (650 selected tests; 1,165 cases/subcases; 230 seconds including infrastructure).

### P0.6 Money-path defects exposed by the honest gates

- [x] Reject a reused reservation tuple when the amount differs or the persisted hold is already terminal.
- [x] Keep reservation TTL separate from the financial counter's period-retention expiry.
- [x] Prove `reserve -> confirm -> cleanup` cannot erase valid daily, weekly, monthly, or custom usage.
- [ ] Support V1 `remaining` end to end: every resolved leg moves balances, persists an operation, and preserves double-entry across direct, pending, commit, cancel, revert, and fee paths.
- [ ] Scope revert idempotency by the origin transaction without opening a rolling-deploy window that can double-revert.
- [ ] Add Tracer's durable outcome receiver: serialize Reserve versus outcome, persist an idempotent terminal receipt, apply every reservation/counter/audit atomically, and keep V2 reservations out of autonomous expiry and cleanup.
- [ ] Record the Ledger outcome in the same Redis/Lua commit that moves balances, then deliver and retry it until Tracer's durable acknowledgement.
- [ ] Replace the incorrect "reaper is a durability backstop" assumption for a lost post-commit confirmation with a durable transaction-outcome mechanism.
- [x] Make tests, logs, and architecture docs expose lost-confirm undercounting as a known defect instead of describing it as successful reconciliation.
- [ ] Replace the pinned lost-confirm undercount with the chosen durable money-path invariant.

**Money-path defects OPEN — delivery abandoned (state corrected 2026-08-28):**

Both defects were implemented on the source branch and **neither reached `develop`**. The delivery was
split into four sequential review layers; only layer 1 landed, layers 2-4 and the parent PR were
**closed without merge**, and nothing open replaces them. The paragraphs below describe the *designed*
contract, not shipped behavior.

| PR | branch | state | branch tip | content |
|---|---|---|---|---|
| #2341 | `agent/pr2337-layer1-platform` | **MERGED** 2026-08-19 | — | reusable integration gate platform (1/4) |
| #2342 | `agent/pr2337-layer2-money` | **CLOSED**, no merge | `984bb0884` (2026-08-19) | make transaction economics and recovery durable |
| #2348 | `agent/pr2337-layer2-develop` | **CLOSED**, no merge | `e5ee68382` (2026-08-20) | same layer, rebased onto `develop` |
| #2343 | `agent/pr2337-layer3-fees` | **CLOSED**, no merge | `1ebb09b88` (2026-08-19) | preserve v1 `remaining` legs and fee economics |
| #2344 | `agent/pr2337-layer4-tracer` | **CLOSED**, no merge | `11309b994` (2026-08-19) | deliver durable Ledger outcome protocol |
| #2337 | `agent/midaz-clean-e2e-streaming` | **CLOSED**, no merge | `a8ce2cfe9` (2026-08-19) | parent PR, P0-P3 |

Verified against `develop` at `2707cbdc0` (2026-08-28): a bounded `git grep` for
`originClaim`, `operationRouteFromId`, and `operationRouteToId` under `components/ledger` and
`components/tracer` returns no matches, and listing those trees finds no path whose name contains
`outcome`. These checks did not find the planned identifiers or outcome-named artifacts; they do not
rule out a semantically equivalent implementation under other names. Together with the corresponding
layers having closed without merge, they support recording the designed durable outcome protocol,
PostgreSQL revert claim, and additive fee route UUIDs as not shipped by that delivery. The only revert
artifacts found by a bounded path search on `develop` are three test files
(`transaction_fee_revert_integration_test.go`, `transaction_revert_no_refund_test.go`,
`transaction_revert_replayed_test.go`), and `components/tracer/tests/integration/19_reservation_crash_convergence_test.go`
still pins the lost-confirm undercount as expected behavior. All five closed branches survive in
`origin` and are additionally preserved in the `midaz-agent-branches-2026-08-28.bundle` git bundle held
by the consignado program workspace. This entry records state only; the delivery is not being replanned
here.

1. **Revert identity — OPEN.** The design scopes revert identity to the origin transaction, protected by a durable PostgreSQL claim plus an executable old-to-bridge-to-final rollout, so that ambiguous Redis/Lua outcomes fail closed and cannot authorize a second movement. None of it is on `develop`.
2. **Durable Ledger outcome — OPEN.** The design records the economic outcome in the same Redis/Lua operation that moves balances, with a dedicated dispatcher retrying the immutable outcome until Tracer commits its receipt and V2 reservations never expiring autonomously while delivery is unknown. None of it is on `develop`; the reaper remains the de-facto backstop the plan set out to replace.

Designed revert rollout contract (not deployed): first deploy a freeze-capable legacy phase without changing revert identity; after every pod honors one shared rollout marker, activate the marker so updates to APPROVED transactions fail closed. Only then roll bridge and final idempotency phases. Final removes the freeze after old pods and in-flight requests are drained. Bridge readiness must reject activation without the shared freeze, so the safety condition is executable rather than a runbook promise.

V1 `remaining` — OPEN, and `develop` moved the other way. The layer-3 design had every resolved leg and balance identity survive direct execution, pending commit, pending cancel, revert, Redis replay, fees, zero-fee no-ops, persistence, and balance synchronization, with fee packages exposing additive `operationRouteFromId` and `operationRouteToId` UUIDs beside passive free-form route labels. That work is in closed PR #2343. Since then the merged direction has been the opposite: **#2407** gated the fee engine off `/v1` transaction routes, **#2408** gated the holder seam off `/v1` routes, **#2409** gated the tracer reservation off `/v1` transaction routes, and **#2412** kept the `/v1` account contract servable on a pre-holder schema. `/v1` on `develop` is now a legacy compatibility surface with those features deliberately switched off — it is not, and is no longer intended to be, the surface where `remaining` works end to end. The measurement once quoted as this item's closure (1,620 selected tests, 1,540 passes, 80 classified skips, 1,320 container starts, 2,972 seconds) was taken on the source branch, not on `develop`.

Final review closure claims — asserted on the source branch. For claims that depend on the durable
outcome protocol, the bounded searches above did not find the planned identifiers or outcome-named
artifacts on `develop`; treat those claims, and the rest, as unverified against `develop` rather than
as shipped: decimal balance arithmetic exact beyond IEEE-754 precision; Ledger discovering multi-tenant outcome backlog from durable state instead of an expiring process cache; outcome, active index, schedule, and tenant discovery sharing one Redis Cluster slot and one atomic prepare; V2 admission requiring non-evicting AOF-backed Valkey; Ledger-to-Tracer REST on a dedicated always-on API key with mTLS and tenant identity; rolling Tracer deployments failing before the money path unless the selected pod explicitly accepts the V2 protocol; async persistence publishing terminal status and the complete operation multiset in one PostgreSQL commit so no reader observes an approved half-entry; and the required E2E proving the complete Ledger-to-Tracer flow, receipt persistence, exact audit context, and replay after a lost acknowledgement.

### P0 exit gate

- [x] Root integration selection includes all intended Ledger and Tracer integration tests.
- [x] Required lanes cannot pass through failed discovery, missing prerequisites, or all-skipped execution.
- [x] Property tests have one real, verified execution contract.
- [x] Pull requests run the required integration signal.
- [x] Tracer integration runs with the race detector enabled.
- [x] Baseline timings and selected-test counts are persisted as CI artifacts.
- [x] Every base skip is either rejected or matched to an exact passing capability test; the current required set proves all 76 chaos identities.

**External enforcement gap, scheduled last:** the new jobs run on every pull request event covered by the workflow, but the repository rulesets do not yet require their new check contexts. Apply the repository-admin mutation only after P0-P3 and the final completion audit.

## P1 — Reuse infrastructure, isolate state

### PostgreSQL

- [x] Classify packages by reusable-database, lifecycle-exclusive, migration-exclusive, and chaos-exclusive behavior.
- [x] Start one PostgreSQL process per reusable package or shard.
- [x] Apply migrations once to a template database where supported.
- [x] Give every test an isolated database or schema and deterministic cleanup.
- [x] Keep connection-loss, startup, migration, and recovery tests on exclusive infrastructure.

### MongoDB, Valkey, and RabbitMQ

- [x] Reuse one MongoDB process per package and allocate a database per test.
- [x] Reuse Valkey only with a test-owned database or key namespace.
- [x] Reuse RabbitMQ only with a test-owned vhost and exchange/queue namespace.
- [x] Keep fixed-port and restart scenarios exclusive.
- [x] Assert cleanup completeness so leaked state fails the owning test.

### P1 exit gate

- [x] Reduce datastore container starts by at least 90% from the P0 baseline.
- [x] Run every reusable package repeatedly with randomized test order and no state leakage.
- [x] Preserve migration, recovery, tenant-isolation, and money-path assertions unchanged.
- [x] Run focused race detection on every newly shared fixture.

P1 final measurement: 1,726 selected tests completed serially in 530 seconds with 94 owner-attributed datastore starts, zero restarts, and zero leaks, down 92.9% from 1,320 starts and 2,972 seconds. The 37 reusable packages also passed two randomized repetitions in 577 seconds. The financial Valkey profile uses no eviction, AOF, and synchronous persistence; lifecycle and configuration-mutating tests remain exclusive.

## P2 — Bounded parallelism

- [x] Split CI into stable Ledger PostgreSQL, Ledger MongoDB/CRM, async/broker, Tracer, and lifecycle/migration shards.
- [x] Give every shard its own Docker and datastore scope.
- [x] Make package parallelism configurable; benchmark `2` before `4`.
- [x] Cap in-package parallelism independently from package parallelism.
- [x] Keep shared-server, fixed-port, BDD journey, and system-chaos families serial.
- [x] Execute chaos capability packages in bounded owner-isolated waves and aggregate their exact terminal outcomes into the stable lifecycle check.
- [x] Set CPU, memory, container-count, and flake-rate budgets.
- [x] Promote a higher parallelism level only when repeated runs stay within every budget.

### P2 exit gate

- [x] Achieve a material wall-clock reduction over P1 without reducing selected-test counts.
- [x] Run repeated seeded CI rounds with zero retries or failures; do not claim statistical significance from a small sample.
- [x] Preserve deterministic failure attribution to one shard and one test.

P2 final measurement: 1,735 exact base tests across five non-overlapping shards (462 PostgreSQL, 216 MongoDB/CRM, 298 async/broker, 656 Tracer, and 103 lifecycle/migration). The base artifacts contain 1,659 passes and 76 classified chaos skips, with zero failures, missing outcomes, or retries. A sixth required capability lane executes those same 76 `package#test` identities under real chaos: 76 passes, zero skips/failures/retries, and exact aggregation back into the stable lifecycle check. The base current-head critical path is 157 seconds versus P1's 530 seconds (-70.4%); the expanded required path including chaos is 478 seconds. Every lane includes process-tree and owner-container resources and stayed inside its CPU, RSS, and container budgets; the chaos lane peaked at 840 MiB RSS and seven live containers. Completed fixtures are cleaned at safe barriers, no non-Ryuk container survives, and expected chaos restarts are attributed to the owning lane. Tracer's 656 tests run with the race detector. Higher base parallelism is intentionally not promoted because the shared-server Tracer tests remain serial by contract and added concurrency would not shorten the measured base path.

## P3 — Remove secondary work

- [x] Group Tracer tests by configuration and mock time to eliminate redundant server restarts.
- [x] Replace fixed sleeps with condition-based waits and bounded backoff.
- [x] Reduce semantic cleanup round-trips without bypassing lifecycle and audit behavior under test.
- [x] Start E2E streaming consumers before the action or consume from a captured offset.
- [x] Parallelize only Ledger E2E cases with independent organizations and ledgers, initially at four workers.
- [x] Make every E2E identity globally unique and clean or rotate persistent test data.
- [x] Keep retries limited to classified readiness and asynchronous convergence; never retry money-path assertions blindly.

### P3 exit gate

- [x] Tracer restart count and fixed-wait time are measured and materially reduced.
- [x] Streaming duration remains stable as broker history grows.
- [x] Ledger E2E remains deterministic under its bounded worker count.
- [x] Every optimization preserves the consolidated P0 selected-test counts and P2 resource budgets.

P3 measurement: Tracer restarts fell from 100 to 20, fixed waits from 4.9 seconds to zero, and the measured duration from 147.5 to 51.9 seconds. Streaming with 5,000 historical events examines one event instead of 5,001. Ledger E2E passed all 39 cases across three four-worker rounds; the full E2E lane passed 111 tests with 13 explicit capability skips in 9 seconds.
