# Ledger Streaming Event Catalog

Canonical reference for every streaming event the **Ledger** component emits. It
complements — does not duplicate — the producer conventions in `CLAUDE.md`
(Streaming section) and `docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/*.go`, and the field-count assertions in the matching
> `*_test.go` JSONShape tests are ONE contract. A wire change updates all three
> in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v1.9.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** Ledger (`components/ledger`).
- **CloudEvents source (`ce-source`):** `lerian.midaz.ledger`.
- **Posture:** all 35 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the HTTP request**. Durability of the mutation itself
  is owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only. The `transaction.*`,
  `balance.changed`, and `balance.overdraft-*` docstrings mark their catalog
  posture as CRITICAL (outbox: always, direct: skip), but the outbox is not
  wired today (`WithOutboxRepository` is not passed at build). When an outbox
  lands, only the emit call sites change; the Definitions and payload contracts
  below stay put.
- **No HTTP event-manifest endpoint.** Out of scope for this pilot (same as the
  CRM).
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled — or
  when `STREAMING_BROKERS` is empty, or no events are registered — bootstrap
  injects a `NoopEmitter` and no broker connection is attempted.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `midazEventDefinitions()` (`components/ledger/internal/bootstrap/streaming.go`),
which feeds both the Catalog and the route table:

