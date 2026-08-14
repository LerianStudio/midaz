# Streaming Topic ACL-Prefix Convergence (midaz v3) — Implementation Plan

> **For implementers:** Use ring-default:executing-plans, ring-default:dispatching-workflows,
> or ring-dev-team:running-dev-cycle. This document is the living source of truth.
> Baseline verified against `origin/main` @ `01bfd13c9` — streaming code byte-identical to exploration; all line refs exact.

**Goal:** Rename every Kafka wire topic from `lerian.streaming.<service>_<resource>.<event>` to `{service}.{resource}.{event}`, make underscore the canonical event identity, and align `ce-source` to bare `ledger`/`crm`. No dual-publish (renamed topic only). No go.mod major bump.

**Target topics after this change:** `ledger.account.created`, `ledger.operation_route.created`, `ledger.balance.config_changed`, `ledger.balance.overdraft_drawn`, `crm.holder.created`, `crm.alias.related_party_deleted`, etc.

## Card #3786 coverage

- **6.1** underscore definitions → Change Set A
- **6.2** `RouteKey()` → Change Set B
- **6.3** `TopicName` no prefix/fold → Change Set C
- **6.4** ce-source → Change Set E
- **6.5** dual-publish → **OUT OF SCOPE** (no consumers; single-topic rename — see card comment #1979)
- **6.6** tests + docs → Change Sets D, F
- **6.7** no go.mod major → Change Set G

## Phase Overview

| Phase | Milestone | Change Sets | Status |
|-------|-----------|-------------|--------|
| 1 | Both components build + emit `{service}.{resource}.{event}`; all tests green | A, B, C, D | Detailed |
| 2 | ce-source aligned; docs synced; go.mod verified; lint/sec pass | E, F, G | Detailed |

---

# Phase 1 — Identity + topic rename (detailed)

## Change Set A — Flip the 11 hyphenated Definitions to underscore (card 6.1)

Change only the string literal (`-` → `_`) in each `Definition` var, and update the doc comment above it (each currently explains the hyphen/regex — that rationale is inverted now).

- **`pkg/streaming/events/operation_route_created.go:29`** — `ResourceType: "operation-route"` → `"operation_route"` (+ fix comment at lines 24-27)
- **`pkg/streaming/events/operation_route_updated.go`** — `ResourceType: "operation-route"` → `"operation_route"`
- **`pkg/streaming/events/operation_route_deleted.go`** — `ResourceType: "operation-route"` → `"operation_route"`
- **`pkg/streaming/events/transaction_route_created.go`** — `ResourceType: "transaction-route"` → `"transaction_route"`
- **`pkg/streaming/events/transaction_route_updated.go`** — `ResourceType: "transaction-route"` → `"transaction_route"`
- **`pkg/streaming/events/transaction_route_deleted.go`** — `ResourceType: "transaction-route"` → `"transaction_route"`
- **`pkg/streaming/events/balance_config_changed.go`** — `EventType: "config-changed"` → `"config_changed"`
- **`pkg/streaming/events/balance_overdraft.go:51`** — `EventType: "overdraft-drawn"` → `"overdraft_drawn"` (+ fix comment lines 45-48)
- **`pkg/streaming/events/balance_overdraft.go:69`** — `EventType: "overdraft-repaid"` → `"overdraft_repaid"` (+ fix comment lines 64-66)
- **`pkg/streaming/events/balance_overdraft.go:86`** — `EventType: "overdraft-cleared"` → `"overdraft_cleared"` (+ fix comment lines 82-83)
- **`pkg/streaming/events/alias_related_party_deleted.go`** — `EventType: "related-party-deleted"` → `"related_party_deleted"`

**Effect:** `Definition.Key()` and the CloudEvents `ce-type` for these 11 events change to the underscore form (a wire-contract change — safe, no consumers).

## Change Set B — Add `RouteKey()` and use it in both `buildRoutes` (card 6.2)

The route Key must stay hyphenated (lib-streaming's `^[a-z0-9][a-z0-9-]*(\.[…])+$` rejects `_`). So we fold underscore→hyphen for the route Key only, while `DefinitionKey`/catalog/topic use the underscore `Key()`.

- **`pkg/streaming/events/events.go`** — add method after `Key()` (line 51):
  - `func (d Definition) RouteKey() string { return strings.ReplaceAll(d.Key(), "_", "-") }`
  - add `import "strings"` (file currently has no imports)
- **`components/ledger/internal/bootstrap/streaming.go:242-250`** — inside the loop:
  - keep `key := d.Key()` (underscore) for `DefinitionKey` and `Destination`
  - add `routeKey := d.RouteKey()` (hyphen)
  - change `Key: key + "." + targetName` → `Key: routeKey + "." + targetName`
  - update the `buildRoutes` doc comment (lines 226-237) — remove the "hyphens→underscores in topic" + "shadow route" wording
- **`components/crm/internal/bootstrap/streaming.go:220-228`** — identical change (+ comment lines 205-215)

**Result per route:** `Key = operation-route.created.primary` (hyphen), `DefinitionKey = operation_route.created` (underscore), `Destination = ledger.operation_route.created`.

## Change Set C — Rewrite `TopicName`, delete `TopicPrefix` + fold (card 6.3)

- **`pkg/streaming/topic.go:24-26`** — body becomes:
  - `return service + "." + key`  (was `TopicPrefix + service + "_" + strings.ReplaceAll(key, "-", "_")`)
- **`pkg/streaming/topic.go:10`** — delete `const TopicPrefix = "lerian.streaming."`
- **`pkg/streaming/topic.go:7`** — delete `import "strings"` (no longer used)
- **`pkg/streaming/topic.go:12-23`** — rewrite doc comment to the new `{service}.{resource}.{event}` grammar
- Grep tree for any remaining `TopicPrefix` referent before deleting (only `topic_test.go` + `crm streaming_test.go:98-99` are expected — both fixed in Change Set D)

## Change Set D — Update all tests to the new format (card 6.6)

- **`pkg/streaming/topic_test.go:18-74`** — rewrite `TestTopicName` expectations to new topics; **delete** `TestTopicPrefix` (const gone)
- **9 event `_test.go` files** (matching Change Set A) — update Definition-key lock tests to expect underscore `Key()`/`ResourceType`/`EventType`
- **`components/ledger/internal/bootstrap/streaming_test.go:81-94`** — topic literals → new form (`ledger.balance.changed`, `ledger.operation_route.created`, `ledger.balance.config_changed`, `ledger.balance.overdraft_drawn`)
- **`components/crm/internal/bootstrap/streaming_test.go:98-99,163-165,188`** — replace `HasPrefix(TopicPrefix)` → `HasPrefix("crm.")`; literal → `crm.alias.related_party_deleted`
- **`components/crm/internal/bootstrap/streaming_integration_test.go:192-256`** — recompute expected topics (derived via `TopicName`; confirm assertions hold)
- Grep all `*_test.go` for `AssertEventEmitted(...)` / `MockEmitter` calls passing the old hyphen strings for the 11 events → switch to underscore

**Phase 1 done when:** `go build ./...` passes; both emitters `Build()` without route-key rejection; `go test ./pkg/streaming/... ./components/ledger/internal/bootstrap/... ./components/crm/internal/bootstrap/...` green.

---

# Phase 2 — ce-source, docs, gate (epic-level)

## Change Set E — Align ce-source to bare `ledger` / `crm` (card 6.4)

- **`components/ledger/.env.example:356`** — `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger` → `=ledger`
- **`components/crm/.env.example:233`** — `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.crm` → `=crm`
- **`components/ledger/.env:275`** — live file (user-owned): flag that the operator must set `=ledger` on deploy; do not silently rewrite
- Grep for any test/log asserting `lerian.midaz.*` or `ce_source` and update
- No config-struct change — value flows through `libStreaming.LoadConfig()` (ledger `streaming.go:74`, crm `:87`)

> **Verified (2026-08-14, this branch):** No test *asserts* on the ce-source value; every `lerian.midaz.*`
> in `*_test.go` is a `t.Setenv` *seed* fed to `LoadConfig()` (which requires a non-empty, valid source).
> No test couples ce-source to the topic — topics are derived via `pkgStreaming.TopicName(streamingServiceName, key)`
> (ledger `streaming.go:247`, crm `streaming.go:226`), never from `CloudEventsSource`. So changing ce-source
> cannot shift any published topic or break any assertion. `go.mod` currently shows **zero** diff vs `main`.

#### Task 2.1.1: Flip ce-source default to bare service name in both .env.example
- [ ] Done
- **Context:** `STREAMING_CLOUDEVENTS_SOURCE` is a commented example in both env files. Current (verified this branch):
  - `components/ledger/.env.example:356` → `# STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger`
  - `components/crm/.env.example:233` → `# STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.crm`
  The value is read at bootstrap by `libStreaming.LoadConfig()` (ledger `components/ledger/internal/bootstrap/streaming.go:74`, crm `components/crm/internal/bootstrap/streaming.go:87`) and applied via `.Source(streamingCfg.CloudEventsSource)` on the Builder (ledger `:109`, crm `:121`). Bare `ledger`/`crm` passes the consumer/route charset (`[a-z0-9_]`, verified `streaming_test.go:173`). No config-struct change (`StreamingCloudEventsSource` field, `config.go:218`, keeps its env tag).
- **Implementation vision:** Change the commented value only — keep the line commented (default stays "operator supplies it"), keep the `#` prefix. `lerian.midaz.ledger` → `ledger`; `lerian.midaz.crm` → `crm`.
- **Files:**
  - `components/ledger/.env.example` (line 356)
  - `components/crm/.env.example` (line 233)
  - **NOTE — do NOT edit:** `components/ledger/.env:275` (live, user-owned, uncommented `=lerian.midaz.ledger`) and `components/tracer/.env:356` (tracer has no v3 streaming — out of scope). Leave both; call them out in the PR body so the operator sets `=ledger` on the live ledger deploy.
- **Verification:** `grep -n STREAMING_CLOUDEVENTS_SOURCE components/ledger/.env.example components/crm/.env.example`
- **Expected outcome:** ledger line shows `# STREAMING_CLOUDEVENTS_SOURCE=ledger`; crm line shows `# STREAMING_CLOUDEVENTS_SOURCE=crm`. `grep -rn "lerian.midaz" components/ledger/.env.example components/crm/.env.example` returns nothing.
- **Done when:** both `.env.example` files carry the bare-source value and no `lerian.midaz` residue; live `.env` and tracer `.env` untouched.

#### Task 2.1.2: Align ce-source seed values in streaming tests to the bare-source convention
- [ ] Done
- **Context:** The only remaining `lerian.midaz.*` references in code are test *seeds* (not assertions), verified this branch:
  - `components/crm/internal/bootstrap/streaming_test.go:65,260,284` — `t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")`
  - `components/crm/internal/bootstrap/streaming_integration_test.go:63` — `streamingITSource = "lerian.midaz.crm"` (const, consumed once at `:115` via `t.Setenv`)
  - `components/ledger/internal/bootstrap/streaming_test.go:201,225` — `t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")`
  These feed `LoadConfig()`'s required-source validation; no test reads back or asserts the value, and no emitted-event assertion checks ce-source (integration test asserts only ce-type/ce-subject/ce-tenantid + PII forbidden-keys, `streaming_integration_test.go:59-79`). This task exists purely to remove `lerian.midaz` residue so a reviewer does not flag the rename as incomplete — it is convention alignment, not behavior change.
- **Implementation vision:** Replace the seed strings with the bare-source form, preserving the `.test` suffix where present so intent (a test-scoped source) stays obvious: `lerian.midaz.crm.test` → `crm.test`; `lerian.midaz.ledger.test` → `ledger.test`; the integration const `lerian.midaz.crm` → `crm`. Do not add any new ce-source assertion (out of scope).
- **Files:**
  - `components/crm/internal/bootstrap/streaming_test.go` (lines 65, 260, 284)
  - `components/crm/internal/bootstrap/streaming_integration_test.go` (line 63)
  - `components/ledger/internal/bootstrap/streaming_test.go` (lines 201, 225)
- **Verification:** `grep -rn "lerian.midaz" components/ pkg/ && echo REMAIN || echo CLEAN` then `go test ./components/ledger/internal/bootstrap/... ./components/crm/internal/bootstrap/... -run 'Streaming|Emit|Build' -count=1`
- **Expected outcome:** grep prints `CLEAN` (zero `lerian.midaz` in `components/`+`pkg/`); the scoped unit tests pass.
- **Done when:** all seed values use the bare-source form, tests green, no `lerian.midaz` left in `components/` or `pkg/`.

## Change Set F — Sync wire-contract docs (card 6.6)

- **`docs/streaming/ledger-events.md`** — topic tables → `ledger.<res>.<evt>`; ce-source `ledger`; underscore ce-type for the 11 events
- **`docs/streaming/crm-events.md`** — topic tables → `crm.<res>.<evt>`; ce-source `crm`; fix stale `lib-streaming v1.4.0` (line 14) → `v1.9.0`

> **The 11 renamed events** (ce-type/event-key flip hyphen→underscore, per Phase 1 Change Set A):
> `operation_route.{created,updated,deleted}`, `transaction_route.{created,updated,deleted}`,
> `balance.config_changed`, `balance.{overdraft_drawn,overdraft_repaid,overdraft_cleared}` (ledger),
> `alias.related_party_deleted` (crm). For every OTHER event only the **Kafka topic** column changes format
> (`lerian.streaming.<svc>_<key>` → `<svc>.<key>`); its event key and ce-type were already underscore-free and stay put.

#### Task 2.2.1: Rewrite ledger-events.md to the new topic grammar, underscore ce-type, and bare ce-source
- [ ] Done
- **Context (all line refs verified this branch):**
  - `:14` — producer version already reads `v1.9.0` (correct, no change).
  - `:17` — `- **CloudEvents source (`ce-source`):** `lerian.midaz.ledger`.` → bare `ledger`.
  - `:41-45` — the format bullets: `ce-type` = `studio.lerian.<key>`; `Kafka topic` = `lerian.streaming.ledger_<key>` with the "hyphens→underscores in the topic name only … key and ce-type keep the hyphen" prose. This is now inverted — key/ce-type/topic are ALL underscore-canonical, and the topic is `ledger.<key>`.
  - `:57-93` — the 35-row event table. Kafka-topic column: rewrite EVERY row `lerian.streaming.ledger_<x>.<y>` → `ledger.<x>.<y>`. For the 11 renamed events (rows at `:77-82`, `:85`, `:87-89`) ALSO flip the Event-key column and ce-type column hyphen→underscore (e.g. `operation-route.created` → `operation_route.created`, `studio.lerian.operation-route.created` → `studio.lerian.operation_route.created`).
  - `:98-101` — the "**Hyphen, not underscore.**" callout is now false; replace with an "underscore-canonical" note explaining the route-Key is the ONLY hyphenated artifact (folded via `Definition.RouteKey()`), while event key / ce-type / topic use underscore.
  - `:331,347,350,353,355,433,471,487,528,543` and any other prose/heading using an 11-event key (`operation-route.*`, `transaction-route.*`, `balance.config-changed`, `balance.overdraft-*`) — flip to underscore to match `Definition.Key()`. Payload-field tables/counts are unchanged.
  - `:665` — `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger` → `=ledger`.
- **Implementation vision:** Mechanical find/replace scoped to this file: (1) `lerian.streaming.ledger_` → `ledger.` everywhere; (2) for the 11 renamed event keys, hyphen→underscore in key + ce-type + prose/headings; (3) ce-source line + `:665` env line to bare `ledger`; (4) rewrite the format bullets `:41-45` and the callout `:98-101`. Do NOT touch payload field counts or JSONShape references — Phase 1 already reconciled those.
- **Files:** `docs/streaming/ledger-events.md`
- **Verification:** `grep -n "lerian.streaming\|lerian.midaz\|operation-route\|transaction-route\|config-changed\|overdraft-drawn\|overdraft-repaid\|overdraft-cleared" docs/streaming/ledger-events.md`
- **Expected outcome:** command returns nothing (all old-topic-prefix, old-source, and hyphenated-11-event references gone). Spot-check: table row for account.created shows topic `ledger.account.created`; operation_route.created row shows key `operation_route.created`, ce-type `studio.lerian.operation_route.created`, topic `ledger.operation_route.created`.
- **Done when:** the doc describes `{service}.{resource}.{event}` single-topic publishing, underscore ce-type for the 11 events, bare `ledger` ce-source, and the verification grep is empty.

#### Task 2.2.2: Rewrite crm-events.md to the new topic grammar, underscore ce-type, bare ce-source, and fix the stale version
- [ ] Done
- **Context (all line refs verified this branch):**
  - `:14` — `lib-streaming` **v1.4.0** (STALE) → `v1.9.0` (matches `go.mod:105` and ledger-events.md `:14`).
  - `:17` — `- **CloudEvents source (`ce-source`):** `lerian.midaz.crm`.` → bare `crm`.
  - `:38-42` — format bullets: `ce-type` = `studio.lerian.<key>`; `Kafka topic` = `lerian.streaming.crm_<key>` with the "hyphens→underscores in the topic only … key and ce-type keep the hyphen" prose (example `lerian.streaming.crm_alias.related_party_deleted`). Rewrite to underscore-canonical + `crm.<key>` topic.
  - `:51-59` — 7-row event table. Kafka-topic column: `lerian.streaming.crm_<x>.<y>` → `crm.<x>.<y>` on all rows. Row `:59` (`alias.related-party-deleted`): ALSO flip event key → `alias.related_party_deleted`, ce-type → `studio.lerian.alias.related_party_deleted`, topic → `crm.alias.related_party_deleted`.
  - `:61-64` — the "**Hyphen, not underscore.**" callout is now false; replace with the underscore-canonical / route-Key-only-hyphenated note.
  - `:66` — `> **`ce-subject` on `alias.related-party-deleted`.**` heading and `:137` `### `alias.related-party-deleted` — 5 fields` and any other prose using `alias.related-party-deleted` → `alias.related_party_deleted`.
- **Implementation vision:** Same mechanical approach as 2.2.1, scoped to this file: version bump `:14`; `lerian.streaming.crm_` → `crm.`; the single renamed event `alias.related-party-deleted` → `alias.related_party_deleted` in key/ce-type/prose/headings; ce-source → `crm`; rewrite format bullets `:38-42` and callout `:61-64`. Payload field count (5) unchanged.
- **Files:** `docs/streaming/crm-events.md`
- **Verification:** `grep -n "v1.4.0\|lerian.streaming\|lerian.midaz\|related-party-deleted" docs/streaming/crm-events.md`
- **Expected outcome:** command returns nothing. Spot-check: `:14` reads v1.9.0; the alias row shows key `alias.related_party_deleted`, ce-type `studio.lerian.alias.related_party_deleted`, topic `crm.alias.related_party_deleted`; holder.created topic `crm.holder.created`.
- **Done when:** doc matches shipped wire contract, version is v1.9.0, ce-source is `crm`, and the verification grep is empty.

#### Task 2.2.3: DECISION-REQUIRED — reconcile the root CLAUDE.md Streaming section (do NOT auto-edit)
- [ ] Done
- **Context:** The project root `CLAUDE.md` "Streaming (lib-streaming events)" section still documents the PRE-Phase-1 grammar and now contradicts shipped code:
  - `Topic: `lerian.streaming.<service>_<resource>.<event>`` and "hyphens in the resource/event become underscores in the topic name only" — now the topic is `<service>.<resource>.<event>` with underscore-canonical resource/event.
  - "The route key / DefinitionKey and ce-type stay hyphenated" — after Phase 1 only the **route Key** stays hyphenated (folded via `Definition.RouteKey()`); `DefinitionKey` and ce-type are underscore.
  - "Route/event-type keys use hyphens, never underscores … e.g. `alias.related-party-deleted`" — the *route Key* still uses hyphens, but the Definition key / ce-type / topic example should read `alias.related_party_deleted`.
  A Phase-1 reviewer flagged this drift. `CLAUDE.md` is project instructions (sensitive; changes alter agent behavior repo-wide), so this task is **decision + note only** — it MUST NOT edit `CLAUDE.md` without explicit user approval.
- **Implementation vision:** Present the user a diff proposal for the CLAUDE.md Streaming subsection (topic grammar line, the hyphen/underscore rule, and the route-key example) aligning it with shipped code and the two updated docs. On approval, apply exactly that diff and nothing else. If not approved, leave `CLAUDE.md` untouched and record the decision in the PR body.
- **Files:** `CLAUDE.md` (project root) — **edit only after explicit user approval.**
- **Verification (after approval only):** `grep -n "lerian.streaming\|stay hyphenated\|related-party-deleted" CLAUDE.md`
- **Expected outcome:** if approved, grep shows the topic-grammar and DefinitionKey/ce-type wording updated to underscore-canonical while the route-Key hyphen rule is preserved; if not approved, this task closes as "deferred by user, noted in PR."
- **Done when:** the user has explicitly decided (apply vs defer) and the outcome is recorded; no silent CLAUDE.md edit occurred.

## Change Set G — Release gate (card 6.7)

- **`go.mod`** — verify `git diff` shows no major/import-path change: lib-streaming stays `v1.9.0`, lib-commons stays `/v5 v5.10.0`, lib-observability stays `v1.1.0`
- Run `make test-unit`, `make test-integration` (streaming paths), `make lint`, `make sec`
- PR description: record the merge-timing coordination gate (LerianStudio/tenant-manager#452, LerianStudio/streaming-hub#37) — renaming into an ACL-matched namespace; safe now (no consumers) but note it

> **Verification epic — no source edits.** This entire change set only runs checks and records notes.
> Current versions confirmed this branch: `go.mod:105` lib-streaming `v1.9.0`, `go.mod:102` lib-commons `/v5 v5.10.0`,
> `go.mod:103` lib-observability `v1.1.0`; `git diff main -- go.mod` is already empty.

#### Task 2.3.1: Verify go.mod carries no major/import-path bump
- [ ] Done
- **Context:** The rename is a wire/doc/config change only; no dependency should move. Current pins (verified `go.mod`): lib-streaming `v1.9.0` (`:105`), lib-commons `/v5 v5.10.0` (`:102`), lib-observability `v1.1.0` (`:103`). Constraint carried from user: NO go.mod major bump.
- **Implementation vision:** Diff go.mod against the base branch and confirm the three Lerian libs are byte-identical; no `/v6`, `/v2`, or import-path change anywhere.
- **Files:** none edited — read-only check on `go.mod`.
- **Verification:** `git diff main -- go.mod go.sum` and `grep -n "lib-streaming\|lib-commons\|lib-observability" go.mod`
- **Expected outcome:** `git diff` prints nothing (or, if the branch later touches unrelated deps, shows NO change to the three lib lines and no major-version path change); grep shows `lib-streaming v1.9.0`, `lib-commons/v5 v5.10.0`, `lib-observability v1.1.0`.
- **Done when:** the three libs are confirmed unchanged at their current majors and no import path shifted.

#### Task 2.3.2: Run the full gate — unit, integration (streaming paths), lint, sec
- [ ] Done
- **Context:** Phase 1 landed the identity/topic rename; Phase 2 adds ce-source + docs + test-seed edits. This task is the release-gate sweep across both components. Streaming-touching test packages: `pkg/streaming/...`, `components/ledger/internal/bootstrap/...`, `components/crm/internal/bootstrap/...`, plus the ledger `internal/services/command` streaming tests. Accepted sub-floor from Phase 1: `pkg/streaming/events` sits at 82.1% package coverage (changed lines 100%); do NOT chase unrelated payload-file coverage here — that is scope creep.
- **Implementation vision:** Run the repo make targets from the repo root. If a target is slow, additionally run the scoped streaming test packages directly to prove the renamed paths are green. Capture pass/fail output for the PR body.
- **Files:** none edited — verification only.
- **Verification (run all; all must pass):**
  - `make test-unit`
  - `make test-integration` (streaming paths — Redpanda-backed `streaming_integration_test.go`)
  - `make lint`
  - `make sec`
  - Scoped backstop: `go test ./pkg/streaming/... ./components/ledger/internal/bootstrap/... ./components/crm/internal/bootstrap/... -count=1`
- **Expected outcome:** unit + integration suites green; lint reports no NEW issues on changed files; `make sec` clean; the scoped streaming packages pass. `pkg/streaming/events` coverage ≈82.1% is expected and accepted (note it, do not "fix" it).
- **Done when:** all four make targets pass (integration exercised on streaming paths) and the scoped streaming packages are green, with output captured for the PR.

#### Task 2.3.3: Record the merge-timing coordination gate in the PR description
- [ ] Done
- **Context:** This rename moves topics into an ACL-matched namespace (`{service}.{resource}.{event}`) that platform teams must have provisioned before/with the merge. Two external tracking items gate merge timing: `LerianStudio/tenant-manager#452` and `LerianStudio/streaming-hub#37`. Safe now because there are no live consumers (single-topic rename, no dual-publish — 6.5 out of scope), but the dependency must be visible to reviewers.
- **Implementation vision:** Add a "Coordination / merge timing" section to the PR body noting: (1) topics renamed into the ACL-matched namespace; (2) blocked-on / coordinate-with `tenant-manager#452` and `streaming-hub#37`; (3) no dual-publish, no consumers today, so no data-plane break; (4) operator action from Task 2.1.1 — set `STREAMING_CLOUDEVENTS_SOURCE=ledger` in the live ledger `.env` on deploy. Use the Midaz PR template (per user CLAUDE.md), base branch to be confirmed with the user.
- **Files:** PR description (written to a local file per user convention, e.g. under the project root; not committed).
- **Verification:** manual — confirm the PR body contains the two issue references and the operator `.env` note.
- **Expected outcome:** PR description includes the coordination gate section referencing both external issues and the live-`.env` operator step.
- **Done when:** the coordination note is present in the PR body and the base branch has been confirmed with the user.

---

## Key facts (so nothing drifts)

- **Why RouteKey() exists:** lib-streaming validates only `RouteDefinition.Key` against the no-underscore regex; `DefinitionKey`, catalog `EventDefinition`, and the topic have no such constraint. So underscore is canonical everywhere except the route Key, which folds to hyphen. (Same shape already live on `origin/develop`.)
- **Two v3 binaries, two services:** `streamingServiceName = "ledger"` (`ledger streaming.go:27`), `= "crm"` (`crm streaming.go:26`). No fee/tracer streaming on v3 — the v4 multi-prefix collapse does NOT apply.
- **Keep `alias`:** develop renamed it to `instrument`; v3 keeps `alias` (`crm.alias.related_party_deleted`).
- **Reference:** `origin/develop` already has underscore `Definition`s + `RouteKey()` in the exact shape above (on lib-streaming/v2) — port the form, keep v3's v1.9.0 imports and `alias` naming.
