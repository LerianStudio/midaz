# Fees Streaming Event Catalog

Canonical reference for every streaming event the **fees** surface of the
`components/ledger` component emits. It complements — does not duplicate — the
producer conventions in `CLAUDE.md` (Streaming section) and
`docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/fees_*.go`, and the field-count assertions in the
> matching `*_test.go` JSONShape tests are ONE contract. A wire change updates
> all three in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v3.1.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** ledger (`components/ledger`). Fees are embedded in the ledger
  binary; there is no standalone fees service.
- **Application name / CloudEvents source (`ce-source`):** `ledger` — the
  application name of the binary fees ride on. It must be ONE dot-free lowercase
  segment matching `^[a-z0-9][a-z0-9_-]*$`, at most 223 bytes; a malformed value is
  REJECTED at startup, never normalized.
- **Kafka topics:** ONE topic per producing application. Fee events ride
  `lerian.streaming.ledger` alongside every other event the ledger binary emits,
  with `lerian.streaming.ledger.dlq` as the single dead-letter topic. There is no
  `fee` topic, no per-event topic, and no `.v<major>` topic suffix:
  `ce-schemaversion` is the only version carrier on the wire. Consumers subscribe to
  the application and dispatch on the event key.
- **Posture:** all 7 events invoke `pkgStreaming.EmitBrokerBestEffort` at the
  post-commit slot. It bounds the synchronous `Emitter.Emit` call, span-records
  and Warn-logs build/emit failures, and **never fails the HTTP request**. The
  library resolves delivery policy and any configured fallback behavior.
- **No Midaz transactional outbox.** Bootstrap passes neither an outbox writer nor
  repository, and registers no relay. The helper does not persist a local fallback record or
  make the database mutation and broker delivery atomic; definitions and payload
  contracts stay unchanged.
- **HTTP event-manifest endpoint.** The ledger binary serves
  `GET /v1/streaming/manifest` (auth `streaming-manifest`/`get`) — a catalog-only
  view of the registered event Definitions, including the `fee_*` events, at
  manifest wire version `1.0.0`. The application's `topic` / `dlqTopic` pair sits at
  DOCUMENT level; each event entry names its `eventKey`
  (`"<resourceType>.<eventType>"`), its `schemaVersion`, and its `class` — always
  `"fact"` here. It is independent of `STREAMING_ENABLED` and degraded-safe.
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled, bootstrap
  injects a `NoopEmitter` and no broker connection is attempted (the ledger binary
  has no streaming readiness prober, so `/readyz` carries no streaming check).
  `STREAMING_ENABLED=true` with an empty `STREAMING_BROKERS` REFUSES BOOT
  (`pkgStreaming.RequireBrokers`, which validates only the broker list): an
  enabled producer with nowhere to publish discards every event silently while
  readiness stays green, which is the same invisible-total-loss failure the
  roster source gate exists to kill. To run without streaming, set
  `STREAMING_ENABLED=false`.
- **Local broker:** the infra Redpanda. Set `STREAMING_ENABLED=true` and
  `STREAMING_BROKERS=localhost:19092` to exercise the real emit path locally.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `midazEventDefinitions()`
(`components/ledger/internal/bootstrap/streaming.go`), which feeds both the
Catalog (`buildCatalog`) and the manifest:

- **Event key** = `<resourceType>.<eventType>` via `Definition.Key()` (e.g.
  `fee_packages.created`) — the dispatch selector a consumer registers a handler
  under inside the `ledger` stream.
- **`ce-type`** = `studio.lerian.<app>.<resourceType>.<eventType>`, i.e.
  `studio.lerian.ledger.<key>` (e.g.
  `studio.lerian.ledger.fee_packages.created`). The `ledger` segment names the
  producing application, which is what keeps two services emitting a same-named
  event from producing byte-identical `ce-type` values.
