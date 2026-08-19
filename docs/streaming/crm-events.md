# CRM Streaming Event Catalog

Canonical reference for every streaming event the **CRM** component emits. It
complements — does not duplicate — the producer conventions in `CLAUDE.md`
(Streaming section) and `docs/PROJECT_RULES.md`.

> **Drift discipline.** This document, the Payload structs in
> `pkg/streaming/events/*.go`, and the field-count assertions in the matching
> `*_test.go` JSONShape tests are ONE contract. A wire change updates all three
> in the same PR. When this doc and the code disagree, the code wins.

## Overview

- **Producer:** [`github.com/LerianStudio/lib-streaming`](https://github.com/LerianStudio/lib-streaming) v3.0.0.
- **Wire format:** CloudEvents 1.0, binary mode, over Kafka.
- **Component:** CRM is embedded in the ledger binary
  (`components/ledger/internal/crm`); there is no standalone CRM service.
- **Application name / CloudEvents source (`ce-source`):** `ledger` — the
  application name of the binary CRM rides on, NOT a separate `crm` source. It must
  be ONE dot-free lowercase segment matching `^[a-z0-9][a-z0-9_-]*$`, at most 223
  bytes; a malformed value is REJECTED at startup, never normalized. Consumers
  attribute events to CRM via the `holder` / `instrument` resource types, never via
  `ce-source` or a topic segment.
- **Kafka topics:** ONE topic per producing application. CRM events ride
  `lerian.streaming.ledger` alongside every other event the ledger binary emits,
  with `lerian.streaming.ledger.dlq` as the single dead-letter topic. There is no
  `crm` topic, no per-event topic, and no `.v<major>` topic suffix:
  `ce-schemaversion` is the only version carrier on the wire. Consumers subscribe to
  the application and dispatch on the event key.
- **Posture:** all 7 events are **IMPORTANT** — direct-emit, synchronous, via
  `pkgStreaming.EmitImportant`. Emit is best-effort at the post-commit slot in
  the command use case: a build/emit failure logs a Warn and is recorded on the
  span, but **never fails the HTTP request**. Durability of the mutation itself
  is owned by the database write, not by the emit.
- **No outbox.** Emission is direct-emit only, identical to the current ledger
  state. When an outbox lands, only the emit call sites change; the Definitions
  and payload contracts below stay put.
- **HTTP event-manifest endpoint.** The ledger binary serves
  `GET /v1/streaming/manifest` (auth `streaming-manifest`/`get`) — a catalog-only
  view of the registered event Definitions, including the CRM `holder.*` /
  `instrument.*` events, at manifest wire version `1.0.0`. The application's
  `topic` / `dlqTopic` pair sits at DOCUMENT level; each event entry names its
  `eventKey` (`"<resourceType>.<eventType>"`), its `schemaVersion`, and its
  `class` — always `"fact"` here. It is independent of `STREAMING_ENABLED` and
  degraded-safe (it reflects the static Catalog, not a live broker connection).
- **Master flag:** `STREAMING_ENABLED` (default `false`). When disabled, bootstrap
  injects a `NoopEmitter`, no broker connection is attempted, and `/readyz` reports
  the streaming check as `skipped` rather than healthy. `STREAMING_ENABLED=true`
  with an empty `STREAMING_BROKERS` — or with no events registered — REFUSES BOOT
  (`pkgStreaming.RequireBrokers`): an enabled producer with nowhere to publish
  discards every event silently while readiness stays green, which is the same
  invisible-total-loss failure the roster source gate exists to kill. To run without
  streaming, set `STREAMING_ENABLED=false`.

Routing constants are assembled from `Definition{ResourceType, EventType,
SchemaVersion}` (`pkg/streaming/events/events.go`) and registered exactly once
in `midazEventDefinitions()`
(`components/ledger/internal/bootstrap/streaming.go`), which feeds both the
Catalog and the manifest:

- **Event key** = `<resourceType>.<eventType>` (e.g. `holder.created`) — the
  dispatch selector a consumer registers a handler under inside the `ledger` stream.
- **`ce-type`** = `studio.lerian.<app>.<resourceType>.<eventType>`, i.e.
  `studio.lerian.ledger.<key>` (e.g. `studio.lerian.ledger.holder.created`). The
  resource stays `holder` / `instrument`; the `ledger` segment names the producing
  application, which is what keeps two services emitting a same-named event from
  producing byte-identical `ce-type` values — a homonym collision a consumer reading
  only `ce-type` cannot detect.
- **Kafka topic** = `lerian.streaming.ledger` for every CRM event, derived from
  `ce-source` via `libStreaming.AppTopic` and shared with the rest of the
  ledger-binary catalog. CRM events are distinguished only by the `holder` /
  `instrument` resource type — never by a topic of their own. One catch-all route
  carries every fact; nothing fans out per event.
- **`ce-subject`** = the aggregate ID (`EmitRequest.Subject`).
- **`ce-tenantid`** = `EmitRequest.TenantID`, resolved by
  `pkgStreaming.ResolveTenantID(ctx)` (see [ce-tenantid](#ce-tenantid)).

## Event summary

All 7 events carry `SchemaVersion = 1.0.0`.

| Event key | Resource / Event | `ce-type` | `ce-subject` | Trigger (use case) |
|-----------|------------------|-----------|--------------|--------------------|
| `holder.created` | holder / created | `studio.lerian.ledger.holder.created` | holder ID | `CreateHolder` |
| `holder.updated` | holder / updated | `studio.lerian.ledger.holder.updated` | holder ID | `UpdateHolderByID` |
| `holder.deleted` | holder / deleted | `studio.lerian.ledger.holder.deleted` | holder ID | `DeleteHolderByID` |
| `instrument.created` | instrument / created | `studio.lerian.ledger.instrument.created` | instrument ID | `CreateInstrument` |
| `instrument.updated` | instrument / updated | `studio.lerian.ledger.instrument.updated` | instrument ID | `UpdateInstrumentByID` |
| `instrument.deleted` | instrument / deleted | `studio.lerian.ledger.instrument.deleted` | instrument ID | `DeleteInstrumentByID` |
| `instrument.related_party_deleted` | instrument / related_party_deleted | `studio.lerian.ledger.instrument.related_party_deleted` | **instrument ID** (not the related-party ID) | `DeleteRelatedPartyByID` |

> **Underscores are preserved everywhere.** The
> `instrument.related_party_deleted` event is multi-word. Its **event key**
> (`Definition.Key()`) and **`ce-type`** are underscore-canonical — that is what
> consumers see on the wire and register a handler under
> (`studio.lerian.ledger.instrument.related_party_deleted`). Route keys accept
> underscores, so nothing is folded and the event name has no hyphenated variant
> anywhere.

> **`ce-subject` on `instrument.related_party_deleted`.** The aggregate is the instrument,
> so `ce-subject` is the **instrument ID**, and the removed party's ID travels in the
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

### `instrument.created` / `instrument.updated` — 9 fields

Source: `pkg/streaming/events/instrument_created.go`, `instrument_updated.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Instrument ID. |
| `holderId` | string | Owning holder ID. |
| `organizationId` | string | Organization scope. Supplied by the emit site — `mmodel.Instrument` carries no organization field. |
| `ledgerId` | string | Ledger scope. |
| `accountId` | string | Account scope. |
| `type` | string | Instrument classification. Non-identifying. |
| `createdAt` | string | RFC3339. |
| `updatedAt` | string | RFC3339. `instrument.updated` stamps `UpdatedAt` as `ce-time`. |
| `relatedParties` | array \| null | List of `{relatedPartyId, role}`. Encodes as JSON `null` (never `[]`) when the instrument has no related parties. |

> **No `externalId` on instrument.** `mmodel.Instrument` has no `ExternalID` field, so —
> unlike holder — instrument payloads do not emit `externalId`. This follows the
> code; the plan's prose payload contract mentioned `externalId (*string if it
> exists)`, and it does not exist.

Each `relatedParties` element has exactly 2 fields — `relatedPartyId` (string)
and `role` (string). The related party's `document`, `name`, and relationship
dates (`startDate` / `endDate`) are PII and never cross the wire.

### `instrument.deleted` — 5 fields

Source: `pkg/streaming/events/instrument_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `id` | string | Instrument ID. |
| `holderId` | string | Owning holder ID. Carried so consumers can attribute the removal without a lookup (holder.deleted has no such field). |
| `organizationId` | string | Organization scope. |
| `deletionType` | string | `"soft"` or `"hard"`, derived from the `hardDelete` flag. |
| `deletedAt` | string | RFC3339 deletion timestamp. |

### `instrument.related_party_deleted` — 5 fields

Source: `pkg/streaming/events/instrument_related_party_deleted.go`.

| Key | Type | Notes |
|-----|------|-------|
| `instrumentId` | string | Instrument the party was removed from. Also the `ce-subject`. |
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
PII key (at the top level, and inside the `relatedParties` element for instrument
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

> **Before upgrading:** the ce-source is now REFUSED at startup unless it is exactly
> `ledger`. A value carried over from before the one-topic contract — the dotted
> `lerian.midaz.ledger` or URI `//lerian.midaz/ledger` shapes, or any other legal
> name — must be removed from every env file first, or the service will not boot. The
> refusal is deliberate: broker topics and Kafka ACLs are provisioned for the roster
> name alone, so any other value would publish into a stream that neither exists nor
> is granted, and midaz would swallow every one of those failures as a Warn while
> reporting healthy. The check runs whether or not `STREAMING_ENABLED` is set.

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
