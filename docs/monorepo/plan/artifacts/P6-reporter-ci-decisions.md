# P6 Reporter CI Decisions

Records the CI / scope decisions made during the reporter co-location so the
deferred work (P8 harmonization, integration-tagged heavy e2e) consumes them as
givens, not open questions.

- Phase: P6 (reporter two-component co-location, Option C)
- Branch: `phase/p6-reporter-colocation`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Decision 1 — Helm/gitops key mappings deferred to P8 (mirror P5 tracer precedent)

### Question
T13 wires the two reporter components into midaz CI. Should P6 also extend
`helm_values_key_mappings` (build.yml) and `yaml_key_mappings` (gitops-update.yml)
with the `midaz-reporter-manager` / `midaz-reporter-worker` keys, flipping the
deploy routes on the default branch?

### Decision: **NO — additive `filter_paths` only; deploy-key mappings deferred to P8 / T19.**
- The live `helm_values_key_mappings` is `{"midaz-crm":"crm","midaz-ledger":"ledger"}`
  and the live `yaml_key_mappings` is `{"midaz-crm.tag":...,"midaz-ledger.tag":...}`.
  **`midaz-tracer` is ABSENT from both** — P5 explicitly deferred tracer's
  helm/gitops mappings to P8 (see `P5-tracer-ci-decisions.md`, orchestrator scope
  note). P6 follows that established precedent exactly.
- P6 adds `components/reporter-manager` + `components/reporter-worker` to
  `filter_paths` in `build.yml`, `go-combined-analysis.yml`, and
  `pr-security-scan.yml` (delta = exactly +2 image candidates under the existing
  `app_name_prefix: midaz`), and `reporter` to `pr-validation.yml` scopes. The
  build fan-out therefore EMITS `midaz-reporter-manager` / `midaz-reporter-worker`
  image candidates, but no deploy-key routes them yet.
- **Rationale (R12).** Adding the helm/gitops key mappings before the external
  `midaz` Helm chart and `midaz-firmino-gitops` repo gain `manager`/`worker` keys
  would produce a "builds-but-never-deploys" (or worse, a broken ArgoCD sync). The
  P6-T19 lockstep coordinates the chart/gitops/APIDog extension; until that lands,
  the prefix/key flip stays off the default branch (T19 owner-unavailable fallback).
- A build.yml comment records this deferral inline so it is not a silent omission.

### Consumed by
- **P6-T19** Helm/gitops/APIDog lockstep — adds the reporter keys WITH the external
  chart/gitops change, not before.
- **P8** CI harmonization — folds the reporter deploy-key mappings into the
  consolidated config once lockstep is coherent.

---

## Decision 2 — 85% coverage-gate backfill deferred to P8 (P6-T15)

### Question
`go-combined-analysis.yml` enforces `coverage_threshold: 85` with
`fail_on_coverage_threshold: true` per filtered directory. Adding the two reporter
components to `filter_paths` subjects them to that gate. Reporter's origin
threshold/exclusions (`.ignorecoverunit`) may differ. Backfill now, or defer?

### Decision: **DEFER coverage-gate go-live for reporter to P8 (P6-T15).**
- P6's gate is the in-module subset: `go build ./...` green, reporter unit tests
  (the unit-level subset, integration-tagged excluded) green. The full 85%
  per-directory coverage backfill for the moved reporter code — reconciling
  reporter's `.ignorecoverunit` exclusions against midaz's gate and excluding
  `tests/reporter` + `pkg/reporter/itestkit` — is **P6-T15**, deferred to P8.
- Mirrors P5: tracer's coverage-gate go-live was likewise deferred to P8.

### Consumed by
- **P6-T15 / P8** coverage harmonization.

---

## Decision 3 — Heavy e2e is integration-tagged, runs in P8/integration env (P6-T16/T17)

### Question
The reporter suites include a full PDF pipeline e2e (T17: manager → RabbitMQ →
worker → SeaweedFS → Mongo status) and a fetcher external-DB reachability suite
(T16, R21). These need a SeaweedFS service (deferred to P8) and external DB
targets. Should P6 run them as part of the phase gate?

### Decision: **NO — heavy e2e is integration-tagged; runs in P8 / the integration env.**
- T17's PDF pipeline depends on a running SeaweedFS S3 service, whose service
  definition is owned by P8 (config staged at `components/infra/seaweedfs/`; the
  worker compose carries a Phase-8 TODO marker). T16's fetcher suite depends on
  external customer DB targets reachable over `infra-network`.
- Both are integration-tagged and run in the P8 / integration environment, not the
  P6 phase gate. P6 verifies the carried suites COMPILE and the unit-level subset
  passes; the broker→worker→S3 round-trip is a P8 concern.

### Consumed by
- **P8** integration env bring-up (SeaweedFS service + KEDA), at which point
  `tests/reporter/e2e/template_report_validation_test.go` and
  `tests/reporter/e2e/infra_datasources_test.go` run end-to-end.

---

## Scope summary (what P6 DID land in CI)

- `build.yml`: `filter_paths += components/reporter-{manager,worker}`; helm/gitops
  mappings unchanged (Decision 1).
- `go-combined-analysis.yml`: `filter_paths += components/reporter-{manager,worker}`.
- `pr-security-scan.yml`: `filter_paths += components/reporter-{manager,worker}`.
- `pr-validation.yml`: `pr_title_scopes += reporter`.
- `pkg/reporter` covered by the existing `shared_paths: pkg/` entry — no extra map.
- `go_private_modules`: absent (reporter never needed it); `go mod download` clean
  with no netrc/token.
