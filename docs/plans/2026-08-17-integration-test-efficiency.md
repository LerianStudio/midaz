# Integration Test Efficiency Plan

**Goal:** Make the Midaz integration signal complete and trustworthy, then reduce wall-clock time without weakening double-entry, money-path, tenant-isolation, or failure-recovery coverage.

**Architecture:** Integration build tags define what belongs to each lane. Required gates fail closed when discovery or prerequisites are missing. Datastore processes are eventually reused at package or shard scope, while every test keeps an isolated database, schema, namespace, or vhost. Parallelism is introduced only after isolation is explicit and measured.

**Status:** P0-P3 are implemented together and their consolidated gates are green. The current signal contains 1,726 exact integration tests; P1 reduces datastore starts by 92.9%, P2 reduces the serial critical path by 74%, and P3 removes redundant restarts, waits, cleanup, and history scans. The single final independent review is now the active step. Repository ruleset enforcement remains deliberately last.

## Phase overview

| Phase | Outcome | Status |
|---|---|---|
| P0 | Every required gate executes the coverage it claims and emits usable timing evidence | Complete — consolidated gates green |
| P1 | Ledger datastore startup and migrations are reused without sharing mutable test state | Complete — consolidated randomized and serial gates green |
| P2 | Independent families run concurrently within an explicit resource budget | Complete — five current-head shards green within budget |
| P3 | Tracer restarts, fixed waits, polling, cleanup, and streaming history scans are reduced | Implemented and measured |

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

### P0.4 CI contract and measurement

- [x] Enable a real integration gate for pull requests, using isolated jobs when the shared workflow cannot express the monorepo contract.
- [x] Record selected-test counts so naming or tag drift cannot silently shrink coverage.
- [x] Emit per-package and per-test durations in a machine-readable artifact.
- [x] Establish wall-clock, container-start, and restart baselines for every lane.
- [x] Fail closed when the Docker event observer cannot start or dies before the lane finishes.
- [x] Keep test-result caching disabled in CI; compilation and module caches remain enabled.

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
- [x] Support V1 `remaining` end to end: every resolved leg moves balances, persists an operation, and preserves double-entry across direct, pending, commit, cancel, revert, and fee paths.
- [x] Scope revert idempotency by the origin transaction without opening a rolling-deploy window that can double-revert.
- [x] Add Tracer's durable outcome receiver: serialize Reserve versus outcome, persist an idempotent terminal receipt, apply every reservation/counter/audit atomically, and keep V2 reservations out of autonomous expiry and cleanup.
- [x] Record the Ledger outcome in the same Redis/Lua commit that moves balances, then deliver and retry it until Tracer's durable acknowledgement.
- [x] Replace the incorrect "reaper is a durability backstop" assumption for a lost post-commit confirmation with a durable transaction-outcome mechanism.
- [x] Make tests, logs, and architecture docs expose lost-confirm undercounting as a known defect instead of describing it as successful reconciliation.
- [x] Replace the pinned lost-confirm undercount with the chosen durable money-path invariant.

**Money-path defects closed:**

1. Revert identity is now scoped to the origin transaction and protected by a durable PostgreSQL claim plus an executable old-to-bridge-to-final rollout. Ambiguous Redis/Lua outcomes fail closed and cannot authorize a second movement.
2. Ledger now records the economic outcome in the same Redis/Lua operation that moves balances. A dedicated dispatcher retries the immutable outcome until Tracer commits its receipt; V2 reservations never expire autonomously while delivery is unknown.

Chosen revert rollout contract: first deploy a freeze-capable legacy phase without changing revert identity; after every pod honors one shared rollout marker, activate the marker so updates to APPROVED transactions fail closed. Only then roll bridge and final idempotency phases. Final removes the freeze after old pods and in-flight requests are drained. Bridge readiness must reject activation without the shared freeze, so the safety condition is executable rather than a runbook promise.

V1 `remaining` closure: every resolved leg and balance identity survives direct execution, pending commit, pending cancel, revert, Redis replay, fees, zero-fee no-ops, persistence, and balance synchronization. Fee packages expose additive `operationRouteFromId` and `operationRouteToId` UUIDs while the existing free-form route labels remain passive; omission preserves an existing UUID, `null` clears only that UUID, and multi-fee partial updates preserve stored priorities atomically. The full low-resource lane passed with 1,620 selected tests, 1,540 passes, 80 classified skips, 1,320 container starts, zero restarts, and 2,972 seconds of wall time.

**Next P0:** run the single final money-path review with the rest of P0-P3, then resolve any finding against the same consolidated gates.

### P0 exit gate

- [x] Root integration selection includes all intended Ledger and Tracer integration tests.
- [x] Required lanes cannot pass through failed discovery, missing prerequisites, or all-skipped execution.
- [x] Property tests have one real, verified execution contract.
- [x] Pull requests run the required integration signal.
- [x] Tracer integration runs with the race detector enabled.
- [x] Baseline timings and selected-test counts are persisted as CI artifacts.

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
- [x] Set CPU, memory, container-count, and flake-rate budgets.
- [x] Promote a higher parallelism level only when repeated runs stay within every budget.

### P2 exit gate

- [x] Achieve a material wall-clock reduction over P1 without reducing selected-test counts.
- [x] Show no statistically meaningful increase in flakes across repeated CI runs.
- [x] Preserve deterministic failure attribution to one shard and one test.

P2 final measurement: 1,726 exact tests across five non-overlapping shards, zero retries, failures, or container restarts. At `p=2`, the current-head critical path is 138 seconds versus P1's 530 seconds (-74%). Peak memory remained below 1.5 GiB in every shard and peak live containers stayed inside each explicit budget. `p=4` is intentionally not promoted because 628 shared-server Tracer tests remain serial and dominate the critical path.

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
