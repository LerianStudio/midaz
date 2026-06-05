# F1 — Execution Note

> Live record of v4 phase F1 (Identity: ownership in the ledger) execution: decisions made and commit SHAs as each task landed, mirroring `docs/v4/plan/F0-EXECUTION-NOTE.md`. Authoritative for the F1 commit ledger, the at-`324c10ee4` baseline counts, the load-bearing identity-design decisions, the gate-closure map, and the gate-2 behavioral-equivalence proof.
> - **Date:** 2026-06-04
> - **Branch:** `feat/monorepo-consolidation`
> - **F0 epoch (diff base for every F1 claim):** `46706c9eaff0a85fab80f693b1d4f82de56e239b`
> - **F1 tip (baseline-capture SHA):** `324c10ee4dc25759c5368f2b02a057b86cbb5140`

---

## Commit ledger (chronological, on top of the F0 epoch)

F1 landed in four task waves plus two repair commits, all on top of `46706c9ea`.

| SHA | Wave | Tasks closed | What it closed |
|-----|------|--------------|----------------|
| `5fba8794c` | W1 — foundation | F1-T01, T02, T03, T04, T06, T07, T12, T14 | `holder_id` on domain + PG model; migrations `000017` (column) / `000018` (`idx_account_holder` CONCURRENTLY); INSERT/SELECT/`ToEntity`/`FromEntity` round-trip; `EntityHolder` constant (`ErrHolderNotFound` CRM-0006 reused, not duplicated); `RequireHolder` through every settings surface; `holder_id = ?` account-list filter; D-12 immutability greps + `entityId` doc clarifications. |
| `9e58edb8d` | W2 — semantics | F1-T05, T08, T09, T11 | `HolderReader` port + `RequireHolder`-gated existence check; self-holder default materialisation (derived, `@external` null) routed through the cached settings reader; eager self-holder in `CreateOrganization` (non-fatal post-commit); `account.created` payload gains `holderId` (drift-discipline trio, field count 15→16). |
| `4adf3e122` | W3 — route + backfill | F1-T13, T10 | `GET /v1/holders/:id/accounts` under the `plugin-crm` namespace via a thin org-scoped reader port; cross-store backfill runner (`components/ledger/internal/services/backfill`, Mongo-first ordering, idempotent, MT, `@external`-exempt). |
| `6d71b44d8` | W4 — bootstrap wiring | F1-T15 | Injected `HolderReader` (CRM-service adapter), the cached settings reader (adapter over `queryUseCase.GetParsedLedgerSettings` — the L3 seam), the self-holder provisioner (`CreateHolderWithID`), and the holders-accounts reader into the composition root. |
| `4879b9853` | repair (tagged) | — | Fixed 3 tagged call sites in `crm_collapse_integration_test.go` (`:241,:358,:395`) carrying the old `RegisterCRMRoutesToApp` arity after F1-T13 extended the signature. Caught by the baseline, not by the untagged commit gates (process lesson below). |
| `324c10ee4` | repair (property) | — | `sanitizeQuickTenantID*` in both redis property suites (6 sites): the generators fed `':'`-containing tenant IDs that `GetKey` rejects by contract. **= F1 baseline-capture SHA.** |

Wave boundaries match the dependency DAG in `F1.md` §1: foundation (compile floor) → semantics (gates + emit) → route/backfill (reads + repair path) → wiring (composition root). The two repairs are not new tasks; they close breakage the baseline surfaced.

---

## Baseline (FINAL, captured at `324c10ee4` with a dedicated `GOCACHE=/tmp/midaz-gocache-f0`)

| Command | Exit | Result | Δ vs F0 epoch |
|---------|------|--------|---------------|
| `make test-unit` | 0 | 15,913 tests, 6 skipped | +36 tests |
| `make test-integration` | 0 | 983 tests, 80 skipped (**`RETRY_ON_FAIL=1` declared**) | +5 packages' worth of new F1 integration tests |
| `make test-property` | 0 | 70 tests, 7 skipped | — |
| `make test-reporter-chaos` | 0 | 39 tests, 39 skipped (`CHAOS=1` opt-in by design) | — |
| `make ci` | 0 | single exit code; all four legs reproduced (15,913 / 983 / 70 / 39) | — |

