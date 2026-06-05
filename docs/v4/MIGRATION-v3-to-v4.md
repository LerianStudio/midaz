# Migrating from Midaz v3 to v4

> **Audience.** Two distinct consumers, two tables below:
> 1. **REST API consumers** (HTTP clients, SDKs, APIDog/Postman scenarios, partner integrations).
> 2. **Go importers** (code that imports `github.com/LerianStudio/midaz/...`).
>
> v4 is a **breaking** major. The breaking surface is concentrated: a CRM entity rename
> (`Alias` → `Instrument`), an authz namespace flip (`plugin-crm` → `midaz`), the Go module
> path bump (`/v3` → `/v4`), plus net-new endpoints that do not affect existing callers.
> Money-path behavior (double-entry, transaction create/commit/cancel/revert, balances) is
> **unchanged**. No data migration is required for existing ledgers.

---

## Part 1 — REST API consumers

### 1.1 Route renames (Alias → Instrument)

The CRM "alias" entity is renamed to "instrument" across the wire. The old routes are
**hard-cut** (removed, not dual-served): a v4 server returns `404` for the old paths.

| v3 route | v4 route | Method(s) |
|----------|----------|-----------|
| `POST /v1/holders/{holder_id}/aliases` | `POST /v1/holders/{holder_id}/instruments` | POST |
| `GET /v1/holders/{holder_id}/aliases/{alias_id}` | `GET /v1/holders/{holder_id}/instruments/{instrument_id}` | GET |
| `PATCH /v1/holders/{holder_id}/aliases/{alias_id}` | `PATCH /v1/holders/{holder_id}/instruments/{instrument_id}` | PATCH |
| `DELETE /v1/holders/{holder_id}/aliases/{alias_id}` | `DELETE /v1/holders/{holder_id}/instruments/{instrument_id}` | DELETE |
| `GET /v1/aliases` | `GET /v1/instruments` | GET |
| `DELETE /v1/holders/{holder_id}/aliases/{alias_id}/related-parties/{related_party_id}` | `DELETE /v1/holders/{holder_id}/instruments/{instrument_id}/related-parties/{related_party_id}` | DELETE |

Path-param rename: `{alias_id}` → `{instrument_id}` everywhere. The `{holder_id}` and
`{related_party_id}` segments are unchanged.

Request/response model rename (OpenAPI `@name` schema names):

| v3 schema | v4 schema |
|-----------|-----------|
| `CreateAliasRequest` | `CreateInstrumentRequest` |
| `UpdateAliasRequest` | `UpdateInstrumentRequest` |
| `AliasResponse` | `InstrumentResponse` |

The **field shapes inside these payloads are unchanged** (e.g. `bankingDetails`,
`regulatoryFields`, `relatedParties`, `ledgerId`, `accountId`, `holderId`). Only the
schema/route nouns change.

> **DO NOT confuse with `Account.Alias`.** The transaction routing handle
> (`@treasury_checking`-style alias on accounts and balances) is a **different** concept and is
> **NOT renamed**. Routes like `GET /v1/.../accounts/alias/{alias}` and the
> `alias` field on account/balance bodies stay exactly as in v3. The rename is entity-scoped to
> the CRM instrument surface only.

### 1.2 Error-body `entity` field flip (codes unchanged)

CRM error responses carry an `entity` field naming the failing entity type. For the renamed
entity, this value flips:

| Surface | v3 value | v4 value |
|---------|----------|----------|
| Error-body `entity` for instrument errors | `Alias` | `Instrument` |

The **error code strings are byte-for-byte unchanged**. Clients that branch on `code` need no
change; clients that branch on the `entity` string must accept `Instrument`:

| Code (unchanged) | v3 meaning | v4 meaning |
|------------------|-----------|-----------|
| `CRM-0008` | alias not found | instrument not found |
| `CRM-0017` | holder has aliases | holder has instruments |
| `CRM-0023` | alias closing date before creation | instrument closing date before creation |
| `CRM-0006` | holder not found | holder not found (unchanged) |

The `RelatedParty` error `entity` value stays `RelatedParty` in both v3 and v4 (no flip).

### 1.3 Authz namespace flip: `plugin-crm` → `midaz`

> **Critical for auth-enabled deployments.** This is the most dangerous coordination item in v4.

CRM routes (holders + instruments + related-parties) authorize under the host ledger's `midaz`
namespace in v4, instead of the standalone `plugin-crm` namespace. Tenant-manager RBAC policies
key on the literal namespace string, so the in-code flip **orphans every tenant's `plugin-crm:*`
grant** until the policies are migrated. The grant migration is owner-driven (Fred + the
plugin-auth team), gated at **release/deploy, not at code merge**:

```
plugin-crm:holders:{get,post,patch,delete}   ->  midaz:holders:{get,post,patch,delete}      (resource unchanged)
plugin-crm:aliases:{get,post,patch,delete}   ->  midaz:instruments:{get,post,patch,delete}  (resource renamed)
   (related-parties DELETE)                   ->  midaz:instruments:delete                    (sub-resource, not its own resource)
```

