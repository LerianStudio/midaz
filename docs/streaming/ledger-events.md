# Ledger Streaming Event Catalog

Canonical reference for every streaming event the **Ledger** component emits. It
complements — does not duplicate — the producer conventions in `CLAUDE.md`
(Streaming section) and `docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/*.go`, and the field-count assertions in the matching
> `*_test.go` JSONShape tests are ONE contract. A wire change updates all three
> in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v3.1.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** Ledger (`components/ledger`).
- **Application name / CloudEvents source (`ce-source`):** `ledger`. It must be ONE
  dot-free lowercase segment matching `^[a-z0-9][a-z0-9_-]*$`, at most 223 bytes; a
  malformed value is REJECTED at startup, never normalized. The resolved value is
  load-bearing three times over — it is stamped as `ce-source`, it derives the one
  topic every event rides, and it is what the streaming manifest advertises — so
  changing `STREAMING_CLOUDEVENTS_SOURCE` moves all three together.
- **Kafka topics:** ONE topic per producing application. Every event this binary
  emits — ledger core, fees, and CRM; every resource type, every event type, every
  schema version — rides `lerian.streaming.ledger`, with
  `lerian.streaming.ledger.dlq` as its single dead-letter topic. There is no
  per-event topic and no `.v<major>` topic suffix: `ce-schemaversion` is the only
  version carrier on the wire. Consumers subscribe to the application and dispatch
  on the event key. (The one destination the binary writes outside this pair is the
  billing event's fixed topic — see
  [`docs/architecture/billing-active-account-streaming.md`](../architecture/billing-active-account-streaming.md).)
- **Posture:** all 35 events invoke `pkgStreaming.EmitBrokerBestEffort` at the
  post-commit slot. The helper bounds the synchronous `Emitter.Emit` call, records
  build/emit failures on the span, logs a Warn, and **never fails the HTTP request**.
  The library, not the wrapper, resolves the delivery policy and any configured
  fallback behavior.
- **No Midaz transactional outbox.** Bootstrap passes neither an outbox writer nor
  repository, and registers no relay. The helper does not persist a local fallback record or
  make the database mutation and broker delivery atomic; do not infer a delivery
  guarantee beyond the configured lib-streaming policy. Definitions and payload
  contracts remain unchanged.
- **HTTP event-manifest endpoint.** The ledger binary serves
  `GET /v1/streaming/manifest` (auth `streaming-manifest`/`get`) — a catalog-only
  view of the registered event Definitions, at manifest wire version `1.0.0`. The
  document carries `publisher.source` plus the application's `topic` / `dlqTopic`
  pair at DOCUMENT level (no commands queue: the ledger emits facts only), and each
  event entry names its `eventKey` (`"<resourceType>.<eventType>"` — the consumer's
  dispatch selector), its `schemaVersion`, and its `class`, always `"fact"` here.
  The topic it advertises is derived from the same `ce-source` the emitter publishes
  under, so what the streaming hub discovers is by construction the stream the
  ledger writes. It is independent of `STREAMING_ENABLED` and degraded-safe (it
  reflects the static Catalog, not a live broker connection); an illegal `ce-source`
  leaves the route unmounted rather than advertising a topic built from a malformed
  name. The billing event is excluded — it rides a fixed topic owned by
  lib-streaming, not the ledger's application topic.
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled, bootstrap
  injects a `NoopEmitter` and no broker connection is attempted. The ledger has no
  streaming readiness prober, so `/readyz` carries no streaming check in either
  mode (only the tracer reports one). `STREAMING_ENABLED=true` with an empty
  `STREAMING_BROKERS` REFUSES BOOT (`pkgStreaming.RequireBrokers`, which validates
  only the broker list): an enabled producer with nowhere to publish discards
  every event silently while readiness stays green, which is the same
  invisible-total-loss failure the roster source gate exists to kill. To run
  without streaming, set `STREAMING_ENABLED=false`.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `midazEventDefinitions()` (`components/ledger/internal/bootstrap/streaming.go`),
which feeds both the Catalog and the manifest:

- **Event key** = `<resourceType>.<eventType>` (e.g. `balance.changed`) — the
  dispatch selector a consumer registers a handler under inside the `ledger`
  stream. Underscore-preserving.
