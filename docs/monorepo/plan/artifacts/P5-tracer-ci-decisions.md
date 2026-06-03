# P5 Tracer CI Decisions

Records the two CI decisions that P5-T12a-decide and P5-T12b require so downstream
implementation tasks (P5-T12a godog plumbing, P8 release/coverage harmonization)
consume them as givens, not open questions.

- Phase: P5 (tracer co-location)
- Branch: `phase/p5-tracer-colocation`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Decision 1 — godog shared-vs-bespoke CI workflow (P5-T12a-decide)

### Question
Does the shared `LerianStudio/github-actions-shared-workflows` expose a godog/BDD job
hook midaz can call, or must midaz stand up a **bespoke** local workflow for the
tracer BDD suite (9 `.feature` files under `components/tracer/tests/end2end/`)?

### Verified facts
- There is **ZERO** godog/cucumber/BDD precedent anywhere in midaz CI today.
  `grep -rniE 'godog|cucumber|\.feature|bdd' .github/workflows/` returns nothing.
- midaz's two shared-workflow callers (`build.yml` → `build.yml@v1.27.5`,
  `go-combined-analysis.yml` → `go-pr-analysis.yml@v1.27.5`) expose lint / security /
  unit-test / coverage / build job hooks — none is a godog/BDD runner.
- tracer ships its own godog orchestration in `components/tracer/mk/tests.mk`
  (the 19KB `tests.mk`, `make test-e2e`, build tag `e2e`) — a self-contained runner
  that does not depend on any shared-workflow plumbing.

### Decision: **BESPOKE, non-gating, fast-follow.**
- midaz will stand up a **bespoke** GitHub Actions job for the tracer godog suite.
  There is no shared-workflow godog hook to call, and inventing a cross-repo
  shared-workflow change for a single component's BDD suite is not justified at
  tracer-move time. The bespoke job wraps the component's existing
  `make tracer COMMAND=test-e2e` (or the `mk/tests.mk` godog target).
- **Gating call: NON-GATING fast-follow.** godog does NOT gate the P5 merge. The
  harness is unproven inside the monorepo CI; blocking phase exit on it trades a
  proven win (lint/unit/integration green in-module) for an unproven one. godog
  MUST be green *somewhere* before tracer is declared deployable (P5-T14), but it
  rides as a same-week fast-follow CI job.
- No cross-repo request against the shared-workflows repo is required (bespoke path
  needs no shared-workflow change), so there is **no cross-repo lead time to start**.

### Scope boundary (vs P7-T13a)
This decision owns the TRACER-MOVE-TIME choice only: bespoke + non-gating fast-follow
for standing up godog at co-location. How godog ultimately runs in the *consolidated
unified-module* CI is owned by **P7-T13a** and is explicitly out of scope here.

### Consumed by
- **P5-T12a** (godog plumbing impl) — DEFERRED TO P8 per orchestrator scope. When
  executed, implement the bespoke job per this decision; do not re-open shared-vs-bespoke.

---

## Decision 2 — tracer migration S3 distribution (P5-T12b)

### Question
`build.yml` runs two `s3-upload.yml@v1.27.5` jobs publishing ledger's onboarding +
transaction migrations to the `lerian-migration-files` bucket for out-of-band ops
consumption. tracer ships 34 migration files (17 up + 17 down) including
audit-hash-chain DDL (`000001`/`000002` PL/pgSQL functions, `000003` prevent_truncate).
Mirror ledger (add a tracer s3-upload job) OR explicitly exclude tracer with rationale?

### What ledger does (grounding)
`build.yml` jobs `upload-onboarding-migrations` and `upload-transaction-migrations`:
- `file_pattern: components/ledger/migrations/<sub>/*.sql`
- `s3_prefix: ledger/<sub>/postgresql`
- `strip_prefix: components/ledger/migrations/<sub>`
- `secrets.AWS_ROLE_ARN: ${{ secrets.AWS_MIGRATIONS_ROLE_ARN }}`

### Decision: **OPTION (b) — EXCLUDE from S3 ops-migration distribution.**
tracer is **NOT** part of the S3 ops-migration distribution. Rationale:

1. **Integrity argument (decisive).** tracer's migrations are an ordered,
   hash-chained sequence: `000001`/`000002` install the audit-hash-chain PL/pgSQL
   functions, `000003` installs the `prevent_truncate` trigger, and later migrations
   evolve enums and convert cents→decimal. Out-of-band, hand-applied S3 distribution
   invites partial/out-of-order application that would silently break the
   hash-chain invariant. tracer's **bootstrap is the single applied-by path**
   (`libPostgres.NewMigrator` in `components/tracer/internal/bootstrap/config.go`
   runs the full ordered sequence with `AllowMultiStatements=false` to preserve the
   dollar-quoted function bodies). Keeping ONE applied-by path is a correctness
   property, not a convenience.
2. **No ops demand today.** Unlike ledger (where ops consume migrations out-of-band
   for managed-Postgres customers), tracer has no established out-of-band ops-apply
   workflow. Adding an S3 job now is speculative plumbing.
3. **Reversible.** If a managed-Postgres deployment later needs out-of-band tracer
   migrations, option (a) — a mirrored `s3-upload` job with
   `file_pattern: components/tracer/migrations/*.sql`,
   `s3_prefix: tracer/postgresql`, reusing `AWS_MIGRATIONS_ROLE_ARN` — can be added
   in P8 with no migration to the bootstrap path. The exclusion is not a one-way door.

### No silent gap
This is an explicit, signed-off exclusion — not an omission. The tracer migrations
are applied exclusively via tracer's own bootstrap migration runner.

### Consumed by
- **P8** release/distribution harmonization — revisit only if a managed-Postgres
  out-of-band ops-apply requirement materializes.

---

## Orchestrator scope note (P5-T12 narrowing)

Per the P5 execution scope, T12 in this commit is **ADDITIVE filter_paths ONLY**:
`components/tracer` was added to `filter_paths` in BOTH `build.yml` and
`go-combined-analysis.yml`. The full helm/gitops key-mapping harmonization
(`helm_values_key_mappings` + `yaml_key_mappings` `midaz-tracer` entries) and the
coverage-gate go-live for tracer are explicitly **DEFERRED to P8** and are NOT in
this commit. The build.yml fan-out will produce a `midaz-tracer` image candidate
once the path filter matches, but the deploy-key maps that route it are a P8 task.
