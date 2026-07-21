# Tracer Streaming Event Catalog

Canonical reference for every streaming event the **tracer** component
(`components/tracer`, :4020) emits. It complements — does not duplicate — the
producer conventions in `CLAUDE.md` (Streaming section) and
`docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/{rule,limit}_*.go`, and the field-count assertions in the
> matching `*_test.go` JSONShape tests are ONE contract. A wire change updates
> all three in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v1.6.2.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka/Redpanda.
- **Component:** tracer (`components/tracer`). Tracer is a standalone Go service
  with its own self-contained emitter bootstrap at
  `components/tracer/internal/bootstrap/streaming.go`.
- **CloudEvents source (`ce-source`):** `lerian.midaz.tracer` (set on the
  producer Builder at construction; there is no per-emit source).
- **Posture:** all 12 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the request**. Durability of the mutation itself is
  owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only. When an outbox lands, only the
  emit call sites change; the Definitions and payload contracts below stay put.
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled — or
  when `STREAMING_BROKERS` is empty, or no events are registered — bootstrap
  injects a `NoopEmitter` and no broker connection is attempted.
- **No `organizationId` / `ledgerId` on the wire.** Those dimensions do not
  exist anywhere in Tracer's domain. Tenant isolation travels only in the
  `ce-tenantid` header (see [ce-tenantid](#ce-tenantid)).

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `tracerEventDefinitions()`
(`components/tracer/internal/bootstrap/streaming.go`), which feeds both the
Catalog (`buildCatalog`) and the route table (`buildRoutes`):

- **Event key** = `<resourceType>.<eventType>` via `Definition.Key()` (e.g.
  `rule.created`). `resource` ∈ {`rule`, `limit`}; `event` ∈ {`created`,
  `updated`, `activated`, `deactivated`, `drafted`, `deleted`}.
- **`ce-type`** = lib-streaming auto-prefixes the key: `studio.lerian.<key>`
  (resource unchanged, e.g. `studio.lerian.rule.created`).
- **Kafka topic** = `pkgStreaming.TopicName("tracer", key)` =
  `lerian.streaming.tracer_<resource>.<event>` — the producing-service segment
  (`tracer`) is folded in (e.g. `lerian.streaming.tracer_rule.created`).
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`) — the rule UUID or
  limit UUID.
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` inside `EmitImportant`.

## Conventions

| Aspect | Rule |
|--------|------|
| Event key | `<resource>.<event>`, lowercase; tokens are single words (no separator); underscores are rejected by the route-key validator |
| Kafka topic | `lerian.streaming.tracer_<resource>.<event>` |
| `ce-type` | `studio.lerian.<resource>.<event>` (auto-prefixed by lib-streaming) |
| `ce-source` | `lerian.midaz.tracer` |
| `ce-subject` | aggregate ID — rule UUID or limit UUID |
| `ce-tenantid` | `pkgStreaming.ResolveTenantID(ctx)`, falls back to `"default"` |
| Schema version | `1.0.0` (all 12 events) |

## Event catalog

All 12 events carry `SchemaVersion = 1.0.0`.

| Event key | `ce-type` | Kafka topic | `ce-subject` | Schema version |
|-----------|-----------|-------------|--------------|----------------|
| `rule.created` | `studio.lerian.rule.created` | `lerian.streaming.tracer_rule.created` | rule ID | `1.0.0` |
| `rule.updated` | `studio.lerian.rule.updated` | `lerian.streaming.tracer_rule.updated` | rule ID | `1.0.0` |
| `rule.activated` | `studio.lerian.rule.activated` | `lerian.streaming.tracer_rule.activated` | rule ID | `1.0.0` |
| `rule.deactivated` | `studio.lerian.rule.deactivated` | `lerian.streaming.tracer_rule.deactivated` | rule ID | `1.0.0` |
| `rule.drafted` | `studio.lerian.rule.drafted` | `lerian.streaming.tracer_rule.drafted` | rule ID | `1.0.0` |
| `rule.deleted` | `studio.lerian.rule.deleted` | `lerian.streaming.tracer_rule.deleted` | rule ID | `1.0.0` |
| `limit.created` | `studio.lerian.limit.created` | `lerian.streaming.tracer_limit.created` | limit ID | `1.0.0` |
| `limit.updated` | `studio.lerian.limit.updated` | `lerian.streaming.tracer_limit.updated` | limit ID | `1.0.0` |
| `limit.activated` | `studio.lerian.limit.activated` | `lerian.streaming.tracer_limit.activated` | limit ID | `1.0.0` |
| `limit.deactivated` | `studio.lerian.limit.deactivated` | `lerian.streaming.tracer_limit.deactivated` | limit ID | `1.0.0` |
| `limit.drafted` | `studio.lerian.limit.drafted` | `lerian.streaming.tracer_limit.drafted` | limit ID | `1.0.0` |
| `limit.deleted` | `studio.lerian.limit.deleted` | `lerian.streaming.tracer_limit.deleted` | limit ID | `1.0.0` |

## Shared `scopes[]` nested shape

Both `rule.created`/`rule.updated` and `limit.created`/`limit.updated` carry a
`scopes[]` array. Every element is the same `RuleScopePayload`
(`pkg/streaming/events/rule_scope.go`) — six structural identifiers/enums, each
`*string` on the wire so JSON `null` distinguishes "unset" from empty. An empty
domain scope slice serializes as `"scopes": []` (non-null).

```jsonc
{
  "segmentId":       "uuid | null",
  "portfolioId":     "uuid | null",
  "accountId":       "uuid | null",
  "merchantId":      "uuid | null",
  "transactionType": "CARD | WIRE | PIX | CRYPTO | null",
  "subType":         "string | null"
}
```

The nested object is locked to exactly **6 keys**. `subType` is a structural
sub-classifier and is deliberately INCLUDED; no free text otherwise appears in
a scope.

## Payload contracts

The wire keys below are the exact JSON field set produced by the Payload structs
in `pkg/streaming/events/`. The "field count" is the number the JSONShape test
locks.

### `rule.created` / `rule.updated` — 6 fields

Source: `pkg/streaming/events/rule_created.go`, `rule_updated.go`.
`ce-subject` = rule ID.

```jsonc
{
  "id":        "uuid",
  "status":    "DRAFT | ACTIVE | INACTIVE | DELETED",
  "action":    "ALLOW | DENY | REVIEW",
  "scopes":    [ { /* RuleScopePayload — 6 keys */ } ],
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339"
}
```

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Rule ID. |
| `status` | string | `DRAFT` / `ACTIVE` / `INACTIVE` / `DELETED`. |
| `action` | string | Decision: `ALLOW` / `DENY` / `REVIEW`. |
| `scopes` | array | Shared `RuleScopePayload` elements (6 keys each); `[]` when empty. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

**Excluded / never on the wire** — the JSONShape test explicitly asserts `name`,
`description`, `expression`, and `compiledProgram` are absent at the top level,
and additionally asserts `name`, `description`, and `expression` are absent
inside each `scopes` element.

### `rule.activated` — 4 fields

Source: `pkg/streaming/events/rule_activated.go`. `ce-subject` = rule ID.

```jsonc
{
  "id":          "uuid",
  "status":      "DRAFT | ACTIVE | INACTIVE | DELETED",
  "activatedAt": "RFC3339 | null",
  "updatedAt":   "RFC3339"
}
```

`activatedAt` is `*string`: the key is always present (not `omitempty`); a
defensive nil serializes as JSON `null`. In practice `Rule.SetStatus(ACTIVE)`
guarantees it non-nil at the emit site.

**Excluded / not present on the wire:** `action`, `scopes`, `name`,
`description`, `expression`, `compiledProgram`, `deactivatedAt`. The JSONShape
test names `name`, `description`, `expression`, `action`, and `scopes` in an
explicit forbidden-key check; `compiledProgram` and `deactivatedAt` are kept off
the wire by the exact-key-set + field-count lock (not a dedicated NotContains).

### `rule.deactivated` — 4 fields

Source: `pkg/streaming/events/rule_deactivated.go`. `ce-subject` = rule ID.

```jsonc
{
  "id":            "uuid",
  "status":        "DRAFT | ACTIVE | INACTIVE | DELETED",
  "deactivatedAt": "RFC3339 | null",
  "updatedAt":     "RFC3339"
}
```

`deactivatedAt` is `*string` with the same key-always-present / nil-guard
treatment as `rule.activated`.

**Excluded / not present on the wire:** `action`, `scopes`, `name`,
`description`, `expression`, `compiledProgram`, `activatedAt`. The JSONShape test
names `name`, `description`, `expression`, `action`, `scopes`, and `activatedAt`
in an explicit forbidden-key check; `compiledProgram` is kept off the wire by the
exact-key-set + field-count lock (not a dedicated NotContains).

### `rule.drafted` — 3 fields

Source: `pkg/streaming/events/rule_drafted.go`. `ce-subject` = rule ID.

```jsonc
{
  "id":        "uuid",
  "status":    "DRAFT | ACTIVE | INACTIVE | DELETED",
  "updatedAt": "RFC3339"
}
```

`activatedAt` and `deactivatedAt` are deliberately omitted:
`Rule.SetStatus(DRAFT)` nils both, so carrying them would emit `null` noise. The
drafted contract stays minimal.

**Excluded:** `action`, `scopes`, `activatedAt`, `deactivatedAt`, `name`,
`description`, `expression`, `compiledProgram`.

### `rule.deleted` — 2 fields

Source: `pkg/streaming/events/rule_deleted.go`. `ce-subject` = rule ID.

```jsonc
{
  "id":        "uuid",
  "deletedAt": "RFC3339"
}
```

The delete use case returns no entity, so the payload is built from a
primitive-arg constructor `NewRuleDeleted(id, deletedAt)`. No `status` field.

**Excluded** (asserted absent by the JSONShape test): `status`, `name`,
`description`, `expression`, `scopes`, `action`.

### `limit.created` / `limit.updated` — 12 fields

Source: `pkg/streaming/events/limit_created.go`, `limit_updated.go`.
`ce-subject` = limit ID.

```jsonc
{
  "id":              "uuid",
  "status":          "DRAFT | ACTIVE | INACTIVE | DELETED",
  "limitType":       "DAILY | WEEKLY | MONTHLY | CUSTOM | PER_TRANSACTION",
  "currency":        "ISO-4217",
  "scopes":          [ { /* RuleScopePayload — 6 keys */ } ],
  "activeTimeStart": "HH:MM | null",
  "activeTimeEnd":   "HH:MM | null",
  "customStartDate": "RFC3339 | null",
  "customEndDate":   "RFC3339 | null",
  "resetAt":         "RFC3339 | null",
  "createdAt":       "RFC3339",
  "updatedAt":       "RFC3339"
}
```

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Limit ID. |
| `status` | string | `DRAFT` / `ACTIVE` / `INACTIVE` / `DELETED`. |
| `limitType` | string | `DAILY` / `WEEKLY` / `MONTHLY` / `CUSTOM` / `PER_TRANSACTION`. |
| `currency` | string | ISO-4217 code. |
| `scopes` | array | Shared `RuleScopePayload` elements (6 keys each); `[]` when empty. |
| `activeTimeStart` | string \| null | Time-of-day window start (`HH:MM`), `null` when unset. |
| `activeTimeEnd` | string \| null | Time-of-day window end (`HH:MM`), `null` when unset. |
| `customStartDate` | string \| null | RFC3339, `null` unless the period is `CUSTOM`. |
| `customEndDate` | string \| null | RFC3339, `null` unless the period is `CUSTOM`. |
| `resetAt` | string \| null | RFC3339 next-reset time, `null` when unset. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`name`, `description`, `maxAmount`.

### `limit.activated` / `limit.deactivated` / `limit.drafted` — 3 fields each

Source: `pkg/streaming/events/limit_activated.go`, `limit_deactivated.go`,
`limit_drafted.go`. `ce-subject` = limit ID.

```jsonc
{
  "id":        "uuid",
  "status":    "DRAFT | ACTIVE | INACTIVE | DELETED",
  "updatedAt": "RFC3339"
}
```

Unlike Rule, the Limit domain model has no `ActivatedAt` / `DeactivatedAt`
fields, so all three status-transition events carry the same minimal shape.

**Excluded:** `limitType`, `currency`, `scopes`, all time-window fields,
`name`, `description`, `maxAmount`.

### `limit.deleted` — 2 fields

Source: `pkg/streaming/events/limit_deleted.go`. `ce-subject` = limit ID.

```jsonc
{
  "id":        "uuid",
  "deletedAt": "RFC3339"
}
```

**Excluded:** `status`, `limitType`, `currency`, `scopes`, all time-window
fields, `name`, `description`, `maxAmount`.

## What is deliberately off the wire, and why

Every payload carries only stable identifiers, classifier enums, structural
scope references, time-window/period fields, and timestamps. The following are
**deliberately excluded** from every event body and are asserted absent by the
JSONShape tests. Consumers that need any of these fetch the full record by `id`.

| Field | Aggregate | Reason for exclusion |
|-------|-----------|----------------------|
| `name` | rule, limit | Free text — human-facing label, not needed for routing. |
| `description` | rule, limit | Free text — narrative, not needed for routing. |
| `expression` | rule | Rule logic (CEL body). Business logic does not belong on a lifecycle event. |
| `compiledProgram` | rule | Transient compiled artifact of `expression`; never persisted meaning on the wire. |
| `maxAmount` | limit | **Monetary value.** Excluded to keep financial values off the wire, mirroring the ledger's "no monetary values on the wire" fence. Consumers fetch the amount by `id`. |

**Enforcement.** Two mechanisms back these guarantees, and the first is what
makes them airtight. First, the `JSONShape` unit test in each event's `*_test.go`
pins the EXACT present-key set and the field COUNT (`Lenf(..., N)`). Together
these reject ANY top-level key that is not in the documented shape — which is
what guarantees `compiledProgram`, `deactivatedAt`, and any other stray field
never leak, even though those two are not named in an explicit forbidden-key
check on the `activated` / `deactivated` events. Any excluded field added to a
payload changes the key set or count and fails that test.

Second, and in addition, each `JSONShape` test carries explicit forbidden-key
assertions (`assert.False` / `NotContains`) over a subset of the shared
high-risk fields — the free-text, rule-logic, and monetary fields (`name`,
`description`, `expression`, `compiledProgram` for rule; `name`, `description`,
`maxAmount` for limit). The fixtures deliberately POPULATE those fields
(`minimalRule()` sets `Name`, `Description`, `Expression`, and `CompiledProgram`
to non-empty values) so the tests prove those specific fields are dropped even
when set upstream. The subset checked explicitly varies per event; the exact
key-set + count lock above covers the remainder.

## `ce-tenantid`

Every emission carries a `ce-tenantid` header sourced from
`pkgStreaming.ResolveTenantID(ctx)`:

- **Multi-tenant deployments:** the resolved tenant ID from the lib-commons
  multitenancy middleware (JWT auth or the `X-Tenant-Id` seam header).
- **Single-tenant deployments and tenantless paths** (e.g. workers): the literal
  `"default"` (`pkgStreaming.DefaultTenantID`). lib-streaming requires a
  non-empty tenant ID, so the fallback guarantees a valid header.

Tracer has no `organizationId` / `ledgerId` dimension, so tenant isolation lives
solely in this header.

## A real consumed event

A `rule.created` emission, as a consumer sees it (CloudEvents 1.0 binary mode —
metadata in Kafka headers, payload in the record value):

**Headers**

```
ce-specversion: 1.0
ce-type:        studio.lerian.rule.created
ce-source:      lerian.midaz.tracer
ce-id:          0f9c1a3e-6b2d-4e7f-9a10-2c8d5f4b1a22
ce-subject:     7b3e2c14-9d5a-4f61-8c2b-1e0a9d7f4c33
ce-tenantid:    default
ce-time:        2026-07-03T14:22:07Z
content-type:   application/json
```

**Payload (record value)**

```json
{
  "id": "7b3e2c14-9d5a-4f61-8c2b-1e0a9d7f4c33",
  "status": "DRAFT",
  "action": "DENY",
  "scopes": [
    {
      "segmentId": "3a1f8b02-4c6d-4e90-a2b7-9f0c1d2e3a4b",
      "portfolioId": null,
      "accountId": null,
      "merchantId": "c4d5e6f7-8a9b-40c1-92d3-e4f5a6b7c8d9",
      "transactionType": "PIX",
      "subType": "instant"
    }
  ],
  "createdAt": "2026-07-03T14:22:07Z",
  "updatedAt": "2026-07-03T14:22:07Z"
}
```

`ce-type` is the auto-prefixed key (`studio.lerian.` + `rule.created`);
`ce-subject` is the rule UUID and matches the payload `id`; the payload carries
no `name`, `description`, or `expression`. All UUIDs above are illustrative
placeholders.

## Local testing

To exercise the real emit path against a broker, run a Redpanda instance and
point tracer at it:

- Bind the broker on host port `19092`; join `infra-network` so it is reachable
  from both host (`localhost:19092`) and containers (`<container>:9092`).
- Set `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:19092`, and
  `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.tracer`.
- Pre-provision these 12 topics explicitly; do not rely on auto-create:

  ```
  lerian.streaming.tracer_rule.created
  lerian.streaming.tracer_rule.updated
  lerian.streaming.tracer_rule.activated
  lerian.streaming.tracer_rule.deactivated
  lerian.streaming.tracer_rule.drafted
  lerian.streaming.tracer_rule.deleted
  lerian.streaming.tracer_limit.created
  lerian.streaming.tracer_limit.updated
  lerian.streaming.tracer_limit.activated
  lerian.streaming.tracer_limit.deactivated
  lerian.streaming.tracer_limit.drafted
  lerian.streaming.tracer_limit.deleted
  ```

The default unit suite never touches a broker — the JSONShape and mapping tests
in `pkg/streaming/events/` marshal payloads in memory. See the `CLAUDE.md`
Streaming → Local testing section for the broker/environment conventions.

## Canonical code locations

- **Wire structs + JSONShape tests:** `pkg/streaming/events/{rule,limit}_*.go`
  and the matching `*_test.go`.
- **Shared scope shape:** `pkg/streaming/events/rule_scope.go`
  (`RuleScopePayload`, `newRuleScopePayloads`).
- **Event registry (single source of truth for catalog + routes):**
  `tracerEventDefinitions()` in
  `components/tracer/internal/bootstrap/streaming.go`.
- **Emit helpers (post-commit):** `emit<Event>Event` on each command in
  `components/tracer/internal/services/command/`.