`make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor; `make ci` is the single-verdict superset. The F0 baseline (15,877 / 978 / 70 / 39) is the diff base; F1 adds 36 unit tests and 5 integration packages and removes nothing.

---

## Environment disclosure (recorded honestly)

The integration leg required **multiple runs** to capture a green. This is an environmental constraint, not an F1 code defect, and is recorded in full because it conditions how this baseline must be read.

- **Class:** docker.sock inspect-deadline on macOS Docker Desktop under sustained sequential testcontainers load. Containers start fine; the daemon's inspect API wedges at a random position in the matrix. Across runs it struck `postgres/organization` (twice), `postgres/transaction`, the tracer workers, and `http/in`.
- **Isolation dossier (the load-bearing fact):** **every** flaked test passed in isolation — `ORG_EXIT=0`, `ISO_TX=0`, `ISO_TRACER=0`. There were **zero assertion failures** in any run; run 4 was 983/983 assertions green. Daemon restarts clear the wedge temporarily.
- **Mitigations applied:** `ForListeningPort` wait strategy (landed in F0), `RETRY_ON_FAIL=1` declared for the integration leg, daemon restart before the capture run.
- **Authority:** Linux CI runners are the authoritative environment for this matrix. The macOS inspect-deadline is a host-daemon characteristic; a green on the authoritative runner is the binding signal, and the isolation passes prove the F1 code is correct under it.

This is the same Docker-inspect flake class flagged in the F0 environment notes, now reproduced at F1 scale and discharged the same way: isolation passes + an authoritative-runner verdict, not a clean first macOS run.

---

## Behavioral equivalence — gate 2's record obligation

Gate 2 requires that with `RequireHolder=false` (the default), the transaction and read paths behave identically to the F0 epoch — F1 must add ownership without touching the money path. The proof is a zero diff on the two transaction handlers across the entire F1 delta:

```
git diff 46706c9ea..324c10ee4 --stat -- \
  components/ledger/internal/adapters/http/in/transaction_create.go \
  components/ledger/internal/adapters/http/in/transaction_state_handlers.go