`related-parties` DELETE authorizes under the `instruments` resource (verb `delete`), not as a
separate resource. Local/dev stacks with auth disabled are unaffected. The full policy contract
lives in `docs/auth/RBAC-NAMESPACES.md` (see the "X1 — policy migration" section). The remaining
namespaces (`routing`, `plugin-fees`) are unchanged.

### 1.4 New surfaces (additive — no action required for existing callers)

These endpoints are **net-new in v4**. Existing v3 callers are unaffected; they exist for new
ownership and reservation workflows.

| New endpoint | Purpose | Notes |
|--------------|---------|-------|
| `GET /v1/holders/{id}/accounts` | List the accounts owned by a holder | Org-scoped ownership read. Requires `ledger_id` as a query param (the read is ledger-partitioned); a `4xx` is returned when it is absent, never a silent empty result. |
| `POST /v1/holders/{id}/accounts` | Composition: open a holder-owned account and (optionally) its instrument in one call | Requires an **`X-Ledger-Id` header** (the path is holder-scoped and carries no ledger segment), mirroring how org arrives via `X-Organization-Id`. Authorizes under `midaz:accounts:post` — a tenant that can already open accounts needs no new grant. See §1.5 for the partial-failure contract. |
| `POST /v1/reservations` (tracer, `:4020`) | Two-phase usage-limit reservation: reserve before commit | Additive. The legacy `POST /v1/validations` and `GET /v1/validations*` are **untouched**. |
| `POST /v1/reservations/{id}/confirm` | Confirm a single reservation | Direct (non-pending) path. |
| `POST /v1/reservations/{id}/release` | Release a single reservation | Direct path. |
| `POST /v1/reservations/transaction/{transaction_id}/confirm` | Confirm all reservations of a transaction | **By-transaction transition** for the PENDING lifecycle — flips every RESERVED row of the transaction in one tx, idempotent. |
| `POST /v1/reservations/transaction/{transaction_id}/release` | Release all reservations of a transaction | By-transaction transition; idempotent. |

The tracer reservation API is served by the tracer component on `:4020` (an independent
deploy unit), not on the ledger `:3002`.

### 1.5 Composition partial-failure contract (`POST /v1/holders/{id}/accounts`)

The composition endpoint is **non-compensating** by design. When the account commits but the
instrument write fails, the account **remains persisted** (no rollback) and the response is a
`201` carrying a typed `instrumentError` block:

```jsonc
{
  "account": { /* the created account, always present on success */ },
  "instrument": null,
  "instrumentError": {
    "status": "FAILED",
    "reason": "0001"   // stable, client-actionable code — never raw internal error text
  }
}
```

- `instrumentError` is present **only** when the account succeeded but the instrument write
  failed. It is omitted on full success and on the account-only path (no instrument fields sent).
- On full success, `instrument` carries the created `InstrumentResponse` and `instrumentError` is
  absent.
- On the account-only path (no banking/regulatory/related-party fields present), `instrument` is
  `null` and no instrument is written.

**Recovery contract (client-driven retry):** on a partial failure, retry the standalone
`POST /v1/holders/{holder_id}/instruments` against the surviving account, or re-call composition.
There is no server-side retry. Note the standalone instrument-create validates **holder**
existence only — it does not validate account existence — so retry against the account ID returned
in the `account` block.

### 1.6 Query-param filters (no wire change — informational)

The CRM instrument-listing query parameters are **unchanged on the wire**. The
`?banking_details_branch=`, `?banking_details_account=`, `?banking_details_iban=`,
`?regulatory_fields_participant_document=`, `?related_party_document=`, and
`?related_party_role=` query keys are byte-for-byte identical to v3. The corresponding Go struct
field identifiers were renamed (`Instrument`-prefixed) — see Part 2 — but that is a Go-internal
rename only and does **not** affect REST clients.

---

## Part 2 — Go importers

External Go code that imports `github.com/LerianStudio/midaz/...` must apply the module-path bump
**and** the identifier renames below. The wire codes referenced by these identifiers are unchanged
(see §1.2) — only the Go symbol names move.

### 2.1 Module path

| v3 | v4 |
|----|----|
| `github.com/LerianStudio/midaz/v3` | `github.com/LerianStudio/midaz/v4` |

Every import path bumps the major segment, e.g.
`github.com/LerianStudio/midaz/v3/pkg/mmodel` → `github.com/LerianStudio/midaz/v4/pkg/mmodel`.
The token to rewrite is the full path `github.com/LerianStudio/midaz/v3` — never a bare `v3`.

### 2.2 Domain model identifiers (`pkg/mmodel`)

