## Midaz Quality Report — Test and Chaos Findings (2025-09-22)

This report consolidates current test results, failures, suspected/confirmed bugs, and recommended actions. It is organized for quick triage by the team.

---

**Executive Summary**

- Integration tests reveal functional gaps: metadata multi-filter semantics, Account GET schema stability, DSL vs JSON parity, and revert 500s.
- Temporal/consistency concerns: createdAt monotonicity and bounded read-after-write visibility for accounts.
- Chaos tests consistently fail with durability/ack/order-of-commit problems and recovery behavior (Postgres restart, RMQ backlog churn, network partition), plus onboarding health dependence on replica.
- Alias→balance readiness was ruled out as the root cause for chaos failures after adding prechecks; core issues remain on durability/idempotency.
- Long/chaos tests should be gated to nightly; keep fast smoke in routine CI.

---

**Current Status**

- Unit/Property: PASS (1 placeholder skipped)
- Integration: Multiple FAILs (see “Integration Findings”), several tied to alias readiness and functional semantics.
- Chaos: 5 FAILs (see “Chaos Findings”) persisting after readiness prechecks.

---

**Priority Matrix (proposed)**

- P0 (Blockers)
  - Revert intermittently 500 (MZ-001)
  - Durability/ack/idempotency under restart/partition/backlog (MZ-013–MZ-017)

- P1 (High)
  - Metadata multi-filter uses OR/ignores one dimension (MZ-002)
  - DSL vs JSON parity discrepancy (MZ-006)
  - createdAt monotonicity/sort/format issues (MZ-012)

- P2 (Medium)
  - Account GET missing metadata object or schema optionality (MZ-004)
  - Account list read-after-write bound exceeded (MZ-011)
  - Fuzzed org names 500 instead of 4xx (MZ-008)

---

**Integration Findings (latest run)**

Command: `make test-integration | grep -E "FAIL:|SKIP:"` (2025-09-22)

- Multi-filter semantics fail for accounts/ledgers/orgs → MZ-002
- Balance permission flags and batch partial failure blocked by alias readiness → MZ-003
- Contract smoke (Account GET) missing metadata → MZ-004
- DSL vs JSON parity → MZ-006
- Revert path 500s → MZ-001
- Eventual consistency (accounts list, operations list after inflow) → MZ-011, MZ-003
- Operations filters (date/type) and aggregation by metadata → affected by MZ-003; re-verify after fix
- Temporal createdAt monotonicity → MZ-012

Skipped (gated/not bugs): see Appendix C.

---

**Chaos Findings (latest run)**

Command: `make test-chaos` (2025-09-22)

- PostChaosIntegrity_MultiAccount: final balances mismatch after DB pause/restart → MZ-017
- PostgresRestart_DuringWrites: final mismatch after primary restart → MZ-013
- RabbitMQ_BacklogChurn_AcceptsTransactions: mismatch under backlog churn → MZ-014
- Startup_MissingReplica_NoPanic: onboarding health fails without replica → MZ-015
- TargetedPartition_TransactionVsPostgres: no convergence after reconnect → MZ-016

Note: After adding alias-readiness prechecks in chaos (RMQ test) the failures persisted; not attributable to MZ-003.

---

**Recommendations and Next Actions**

- Durability & Idempotency (P0)
  - Ensure HTTP 201 after durable commit or append to an exactly-once application log. Avoid ack-before-commit.
  - Enforce/derive idempotency keys on write paths; dedupe replays across restart/partition.
  - Validate DB pool reconnection logic and backoff; confirm recovery before resuming 201s.

- Functional Semantics (P1)
  - Implement AND semantics for multi-dimensional metadata filters or document/expose explicit operators.
  - Align DSL/JSON validators and eligibility checks.
  - Return `metadata: {}` in Account GET or mark optional in OpenAPI + clients.
  - Ensure operations carry valid RFC3339 createdAt and honor sort order.

- Consistency/UX (P2)
  - Guarantee bounded read-after-write for listing (document SLA; add polling hints if needed).
  - Harden onboarding input validation to return 4xx for invalid organization names.

- CI Strategy
  - Keep chaos/heavy under `MIDAZ_TEST_HEAVY`/nightly. Add `make test-chaos-nightly` for schedulers.
  - Maintain fast smoke for PR feedback.

