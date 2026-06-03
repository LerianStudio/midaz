# P5 Tracer Move — Abort / Rollback Runbook (P5-T16)

The tracer co-location is the highest-blast-radius P5 work: a scripted ~448-file
module-path rename + go.mod dep-fold + compose rewrite + CI fold. This runbook
defines the revert path **before** the move is declared done so failure is
recoverable, and states the hard ordering invariant that keeps a fallback alive.

- Phase: P5 (tracer co-location)
- Branch: `phase/p5-tracer-colocation`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Canonical-source invariant (the one rule that matters)

> **The origin `tracer` repo stays CANONICAL and WRITABLE until P5-T15 is green
> in-module AND the P5-T16 abort invariant is satisfied.**

Until then, the standalone tracer service keeps building and deploying from origin.
The monorepo copy is additive; it does not displace origin until proven. This is
why the destructive origin archival (P5-T13b) is gated STRICTLY AFTER P5-T15.

This invariant is enforced **structurally**, not by prose:
- `P5-T13b` depends on `P5-T15` AND `P5-T16` in the dependency graph.
- `P5-T13b` carries a HARD runbook precondition check: "P5-T15 green recorded"
  must be true before it starts.
- The in-repo release-artifact assertions (`P5-T13a`) are split OUT of `P5-T13b`
  so they can be a PREREQUISITE of `P5-T15` WITHOUT dragging origin archival
  before the gate. This split resolves the former T13→T15 prerequisite collision.

**Do NOT archive / set-read-only the origin tracer repo until P5-T15 is green.**
Archiving the source before the monorepo is proven removes the only fallback.

---

## The move commit range (bisectable)

The move was landed as SEPARATE commits for bisectability — import, rename, and
dep-fold are distinct so a `git bisect` can isolate which step broke something:

| Commit      | Subject                                                          |
|-------------|-----------------------------------------------------------------|
| `257eefc97` | docs(p4): teardown rollback... — **PRE-MOVE parent (clean tree)** |
| `4f8c1ebef` | feat(p5): import tracer (component co-location)                 |
| `521a9ca76` | refactor(p5): rename tracer module path to components/tracer    |
| `69e110d54` | build(p5): fold tracer deps into root go.mod + compile          |
| `<this>`    | feat(p5): integrate tracer component infra (Dockerfile/compose/Makefile/CI) |

The integration commit(s) in this task sit on top of `69e110d54` and are part of
the revert range (they only touch tracer infra + additive CI + docs — reverting
them is safe and leaves a compiling pre-integration tree).

---

## Abort command

Revert the entire move range on the midaz branch (origin tracer repo untouched —
it is still canonical and writable, so nothing is lost):

```bash
# Revert the full move range (import .. latest integration commit), newest-first.
# `^..` includes the import commit itself.
git revert --no-edit 4f8c1ebef^..HEAD

# OR, equivalently, revert from the pre-move parent forward:
git revert --no-edit 257eefc97..HEAD
```

`git revert` (not `reset`) is used deliberately: the branch may have been pushed
and the move commits may be referenced; reverting preserves history and is safe
on a shared branch. The range is newest-commit-first internally; `git revert`
handles the ordering.

### Partial abort (rare)
If only the integration layer is broken but import+rename+dep-fold are sound,
revert just the integration commit(s) on top of `69e110d54`:

```bash
git revert --no-edit <integration-commit-sha>
```

This drops the Dockerfile/compose/Makefile/CI changes and leaves tracer co-located
and compiling but un-integrated (back to the post-`69e110d54` state).

---

## Trigger conditions (when to abort)

Abort the move (full range) if ANY of these hold and cannot be fixed forward
within the phase budget:

1. **Compile / dep break that resists MVS resolution** — the dep-fold moves a
   shared consumer (ledger/crm) off its P1 lib-commons line and the cross-component
   regression suite cannot be brought green.
2. **In-module gate (P5-T15) cannot go green** — lint/unit/integration fail in a
   way rooted in the move (path rename, go.mod merge), not a pre-existing flake.
3. **Migration integrity regression** — tracer's audit-hash-chain migrations
   (`000001`/`000002`) or `prevent_truncate` (`000003`) fail to apply byte-identically
   on the shared `postgres:17`, or behave incorrectly under logical replication.
4. **Shared-infra collision** — wiring tracer into the shared compose breaks an
   existing service (ledger/crm/otel-lgtm/postgres) in a way that cannot be isolated.

Abort is REVERSIBLE here precisely because the origin repo is still canonical.
Once P5-T13b runs (origin archived), abort is no longer cheap — which is exactly
why P5-T13b is gated after P5-T15.

---

## Dry-run verification

The abort path is dry-run-verifiable on a throwaway branch BEFORE relying on it:

```bash
git switch -c throwaway/p5-abort-dryrun
git revert --no-edit 257eefc97..HEAD
go build ./...            # must compile — restores a pre-move-equivalent tree
git switch - && git branch -D throwaway/p5-abort-dryrun
```

A green `go build ./...` on the reverted throwaway branch confirms the revert
restores a compiling pre-move midaz tree.

---

## Origin-archival unlock precondition (cross-reference)

P5-T13b (origin archive + disable all 7 origin workflows) MUST NOT run until this
runbook records **"P5-T15 GREEN"** with a timestamp. That record is the explicit
unlock precondition. Until it exists, the origin tracer repo stays the canonical,
deployable source and this abort path stays cheap.