| v3 identifier | v4 identifier | Notes |
|---------------|---------------|-------|
| `mmodel.Alias` | `mmodel.Instrument` | The entity struct (`pkg/mmodel/alias.go` → `pkg/mmodel/instrument.go`). |
| `mmodel.CreateAliasInput` | `mmodel.CreateInstrumentInput` | OpenAPI `@name` also flips `CreateAliasRequest` → `CreateInstrumentRequest`. |
| `mmodel.UpdateAliasInput` | `mmodel.UpdateInstrumentInput` | OpenAPI `@name` also flips `UpdateAliasRequest` → `UpdateInstrumentRequest`. |
| `mmodel.RelatedParty` | `mmodel.RelatedParty` | Unchanged (nested type). |
| `mmodel.BankingDetails` | `mmodel.BankingDetails` | Unchanged (nested type). |
| `mmodel.RegulatoryFields` | `mmodel.RegulatoryFields` | Unchanged (nested type). |

> **`Account.Alias` / `Balance.Alias` are NOT renamed.** The `Alias *string` field on
> `mmodel.Account` and `mmodel.Balance` is the transaction routing handle — a different concept,
> kept verbatim. Do not blanket-rename the word `alias`; the rename is entity-scoped to the CRM
> `Alias` struct and its `Create`/`Update` inputs.

### 2.3 Error sentinels (`pkg/constant/errors.go`) — codes unchanged

| v3 sentinel | v4 sentinel | Code (unchanged) |
|-------------|-------------|------------------|
| `ErrAliasNotFound` | `ErrInstrumentNotFound` | `CRM-0008` |
| `ErrHolderHasAliases` | `ErrHolderHasInstruments` | `CRM-0017` |
| `ErrAliasClosingDateBeforeCreation` | `ErrInstrumentClosingDateBeforeCreation` | `CRM-0023` |

> **The 6 numeric `Account.Alias` routing-handle sentinels are NOT renamed** and keep their
> numeric codes: `ErrAliasUnavailability` (`0020`), `ErrFailedToRetrieveAccountsByAliases`
> (`0063`), `ErrAccountAliasNotFound` (`0085`), `ErrAccountAliasInvalid` (`0096`),
> `ErrAccountingAliasValidationFailed` (`0118`), `ErrDuplicatedAliasKeyValue` (`0123`). These
> belong to the transaction routing handle, not the CRM entity.

### 2.4 Entity constants (`pkg/constant/entity.go`)

| Constant | Value | Status |
|----------|-------|--------|
| `constant.EntityInstrument` | `"Instrument"` | **New in v4.** Drives the error-body `entity` flip in §1.2; replaces the `reflect.TypeOf(mmodel.Alias{})` pattern. |
| `constant.EntityRelatedParty` | `"RelatedParty"` | **New in v4.** Replaces `reflect.TypeOf(mmodel.RelatedParty{})`; value unchanged on the wire. |
| `constant.EntityHolder` | `"Holder"` | Present since the v4 ownership work; reuses `ErrHolderNotFound` (`CRM-0006`). |

### 2.5 `holderId` on accounts

`mmodel.Account` and the account-create input gain an optional `HolderID *string`
(`json:"holderId"`, omittable, UUID-validated). It is **immutable after create** — absent from
`UpdateAccountInput` and never updated via PATCH. For existing ledgers it materializes to the
organization's self-holder; ownership is org-scoped (not ledger-scoped). Importers that
construct accounts can ignore it (it defaults); importers that read accounts gain the field.

### 2.6 Query-filter struct fields (`pkg/net/http`)

The `QueryHeader` instrument-filter fields are renamed `Instrument`-prefixed:
`BankingDetailsBranch` → `InstrumentBankingDetailsBranch`, `BankingDetailsAccount` →
`InstrumentBankingDetailsAccount`, `BankingDetailsIban` → `InstrumentBankingDetailsIban`,
`RegulatoryFieldsParticipantDocument` → `InstrumentRegulatoryFieldsParticipantDocument`,
`RelatedPartyDocument` → `InstrumentRelatedPartyDocument`, `RelatedPartyRole` →
`InstrumentRelatedPartyRole`. The `QueryHeader.Alias *string` field (account routing-handle
listing filter) is **unchanged**. As noted in §1.6, the **wire query-param strings are
preserved** — this is a Go-identifier rename only.

---

## Summary of breaking changes

1. **REST:** `/v1/.../aliases*` → `/v1/.../instruments*` (hard-cut, `404` on old paths);
   `{alias_id}` → `{instrument_id}`; schema names `*Alias*` → `*Instrument*`.
2. **REST:** error-body `entity` value `Alias` → `Instrument` (codes unchanged).
3. **REST/Auth:** authz namespace `plugin-crm` → `midaz` (RBAC grant migration required for
   auth-enabled deployments — release/deploy gate).
4. **Go:** module path `/v3` → `/v4`.
5. **Go:** `mmodel.Alias` → `mmodel.Instrument` (+ `Create`/`Update` inputs), CRM error sentinels
   renamed (codes unchanged).

## Non-breaking additions

- `GET` / `POST /v1/holders/{id}/accounts` (ownership read + composition).
- Tracer `POST /v1/reservations*` (incl. by-transaction confirm/release); legacy
  `/v1/validations` untouched.
- `holderId` on accounts (optional, immutable, defaults to org self-holder).