- **Kafka topic** = `lerian.streaming.ledger` for every fee event, derived from
  `ce-source` via `libStreaming.AppTopic` and shared with the rest of the
  ledger-binary catalog. The `fee_` prefix on the ResourceType STAYS — with no
  per-product topic segment left, it is what namespaces fees inside the
  application's event space (e.g. `fee_packages.created`, `fee_charge.applied`).
  One catch-all route carries every fact; nothing fans out per event.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` inside `EmitBrokerBestEffort` (see
  [ce-tenantid](#ce-tenantid)).

## Event summary

All 7 events carry `SchemaVersion = 1.0.0`.

| Event key | Resource / Event | `ce-type` | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|--------------|--------------------|
| `fee_packages.created` | fee_packages / created | `studio.lerian.ledger.fee_packages.created` | package ID | create fee package |
| `fee_packages.updated` | fee_packages / updated | `studio.lerian.ledger.fee_packages.updated` | package ID | update fee package |
| `fee_packages.deleted` | fee_packages / deleted | `studio.lerian.ledger.fee_packages.deleted` | package ID | delete fee package |
| `fee_billing_packages.created` | fee_billing_packages / created | `studio.lerian.ledger.fee_billing_packages.created` | billing package ID | create billing package |
| `fee_billing_packages.updated` | fee_billing_packages / updated | `studio.lerian.ledger.fee_billing_packages.updated` | billing package ID | update billing package |
| `fee_billing_packages.deleted` | fee_billing_packages / deleted | `studio.lerian.ledger.fee_billing_packages.deleted` | billing package ID | delete billing package |
| `fee_charge.applied` | fee_charge / applied | `studio.lerian.ledger.fee_charge.applied` | **transaction ID** | fee charged on a posted transaction |

> **Underscores are preserved everywhere.** The `fee_packages` and
> `fee_billing_packages` resource types are multi-word. Their **event key**
> (`Definition.Key()`) and **`ce-type`** are underscore-canonical — that is what
> consumers see on the wire and register handlers under (e.g. key
> `fee_packages.created`, ce-type `studio.lerian.ledger.fee_packages.created`), with
> the `fee_` prefix remaining as the fees namespace inside the `ledger`
> application. Route keys accept underscores, so nothing is folded and no event name
> has a hyphenated variant.

> **`ce-subject` on `fee_charge.applied`.** The aggregate is the transaction the fee
> was charged against, so `ce-subject` is the **transaction ID**, and the
> charged fee package's ID travels in the body as `feePackageId`. The
> package/billing-package events use their own record ID as subject.

## Payload contracts

The wire keys below are the exact JSON field set produced by the Payload structs
in `pkg/streaming/events/`. The "field count" is the number the JSONShape test
locks.

### `fee_packages.created` / `fee_packages.updated` — 8 fields

Source: `pkg/streaming/events/fees_package_created.go`,
`fees_package_updated.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Fee package ID. |
| `organizationId` | string | Organization scope. |
| `ledgerId` | string | Ledger scope. |
| `segmentId` | string \| null | Optional segment classification. JSON `null` when unset. |
| `transactionRoute` | string \| null | Optional transaction-route classification. JSON `null` when unset. |
| `enable` | bool | Whether the package is enabled. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`feeGroupLabel`, `description`, `minimumAmount`, `maximumAmount`, `fees`,
`waivedAccounts`.

### `fee_packages.deleted` — 4 fields

Source: `pkg/streaming/events/fees_package_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Fee package ID. |
| `organizationId` | string | Organization scope. |
| `ledgerId` | string | Ledger scope. |
| `deletedAt` | string | RFC3339 deletion timestamp. `id` + `deletedAt` is unique per deletion — the idempotency hint for consumers. |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`feeGroupLabel`, `description`, `minimumAmount`, `maximumAmount`, `fees`,
`waivedAccounts`, `segmentId`, `transactionRoute`, `enable`.

### `fee_billing_packages.created` / `fee_billing_packages.updated` — 9 fields

Source: `pkg/streaming/events/fees_billing_package_created.go`,
`fees_billing_package_updated.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Billing package ID. |
| `organizationId` | string | Organization scope. |
| `ledgerId` | string | Ledger scope. |
| `type` | string | Package classification: `"volume"` or `"maintenance"`. |
| `pricingModel` | string \| null | Optional pricing-model classification. JSON `null` when unset. |
| `countMode` | string \| null | Optional count-mode classification. JSON `null` when unset. |
| `enable` | bool | Whether the package is enabled. `nil` on the domain model resolves to `false`. |
| `createdAt` | string | RFC3339 (pass-through string from the domain model). |
| `updatedAt` | string | RFC3339 (pass-through string from the domain model). |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`label`, `description`, `assetCode`, `feeAmount`, `tiers`, `discountTiers`,
`freeQuota`, `eventFilter`, `accountTarget`, `debitAccountAlias`,
`creditAccountAlias`, `maintenanceCreditAccount`.

### `fee_billing_packages.deleted` — 4 fields