- **Event key** = `<resourceType>.<eventType>` (e.g. `balance.changed`).
- **`ce-type`** = lib-streaming auto-prefixes the key: `studio.lerian.<key>`.
- **Kafka topic** = `lerian.streaming.ledger_<key>`, with hyphens in `<key>`
  converted to underscores in the topic name only (e.g.
  `lerian.streaming.ledger_operation_route.created`). The event key and
  `ce-type` keep the hyphen.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`). Five exceptions
  exist — see [ce-subject](#ce-subject).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` (see [ce-tenantid](#ce-tenantid)).

## Event summary

All 35 events carry `SchemaVersion = 1.0.0`. The `account_type.*` events are
intentionally NOT registered — the type label flows through `account.*` events
as a string field.

| Event key | Resource / Event | `ce-type` | Kafka topic | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|-------------|--------------|--------------------|
| `organization.created` | organization / created | `studio.lerian.organization.created` | `lerian.streaming.ledger_organization.created` | org ID | `CreateOrganization` |
| `organization.updated` | organization / updated | `studio.lerian.organization.updated` | `lerian.streaming.ledger_organization.updated` | org ID | `UpdateOrganizationByID` |
| `organization.deleted` | organization / deleted | `studio.lerian.organization.deleted` | `lerian.streaming.ledger_organization.deleted` | org ID | `DeleteOrganizationByID` |
| `ledger.created` | ledger / created | `studio.lerian.ledger.created` | `lerian.streaming.ledger_ledger.created` | ledger ID | `CreateLedger` |
| `ledger.updated` | ledger / updated | `studio.lerian.ledger.updated` | `lerian.streaming.ledger_ledger.updated` | ledger ID | `UpdateLedgerByID` |
| `ledger.deleted` | ledger / deleted | `studio.lerian.ledger.deleted` | `lerian.streaming.ledger_ledger.deleted` | ledger ID | `DeleteLedgerByID` |
| `account.created` | account / created | `studio.lerian.account.created` | `lerian.streaming.ledger_account.created` | account ID | `CreateAccount` |
| `account.updated` | account / updated | `studio.lerian.account.updated` | `lerian.streaming.ledger_account.updated` | account ID | `UpdateAccount` |
| `account.deleted` | account / deleted | `studio.lerian.account.deleted` | `lerian.streaming.ledger_account.deleted` | account ID | `DeleteAccountByID` |
| `asset.created` | asset / created | `studio.lerian.asset.created` | `lerian.streaming.ledger_asset.created` | asset ID | `CreateAsset` |
| `asset.updated` | asset / updated | `studio.lerian.asset.updated` | `lerian.streaming.ledger_asset.updated` | asset ID | `UpdateAssetByID` |
| `asset.deleted` | asset / deleted | `studio.lerian.asset.deleted` | `lerian.streaming.ledger_asset.deleted` | asset ID | `DeleteAssetByID` |
| `portfolio.created` | portfolio / created | `studio.lerian.portfolio.created` | `lerian.streaming.ledger_portfolio.created` | portfolio ID | `CreatePortfolio` |
| `portfolio.updated` | portfolio / updated | `studio.lerian.portfolio.updated` | `lerian.streaming.ledger_portfolio.updated` | portfolio ID | `UpdatePortfolioByID` |
| `portfolio.deleted` | portfolio / deleted | `studio.lerian.portfolio.deleted` | `lerian.streaming.ledger_portfolio.deleted` | portfolio ID | `DeletePortfolioByID` |
| `segment.created` | segment / created | `studio.lerian.segment.created` | `lerian.streaming.ledger_segment.created` | segment ID | `CreateSegment` |
| `segment.updated` | segment / updated | `studio.lerian.segment.updated` | `lerian.streaming.ledger_segment.updated` | segment ID | `UpdateSegmentByID` |
| `segment.deleted` | segment / deleted | `studio.lerian.segment.deleted` | `lerian.streaming.ledger_segment.deleted` | segment ID | `DeleteSegmentByID` |
| `operation-route.created` | operation-route / created | `studio.lerian.operation-route.created` | `lerian.streaming.ledger_operation_route.created` | op-route ID | `CreateOperationRoute` |
| `operation-route.updated` | operation-route / updated | `studio.lerian.operation-route.updated` | `lerian.streaming.ledger_operation_route.updated` | op-route ID | `UpdateOperationRoute` |
| `operation-route.deleted` | operation-route / deleted | `studio.lerian.operation-route.deleted` | `lerian.streaming.ledger_operation_route.deleted` | op-route ID | `DeleteOperationRouteByID` |
| `transaction-route.created` | transaction-route / created | `studio.lerian.transaction-route.created` | `lerian.streaming.ledger_transaction_route.created` | txn-route ID | `CreateTransactionRoute` |
| `transaction-route.updated` | transaction-route / updated | `studio.lerian.transaction-route.updated` | `lerian.streaming.ledger_transaction_route.updated` | txn-route ID | `UpdateTransactionRoute` |
| `transaction-route.deleted` | transaction-route / deleted | `studio.lerian.transaction-route.deleted` | `lerian.streaming.ledger_transaction_route.deleted` | txn-route ID | `DeleteTransactionRouteByID` |
| `balance.created` | balance / created | `studio.lerian.balance.created` | `lerian.streaming.ledger_balance.created` | balance ID | `CreateAdditionalBalance` |
| `balance.changed` | balance / changed | `studio.lerian.balance.changed` | `lerian.streaming.ledger_balance.changed` | **`transactionId:operationId`** | `SendBalanceChangedEvents` (per op, post-commit) |
| `balance.config-changed` | balance / config-changed | `studio.lerian.balance.config-changed` | `lerian.streaming.ledger_balance.config_changed` | balance ID† | `UseCase.Update` (`settings_updated`) + `ensureOverdraftBalance` (`overdraft_enabled`) |
| `balance.deleted` | balance / deleted | `studio.lerian.balance.deleted` | `lerian.streaming.ledger_balance.deleted` | balance ID | `DeleteBalance` |
| `balance.overdraft-drawn` | balance / overdraft-drawn | `studio.lerian.balance.overdraft-drawn` | `lerian.streaming.ledger_balance.overdraft_drawn` | **`transactionId:operationId`** | `SendOverdraftEvents` (per companion op) |
| `balance.overdraft-repaid` | balance / overdraft-repaid | `studio.lerian.balance.overdraft-repaid` | `lerian.streaming.ledger_balance.overdraft_repaid` | **`transactionId:operationId`** | `SendOverdraftEvents` |
| `balance.overdraft-cleared` | balance / overdraft-cleared | `studio.lerian.balance.overdraft-cleared` | `lerian.streaming.ledger_balance.overdraft_cleared` | **`transactionId:operationId`** | `SendOverdraftEvents` |
| `transaction.posted` | transaction / posted | `studio.lerian.transaction.posted` | `lerian.streaming.ledger_transaction.posted` | transaction ID | `SendTransactionEvents` (created, APPROVED, no parent) |
| `transaction.committed` | transaction / committed | `studio.lerian.transaction.committed` | `lerian.streaming.ledger_transaction.committed` | transaction ID | `SendTransactionEvents` (updated, APPROVED) |
| `transaction.canceled` | transaction / canceled | `studio.lerian.transaction.canceled` | `lerian.streaming.ledger_transaction.canceled` | transaction ID | `SendTransactionEvents` (updated, CANCELED) |
| `transaction.reverted` | transaction / reverted | `studio.lerian.transaction.reverted` | `lerian.streaming.ledger_transaction.reverted` | **child** transaction ID | `SendTransactionEvents` (created, APPROVED, parent non-nil) |

† On `balance.config-changed` the `ce-subject` is the companion overdraft
balance's ID in the `overdraft_enabled` branch, not the parent's.

> **Hyphen, not underscore.** The `operation-route.*`, `transaction-route.*`,
> `balance.config-changed`, and `balance.overdraft-{drawn,repaid,cleared}` event
> types are hyphenated. The lib-streaming route-key validator rejects
> underscores, so the key, topic, and `ce-type` all keep the hyphen. Payload
> field *values* (e.g. `changeType="settings_updated"`) may keep snake_case
> because they are payload data, not routing identifiers.

## ce-subject

Most events carry their own record ID as `ce-subject`. Five exceptions:

- **`balance.changed`** and the three **`balance.overdraft-*`** events carry the
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

#### `operation-route.created` / `operation-route.updated` — 7 (or 11) / 6 fields

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
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `operation-route.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> `operation-route.created` field count is 7 when all optionals are empty/nil,
> 11 when `description`, `code`, `account`, and `accountingEntries` are all set.

#### `operation-route.deleted` — 4 fields

Source: `pkg/streaming/events/operation_route_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Operation-route ID. |
| `organizationId` | string | |
| `ledgerId` | string | |
| `deletedAt` | string | RFC3339. |

### Transaction route

#### `transaction-route.created` / `transaction-route.updated` — 7 (or 8) / 6 fields

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
| `createdAt` | string | ✓ | — | RFC3339. Test asserts `createdAt` ABSENT on `transaction-route.updated`. |
| `updatedAt` | string | ✓ | ✓ | RFC3339. |

> `transaction-route.created` field count is 7 when `description` is empty, 8
> when set. `operationRouteIds` is always non-nil in practice (create requires
> ≥1 op route) but `omitempty` guards against a future validation loosening.

#### `transaction-route.deleted` — 4 fields

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
| `settings` | object \| null | `omitempty` when nil. `balanceScope` (string, `omitempty`), `allowOverdraft` (bool), `overdraftLimitEnabled` (bool), `overdraftLimit` (string\|null, `omitempty`). |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. |

> **`scale` is intentionally omitted** (asset-level property). Test asserts
> `scale` ABSENT.

> **Suppressed paths.** `CreateDefaultBalance` (implicit default-balance
> auto-create from `CreateAccount`/`CreateAsset`) and `ensureOverdraftBalance`
> (system-managed overdraft companion) do NOT emit `balance.created`. The former
> emits `account.created`/`asset.created`; the latter emits
> `balance.config-changed` with `changeType=overdraft_enabled`.

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

#### `balance.config-changed` — no pinned count (11 in fixture)

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
> `config-changed` events: `overdraft_enabled` (companion) then
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
| `settings` | object \| null | `omitempty` when nil. Same nested shape as `balance.created`. |
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

#### `balance.overdraft-drawn` / `balance.overdraft-repaid` / `balance.overdraft-cleared` — no pinned count (11 in fixture)

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

> Coexists with the legacy `transaction.transaction_events` rabbit publish
> during cutover (`RABBITMQ_TRANSACTION_EVENTS_ENABLED`); the flag short-circuits
> BOTH transports together.

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
| `operations` | array | Each operation marshalled verbatim by the caller so the events package stays decoupled from the internal `operation.Operation` type. Wire bytes match the legacy rabbit publish. Always present (no omitempty). |
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
- **Money fields on `balance.config-changed`** — `available`/`onHold` are
  absent; config mutation is a separate signal family from money movement.
- **`createdAt`** — omitted on every `*.updated` event (the `updatedAt` stamp
  is the mutation marker).
- **`legalDocument`** — present on `organization.created` but dropped on
  `organization.updated`.
- **`deletionType`** — ledger `*.deleted` payloads carry no soft/hard
  discriminator (unlike CRM).
- **`overdraftLimit`** — `omitempty` on `balance.overdraft-*`; currently always
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
`STREAMING_BROKERS` + `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger` on the
ledger accordingly.

See the `CLAUDE.md` Streaming → Local testing section for the
broker/environment conventions.
