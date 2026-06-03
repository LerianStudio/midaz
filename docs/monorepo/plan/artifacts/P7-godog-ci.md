# P7-T13a — godog CI delivery model decision (unified module)

Records the single owning decision P7-T13a requires: how the tracer godog BDD
suite ultimately runs in the consolidated monorepo CI. This binds P7-T15's
implementation (the CI job itself), which is DEFERRED to P8.

- Phase: P7 (unified go.mod / monorepo convergence)
- Branch: `phase/p7-gomod-unification`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Question

Does the unified-module CI run the tracer godog BDD suite
(`components/tracer/tests/end2end/`, 9 `.feature` files, build tag `e2e`) via the
existing `LerianStudio/github-actions-shared-workflows` surface, or as a bespoke
midaz-local workflow file?

## Verified facts (unified tree, this branch)

- **No godog hook in the shared workflow.** midaz's shared-workflow callers all
  pin `@v1.27.5`: `build.yml` (build/gitops/s3/apidog fan-out),
  `go-combined-analysis.yml` (`go-pr-analysis.yml` — lint/security/unit/coverage),
  plus `gptchangelog`, `pr-security-scan`, `pr-validation`, `release-notification`.
  NONE exposes a godog/BDD entrypoint or a generic "run an arbitrary make target"
  hook. `grep -rniE 'godog|cucumber|\.feature|bdd|test-e2e|test-bdd' .github/workflows/`
  returns zero matches — there is still no godog precedent in midaz CI.
- **The godog dependency resolves in the unified module** (P7-T13):
  `go list -m github.com/cucumber/godog` -> `v0.15.1`; `cucumber/gherkin/go/v26`
  and `cucumber/messages/go/v21` are recorded indirects. `go vet -tags e2e
  ./components/tracer/tests/end2end/...` compiles clean — the dep fold did not
  break the step-definition API (no OI-4 rewrite needed).
- **A unified runner target exists** (P7-T14): `make test-bdd` in `mk/tests.mk`
  runs `go test -tags e2e ./components/tracer/tests/end2end/...` against a tracer
  service addressed by `SERVER_ADDRESS` (the suite is e2e — it drives a running
  service over the wire, not testcontainers). The target hard-errors if
  `SERVER_ADDRESS` is unset so it never silently no-ops.

## Decision: BESPOKE midaz-local workflow, non-gating, path-filtered to tracer.

- **Bespoke, not shared.** The shared workflow cannot invoke `make test-bdd`
  without an upstream `github-actions-shared-workflows` change that adds a
  godog/BDD (or generic make-target) entrypoint. That cross-repo change is
  out-of-scope for P7 and would block on an external repo's release cadence.
  P7-T15 therefore stands up a bespoke midaz-local workflow file
  (proposed name: `.github/workflows/bdd-tracer.yml`) whose job runs `make test-bdd`
  after standing up the tracer service + its shared Postgres.
- **Non-gating.** Consistent with the P5 tracer-move-time call (P5-T12a-decide:
  bespoke + non-gating fast-follow) and the P6 reporter posture (heavy e2e runs in
  the integration env, not as a blocking PR gate). The godog harness is net-new to
  midaz CI; blocking PR merges on an unproven-in-monorepo harness trades a proven
  win (lint/unit/integration green in-module) for an unproven one. The job reports
  pass/fail and a deliberately-broken `.feature` must turn it red (P7-T15
  acceptance), but a red godog run does not block the merge queue at stand-up time.
- **Path-filtered to `components/tracer`.** The job runs only on PRs touching
  `components/tracer` (plus the shared `go.mod`/`go.sum`/`pkg/`/`Makefile` set), so
  it does not run on ledger-only PRs — mirroring the shared-workflow `filter_paths`
  model already in `build.yml`.

## Entrypoint the bespoke job rides

`make test-bdd SERVER_ADDRESS=http://<tracer-host>:<port>` (root delegation; the
target lives in `mk/tests.mk`, included by the root `Makefile`). The job is
responsible for the e2e infra prerequisite: bring up the tracer service + shared
Postgres (apply tracer migrations), then point `SERVER_ADDRESS` at it.

## Scope boundary (vs P5-T12a-decide)

P5-T12a-decide owns the tracer-MOVE-TIME stand-up decision and does NOT pre-bind
this unified-module delivery model. This artifact is the authoritative
unified-module call. The two are consistent (both: bespoke, non-gating), so there
is no conflict to reconcile.

## What is DEFERRED to P8

- **P7-T15 — the CI job itself.** Per orchestrator scope, the bespoke
  `bdd-tracer.yml` workflow file, its tracer+Postgres bring-up steps, and the
  red-scenario verification artifact are DEFERRED to P8 and implemented per this
  decision. P8 must NOT re-litigate shared-vs-bespoke or the gating posture.

## Consumed by / acknowledgement

- **P8-T18 (CI harmonization owner)** — acknowledges this decision so it is not
  re-decided in P8. The bespoke, non-gating, tracer-path-filtered model is the
  binding input to P7-T15's P8 implementation.