Source: `pkg/streaming/events/fees_billing_package_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Billing package ID. |
| `organizationId` | string | Organization scope. |
| `ledgerId` | string | Ledger scope. |
| `deletedAt` | string | RFC3339 deletion timestamp. |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`label`, `description`, `assetCode`, `feeAmount`, `tiers`, `discountTiers`,
`freeQuota`, `eventFilter`, `accountTarget`, `debitAccountAlias`,
`creditAccountAlias`, `maintenanceCreditAccount`, `type`, `pricingModel`,
`countMode`, `enable`, `createdAt`, `updatedAt`.

### `fee_charge.applied` — 5 fields

Source: `pkg/streaming/events/fees_applied.go`.

| Key | Type | Notes |
|-----|------|-------|
| `transactionId` | string | The transaction the fee was charged against. Also the `ce-subject`. |
| `organizationId` | string | Organization scope. |
| `ledgerId` | string | Ledger scope. |
| `feePackageId` | string | The applied fee package reference. |
| `appliedAt` | string | RFC3339 timestamp of when fees were applied (the transaction `CreatedAt`). |

**Excluded / never on the wire** (asserted absent by the JSONShape test):
`amount`, `assetCode`, `source`, `destination`, `metadata`, `operations`,
`description`, `fees`, `waivedAccounts`.

## `fee_charge.applied` semantics

`fee_charge.applied` is a charge signal, not a transaction signal:

- **Charged only.** It is emitted only when a fee was actually **charged** —
  `emitFeesAppliedEvent` fires only when `feeApplied=true` and a non-empty
  `packageAppliedID` are present in the transaction metadata (set by the fee
  engine on the real-charge branch). A pure exemption still sets
  `packageAppliedID` but omits `feeApplied=true`, so the `feeApplied` guard
  suppresses it — **no event is emitted on exemption**.
- **Once.** It rides alongside `transaction.posted` only. Commit, cancel, and
  revert do NOT re-emit it — the fee charge happened once, at post.

## Monetary and detail surface off the wire

Fee packages and charges carry pricing and monetary detail that consumers do not
need for event routing. The payloads carry only stable identifiers, scope IDs,
non-identifying classifications, the enable flag, and timestamps. The following
surfaces are **deliberately excluded** from every event body:

- **Fee-package detail:** `feeGroupLabel`, `description`, `minimumAmount`,
  `maximumAmount`, `fees`, `waivedAccounts`.
- **Billing-package detail:** `label`, `description`, `assetCode`, `feeAmount`,
  `tiers`, `discountTiers`, `freeQuota`, `eventFilter`, `accountTarget`,
  `debitAccountAlias`, `creditAccountAlias`, `maintenanceCreditAccount`.
- **Applied-charge detail:** `amount`, `assetCode`, `source`, `destination`,
  `metadata`, `operations`, `description`, `fees`, `waivedAccounts`.

**Enforcement.** The `JSONShape` unit test in each event's `*_test.go` locks the
exact present-key set, pins the field count, and asserts the absence of every
excluded key. Any monetary/detail field added to a payload fails that test.

## `ce-tenantid`

Every emission carries a `ce-tenantid` header sourced from
`pkgStreaming.ResolveTenantID(ctx)`:

- **Multi-tenant deployments:** the resolved tenant ID from the lib-commons
  multitenancy middleware.
- **Single-tenant deployments** (and tenantless paths): the literal
  `"default"` (`pkgStreaming.DefaultTenantID`). lib-streaming requires a
  non-empty tenant ID, so the fallback guarantees a valid header.

Note: `organizationId` is a **payload** field (a collection/sub-tenant
dimension), not the tenant. It is never used as `ce-tenantid`.

> **Before upgrading:** the ce-source is now REFUSED at startup unless it is exactly
> `ledger`. A value carried over from before the one-topic contract — the dotted
> `lerian.midaz.ledger` or URI `//lerian.midaz/ledger` shapes, or any other legal
> name — must be removed from every env file first, or the service will not boot. The
> refusal is deliberate: broker topics and Kafka ACLs are provisioned for the roster
> name alone, so any other value would publish into a stream that neither exists nor
> is granted, and midaz would swallow every one of those failures as a Warn while
> reporting healthy. The check runs whether or not `STREAMING_ENABLED` is set.

## Local testing

To exercise the real emit path against a broker, run the infra Redpanda and
point the ledger at it:

- Bind the broker on host port `19092`; join `infra-network` so it is reachable
  from both host (`localhost:19092`) and containers (`<container>:9092`).
- Set `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:19092`, and
  `STREAMING_CLOUDEVENTS_SOURCE=ledger`.
- Pre-provision `lerian.streaming.ledger` and `lerian.streaming.ledger.dlq`
  explicitly; do not rely on auto-create. There is no per-event topic list any
  more.

The default unit suite (`make test-unit`) never touches a broker — the
JSONShape and mapping tests marshal payloads in memory. See the `CLAUDE.md`
Streaming → Local testing section for the broker/environment conventions.
