# CRM Streaming Event Catalog

Canonical reference for every streaming event the **CRM** component emits. It
complements — does not duplicate — the producer conventions in `CLAUDE.md`
(Streaming section) and `docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/*.go`, and the field-count assertions in the matching
> `*_test.go` JSONShape tests are ONE contract. A wire change updates all three
> in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v1.4.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** CRM (`components/crm`).
- **CloudEvents source (`ce-source`):** `lerian.midaz.crm`.
- **Posture:** all 7 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the HTTP request**. Durability of the mutation itself
  is owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only, identical to the current ledger
  state. When an outbox lands, only the emit call sites change; the Definitions
  and payload contracts below stay put.
- **No HTTP event-manifest endpoint.** Out of scope for this pilot (same as the
  ledger).
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled — or
  when `STREAMING_BROKERS` is empty, or no events are registered — bootstrap
  injects a `NoopEmitter` and no broker connection is attempted.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `crmEventDefinitions()` (`components/crm/internal/bootstrap/streaming.go`),
which feeds both the Catalog and the route table:

- **Event key** = `<resourceType>.<eventType>` (e.g. `holder.created`).
- **`ce-type`** = lib-streaming auto-prefixes the key: `studio.lerian.<key>`.
- **Kafka topic** = `midaz.<key>`.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` (see [ce-tenantid](#ce-tenantid)).

## Event summary

All 7 events carry `SchemaVersion = 1.0.0`.

| Event key | Resource / Event | `ce-type` | Kafka topic | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|-------------|--------------|--------------------|
| `holder.created` | holder / created | `studio.lerian.holder.created` | `midaz.holder.created` | holder ID | `CreateHolder` |
| `holder.updated` | holder / updated | `studio.lerian.holder.updated` | `midaz.holder.updated` | holder ID | `UpdateHolderByID` |
| `holder.deleted` | holder / deleted | `studio.lerian.holder.deleted` | `midaz.holder.deleted` | holder ID | `DeleteHolderByID` |
| `alias.created` | alias / created | `studio.lerian.alias.created` | `midaz.alias.created` | alias ID | `CreateAlias` |
| `alias.updated` | alias / updated | `studio.lerian.alias.updated` | `midaz.alias.updated` | alias ID | `UpdateAliasByID` |
| `alias.deleted` | alias / deleted | `studio.lerian.alias.deleted` | `midaz.alias.deleted` | alias ID | `DeleteAliasByID` |
| `alias.related-party-deleted` | alias / related-party-deleted | `studio.lerian.alias.related-party-deleted` | `midaz.alias.related-party-deleted` | **alias ID** (not the related-party ID) | `DeleteRelatedPartyByID` |

> **Hyphen, not underscore.** The `alias.related-party-deleted` event type is
> hyphenated. The lib-streaming route-key validator rejects underscores, so the
> key, topic, and `ce-type` all keep the hyphen.

> **`ce-subject` on `alias.related-party-deleted`.** The aggregate is the alias,
> so `ce-subject` is the **alias ID**, and the removed party's ID travels in the
> body as `relatedPartyId`. Every other event uses its own record ID as subject.

## Payload contracts

The wire keys below are the exact JSON field set produced by the Payload structs
in `pkg/streaming/events/`. The "field count" is the number the JSONShape test
locks.

### `holder.created` / `holder.updated` — 6 fields

Source: `pkg/streaming/events/holder_created.go`, `holder_updated.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Holder ID. |
| `organizationId` | string | Organization scope. Supplied by the emit site — `mmodel.Holder` carries no organization field. |
| `type` | string | Person classification: `NATURAL_PERSON` or `LEGAL_PERSON`. Non-identifying. |
| `externalId` | string \| null | Optional client-supplied correlation ID. JSON `null` when unset. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. `holder.updated` stamps `UpdatedAt` as `ce-time`. |

### `holder.deleted` — 4 fields

