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

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v3.0.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka/Redpanda.
- **Component:** tracer (`components/tracer`). Tracer is a standalone Go service
  with its own self-contained emitter bootstrap at
  `components/tracer/internal/bootstrap/streaming.go`.
- **Application name / CloudEvents source (`ce-source`):** `tracer` (set on the
  producer Builder at construction; there is no per-emit source). It must be ONE
  dot-free lowercase segment matching `^[a-z0-9][a-z0-9_-]*$`, at most 223 bytes; a
  malformed value is REJECTED at startup, never normalized. The resolved value is
  load-bearing three times over — it is stamped as `ce-source`, it derives the one
  topic every event rides, and it is what the streaming manifest advertises.
- **Kafka topics:** ONE topic per producing application. All 12 events — every
  resource type, every event type, every schema version — ride
  `lerian.streaming.tracer`, with `lerian.streaming.tracer.dlq` as its single
  dead-letter topic. There is no per-event topic and no `.v<major>` topic suffix:
  `ce-schemaversion` is the only version carrier on the wire. Consumers subscribe to
  the application and dispatch on the event key.
- **Posture:** all 12 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the request**. Durability of the mutation itself is
  owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only. When an outbox lands, only the
  emit call sites change; the Definitions and payload contracts below stay put.
- **HTTP event-manifest endpoint.** The tracer binary serves
  `GET /v1/streaming/manifest` (inside the `/v1` group; auth
  `streaming-manifest`/`get`) — a catalog-only view of the 12 registered event
  Definitions, at manifest wire version `1.0.0`. The document carries
  `publisher.source` plus tracer's `topic` / `dlqTopic` pair at DOCUMENT level (no
  commands queue: tracer emits facts only), and each event entry names its
  `eventKey` (`"<resourceType>.<eventType>"` — the consumer's dispatch selector),
  its `schemaVersion`, and its `class`, always `"fact"`. The advertised topic is
  derived from the same `ce-source` the emitter publishes under. It is independent
  of `STREAMING_ENABLED` and degraded-safe.
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
Catalog (`buildCatalog`) and the manifest:

- **Event key** = `<resourceType>.<eventType>` via `Definition.Key()` (e.g.
  `rule.created`) — the dispatch selector a consumer registers a handler under
  inside the `tracer` stream. `resource` ∈ {`rule`, `limit`}; `event` ∈ {`created`,
  `updated`, `activated`, `deactivated`, `drafted`, `deleted`}.
- **`ce-type`** = `studio.lerian.<app>.<resourceType>.<eventType>`, i.e.
  `studio.lerian.tracer.<key>` (resource unchanged, e.g.
  `studio.lerian.tracer.rule.created`). The `tracer` segment names the producing
  application, which is what keeps two services emitting a same-named event from
  producing byte-identical `ce-type` values — a homonym collision a consumer reading
  only `ce-type` cannot detect.
- **Kafka topic** = `lerian.streaming.tracer` for every event in the catalog,
  derived from `ce-source` via `libStreaming.AppTopic`. One catch-all route carries
  every fact; nothing fans out per event.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`) — the rule UUID or
  limit UUID.
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` inside `EmitImportant`.

## Conventions

| Aspect | Rule |
|--------|------|
| Event key | `<resource>.<event>`, lowercase; tokens are single words (no separator); it is the consumer's dispatch selector |
| Kafka topic | `lerian.streaming.tracer` — one topic for the whole catalog; `lerian.streaming.tracer.dlq` for a failed publish |
| `ce-type` | `studio.lerian.tracer.<resource>.<event>` (auto-prefixed by lib-streaming) |
| `ce-source` | `tracer` |
| `ce-subject` | aggregate ID — rule UUID or limit UUID |
| `ce-tenantid` | `pkgStreaming.ResolveTenantID(ctx)`, falls back to `"default"` |
| Schema version | `1.0.0` (all 12 events) |

## Event catalog

All 12 events carry `SchemaVersion = 1.0.0`.

| Event key | `ce-type` | `ce-subject` | Schema version |
|-----------|-----------|--------------|----------------|
| `rule.created` | `studio.lerian.tracer.rule.created` | rule ID | `1.0.0` |
| `rule.updated` | `studio.lerian.tracer.rule.updated` | rule ID | `1.0.0` |
| `rule.activated` | `studio.lerian.tracer.rule.activated` | rule ID | `1.0.0` |
| `rule.deactivated` | `studio.lerian.tracer.rule.deactivated` | rule ID | `1.0.0` |
| `rule.drafted` | `studio.lerian.tracer.rule.drafted` | rule ID | `1.0.0` |
| `rule.deleted` | `studio.lerian.tracer.rule.deleted` | rule ID | `1.0.0` |
| `limit.created` | `studio.lerian.tracer.limit.created` | limit ID | `1.0.0` |
| `limit.updated` | `studio.lerian.tracer.limit.updated` | limit ID | `1.0.0` |
| `limit.activated` | `studio.lerian.tracer.limit.activated` | limit ID | `1.0.0` |
| `limit.deactivated` | `studio.lerian.tracer.limit.deactivated` | limit ID | `1.0.0` |
| `limit.drafted` | `studio.lerian.tracer.limit.drafted` | limit ID | `1.0.0` |
| `limit.deleted` | `studio.lerian.tracer.limit.deleted` | limit ID | `1.0.0` |

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
ce-type:        studio.lerian.tracer.rule.created
ce-source:      tracer
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

`ce-type` is the application-qualified key — `studio.lerian.` plus the
application name (`tracer`) plus the event key (`rule.created`);
`ce-subject` is the rule UUID and matches the payload `id`; the payload carries
no `name`, `description`, or `expression`. All UUIDs above are illustrative
placeholders.

> **Before upgrading:** the ce-source is now REFUSED at startup unless it is exactly
> `tracer`. A value carried over from before the one-topic contract — the dotted
> `lerian.midaz.tracer` or URI `//lerian.midaz/tracer` shapes, or any other legal
> name — must be removed from every env file first, or the service will not boot. The
> refusal is deliberate: broker topics and Kafka ACLs are provisioned for the roster
> name alone, so any other value would publish into a stream that neither exists nor
> is granted, and midaz would swallow every one of those failures as a Warn while
> reporting healthy. The check runs whether or not `STREAMING_ENABLED` is set.

## Partitioning

lib-streaming picks a record's partition key by falling back through: system event
→ tenant → `ce-subject` → event id. Under one topic per application that has one
consequence worth stating plainly.

- **Multi-tenant deployments** key by the resolved tenant, so the stream spreads
  across `lerian.streaming.tracer`'s partitions and every tenant's events keep a
  stable partition affinity.
- **Single-tenant deployments** carry the literal tenant `"default"` on every event
  (see `ce-tenantid` above), so the ENTIRE stream hashes to ONE partition regardless
  of how many the topic has. Consumer parallelism on that stream is capped at one.

That is a throughput ceiling, not a correctness problem: a single partition makes
ordering stronger, not weaker — total order across the stream instead of order per
tenant — and nothing is dropped or misrouted. Plan capacity for it in single-tenant
deployments.

The ceiling stands until the platform-wide partition-key default is decided in
lib-streaming (tenant+subject is the likely shape). midaz deliberately does not
override the key locally: partitioning is a fleet contract, and one service opting
out of it privately is how two consumers of the same stream end up disagreeing about
what a partition means.

## Local testing

To exercise the real emit path against a broker, run a Redpanda instance and
point tracer at it:

- Bind the broker on host port `19092`; join `infra-network` so it is reachable
  from both host (`localhost:19092`) and containers (`<container>:9092`).
- Set `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:19092`, and
  `STREAMING_CLOUDEVENTS_SOURCE=tracer`.
- Pre-provision these two topics explicitly; do not rely on auto-create. The
  per-event topic list is gone — every event tracer emits rides the first of them:

  ```
  lerian.streaming.tracer
  lerian.streaming.tracer.dlq
  ```

  Give the DLQ a `max.message.bytes` at or above its source topic's: a DLQ record is
  strictly larger than the record it quarantines.

The default unit suite never touches a broker — the JSONShape and mapping tests
in `pkg/streaming/events/` marshal payloads in memory. See the `CLAUDE.md`
Streaming → Local testing section for the broker/environment conventions.

## Canonical code locations

- **Wire structs + JSONShape tests:** `pkg/streaming/events/{rule,limit}_*.go`
  and the matching `*_test.go`.
- **Shared scope shape:** `pkg/streaming/events/rule_scope.go`
  (`RuleScopePayload`, `newRuleScopePayloads`).
- **Event registry (single source of truth for catalog + manifest):**
  `tracerEventDefinitions()` in
  `components/tracer/internal/bootstrap/streaming.go`.
- **Emit helpers (post-commit):** `emit<Event>Event` on each command in
  `components/tracer/internal/services/command/`.
