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

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming).
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** ledger (`components/ledger`). Fees are embedded in the ledger
  binary; there is no standalone fees service.
- **CloudEvents source (`ce-source`):** `lerian.midaz.ledger`.
- **Posture:** all 7 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the HTTP request**. Durability of the mutation itself
  is owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only, identical to the rest of the
  ledger streaming surface. When an outbox lands, only the emit call sites
  change; the Definitions and payload contracts below stay put.
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled — or
  when `STREAMING_BROKERS` is empty, or no events are registered — bootstrap
  injects a `NoopEmitter` and no broker connection is attempted.
- **Local broker:** the infra Redpanda. Set `STREAMING_ENABLED=true` and
  `STREAMING_BROKERS=localhost:19092` to exercise the real emit path locally.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `midazEventDefinitions()`
(`components/ledger/internal/bootstrap/streaming.go`), which feeds both the
Catalog (`buildCatalog`) and the route table (`buildRoutes`):

- **Event key** = `<resourceType>.<eventType>` via `Definition.Key()` (e.g.
  `fee-packages.created`).
- **`ce-type`** = lib-streaming auto-prefixes the key: `studio.lerian.<key>`
  (stays hyphenated, e.g. `studio.lerian.fee-packages.created`).
- **Kafka topic** = `pkgStreaming.TopicName("fee", key)` =
  `lerian.streaming.<service>_<resource>.<event>` — the producing-service
  segment (`fee`) is folded in and hyphens in the resource/event become
  underscores in the topic name only (e.g. `lerian.streaming.fee_packages.created`).
  The service segment is `fee` even though fees ride the ledger binary; the
  process-wide `ce-source` stays `lerian.midaz.ledger`.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` inside `EmitImportant` (see
  [ce-tenantid](#ce-tenantid)).

## Event summary

All 7 events carry `SchemaVersion = 1.0.0`.

| Event key | Resource / Event | `ce-type` | Kafka topic | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|-------------|--------------|--------------------|
| `fee-packages.created` | fee-packages / created | `studio.lerian.fee-packages.created` | `lerian.streaming.fee_packages.created` | package ID | create fee package |
| `fee-packages.updated` | fee-packages / updated | `studio.lerian.fee-packages.updated` | `lerian.streaming.fee_packages.updated` | package ID | update fee package |
| `fee-packages.deleted` | fee-packages / deleted | `studio.lerian.fee-packages.deleted` | `lerian.streaming.fee_packages.deleted` | package ID | delete fee package |
| `fee-billing-packages.created` | fee-billing-packages / created | `studio.lerian.fee-billing-packages.created` | `lerian.streaming.fee_billing_packages.created` | billing package ID | create billing package |
| `fee-billing-packages.updated` | fee-billing-packages / updated | `studio.lerian.fee-billing-packages.updated` | `lerian.streaming.fee_billing_packages.updated` | billing package ID | update billing package |
| `fee-billing-packages.deleted` | fee-billing-packages / deleted | `studio.lerian.fee-billing-packages.deleted` | `lerian.streaming.fee_billing_packages.deleted` | billing package ID | delete billing package |
| `fee-charge.applied` | fee-charge / applied | `studio.lerian.fee-charge.applied` | `lerian.streaming.fee_charge.applied` | **transaction ID** | fee charged on a posted transaction |

> **Hyphen in the key/ce-type, underscore in the topic.** The `fee-packages`
> and `fee-billing-packages` resource types are hyphenated. The lib-streaming
> route-key validator rejects underscores, so the event **key** and **`ce-type`**
> keep the hyphen; only the derived **Kafka topic** name substitutes underscores
> for hyphens (and folds in the `fee` service segment).

> **`ce-subject` on `fee-charge.applied`.** The aggregate is the transaction the fee
> was charged against, so `ce-subject` is the **transaction ID**, and the
> charged fee package's ID travels in the body as `feePackageId`. The
> package/billing-package events use their own record ID as subject.

## Payload contracts

The wire keys below are the exact JSON field set produced by the Payload structs
in `pkg/streaming/events/`. The "field count" is the number the JSONShape test
locks.

### `fee-packages.created` / `fee-packages.updated` — 8 fields

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

### `fee-packages.deleted` — 4 fields

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

### `fee-billing-packages.created` / `fee-billing-packages.updated` — 9 fields

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

### `fee-billing-packages.deleted` — 4 fields

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

### `fee-charge.applied` — 5 fields

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

## `fee-charge.applied` semantics

`fee-charge.applied` is a charge signal, not a transaction signal:

- **Charged only.** It is emitted only when a fee was actually **charged** —
  `emitFeesAppliedEvent` fires only when `feeApplied=true` and a non-empty
  `packageAppliedID` are present in the transaction metadata (set by the fee
  engine on the real-charge branch). A pure exemption carries neither, so **no
  event is emitted on exemption**.
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

## Local testing

To exercise the real emit path against a broker, run the infra Redpanda and
point the ledger at it:

- Bind the broker on host port `19092`; join `infra-network` so it is reachable
  from both host (`localhost:19092`) and containers (`<container>:9092`).
- Set `STREAMING_ENABLED=true`, `STREAMING_BROKERS=localhost:19092`, and
  `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger`.
- Pre-provision topics explicitly; do not rely on auto-create.

The default unit suite (`make test-unit`) never touches a broker — the
JSONShape and mapping tests marshal payloads in memory. See the `CLAUDE.md`
Streaming → Local testing section for the broker/environment conventions.
