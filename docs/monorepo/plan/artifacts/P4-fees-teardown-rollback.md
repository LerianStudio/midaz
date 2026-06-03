# P4 — plugin-fees Teardown Rollback Runway (T26)

**Status:** in-repo embedding COMPLETE and balance-proven; standalone-service teardown DEFERRED
(gated on P7-T18 unified third-rail re-proof + out-of-repo ops lockstep, per the cross-phase teardown gate).

This document is the recovery path required by P4-T26 before any irreversible teardown (P4-T19b / P4-T21)
is executed. It mirrors the P5-T16 abort discipline: keep the standalone fees service recoverable until the
unified balance proof is green and stable in staging.

## What landed in midaz (reversible via git)

The plugin-fees engine was imported (PD-3 fresh import from `plugins-fees@b88de8d`,
branch `chore/p2a-libcommons-v5.4.1`) and embedded into `components/ledger`. NO standalone fees
binary/bootstrap/Dockerfile/compose was imported — only the engine, service, persistence, and HTTP
handlers. The standalone fees **deploy unit still lives in the external `plugin-fees` repo** and is
**untouched** by this work.

P4 commits on branch `phase/p4-fees-collapse` (revert in reverse order to undo the embedding):

| Commit | Scope |
| --- | --- |
| `9acb07de2` | mechanical move of the engine into `components/ledger` |
| `7992e94de` | repoint fee reads to in-process `query.UseCase`, delete HTTP client |
| `80a1aeaaa` | wire fee Mongo persistence + `fees.UseCase` + `initFees` |
| `519c4a400` | mount fee/billing CRUD routes + swagger fold |
| `bafa5fd73` | lock balancing invariant, delete precision table, Send.Asset denomination |
| `af30daf19` | fee seam in `executeCreateTransaction` (single validate reassignment) |
| `061d4d84e` | third-rail integration proof + close fee-leg balance break |

Reverting `af30daf19` alone disables the fee seam (fees stop being applied) without removing the
engine — the smallest "stop charging fees in-process" lever if a staging regression is isolated to the seam.

## Rollback target: rebuild the standalone fees image from the archived origin

PD-3 archives the origin repos read-only. The standalone fees image is rebuildable from the archived
tag with NO live repo:

```
# from the archived read-only plugin-fees origin at b88de8d (branch chore/p2a-libcommons-v5.4.1)
git -C <archived-plugin-fees> checkout b88de8d
docker build -t plugin-fees:rollback-b88de8d <archived-plugin-fees>
# boot with the standalone env (MONGO_*, CLIENT_ID/SECRET, MIDAZ_*_URL, SERVER_PORT=4002, LICENSE_*)
# verify /readyz green
```

**Recoverability check (required before P4-T19b):** perform a build-only dry-run of the above `docker build`
to prove the archived tag still produces a bootable image. The full boot dry-run (health/readyz green)
requires the standalone service's complete env (Mongo + auth + license) and is an **ops-gated** step —
run it in the staging/ops environment, not the dev sandbox. Record the dry-run result here before teardown.

> Dry-run status: **NOT YET RUN** — ops-gated. Build-dry-run + boot must be recorded here before P4-T19b.

## Trigger conditions — HOLD teardown / re-deploy standalone

Do NOT execute P4-T19b (deploy-unit deletion) or P4-T21 (Helm/gitops/APIDog image removal) if any hold:

1. **P7-T18 (unified third-rail re-proof) is RED** after the moves land. The in-phase proof (`061d4d84e`,
   23/23 green) is necessary but P7-T18 is the unified backstop; a red P7-T18 means HOLD.
2. **Embedded fee seam regresses in staging** (fee not applied, double-charge, imbalance, or `0019`/`0073`
   on fee-bearing transactions).

On trigger: HOLD at the current state (engine embedded, standalone still deployed), re-deploy the
standalone `plugin-fees:rollback-b88de8d` image from the archived tag, route fee traffic back to it while
the embedded regression is fixed, and re-run the P4-T16 suite + P7-T18 before retrying teardown.

## Deferred items (require Fred + ops sign-off — NOT done autonomously)

- **P4-T19b** — delete the standalone fees deploy units (Dockerfile, compose, CI image job) in the
  external repo. Irreversible-ish (recoverable only via archived tag); gated on the dry-run above + P7-T18.
- **P4-T21** — remove the `plugin-fees` image from the `midaz` Helm chart, `midaz-firmino-gitops`, and
  APIDog, in lockstep with the deploy-unit deletion. Cross-team; name a deploy-window owner first.
  If the owner is unavailable or the chart change is rejected: do NOT merge — the standalone fees stays
  deployable and teardown degrades to "deferred", not "production broken" (CGap2 fallback).
- **Provisioning confirmation (P4-T22 footgun)** — confirm tenant-manager keys fee DBs on the
  `ModuleFees="plugin-fees"` name (same class as the CRM `crm→crm-api` footgun) before relying on the
  embedded MT fee path in production.

## Note on billing presentation rounding (from P4-T11)

Removing the hard-coded precision table (no-shims mandate) also removed billing-calculate's rounding;
invoice amounts are now full-precision. This is a **presentation** change, not a balance change — if
display rounding is desired it belongs at a presentation layer, not a fees-local table. Flagged for product review.