- **`ce-type`** = `studio.lerian.<app>.<resourceType>.<eventType>`, i.e.
  `studio.lerian.ledger.<key>` (e.g.
  `studio.lerian.ledger.operation_route.created`). The application segment is what
  keeps `ce-type` globally unambiguous: without it, two services emitting a
  same-named event produce byte-identical values — a homonym collision a consumer
  reading only `ce-type` cannot detect, and one that a shared per-application topic
  makes reachable in practice.
- **Kafka topic** = `lerian.streaming.ledger`, derived from `ce-source` via
  `libStreaming.AppTopic` and shared by the whole catalog. One catch-all route
  carries every fact; nothing fans out per event.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`). Five exceptions
  exist — see [ce-subject](#ce-subject).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` (see [ce-tenantid](#ce-tenantid)).

## Event summary

All 35 events carry `SchemaVersion = 1.0.0`. The `account_type.*` events are
intentionally NOT registered — the type label flows through `account.*` events
as a string field.

| Event key | Resource / Event | `ce-type` | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|--------------|--------------------|
| `organization.created` | organization / created | `studio.lerian.ledger.organization.created` | org ID | `CreateOrganization` |
| `organization.updated` | organization / updated | `studio.lerian.ledger.organization.updated` | org ID | `UpdateOrganizationByID` |
| `organization.deleted` | organization / deleted | `studio.lerian.ledger.organization.deleted` | org ID | `DeleteOrganizationByID` |
| `ledger.created` | ledger / created | `studio.lerian.ledger.ledger.created` | ledger ID | `CreateLedger` |
| `ledger.updated` | ledger / updated | `studio.lerian.ledger.ledger.updated` | ledger ID | `UpdateLedgerByID` |
| `ledger.deleted` | ledger / deleted | `studio.lerian.ledger.ledger.deleted` | ledger ID | `DeleteLedgerByID` |
| `account.created` | account / created | `studio.lerian.ledger.account.created` | account ID | `CreateAccount` |
| `account.updated` | account / updated | `studio.lerian.ledger.account.updated` | account ID | `UpdateAccount` |
| `account.deleted` | account / deleted | `studio.lerian.ledger.account.deleted` | account ID | `DeleteAccountByID` |
| `asset.created` | asset / created | `studio.lerian.ledger.asset.created` | asset ID | `CreateAsset` |
| `asset.updated` | asset / updated | `studio.lerian.ledger.asset.updated` | asset ID | `UpdateAssetByID` |
| `asset.deleted` | asset / deleted | `studio.lerian.ledger.asset.deleted` | asset ID | `DeleteAssetByID` |
| `portfolio.created` | portfolio / created | `studio.lerian.ledger.portfolio.created` | portfolio ID | `CreatePortfolio` |
| `portfolio.updated` | portfolio / updated | `studio.lerian.ledger.portfolio.updated` | portfolio ID | `UpdatePortfolioByID` |
| `portfolio.deleted` | portfolio / deleted | `studio.lerian.ledger.portfolio.deleted` | portfolio ID | `DeletePortfolioByID` |
| `segment.created` | segment / created | `studio.lerian.ledger.segment.created` | segment ID | `CreateSegment` |
| `segment.updated` | segment / updated | `studio.lerian.ledger.segment.updated` | segment ID | `UpdateSegmentByID` |
| `segment.deleted` | segment / deleted | `studio.lerian.ledger.segment.deleted` | segment ID | `DeleteSegmentByID` |
| `operation_route.created` | operation_route / created | `studio.lerian.ledger.operation_route.created` | op-route ID | `CreateOperationRoute` |
| `operation_route.updated` | operation_route / updated | `studio.lerian.ledger.operation_route.updated` | op-route ID | `UpdateOperationRoute` |
| `operation_route.deleted` | operation_route / deleted | `studio.lerian.ledger.operation_route.deleted` | op-route ID | `DeleteOperationRouteByID` |
| `transaction_route.created` | transaction_route / created | `studio.lerian.ledger.transaction_route.created` | txn-route ID | `CreateTransactionRoute` |
| `transaction_route.updated` | transaction_route / updated | `studio.lerian.ledger.transaction_route.updated` | txn-route ID | `UpdateTransactionRoute` |
| `transaction_route.deleted` | transaction_route / deleted | `studio.lerian.ledger.transaction_route.deleted` | txn-route ID | `DeleteTransactionRouteByID` |
| `balance.created` | balance / created | `studio.lerian.ledger.balance.created` | balance ID | `CreateAdditionalBalance` |
| `balance.changed` | balance / changed | `studio.lerian.ledger.balance.changed` | **`transactionId:operationId`** | `SendBalanceChangedEvents` (per op, post-commit) |
| `balance.config_changed` | balance / config_changed | `studio.lerian.ledger.balance.config_changed` | balance ID† | `UseCase.Update` (`settings_updated`) + `ensureOverdraftBalance` (`overdraft_enabled`) |
| `balance.deleted` | balance / deleted | `studio.lerian.ledger.balance.deleted` | balance ID | `DeleteBalance` |
| `balance.overdraft_drawn` | balance / overdraft_drawn | `studio.lerian.ledger.balance.overdraft_drawn` | **`transactionId:operationId`** | `SendOverdraftEvents` (per companion op) |
| `balance.overdraft_repaid` | balance / overdraft_repaid | `studio.lerian.ledger.balance.overdraft_repaid` | **`transactionId:operationId`** | `SendOverdraftEvents` |
| `balance.overdraft_cleared` | balance / overdraft_cleared | `studio.lerian.ledger.balance.overdraft_cleared` | **`transactionId:operationId`** | `SendOverdraftEvents` |
| `transaction.posted` | transaction / posted | `studio.lerian.ledger.transaction.posted` | transaction ID | `SendTransactionEvents` (created, APPROVED, no parent) |
| `transaction.committed` | transaction / committed | `studio.lerian.ledger.transaction.committed` | transaction ID | `SendTransactionEvents` (updated, APPROVED) |
| `transaction.canceled` | transaction / canceled | `studio.lerian.ledger.transaction.canceled` | transaction ID | `SendTransactionEvents` (updated, CANCELED) |
| `transaction.reverted` | transaction / reverted | `studio.lerian.ledger.transaction.reverted` | **child** transaction ID | `SendTransactionEvents` (created, APPROVED, parent non-nil) |

† On `balance.config_changed` the `ce-subject` is the companion overdraft
balance's ID in the `overdraft_enabled` branch, not the parent's.

> **Underscores are preserved everywhere.** The `operation_route.*`,
> `transaction_route.*`, `balance.config_changed`, and
> `balance.overdraft_{drawn,repaid,cleared}` events are multi-word. Their **event
> key** (`Definition.Key()`) and **`ce-type`** are underscore-canonical, and that is
> what consumers see on the wire and register handlers under (e.g. event key
> `operation_route.created`, ce-type
> `studio.lerian.ledger.operation_route.created`). Nothing is folded anywhere: route
> keys accept underscores, so no event name has a hyphenated variant. Payload field
> *values* (e.g. `changeType="settings_updated"`) are snake_case for the same
> reason — they are payload data, not routing identifiers.

## ce-subject

Most events carry their own record ID as `ce-subject`. Five exceptions:

- **`balance.changed`** and the three **`balance.overdraft_*`** events carry the
  composite idempotency key `transactionId:operationId` — NOT the balance ID.
  This keys the event to the operation that caused the mutation, so consumers
  can deduplicate replayed emits.
- **`transaction.reverted`** carries the **child** (reversal) transaction's UUID
  as `ce-subject`; consumers correlate back to the original transaction via the
  `parentTransactionId` body field.

## Payload contracts

The wire keys below are the exact JSON field set produced by the Payload structs
in `pkg/streaming/events/`. The "field count" is the number the JSONShape test
locks (when it pins a total via `assert.Lenf`). A few events assert key-presence
plus specific absences instead of a total count — those are marked "no pinned
count" with the observed fixture count.

### Organization

#### `organization.created` / `organization.updated` — 9 / 7 fields

Source: `pkg/streaming/events/organization_created.go`, `organization_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | |
| `parentOrganizationId` | string \| null | ✓ | ✓ | JSON `null` when unset. |
| `legalName` | string | ✓ | ✓ | |
| `doingBusinessAs` | string \| null | ✓ | ✓ | |
| `legalDocument` | string | ✓ | — | Present on the wire (ledger does NOT redact legal document). |
| `address` | object | ✓ | ✓ | `line1`, `line2` (string\|null), `zipCode`, `city`, `state`, `country`, `description` (string\|null, omitted when nil). |
| `status` | object | ✓ | ✓ | `code`, `description` (string\|null, omitted when nil). |
| `createdAt` | string | ✓ | — | RFC3339. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. `organization.updated` stamps `UpdatedAt` as `ce-time`. |

#### `organization.deleted` — 2 fields

Source: `pkg/streaming/events/organization_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Organization ID. |
| `deletedAt` | string | RFC3339. |

> **No `deletionType`.** Ledger `*.deleted` payloads do not carry a soft/hard
> discriminator (unlike CRM's `holder.deleted`). Deletion is minimal: `id` plus
> scope IDs and `deletedAt`.

### Ledger

#### `ledger.created` / `ledger.updated` — 6 / 5 fields

Source: `pkg/streaming/events/ledger_created.go`, `ledger_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | |
| `organizationId` | string | ✓ | ✓ | |
| `name` | string | ✓ | ✓ | |
| `status` | object | ✓ | ✓ | `code`, `description` (string\|null, omitted when nil). |
| `createdAt` | string | ✓ | — | RFC3339. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> Ledger-settings updates (`update_ledger_settings.go`) are NOT covered by
> `ledger.updated`; settings changes are out of the v1 wire contract.

#### `ledger.deleted` — 3 fields

Source: `pkg/streaming/events/ledger_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Ledger ID. |
| `organizationId` | string | Organization scope. |
| `deletedAt` | string | RFC3339. |

### Account

#### `account.created` — 15 fields

Source: `pkg/streaming/events/account_created.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | |
| `organizationId` | string | |
| `ledgerId` | string | |
| `name` | string | |
| `assetCode` | string | |
| `type` | string | Account type label (string); `account_type.*` events are not emitted. |
| `portfolioId` | string \| null | JSON `null` when unset. |
| `segmentId` | string \| null | |
| `parentAccountId` | string \| null | |
| `entityId` | string \| null | |
| `alias` | string \| null | |
| `status` | object | `code`, `description` (string\|null, omitted when nil). |
| `blocked` | bool \| null | Pointer so absence is distinguishable from explicit `false`; emitted as `false` when unset. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

> The implicit default balance auto-created by `CreateAccount` does NOT generate
> a `balance.created` event; the account lifecycle is the signal.

#### `account.updated` — 10 fields

Source: `pkg/streaming/events/account_updated.go`. Drops `assetCode`, `type`,
`parentAccountId`, `alias`, `createdAt` from the created payload.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | |
| `organizationId` | string | |
| `ledgerId` | string | |
| `name` | string | |
| `portfolioId` | string \| null | |
| `segmentId` | string \| null | |
| `entityId` | string \| null | |
| `status` | object | `code`, `description` (string\|null, omitted when nil). |
| `blocked` | bool \| null | |
| `updatedAt` | string | RFC3339. `id + updatedAt` is unique per mutation. |

> External-type accounts never reach the `account.updated` or `account.deleted`
> emit anchors.

#### `account.deleted` — 5 fields

Source: `pkg/streaming/events/account_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Account ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `portfolioId` | string \| null | `null` when not portfolio-scoped. |
| `deletedAt` | string | RFC3339. Wall-clock NOW() captured at the emit site. |

> The cascade `DeleteAllBalancesByAccountID` does NOT generate per-balance
> `balance.deleted` events; the user-visible fact is the account removal.

### Asset

#### `asset.created` / `asset.updated` — 9 / 8 fields

Source: `pkg/streaming/events/asset_created.go`, `asset_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | |
| `organizationId` | string | ✓ | ✓ | |
| `ledgerId` | string | ✓ | ✓ | |
| `name` | string | ✓ | ✓ | |
| `type` | string | ✓ | ✓ | Immutable post-create; mirrored so the payload is a complete identity snapshot. |
| `code` | string | ✓ | ✓ | Immutable post-create; mirrored. |
| `status` | object | ✓ | ✓ | `code`, `description` (string\|null, omitted when nil). |
| `createdAt` | string | ✓ | — | RFC3339. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> The implicit external account auto-created by `CreateAsset` does NOT generate
> a separate `account.created` event (it goes through `AccountRepo` directly,
> not `UseCase.CreateAccount`).

#### `asset.deleted` — 4 fields

Source: `pkg/streaming/events/asset_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Asset ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `deletedAt` | string | RFC3339. |

> The cascade-delete of the implicit external account does NOT produce a separate
> `account.deleted` event.

### Portfolio

#### `portfolio.created` / `portfolio.updated` — 7 (or 8) / 6 fields

Source: `pkg/streaming/events/portfolio_created.go`, `portfolio_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | |
| `organizationId` | string | ✓ | ✓ | |
| `ledgerId` | string | ✓ | ✓ | |
| `name` | string | ✓ | ✓ | |
| `entityId` | string | ✓ | ✓ | `omitempty` (string, not pointer) — omitted when empty. Test asserts absent when empty, present when set. |
| `status` | object | ✓ | ✓ | `code`, `description` (string\|null, omitted when nil). |
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `portfolio.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> `portfolio.created` field count is 7 when `entityId` is empty, 8 when set.

### Segment

#### `segment.created` / `segment.updated` — 7 / 6 fields

Source: `pkg/streaming/events/segment_created.go`, `segment_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | |
| `organizationId` | string | ✓ | ✓ | |
| `ledgerId` | string | ✓ | ✓ | |
| `name` | string | ✓ | ✓ | |
| `status` | object | ✓ | ✓ | `code`, `description` (string\|null, omitted when nil). |
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `segment.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

#### `segment.deleted` — 4 fields

Source: `pkg/streaming/events/segment_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Segment ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `deletedAt` | string | RFC3339. |

### Operation route

#### `operation_route.created` / `operation_route.updated` — 7 (or 11) / 6 fields

Source: `pkg/streaming/events/operation_route_created.go`,
`operation_route_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | UUID stringified. |
| `organizationId` | string | ✓ | ✓ | |
| `ledgerId` | string | ✓ | ✓ | |
| `title` | string | ✓ | ✓ | |
| `description` | string | ✓ | ✓ | `omitempty` — omitted when empty. |
| `code` | string | ✓ | ✓ | `omitempty`. Legacy field (`//nolint:staticcheck`); emitted for backward compatibility. |
| `operationType` | string | ✓ | ✓ | e.g. `source` / `destination`. |
| `account` | object \| null | ✓ | ✓ | `omitempty` when nil. Nested: `ruleType` (string, `omitempty`) + `validIf` (any, `omitempty`). |
| `accountingEntries` | object \| null | ✓ | ✓ | `omitempty` when nil. Nested `direct`/`hold`/`commit`/`cancel`/`revert`/`overdraft`/`block`/`unblock` — each `*AccountingEntry` (`omitempty`); each entry has `debit`/`credit` (`*AccountingRubric`, `null` when nil); `AccountingRubric` = `code` + `description`. |
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `operation_route.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> `operation_route.created` field count is 7 when all optionals are empty/nil,
> 11 when `description`, `code`, `account`, and `accountingEntries` are all set.

#### `operation_route.deleted` — 4 fields

Source: `pkg/streaming/events/operation_route_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Operation-route ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `deletedAt` | string | RFC3339. |

### Transaction route

#### `transaction_route.created` / `transaction_route.updated` — 7 (or 8) / 6 fields

Source: `pkg/streaming/events/transaction_route_created.go`,
`transaction_route_updated.go`.

| Key | Type | `created` | `updated` | Notes |
|-----|------|:---------:|:---------:|-------|
| `id` | string | ✓ | ✓ | UUID stringified. |
| `organizationId` | string | ✓ | ✓ | |
| `ledgerId` | string | ✓ | ✓ | |
| `title` | string | ✓ | ✓ | |
| `description` | string | ✓ | ✓ | `omitempty` — omitted when empty. |
| `operationRouteIds` | []string | ✓ | ✓ | `omitempty`. POST-UPDATE list (not a diff) on `updated` — consumers replace their cached join set. Derived from `OperationRoutes[].ID`. |
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `transaction_route.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> `transaction_route.created` field count is 7 when `description` is empty, 8
> when set. `operationRouteIds` is always non-nil in practice (create requires
> ≥1 op route) but `omitempty` guards against a future validation loosening.

#### `transaction_route.deleted` — 4 fields

Source: `pkg/streaming/events/transaction_route_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Transaction-route ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `deletedAt` | string | RFC3339. |

> The cascade soft-delete of `operation_transaction_route` relations does NOT
> generate per-relation events.

### Balance

#### `balance.created` — 15 (or 16 with settings)

Source: `pkg/streaming/events/balance_created.go`. Trigger: `CreateAdditionalBalance`
(POST `.../accounts/:account_id/balances`).

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | |
| `organizationId` | string | |
| `ledgerId` | string | |
| `accountId` | string | |
| `alias` | string | `omitempty` when empty. |
| `key` | string | e.g. `default`. |
| `assetCode` | string | |
| `accountType` | string | `omitempty` when empty. |
| `available` | decimal | Always zero on create; always present. |
| `onHold` | decimal | Always zero on create; always present. |
| `allowSending` | bool | |
| `allowReceiving` | bool | |
| `direction` | string | `omitempty` when empty. |
| `settings` | object | Omitted (not `null`) when nil. `balanceScope` (string, `omitempty`), `allowOverdraft` (bool), `overdraftLimitEnabled` (bool), `overdraftLimit` (string, `omitempty`). (`BalanceSettingsPayload`, `pkg/streaming/events/balance_created.go`) |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

> **`scale` is intentionally omitted** (asset-level property). Test asserts
> `scale` ABSENT.

> **Suppressed paths.** `CreateDefaultBalance` (implicit default-balance
> auto-create from `CreateAccount`/`CreateAsset`) and `ensureOverdraftBalance`
> (system-managed overdraft companion) do NOT emit `balance.created`. The former
> emits `account.created`/`asset.created`; the latter emits
> `balance.config_changed` with `changeType=overdraft_enabled`.

#### `balance.changed` — 17 fields

Source: `pkg/streaming/events/balance_changed.go`. Trigger:
`SendBalanceChangedEvents`, fired **per balance-affecting operation** of a
committed transaction (3-goroutine post-commit cascade), gated by
`op.BalanceAffected`.

> **Relationship to `balance.created`:** COMPLEMENTARY, not a replacement.
> `balance.created` fires only on lifecycle create; `balance.changed` fires on
> transaction-driven mutations of an existing balance. Domain-agnostic by
> design — carries only Midaz identities + a generic `Reason`; no
> consumer-domain fields.

| Key | Type | Notes |
|-----|------|-------|
| `organizationId` | string | |
| `ledgerId` | string | |
| `accountId` | string | |
| `balanceId` | string | |
| `alias` | string | `omitempty` when empty. |
| `assetCode` | string | |
| `balanceKey` | string | |
| `available` | decimal | State AFTER the operation. Always on wire (even when zero). |
| `onHold` | decimal | State AFTER the operation. Always on wire. |
| `version` | int64 | For ordering/dedup. Always on wire (even when zero). |
| `reason` | string | One of: `credit`, `debit`, `block`, `unblock`, `hold`, `release`, `overdraft`, `adjust` (`adjust` is the fallback for unknown op types). |
| `operationType` | string | Midaz op type (e.g. `CREDIT`, `DEBIT`). |
| `direction` | string | `omitempty` when empty. |
| `amount` | decimal | Always on wire (zero when unset). |
| `transactionId` | string | |
| `operationId` | string | |
| `occurredAt` | string | RFC3339. |

> **`scale` is intentionally omitted** — test asserts `scale` ABSENT. Test build
> tag: `//go:build unit`; white-box package `events`.

#### `balance.config_changed` — no pinned count (11 in fixture)

Source: `pkg/streaming/events/balance_config_changed.go`. Trigger: `UseCase.Update`
(`update_balance.go`), with **two emission branches**:

1. **`changeType = settings_updated`** — ordinary settings PATCH
   (`AllowSending`, `AllowReceiving`, `Settings.*`). `id` = the updated parent
   balance.
2. **`changeType = overdraft_enabled`** — emitted exactly once on the
   false→true overdraft transition, from `ensureOverdraftBalance`. `id` = the
   **newly-materialized companion overdraft balance** (the companion's identity
   becoming known IS the "config changed" signal). This event substitutes for a
   `balance.created` on the companion (suppressed because companions are
   system-managed).

> A single PATCH flipping `AllowOverdraft` false→true produces **TWO**
> `config_changed` events: `overdraft_enabled` (companion) then
> `settings_updated` (parent). Ordering enforced by the use case
> (`ensureOverdraftBalance` runs before `BalanceRepo.Update`). Internal-scope
> balances cannot be updated via the public API (rejected by the scope guard).

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Parent balance (`settings_updated`) OR companion balance (`overdraft_enabled`). |
| `organizationId` | string | |
| `ledgerId` | string | |
| `accountId` | string | |
| `alias` | string | `omitempty` when empty. |
| `key` | string | |
| `allowSending` | bool | |
| `allowReceiving` | bool | |
| `direction` | string | `omitempty` when empty. |
| `settings` | object | Omitted (not `null`) when nil. Same nested shape as `balance.created`. |
| `changeType` | string | `settings_updated` or `overdraft_enabled`. Snake_case value is payload data, not a routing identifier. |
| `updatedAt` | string | RFC3339. |

> **Money fields `available`/`onHold` are intentionally omitted** — config
> mutation signal, not money movement. Test asserts both ABSENT.

#### `balance.deleted` — 5 fields

Source: `pkg/streaming/events/balance_deleted.go`. Trigger: `DeleteBalance`
(explicit `DELETE .../balances/:balance_id`).

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Balance ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `accountId` | string | |
| `deletedAt` | string | RFC3339. Wall-clock NOW() matching the SQL `UPDATE balance SET deleted_at = NOW()`. |

> **Suppressed paths.** Cascade delete via `account.deleted`
> (`DeleteAllByIDs`) does NOT emit per-balance events; internal-scope balance
> deletion is impossible by API contract; balances with non-zero
> `Available`/`OnHold` cannot be deleted.

#### `balance.overdraft_drawn` / `balance.overdraft_repaid` / `balance.overdraft_cleared` — no pinned count (11 in fixture)

Source: `pkg/streaming/events/balance_overdraft.go`. Shared payload across the
three events. Trigger: `SendOverdraftEvents`, called in the same post-commit
cascade as `SendTransactionEvents`. Scans `tran.Operations` for overdraft
companion ops (`BalanceKey == "overdraft"`), classifies via
`classifyOverdraftOperation`, and emits one event per companion op:

- **`drawn`** — `op.Direction == DirectionDebit` on a direction=debit companion
  (overdraft consumed).
- **`repaid`** — `op.Direction == DirectionCredit` AND companion's after-Avail
  is non-zero (overdraft decreased but not fully cleared).
- **`cleared`** — `op.Direction == DirectionCredit` AND after-Avail is zero
  (terminal signal — fully repaid).

> Emission ordering: the lib-streaming emit MUST occur AFTER the parent
> `transaction.{posted,committed,reverted}` in the same cascade. Coexists with
> the legacy `transaction.overdraft_events` rabbit publish during the cutover
> window (`RABBITMQ_OVERDRAFT_EVENTS_ENABLED`).

| Key | Type | Notes |
|-----|------|-------|
| `balanceId` | string | The companion overdraft balance ID. |
| `accountId` | string | |
| `organizationId` | string | |
| `ledgerId` | string | |
| `assetCode` | string | |
| `transactionId` | string | |
| `operationId` | string | |
| `action` | string | `drawn` / `repaid` / `cleared` — matches the EventType suffix; stamped by the per-event constructor. |
| `amount` | decimal | The overdraft movement (deficit added for `drawn`; amount repaid for `repaid`/`cleared`). |
| `overdraftBalance` | decimal | Companion balance's Available AFTER the operation = total outstanding overdraft. |
| `overdraftLimit` | decimal \| null | `omitempty`. Currently always nil — the persisted operation row does not yet carry the configured limit. Field exists so consumers can rely on the shape once the operation-snapshot extension lands. Test asserts ABSENT today. |
| `occurredAt` | string | RFC3339. |

> **`scale` is intentionally omitted** — test asserts ABSENT.

### Transaction

#### `transaction.posted` / `transaction.committed` / `transaction.canceled` / `transaction.reverted` — no pinned count (9 keys asserted; 14 in fixture)

Source: `pkg/streaming/events/transaction_lifecycle.go`. Shared payload across
the four events. Trigger: `SendTransactionEvents` →
`emitTransactionLifecycleEvent`, called in the post-commit cascade. The phase +
status discriminator selects the Definition:

| phase | parent tx | status | Definition |
|-------|-----------|--------|------------|
| `created` | nil | APPROVED | `transaction.posted` |
| `created` | non-nil | APPROVED | `transaction.reverted` |
| `created` | — | PENDING / NOTED / other | skipped |
| `updated` | — | APPROVED | `transaction.committed` |
| `updated` | — | CANCELED | `transaction.canceled` |
| `updated` | — | other | skipped |

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Transaction ID. |
| `parentTransactionId` | string \| null | `omitempty`. Absent on `posted`/`committed`/`canceled`; always present on `reverted` (the child carries the parent's UUID). |
| `organizationId` | string | |
| `ledgerId` | string | |
| `status` | object | `code`, `description` (string\|null, omitted when nil). |
| `amount` | decimal \| null | `omitempty`. Pointer because some PENDING txns have unset amount until ops resolve. |
| `assetCode` | string | |
| `chartOfAccountsGroupName` | string | `omitempty`. |
| `description` | string | `omitempty`. |
| `source` | []string | `omitempty`. |
| `destination` | []string | `omitempty`. |
| `route` | string | `omitempty`. Legacy field (`//nolint:staticcheck`; `routeId` is canonical). |
| `routeId` | string \| null | `omitempty`. |
| `operations` | array | Each operation marshalled verbatim by the caller so the events package stays decoupled from the internal `operation.Operation` type. Always present (no omitempty). |
| `metadata` | object | `omitempty`. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

> **`scale` is intentionally omitted** (asset-level) — test asserts ABSENT.

## Excluded by design

Ledger payloads carry financial identifiers (IDs, asset codes, amounts,
aliases). Some payloads do include fields that may contain human-identifying
data — `organization.created` carries `legalDocument` (CPF/CNPJ), and
`transaction.*` carries free-form `description` and `metadata`. Midaz does not
apply producer-level redaction on these fields today; the exclusions below are
structural choices, not a privacy guarantee.

- **`scale`** — omitted from every `balance.*` and `transaction.*` payload
  (asset-level property; consumers join against `asset.created`).
- **Money fields on `balance.config_changed`** — `available`/`onHold` are
  absent; config mutation is a separate signal family from money movement.
- **`createdAt`** — omitted on every `*.updated` event (the `updatedAt` stamp
  is the mutation marker).
- **`legalDocument`** — present on `organization.created` but dropped on
  `organization.updated`.
- **`deletionType`** — ledger `*.deleted` payloads carry no soft/hard
  discriminator (unlike CRM).
- **`overdraftLimit`** — `omitempty` on `balance.overdraft_*`; currently always
  nil until the operation-snapshot extension lands.

**Enforcement.** The `JSONShape` unit test in each event's `*_test.go` locks the
exact present-key set, pins the field count (where pinned), and asserts the
absence of every excluded key. Any excluded field added to a payload fails that
test.

## `ce-tenantid`

Every emission carries a `ce-tenantid` header sourced from
`pkgStreaming.ResolveTenantID(ctx)`:

- **Multi-tenant deployments:** the resolved tenant ID from the lib-commons
  multitenancy middleware (`tmcore.GetTenantIDContext`).
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

## Partitioning

lib-streaming picks a record's partition key by falling back through: system event
→ tenant → `ce-subject` → event id. Under one topic per application that has one
consequence worth stating plainly.

- **Multi-tenant deployments** key by the resolved tenant, so the stream spreads
  across `lerian.streaming.ledger`'s partitions and every tenant's events keep a
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

The default unit suite (`go test ./...` with no tag) never touches a broker —
the `JSONShape` tests in `pkg/streaming/events/*_test.go` lock the wire contract
offline.

> **Gap vs CRM.** The ledger has **no** build-tagged streaming integration test
> and **no** `test-streaming-integration` Makefile target today. The CRM
> counterpart (`components/crm/internal/bootstrap/streaming_integration_test.go`)
> is the reference pattern for adding one. Until it lands, the unit suite is the
> only wire-contract lock for the ledger.

For a longer-lived local broker (e.g. to point a running ledger service at it),
use the Redpanda compose in the `end-to-end` repo
(`docker-compose.redpanda.yaml`) and set `STREAMING_ENABLED=true` +
`STREAMING_BROKERS` + `STREAMING_CLOUDEVENTS_SOURCE=ledger` on the
ledger accordingly. Pre-provision exactly two topics —
`lerian.streaming.ledger` and `lerian.streaming.ledger.dlq` — and give the DLQ a
`max.message.bytes` at or above its source topic's, since a DLQ record is strictly
larger than the record it quarantines. Do not rely on auto-create.

See the `CLAUDE.md` Streaming → Local testing section for the
broker/environment conventions.