```

**Result at HEAD `324c10ee4`: empty output** (no files listed, no insertions, no deletions). Re-verified at the F1 tip for this note; previously verified empty at `6d71b44d8` (W4). The transaction handlers carry zero F1 edits — no new settings reads, no fee-seam adjacency churn, no ownership plumbing leaked into the money path.

The only intended F1 deltas on the create path are: (a) `account.created` gains `holderId` (gate 6), and (b) non-`external` accounts gain a materialised `holder_id` derived from the org self-holder. Neither touches transaction creation, commit, cancel, or revert. `RequireHolder` defaults to `false`, the `AccountingValidation` struct stays comparable (L1), and `LedgerSettingsIsDefault` still holds for pre-F1 ledgers — so existing ledgers serialize identically.

---

## Load-bearing decisions (distilled from the wave agents)

The full decision/drift log lives in the workflow result; these are the ones future phases must not relitigate.

1. **`midazNamespace` is a hardcoded `uuid.MustParse` literal, immutable forever.** It lives in `components/ledger/internal/services/command/holder_ports.go` and is the single source of truth for the UUIDv5 self-holder derivation (`uuid.NewSHA1(midazNamespace, orgID.Bytes())`). Shared by T08 (create-path default), T09 (org-create eager provision), and T10 (backfill). T10 exports `command.DeriveSelfHolderID` as a thin wrapper over the unexported helper rather than re-declaring the literal — one namespace source, smallest change. **Changing this constant silently re-homes every existing account's owner. It is frozen.**

2. **Nil `HolderReader` under `RequireHolder=true` deliberately panics.** `applyHolderValidation` does **not** guard a nil reader. F1-T15 owns wiring and guarantees non-nil; a nil reader under `RequireHolder=true` is a bootstrap bug that must surface loudly, not be silently no-op'd into a permissive path. The outer guard (`requireHolder && HolderID set`) keeps a nil reader unreachable in the default/permissive config.

3. **The settings reader degrades permissive (`false`) on nil or error.** `requireHolderEnabled` returns `false` when `SettingsReader` is nil or errors — backward-compatible behavior, and it decouples T05/T08 semantics from the T15 wiring so the gate is provably off until both the flag and the reader are wired.

4. **`CreateHolderWithID` is isolated in `create-holder-with-id.go`** (not appended to the legacy `create-holder.go`), so it does not inherit the legacy method's CLAUDE.md violations. The new method fixes them: a `ctx.Err()` guard before the Mongo insert, a single `time.Now()` reused for `CreatedAt`/`UpdatedAt`, and structured `libLog.Err(err)` logging (no `fmt.Sprintf` in logger, no spurious `LevelInfo`). **Idempotency:** a duplicate `_id` is treated as success via the raw `mongo.IsDuplicateKeyError` check, then a re-fetch returns the existing holder; a genuine *document* conflict is wrapped as `pkg.ValidateBusinessError` (CRM-0010, which does not satisfy `IsDuplicateKeyError`) and still propagates as an error. Only the `_id` collision is idempotent.

5. **The holder-existence gate asserts `errors.As(EntityNotFoundError)` with `Code==CRM-0006` and `EntityType==Holder`** — not a fragile error-message substring. This is the proof for gate 9: it exercises the `EntityHolder` constant and the `ErrHolderNotFound` sentinel mapping through `ValidateBusinessError`, not `reflect.TypeOf`. Note: `ValidateBusinessError` leaves the wrapped `Err` field nil (`pkg/errors.go:1136-1140`), so `errors.Is(err, ErrHolderNotFound)` does **not** match through `Unwrap()` — detection must go through `errors.As` + code comparison. Adapters that need to detect not-found do the same.

6. **The backfill runner lives in `components/ledger/internal/services/backfill`** with narrow injected deps (an `OrgLister` port + `command.HolderProvisioner`), so it is unit/integration testable without booting the binary; the entrypoint is a thin `cmd/backfill/main.go` + `bootstrap.InitHolderBackfill` reusing the existing private initialisers. Ordering is **Mongo-first** (provision the self-holder, then materialise PG `holder_id`) so `holder_id` never points at a non-existent holder (R15). A provisioning failure for an org **aborts the whole pass** rather than skip-and-continue — a partial pass that materialised `holder_id` against a holder that failed to provision would violate the no-orphan invariant; the runner is idempotent, so a re-run after the fix is a no-op.

7. **`HolderAccountsHandler` is a dedicated handler** (not a method on the Mongo-backed `HolderHandler`), keeping the ledger-reader dependency isolated. Its reader port is **org-scoped without `ledgerID`** — `ListAccountsByHolder(ctx, organizationID string, holderID uuid.UUID, filter http.QueryHeader)` — matching R14 org-global ownership and the per-org holder collection. The only landed account-list read (`query.GetAllAccount → repo.FindAll`) is ledger-partitioned; the bootstrap adapter (F1-T15) reconciles org-global ownership against the ledger-partitioned read by requiring `ledger_id` from the query params (returning a clean `4xx` when absent, never a silent empty result). A true org-global account-list-by-holder method is left to a future task owning the query/repo layer; the org-scoped port signature already accommodates it without a bootstrap change.

8. **Integration-test timestamps follow the suite-wide `time.Now()` convention** (`time.Now().Truncate(time.Microsecond)`, `time.Now().Add(±24h)`) rather than fixed times. This is a **declared tension with CLAUDE.md** ("do not use `time.Now()` in tests"): CLAUDE.md also says follow existing project style, and every test in `account.postgresql_integration_test.go` uses this exact date-window pattern. Asserted values (`HolderID`) use fixed/generated UUIDs, so assertion determinism holds — only the date-window plumbing is wall-clock, and it is not asserted on.

9. **Swagger regeneration is deferred to F5.** Only the hand-written `@Description`/`@Param` prose was updated (`ledger.go:463`/`:471`); the generated `components/ledger/api/docs.go` copies were left untouched per the F5 file-ownership boundary. The `GET /v1/holders/:id/accounts` `@Param ledger_id` is flagged for the F1-T13 owner to add — surfaced, not silently patched.

---

## Gate-closure summary (F1.md §3, all nine gates)

| Gate | Closed by | Where the proof lives |
|------|-----------|-----------------------|
| **1. Migration applies clean + reversible** | F1-T02 (files + `make migrate-lint` + CONCURRENTLY mirror of `000016`), F1-T03 (column consumed) | `migrations/onboarding/000017*`/`000018*`; `account.postgresql.go` column list + INSERT; boot applier in the integration suite. |
| **2. `RequireHolder=false` behavioral equivalence** | F1-T07 (default false, struct stays comparable — L1), F1-T08 (materialisation + `@external` null), F1-T16 (transaction-path no-diff proof) | The empty `git diff 46706c9ea..324c10ee4` over the two transaction handlers, recorded above. |
| **3. `RequireHolder=true` enforcement** | F1-T05 (gate + sentinel), F1-T07 (flag), F1-T08 (cached read via the L3 settings-reader seam), F1-T15 (wiring) | Gate unit test (`errors.As` CRM-0006/Holder); the wired `SettingsReader`/`HolderReader` in `6d71b44d8`. |
| **4. Backfill idempotency proof** | F1-T06 (`CreateHolderWithID` idempotent on `_id` dup), F1-T09 (eager provision), F1-T10 (idempotent runner) | The new `//go:build integration` tests that ran green inside the 983: provision-twice → no second document; run-backfill-twice → identical PG/Mongo counts; `@external` retains `holder_id IS NULL`; self-holder count == org count. |
| **5. MT isolation proof** | F1-T10 (per-tenant runner + cross-tenant isolation) | MT integration test inside the 983: backfill on tenant A leaves tenant B untouched. |
| **6. JSONShape lock updated** | F1-T11 (field count 15→16, `holderId` in the key slice, no emit-signature change, no e2e mirror) | `pkg/streaming/events/account_created_test.go` `assert.Lenf(..., 16, ...)`; emit method confirmed `ToEmitRequest(tenantID, ts)`. |
| **7. D-12 immutability** | F1-T14 (greps + negative assertion), F1-T01 (`HolderID` absent from `UpdateAccountInput`) | `grep -n "HolderID" update_account.go pkg/mmodel/account.go` shows it absent from `UpdateAccountInput`, `mergePatchAccount`, and the update path; `grep "holder_id" account.postgresql.go | grep "Set("` returns zero (no UPDATE Set). |
| **8. Ownership read + namespace decided** | F1-T12 (`holder_id = ?` filter + `idx_account_holder`), F1-T13 (route under `plugin-crm`), F1-T03 (column round-trip) | `grep "holders/:id/accounts" routes.go` under `ApplicationName = "plugin-crm"`; the account-list filter clause; round-trip integration test. |
| **9. Entity/error constants added** | F1-T04 (`EntityHolder` added, `ErrHolderNotFound` CRM-0006 **reused**), F1-T05 (mapped via `ValidateBusinessError`, not `reflect.TypeOf`) | `entity.go:16` `EntityHolder`; `errors.go:226` reused sentinel; the gate's `errors.As` unit test that asserts Code==CRM-0006 / EntityType==Holder. |

