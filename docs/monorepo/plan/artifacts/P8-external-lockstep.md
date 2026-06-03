# P8-T20 + P8-T22 — External-repo lockstep + owner-unavailability fallback (deferred ops)

**Status:** DOCUMENTED / NOT EXECUTED. These are out-of-repo coordination items. This doc consolidates the lockstep edits (T20) and arms the owner-fallback (T22). No external change is performed from this repo.

**Date:** 2026-06-03

---

## Why this is deferred and not executed here

The image rename to the `midaz-*` prefix and the deletion of the `midaz-crm` / `plugin-fees` images live entirely inside `.github/workflows/build.yml` (already consolidated: 4-image fan-out, key-maps carry only ledger/tracer/manager/worker). But the artifacts ArgoCD actually syncs live in THREE external repos NOT in this monorepo:

1. the external Helm `midaz` chart,
2. `LerianStudio/midaz-firmino-gitops`,
3. the APIDog e2e project.

If `build.yml`'s rename/delete fans out a production tag BEFORE those three update in lockstep, the result is the R12 failure mode: **builds four images, deploys none** (ArgoCD references chart keys that no longer exist, or chart keys reference images that no longer build). That is why the production fan-out tag is GATED (see T22 below), and why the coordinated external edits are owned by a human with repo access, not auto-applied from here.

The in-repo coordination already happened per phase (P7-T19). This doc is the cross-phase consolidation of every remaining out-of-repo item plus the explicit fallback.

---

## T20 — The coordinated external edits (perform once owners confirm)

The single source of truth for the key schemas is `build.yml`:
- `helm_values_key_mappings: {"midaz-ledger":"ledger","midaz-tracer":"tracer","midaz-reporter-manager":"manager","midaz-reporter-worker":"worker"}` — **bare** value keys (Helm chart schema).
- `yaml_key_mappings: {"midaz-ledger.tag":".ledger.image.tag","midaz-tracer.tag":".tracer.image.tag","midaz-reporter-manager.tag":".manager.image.tag","midaz-reporter-worker.tag":".worker.image.tag"}` — **`.tag`-suffixed** keys mapping to dotted value paths (gitops schema). The two schemas are deliberately different; do not cross them.

### (1) External Helm `midaz` chart
- ADD value blocks: `tracer`, `manager` (= reporter-manager), `worker` (= reporter-worker), each with an `image` stanza matching the bare-key schema (`ledger`/`tracer`/`manager`/`worker`).
- REMOVE the `crm` and `plugin-fees` image value blocks/stanzas.
- Acceptance: chart has value keys for ledger/tracer/manager/worker and NO crm/plugin-fees image keys; matches `helm_values_key_mappings`.

### (2) `LerianStudio/midaz-firmino-gitops`
- UPDATE the `yaml_key_mappings` TARGETS — the dotted value paths `.ledger.image.tag`, `.tracer.image.tag`, `.manager.image.tag`, `.worker.image.tag`.
- The KEY side stays `midaz-<x>.tag` (the `.tag`-suffixed schema — NOT the helm bare-key schema).
- REMOVE crm/fees keys.
- Acceptance: a real fan-out tag triggers the `update_gitops` job + ArgoCD sync with no orphaned/missing-key errors.

### (3) APIDog e2e
- EXTEND scenarios to cover the new API-bearing components (tracer, reporter-manager). reporter-worker has no public HTTP API surface (worker health only).
- DROP the crm-standalone scenarios (crm is now ledger-served holder/alias routes — fold any retained coverage into the ledger e2e suite).
- Acceptance: APIDog e2e job green for the new components.

### Sequencing constraint
All three MUST be confirmed/staged BEFORE the image-rename/delete production tag in P8-T18 lands. A co-located component without Helm/gitops entries builds but never deploys. T20's edits are the precondition-satisfying work; T22 is the precondition itself.

---

## T22 — Armed owner-unavailability fallback (precondition for T20)

The likeliest real-world stall is a chart/gitops/APIDog owner being unavailable or rejecting the change on the P8-T18 tag timeline. Without an armed fallback, a missing sign-off silently produces "builds four images, deploys none."

### Armed fallback: **(1) Gate the production rename tag** (preference-order #1).

- **Decision:** Do NOT cut the production fan-out tag (the tag that pushes `midaz-*` images to the ArgoCD-watched DockerHub+ghcr namespace) until ALL THREE external owners confirm their lockstep edits are merged/staged.
- For P8-T18 validation in the meantime, cut **throwaway / test-registry tags only** — exercise the 4-image fan-out, changed-detection, godog, and PD-2 invariant against a non-prod registry namespace ArgoCD does NOT watch. This keeps the renamed image names off the live chart until the chart is ready.
- **Rationale for choosing #1 over the alternatives:** the build.yml key-maps already drop crm/fees, so the rename+delete is a single coupled change. Gating the one production tag is the cleanest control point — it requires no intermediate chart state and no registry juggling. It is reversible (just delay the tag) and has no half-migrated window.

