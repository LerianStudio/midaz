# P6 Reporter Move — Abort / Rollback Runbook (P6-T-rollback)

The reporter co-location is a high-blast-radius move: a scripted multi-file
module-path rename across five prefixes, a go.mod dep-fold (incl. mongo-driver
collapse), an Option C pkg/tests extraction, two Dockerfile rewrites, compose
re-topology, Makefile/CI fold. This runbook defines the revert path **before** the
move is declared done so failure is recoverable, and states the hard ordering
invariant that keeps a fallback alive.

- Phase: P6 (reporter two-component co-location, Option C)
- Branch: `phase/p6-reporter-colocation`
- Date recorded: 2026-06-03
- Sign-off: Fred Amaral (owner)

---

## Canonical-source invariant (the one rule that matters)

> **The origin `reporter` repo stays CANONICAL and WRITABLE — at
> `5b2d7715` (`refactor(deps): reporter to lib-commons v5.4.1 + mongo-driver v2
> [P2b part 2]`) — until the P6 in-module gate is green AND the deploy lockstep
> (P6-T19) is coherent.**

Until then, the standalone reporter manager/worker keep building and deploying from
origin. The monorepo copy is additive; it does not displace origin until proven.
This is why destructive origin archival (P6-T20) is gated STRICTLY AFTER T17
(e2e verified) AND T19 (deploy lockstep set or fallback-resolved).

This invariant is enforced **structurally**, not by prose:
- `P6-T20` depends on `P6-T17` AND `P6-T19` in the dependency graph.
- T13's `app_name_prefix`/helm-key flip is HELD OFF the default branch until T19's
  lockstep is coherent (T19 owner-unavailable fallback). The images may build under
  the renamed prefix behind a preview tag, never promoted to the gitops-tracked tag.

**Do NOT archive / set-read-only the origin reporter repo until T17 is verified
AND T19 lockstep is coherent.** Archiving the source before the monorepo is proven
and deployable removes the only fallback.

---

## The move commit range (bisectable)

The move was landed as SEPARATE commits for bisectability — import, rename,
dep-fold, pkg/tests extraction, and infra integration are distinct so a `git
bisect` can isolate which step broke something:

| Commit      | Subject                                                              |
|-------------|----------------------------------------------------------------------|
| `eb749be1b` | docs(p5): record tracer CI decisions — **PRE-MOVE parent (clean tree)** |
| `70fa6a71b` | feat(p6): import reporter (two-component co-location)                 |
| `d0d8d5160` | refactor(p6): rewrite reporter module paths (5 prefixes)             |
| `daaee6bf2` | build(p6): fold reporter-unique deps into root go.mod (T04)          |
| `b4dc33bbb` | build(p6): fold reporter deps + build both binaries (T06)            |
| `2d146db7f` | fix(p6): rewrite reporter non-Go module-path refs (T08)              |
| `46d4e4c93` | refactor(p6): move reporter shared pkg/tests → pkg/reporter + tests/reporter (Option C) |
| `db0027760` | feat(p6): repoint reporter Dockerfiles to monorepo-root context (T09/T10) |
| `028e79640` | feat(p6): fold reporter compose/Makefiles/env + set-env (T11/T12/T21) |
| `38664ff34` | ci(p6): fold reporter components into midaz CI filter_paths (T13)    |
| `<this>`    | docs(p6): T14 artifact assertion + STRUCTURE/AGENTS + decisions/rollback (T14/T18) |

The integration + docs commits sit on top of the move and are part of the revert
range (they touch reporter infra + additive CI + docs only — reverting them is safe
and leaves a compiling pre-integration tree).

---

## Abort command

Revert the entire move range on the midaz branch (origin reporter repo untouched —
it is still canonical and writable at `5b2d7715`, so nothing is lost):

```bash
# Revert the full move range (import .. latest commit), newest-first.
# `^..` includes the import commit itself.
git revert --no-edit 70fa6a71b^..HEAD

# OR, equivalently, revert from the pre-move parent forward:
git revert --no-edit eb749be1b..HEAD
```

`git revert` (not `reset`) is used deliberately: the branch may have been pushed
and the move commits may be referenced; reverting preserves history and is safe on
a shared branch.

### Partial abort (rare)
If only the infra/CI integration layer is broken but import+rename+dep-fold+Option-C
are sound, revert just the integration commits on top of `46d4e4c93`:

```bash
git revert --no-edit 46d4e4c93..HEAD
```

This drops the Dockerfile/compose/Makefile/CI/docs changes and leaves reporter
co-located and compiling but un-integrated.

---

## Trigger conditions (when to abort)

Abort the move (full range) if ANY of these hold and cannot be fixed forward within
the phase budget:

1. **Compile / dep break that resists MVS resolution** — the dep-fold (incl. the
   mongo-driver v2 → v1.17.9 collapse) moves a shared consumer (ledger/crm/tracer)
   off its line and the cross-component regression cannot be brought green.
2. **In-module gate cannot go green** — `go build ./...` or reporter unit tests
   fail in a way rooted in the move (path rename, go.mod merge, Option C pkg/tests
   placement), not a pre-existing flake.
3. **Image regression** — a reporter image fails to build from repo-root context,
   the worker loses functional Chromium (R20), or the manager's distroless image
   cannot serve `/health` via the orchestrator probe.
4. **Shared-infra collision** — wiring reporter onto `infra-network` breaks an
   existing service (ledger/crm/tracer/otel-lgtm/postgres/mongo/valkey/rabbitmq) or
   a 4005/4006 port collides with the live component set.
5. **Fetcher reachability regression (R21)** — the fetcher can no longer reach
   external customer DBs over the shared `infra-network` with unchanged credentials.

Abort is REVERSIBLE here precisely because the origin repo is still canonical.
Once T20 runs (origin archived), abort is no longer cheap — which is exactly why
T20 is gated after T17 + T19.

---

## Dry-run verification

The abort path is dry-run-verifiable on a throwaway branch BEFORE relying on it:

```bash
git switch -c throwaway/p6-abort-dryrun
git revert --no-edit eb749be1b..HEAD
go build ./...            # must compile — restores a pre-move-equivalent tree
git switch - && git branch -D throwaway/p6-abort-dryrun
```

A green `go build ./...` on the reverted throwaway branch confirms the revert
restores a compiling pre-move midaz tree.

---

## Origin-archival unlock precondition (cross-reference)

P6-T20 (archive origin reporter repo read-only) MUST NOT run until:
- T17 (PDF-pipeline e2e) is verified in the P8/integration env, AND
- T19 (Helm/gitops/APIDog lockstep) is coherent OR its owner-unavailable fallback
  is recorded with the prefix flip held off the default branch.

Until both hold, the origin reporter repo at `5b2d7715` stays the canonical,
deployable source and this abort path stays cheap.