No gate is left without a closing task and a located proof.

---

## Process lessons (binding for later phases)

1. **Commit gates must vet WITH build tags.** The untagged-only commit gates missed the 3 tagged call sites in `crm_collapse_integration_test.go` (old `RegisterCRMRoutesToApp` arity) — they live behind `//go:build integration` and never compiled in an untagged `go build`/`go vet`. The baseline caught it; the fix is `4879b9853`. Any gate that claims "compiles clean" must run `go vet -tags=integration` (and any other live tags) before the claim is load-bearing. This is the same family as the F0 "verify discovery by execution" lesson: untagged checks prove nothing about tagged code.

2. **The redis property-test flake was latent repo debt, not an F1 regression.** The property generators fed `':'`-containing tenant IDs into `GetKey`, which rejects the namespace delimiter by contract — a pre-existing generator bug that only surfaced under the F1 baseline's property leg. Fixed by excluding the delimiter from the generated tenant IDs (`sanitizeQuickTenantID*`, 6 sites, both suites) at `324c10ee4`, then stressed 25× post-fix. Recorded so a later phase does not misattribute it to identity work.

3. **Stray `bin/migration-lint` artifact (gitignore gap).** `make migrate-lint` (F1-T02) builds a `bin/migration-lint` binary; `.gitignore` covers `.bin/` but not `bin/`. The artifact is not committed here but the gap should be closed in F5 alongside the other harness debt.