### Fallbacks held in reserve (if gating becomes infeasible)
- **(2) Stage the chart additions first** — land the additive `tracer`/`manager`/`worker` value blocks in the chart ahead of the tag (no removals), then remove crm/fees keys only after the first successful four-image sync. Use if a partial deploy is acceptable and the tag cannot be delayed.
- **(3) Temporary registry isolation** — push the renamed images to a non-prod registry namespace until the chart catches up, never to the ArgoCD-watched namespace. Use only if a tag must fan out for an unrelated reason before chart readiness.

### Ownership, deadline, and the hard gate
- **Owner:** assign a named human with write access to the external Helm chart, `midaz-firmino-gitops`, and APIDog (the deploy/release owner — NOT this agent; these repos are outside the monorepo and outside this task's execution authority).
- **Deadline:** confirmation from all three external owners is due BEFORE the P8-T18 production (non-throwaway) tag. The deadline is tied to the P8-T18 tag date, not a fixed calendar date — whichever comes first gates the other.
- **Hard gate (the invariant T22 guarantees):** there is NO path where the production rename/delete tag lands with an unconfirmed external owner. If any of the three owners has not confirmed, the production tag does not cut — teardown of the crm/fees image entries degrades to **"deferred,"** NOT **"production broken."** The standalone-deployable / current chart state is preserved until lockstep is real.

### Explicit gating statement
**The `build.yml` crm/fees image-drop + key-map changes are already merged in-repo, but the PRODUCTION fan-out tag that activates them is BLOCKED on external-owner sign-off across all three repos.** Cutting that tag early, without sign-off, is the one action this fallback forbids.

---

## Per-phase deploy-gated items this consolidates

Each phase deferred its own out-of-repo lockstep to this point. They roll up here:

| Phase task | Item |
|---|---|
| `P3-T18` (P3.md:498) | Remove CRM from CI build / gitops / Helm fan-out (lockstep with ops) — drop `midaz-crm` image + crm chart/gitops keys. |
| `P4-T21` (P4.md:788) | Out-of-repo lockstep: remove `plugin-fees` image from Helm / gitops / APIDog. |
| `P5-T14` (P5.md:234) | Out-of-repo deploy lockstep: add tracer to external Helm chart / gitops repo / APIDog. |
| `P6-T19` (P6.md:552) | Out-of-repo Helm / gitops / APIDog lockstep coordination — add reporter-manager (`manager`) + reporter-worker (`worker`). |

Net external delta = ADD {tracer, manager, worker} value/key blocks; DROP {crm, plugin-fees} image entries — applied in ONE lockstep window gated by T22.

---

## T21 — Tracer migration S3 decision (verified consistent)

Cross-checked for consistency across the two places it must agree:

- **P5 decision doc** (`docs/monorepo/plan/artifacts/P5-tracer-ci-decisions.md`, Decision 2): tracer is **EXCLUDED** from the S3 ops-migration distribution. Tracer's migrations are an ordered, hash-chained sequence (`000001`/`000002` install the audit-hash-chain PL/pgSQL functions, `000003` the `prevent_truncate` trigger); out-of-band hand-applied S3 distribution would break the ordered-apply / hash-chain invariant. Tracer's bootstrap migration runner (`libPostgres.NewMigrator` in `components/tracer/internal/bootstrap/config.go`) is the single applied-by path. The exclusion is explicit and reversible (a mirrored `s3-upload` job with `file_pattern: components/tracer/migrations/*.sql`, `s3_prefix: tracer/postgresql` can be added later with no change to the bootstrap path).
- **`build.yml`** (lines 77-82): the migration-S3 block comment states tracer is intentionally EXCLUDED per P5 Decision 2, only ledger ships S3-uploaded migrations (`upload-onboarding-migrations` + `upload-transaction-migrations`), and crm/fees collapsed into ledger emit no image/migrations. build.yml contains exactly the two ledger S3 jobs and NO tracer S3 job.

**Consistent.** tracer migrations are bootstrap-applied and excluded from S3 to preserve the audit hash-chain ordered-apply. No tracer S3-upload job exists in build.yml, matching the decision. P8-T07's migrations-present-in-image assertion is the relevant gate for tracer (image must ship its migrations for the bootstrap runner to apply them) — observed working: the local up confirmed tracer applies migrations from its image against the shared postgres `tracer` DB.