Source: `pkg/streaming/events/holder_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Holder ID. |
| `organizationId` | string | Organization scope. |
| `deletionType` | string | `"soft"` or `"hard"`, derived from the `hardDelete` flag. |
| `deletedAt` | string | RFC3339 deletion timestamp. |

### `alias.created` / `alias.updated` — 9 fields

Source: `pkg/streaming/events/alias_created.go`, `alias_updated.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Alias ID. |
| `holderId` | string | Owning holder ID. |
| `organizationId` | string | Organization scope. Supplied by the emit site — `mmodel.Alias` carries no organization field. |
| `ledgerId` | string | Ledger scope. |
| `accountId` | string | Account scope. |
| `type` | string | Alias classification. Non-identifying. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. `alias.updated` stamps `UpdatedAt` as `ce-time`. |
| `relatedParties` | array \| null | List of `{relatedPartyId, role}`. Encodes as JSON `null` (never `[]`) when the alias has no related parties. |

> **No `externalId` on alias.** `mmodel.Alias` has no `ExternalID` field, so —
> unlike holder — alias payloads do not emit `externalId`. This follows the
> code; the plan's prose payload contract mentioned `externalId (*string if it
> exists)`, and it does not exist.

Each `relatedParties` element has exactly 2 fields — `relatedPartyId` (string)
and `role` (string). The related party's `document`, `name`, and relationship
dates (`startDate` / `endDate`) are PII and never cross the wire.

### `alias.deleted` — 5 fields

Source: `pkg/streaming/events/alias_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Alias ID. |
| `holderId` | string | Owning holder ID. Carried so consumers can attribute the removal without a lookup (holder.deleted has no such field). |
| `organizationId` | string | Organization scope. |
| `deletionType` | string | `"soft"` or `"hard"`, derived from the `hardDelete` flag. |
| `deletedAt` | string | RFC3339 deletion timestamp. |

### `alias.related-party-deleted` — 5 fields

Source: `pkg/streaming/events/alias_related_party_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `aliasId` | string | Alias the party was removed from. Also the `ce-subject`. |
| `holderId` | string | Owning holder ID. |
| `organizationId` | string | Organization scope. |
| `relatedPartyId` | string | The removed related party's ID. |
| `deletedAt` | string | RFC3339 deletion timestamp. |

> **No `deletionType`.** Removing a related party is always a pointwise removal,
> not a soft/hard distinction, so this event carries no `deletionType`.

## PII off the wire

CRM aggregates are regulated entities. The payloads carry only stable
identifiers, scope IDs, non-identifying classifications, and timestamps. The
following fields are **deliberately excluded** from every event body:

- **Documents:** CPF / CNPJ (`document`), participant document
  (`participantDocument`).
- **Names:** holder / related-party names, the natural-person and legal-person
  sub-objects, representatives, filiation.
- **Contact & location:** contact details, addresses.
- **Banking:** the entire `bankingDetails` sub-object — IBAN, branch, account.
- **Regulatory:** the `regulatoryFields` sub-object.
- **Related-party PII:** each related party's `document`, `name`, `startDate`,
  and `endDate` — only `relatedPartyId` and `role` are emitted.

**Enforcement.** The `JSONShape` unit test in each event's `*_test.go` locks the
exact present-key set, pins the field count, and asserts the absence of every
PII key (at the top level, and inside the `relatedParties` element for alias
events). Any PII field added to a payload fails that test.

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

To exercise the real emit path against a broker, run the build-tagged
(`//go:build integration`) smoke test:

- **Smoke test:** `make test-streaming-integration` (in `components/crm`) runs
  `components/crm/internal/bootstrap/streaming_integration_test.go`. With no
  `STREAMING_BROKERS` set it starts a self-contained Redpanda testcontainer
  (needs Docker); set `STREAMING_BROKERS` to an already-running broker to reuse
  it instead. The test emits all 7 events through `BuildStreamingEmitter` +
  `EmitImportant` and asserts `ce-type`, `ce-subject`, `ce-tenantid`, and PII
  absence per event.

For a longer-lived local broker (e.g. to point a running CRM service at it),
use the Redpanda compose in the `end-to-end` repo (`docker-compose.redpanda.yaml`)
and set `STREAMING_ENABLED=true` + `STREAMING_BROKERS` on the CRM accordingly.

The default unit suite (`go test ./...` with no tag) never touches a broker —
the integration test is excluded by its build tag. See the `CLAUDE.md`
Streaming → Local testing section for the broker/environment conventions.
