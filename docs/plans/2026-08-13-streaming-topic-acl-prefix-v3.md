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
| 2 | ce-source aligned; docs synced; go.mod verified; lint/sec pass | E, F, G | Epic-level |

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

## Change Set F — Sync wire-contract docs (card 6.6)

- **`docs/streaming/ledger-events.md`** — topic tables → `ledger.<res>.<evt>`; ce-source `ledger`; underscore ce-type for the 11 events
- **`docs/streaming/crm-events.md`** — topic tables → `crm.<res>.<evt>`; ce-source `crm`; fix stale `lib-streaming v1.4.0` (line 14) → `v1.9.0`

## Change Set G — Release gate (card 6.7)

- **`go.mod`** — verify `git diff` shows no major/import-path change: lib-streaming stays `v1.9.0`, lib-commons stays `/v5 v5.10.0`, lib-observability stays `v1.1.0`
- Run `make test-unit`, `make test-integration` (streaming paths), `make lint`, `make sec`
- PR description: record the merge-timing coordination gate (LerianStudio/tenant-manager#452, LerianStudio/streaming-hub#37) — renaming into an ACL-matched namespace; safe now (no consumers) but note it

---

## Key facts (so nothing drifts)

- **Why RouteKey() exists:** lib-streaming validates only `RouteDefinition.Key` against the no-underscore regex; `DefinitionKey`, catalog `EventDefinition`, and the topic have no such constraint. So underscore is canonical everywhere except the route Key, which folds to hyphen. (Same shape already live on `origin/develop`.)
- **Two v3 binaries, two services:** `streamingServiceName = "ledger"` (`ledger streaming.go:27`), `= "crm"` (`crm streaming.go:26`). No fee/tracer streaming on v3 — the v4 multi-prefix collapse does NOT apply.
- **Keep `alias`:** develop renamed it to `instrument`; v3 keeps `alias` (`crm.alias.related_party_deleted`).
- **Reference:** `origin/develop` already has underscore `Definition`s + `RouteKey()` in the exact shape above (on lib-streaming/v2) — port the form, keep v3's v1.9.0 imports and `alias` naming.