---

**Appendix A — Detailed Issue Tracker**

- MZ-001 Revert intermittently 500
  - Area: `POST /transactions/{id}/revert`
  - Evidence: `tests/integration/diagnostic_revert_test.go`, `tests/e2e/lifecycle_e2e_test.go`, `tests/e2e/refund_flow_test.go`
  - Impact: Breaks refunds and auditability

- MZ-002 Multi-dimensional metadata filters use OR/ignore one dimension
  - Area: Onboarding list filters for accounts/ledgers/organizations
  - Evidence: `tests/integration/accounts_metadata_multi_filter_test.go`, `.../ledgers_...`, `.../organizations_...`
  - Expected: AND semantics across dimensions

- MZ-003 Default balance by alias not ready immediately
  - Area: `GET /accounts/alias/{alias}/balances`, `PATCH /balances/{id}`
  - Evidence: balance flags and batch partial failure tests
  - Notes: Helpers now poll by account ID first; alias readiness still a known eventual consistency area

- MZ-004 Account GET omits metadata object
  - Area: `GET /accounts/{id}`
  - Evidence: `tests/integration/contract_api_smoke_test.go`
  - Options: always return `{}` or make optional in schema

- MZ-005 Diagnostic burst consistency mismatch
  - Area: high-concurrency operation totals
  - Evidence: `tests/integration/diagnostic_burst_consistency_test.go`

- MZ-006 DSL vs JSON parity
  - Area: `/transactions/dsl` vs `/transactions/json`
  - Evidence: `tests/integration/diagnostic_dsl_vs_json_test.go`, `.../idempotency_variants_test.go`

- MZ-007 Test helper RandString race (test-only)
  - Area: `tests/helpers/random.go`

- MZ-008 Fuzzed organization names return 500 (should be 4xx)
  - Area: `POST /v1/organizations`
  - Evidence: `tests/fuzzy/fuzz_test.go`

- MZ-011 Account list read-after-write exceeds bound
  - Area: `GET /accounts` list
  - Evidence: `tests/integration/eventual_consistency_test.go`

- MZ-012 Operations `createdAt` monotonicity/sort/format
  - Area: `GET /accounts/{id}/operations`
  - Evidence: `tests/integration/temporal_monotonicity_test.go`

- MZ-013 Postgres restart during writes — durability/ack
  - Area: transaction acceptance vs DB commit
  - Evidence: `tests/chaos/postgres_restart_writes_test.go`

- MZ-014 RabbitMQ backlog churn — acceptance vs final state
  - Area: async acceptance under paused/unpaused broker
  - Evidence: `tests/chaos/rabbitmq_backlog_churn_test.go`

- MZ-015 Startup missing replica — onboarding health
  - Area: onboarding readiness when replica down
  - Evidence: `tests/chaos/startup_missing_replica_test.go`

- MZ-016 Targeted partition txn↔postgres — recovery
  - Area: network partition handling, reconnection, idempotent re-apply
  - Evidence: `tests/chaos/targeted_partition_transaction_postgres_test.go`

- MZ-017 Post-chaos integrity — multi-account reconciliation
  - Area: net effect accounting correctness after chaos events
  - Evidence: `tests/chaos/post_chaos_integrity_multiaccount_test.go`

Note: Operation Routes empty on fresh env is expected (not a bug).

---

**Appendix B — Fixes and Test-only Notes**

- Test-only fix: Events async sanity now creates USD asset before inflow
  - File: `tests/integration/events_async_test.go:26`
- Chaos test hardening: alias readiness precheck added before enabling flags by alias
  - File: `tests/chaos/rabbitmq_backlog_churn_test.go:46`

---

**Appendix C — Skipped Tests (not bugs)**

- `tests/property/TestPropertyStringRoundTrip` — placeholder
- `tests/e2e/TestE2E_Lifecycle_PendingCommitThenRevert` — skipped due to MZ-001
- `tests/e2e/TestCompleteWorkflow` — pending
- Heavy/nightly gated: cursor stability, large dataset, concurrent creates; security tests require auth gates

---

If you confirm or fix any item above, please update this report with commit hashes, owner, and status.