4. **Down-migration linter blind spot (flagged, out of F1 scope).** `scripts/migration_linter` globs only `*.up.sql` (`main.go:200`), so the `DROP COLUMN IF EXISTS` in `000017`'s down-file — which would trip the ERROR-severity DROP COLUMN rule absent the `IF EXISTS` exclusion — is never linted. Down-migration hazards are structurally invisible to `migrate-lint`. F5 harness debt.

---

## Drift from `F1.md` recorded during execution

All confirmed against HEAD; none changed F1 semantics.

- **Gate 9 sentinel reuse (F1.md §6.1):** `ErrHolderNotFound` (CRM-0006) already existed at `errors.go:226`. F1-T04 reuses it; minting a duplicate would violate CLAUDE.md. Only `EntityHolder` is net-new.
- **`EntityHolder` absent at HEAD (§6.2):** added, placed alphabetically between `EntityBalance` and `EntityLedger` (`entity.go:16`).
- **Migration numbers (§6.4):** `000016` was latest; `000017`/`000018` are the next free, auto-numbered by `make migrate-create -seq`.
- **Emit signature (§6.5):** confirmed `ToEmitRequest(tenantID, ts)` (not `ToEvent`); single-arg `EmitImportant` closure; no concrete "e2e mirror struct" exists despite the test comment at `account_created_test.go:159-162` (left untouched).
- **`Organization.ID` is a `string`, not `uuid.UUID`:** `provisionSelfHolder` parses it before `DeriveSelfHolderID`, and takes the full `*mmodel.Organization` (not just `orgID`) because `CreateHolderInput.Name`/`Document` are `required` and derive from `org.LegalName`/`org.LegalDocument`. Flavor deviation from the F1.md helper signature, not a semantic one.
- **L3 settings seam wired as specified:** `SettingsReader` and `HolderProvisioner` are satisfied by direct assignment (`queryUseCase.GetParsedLedgerSettings` and `crmservices.UseCase.CreateHolderWithID` match the command ports exactly); only `HolderReader` and `HolderAccountsReader` needed real adapter types. The adapters live in a dedicated `holder_wiring.go`, not inlined in `config.go`.

---

## Pre-existing failure isolated (not F1, not a baseline regression)

`FuzzBuildTransactionPostgresConnection_ConfigValues` fails on several seeds (`config.postgres.transaction_fuzz_test.go:110`) — it fuzzes `buildTransactionPostgresConnection` with fuzzed PG config and is unrelated to any F1 task. It belongs to the F0 baseline surface; the serial baseline chain owns it. Recorded here so the F1 baseline read is not muddied.
