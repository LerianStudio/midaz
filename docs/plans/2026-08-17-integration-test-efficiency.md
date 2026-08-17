# Integration Test Efficiency Plan

**Goal:** Make the Midaz integration signal complete and trustworthy, then reduce wall-clock time without weakening double-entry, money-path, tenant-isolation, or failure-recovery coverage.

**Architecture:** Integration build tags define what belongs to each lane. Required gates fail closed when discovery or prerequisites are missing. Datastore processes are eventually reused at package or shard scope, while every test keeps an isolated database, schema, namespace, or vhost. Parallelism is introduced only after isolation is explicit and measured.

**Status:** P0's test infrastructure is implemented and measured. P1-P3 remain blocked by money-path defects that the newly honest lane exposed; the highest-blast-radius decision is the released V1 `remaining` expression, which can currently commit an unbalanced transaction.

## Phase overview

| Phase | Outcome | Status |
|---|---|---|
| P0 | Every required gate executes the coverage it claims and emits usable timing evidence | Blocked on money-path product decisions |
| P1 | Ledger datastore startup and migrations are reused without sharing mutable test state | Pending |
| P2 | Independent families run concurrently within an explicit resource budget | Pending |
| P3 | Tracer restarts, fixed waits, polling, cleanup, and streaming history scans are reduced | Pending |

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
| Unit | 15,391 cases, 6 existing skips | 26.6s | No containers |
| Property | 90 top-level tests in 11 packages | 16s | No containers |
| Tracer integration + race | 650 top-level tests, 1,165 cases/subcases | 230s | 16 container starts, no restart |
| Root integration, low resource | 1,613 top-level tests in 42 packages; 1,533 passed and 80 skipped | 2,874s | 1,283 container starts, no restart; RabbitMQ alone costs 598s |
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
- [ ] Stop V1 `remaining` legs from committing an APPROVED transaction whose persisted debits and credits differ (define whether V1 supports the expression correctly or rejects it).
- [ ] Scope revert idempotency by the origin transaction without opening a rolling-deploy window that can double-revert.
- [ ] Replace the incorrect "reaper is a durability backstop" assumption for a lost post-commit confirmation with a durable transaction-outcome mechanism.
- [x] Make tests, logs, and architecture docs expose lost-confirm undercounting as a known defect instead of describing it as successful reconciliation.
- [ ] Replace the pinned lost-confirm undercount with the chosen durable money-path invariant.

**Product/architecture blockers:**

1. V1 accepts `remaining`, resolves its amount during validation, then drops that leg during persistence and commits an unbalanced APPROVED transaction. The released API must either support the expression end to end or reject it before any ledger effect.
2. Two economically identical origins can share one revert idempotency slot, so the second origin is never reverted. Changing the live Redis key shape without a rollout contract can instead double-revert retries.
3. Ledger commits balances before the Tracer confirmation is durable. If that confirmation is lost, the reaper releases the hold and undercounts usage even though money moved.

### P0 exit gate

- [x] Root integration selection includes all intended Ledger and Tracer integration tests.
- [x] Required lanes cannot pass through failed discovery, missing prerequisites, or all-skipped execution.
- [x] Property tests have one real, verified execution contract.
- [x] Pull requests run the required integration signal.
- [x] Tracer integration runs with the race detector enabled.
- [x] Baseline timings and selected-test counts are persisted as CI artifacts.

**External enforcement gap:** the new jobs run on every pull request event covered by the workflow, but the repository rulesets do not yet require their new check contexts. Ruleset mutation is outside this code change and needs repository-admin authority.

## P1 — Reuse infrastructure, isolate state

### PostgreSQL

- [ ] Classify packages by reusable-database, lifecycle-exclusive, migration-exclusive, and chaos-exclusive behavior.
- [ ] Start one PostgreSQL process per reusable package or shard.
- [ ] Apply migrations once to a template database where supported.
- [ ] Give every test an isolated database or schema and deterministic cleanup.
- [ ] Keep connection-loss, startup, migration, and recovery tests on exclusive infrastructure.

### MongoDB, Valkey, and RabbitMQ

- [ ] Reuse one MongoDB process per package and allocate a database per test.
- [ ] Reuse Valkey only with a test-owned database or key namespace.
- [ ] Reuse RabbitMQ only with a test-owned vhost and exchange/queue namespace.
- [ ] Keep fixed-port and restart scenarios exclusive.
- [ ] Assert cleanup completeness so leaked state fails the owning test.

### P1 exit gate

- [ ] Reduce datastore container starts by at least 90% from the P0 baseline.
- [ ] Run every reusable package repeatedly with randomized test order and no state leakage.
- [ ] Preserve migration, recovery, tenant-isolation, and money-path assertions unchanged.
- [ ] Run focused race detection on every newly shared fixture.

## P2 — Bounded parallelism

- [ ] Split CI into stable Ledger PostgreSQL, Ledger MongoDB/CRM, async/broker, Tracer, and lifecycle/migration shards.
- [ ] Give every shard its own Docker and datastore scope.
- [ ] Make package parallelism configurable; benchmark `2` before `4`.
- [ ] Cap in-package parallelism independently from package parallelism.
- [ ] Keep shared-server, fixed-port, BDD journey, and system-chaos families serial.
- [ ] Set CPU, memory, container-count, and flake-rate budgets.
- [ ] Promote a higher parallelism level only when repeated runs stay within every budget.

### P2 exit gate

- [ ] Achieve a material wall-clock reduction over P1 without reducing selected-test counts.
- [ ] Show no statistically meaningful increase in flakes across repeated CI runs.
- [ ] Preserve deterministic failure attribution to one shard and one test.

## P3 — Remove secondary work

- [ ] Group Tracer tests by configuration and mock time to eliminate redundant server restarts.
- [ ] Replace fixed sleeps with condition-based waits and bounded backoff.
- [ ] Reduce semantic cleanup round-trips without bypassing lifecycle and audit behavior under test.
- [ ] Start E2E streaming consumers before the action or consume from a captured offset.
- [ ] Parallelize only Ledger E2E cases with independent organizations and ledgers, initially at four workers.
- [ ] Make every E2E identity globally unique and clean or rotate persistent test data.
- [ ] Keep retries limited to classified readiness and asynchronous convergence; never retry money-path assertions blindly.

### P3 exit gate

- [ ] Tracer restart count and fixed-wait time are measured and materially reduced.
- [ ] Streaming duration remains stable as broker history grows.
- [ ] Ledger E2E remains deterministic under its bounded worker count.
- [ ] Every optimization preserves the P0 selected-test counts and P2 resource budgets.
