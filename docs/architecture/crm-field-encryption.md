# CRM Field Encryption & KMS Key Management

> **Status: canonical reference.** This document is the single source of truth for how CRM
> encrypts holder and instrument PII at rest, how it searches over that ciphertext, and how
> per-organization key material is provisioned and wrapped. Everything below is a **fact grounded
> in code** in *this* repo, cited inline.
>
> **Scope.** CRM is a package tree inside the ledger binary (`components/ledger/internal/crm`), not a
> separate deploy unit. This subsystem has no image and no port of its own — it runs in-process on
> `:3002` with the rest of ledger. HashiCorp Vault is an **external** dependency an operator supplies;
> this repo owns the client, not the server.
>
> **Citation convention.** Unprefixed paths under `.../crm/services/encryption/` name a file in that
> package. `pkg/crypto/...` is the transport-agnostic crypto layer. Line ranges rot, so the durable
> anchors are the cited **function, type, and const symbols** — grep those, not line numbers.

---

## 1. Mode dichotomy — `KMS_VENDOR` picks everything

The whole subsystem has exactly two runtime shapes, selected once at boot from `KMS_VENDOR` by
`crypto.ModeResolver.Resolve` (`pkg/crypto/mode.go`, `pkg/crypto/resolver.go`) and threaded through
`initCRMEncryption` (`config.crm.encryption.go`):

| `KMS_VENDOR` | Resolved mode | What runs | Key custody |
|---|---|---|---|
| unset / `""` / `none` | **legacy** (`EncryptionModeLegacy`) | lib-commons symmetric crypto (`libCrypto.Crypto`). No Vault, no Tink, no keyset manager, no provisioning surface. | Process-global AES/HMAC keys from env. |
| `hashicorp-vault` | **envelope** (`EncryptionModeEnvelope`) | Per-organization Tink DEKs (data keys) wrapped by a Vault Transit KEK. Keyset manager, provisioning service, and the HTTP surface all wire up. | Per-org DEK, KEK never leaves Vault. |
| anything else | **boot fails** | `ModeResolver.Resolve` returns an "unsupported KMS vendor" error. | — |

`EncryptionMode` (`pkg/crypto/mode.go`) exposes `IsLegacy()` / `IsEnvelope()`; the mode flows into
every wiring decision in `wireEncryptionServices`. In legacy mode the keyset/registry/audit
repositories and the Vault client are all `nil` (`initEncryptionRepos` returns nils early), and the
provisioning HTTP handlers are never registered (§9). **The `FieldEncryptor` is always non-nil in both
modes** so the holder/instrument repositories' non-nil guard is satisfied uniformly.

**Why the split matters.** Legacy mode is the zero-dependency default: a customer can run ledger + CRM
with no Vault at all. Envelope mode is opt-in and adds the `hashicorp/vault/api` + `tink-crypto/tink-go/v2`
dependencies. Everything from §2 onward describes envelope mode unless it says legacy; the legacy path
is the degenerate case where there is no marker, no keyset, and no per-org state.

---

## 2. The `FieldEncryptor` seam

`FieldEncryptor` (`field_encryptor.go`) is the **only** type the CRM Mongo repositories couple to. It
wraps the richer `EncryptionService` (`encryption.go`) and exposes four methods:

| Method | Path | Behavior |
|---|---|---|
| `EncryptField(ctx, FieldContext, plaintext)` | write | Encrypts one PII field. Returns a **marked** ciphertext (`tink:v{version}:{b64}`) in envelope mode, an unmarked ciphertext in legacy mode. |
| `DecryptField(ctx, FieldContext, ciphertext)` | read | Routes on marker presence. Fails closed if the marker version is unreadable or if a legacy read is not permitted (§5). |
| `GenerateSearchToken(ctx, SearchTokenContext, value) → (token, version)` | write / indexing | Exactly **one** deterministic HMAC token from the current primary PRF key, plus the key ID it used. |
| `GenerateSearchTokenCandidates(ctx, SearchTokenContext, value) → []string` | read / query | Tokens for **all** enabled PRF keys (plus a legacy-hex candidate for migrated orgs), for a Mongo `$in`. |

**Two context types, deliberately different in shape:**

- `FieldContext` (`field_context.go`): `TenantID`, `OrganizationID`, `RecordID`, `FieldName`. Its
  `CanonicalAAD()` produces `tenant:{t}:org:{o}:record:{r}:field:{f}` and is fed as AEAD associated
  data. This **binds ciphertext to an exact record and field** — moving a ciphertext blob to a
  different record or field fails AEAD authentication on decrypt.
- `SearchTokenContext` (`search_context.go`): `TenantID`, `OrganizationID`, `FieldName` — it
  **intentionally omits `RecordID`** (documented in the type). A query-time token must be computable
  without knowing which record will match. Its `CanonicalInput(value)` is
  `tenant:{t}:org:{o}:field:{f}:{value}`.

**What is encrypted.** The holder repository (`adapters/mongodb/holder/holder.go`) encrypts the
holder `Name` and `Document`; contact `PrimaryEmail` / `SecondaryEmail` / `MobilePhone` / `OtherPhone`;
natural-person `MotherName` / `FatherName`; and legal-person representative `Name` / `Document` /
`Email`. The instrument repository (`adapters/mongodb/instrument/instrument.go`) encrypts the
instrument `document`, `banking_details.account`, `banking_details.iban`,
`regulatory_fields.participant_document`, and each `related_parties.{id}.document`. Only a subset is
**searchable** (has a deterministic token alongside the ciphertext): holder `document`; instrument
`document`, `banking_details.account`, `banking_details.iban`,
`regulatory_fields.participant_document`, and `related_parties.document`.

---

## 3. Search over ciphertext — write one key, read all keys

This is the least obvious behavior in the subsystem, so it gets room.

Equality search over encrypted fields works by storing a **deterministic HMAC token** next to the
(non-deterministic) AEAD ciphertext. The token is a PRF of the canonical input (§2), so the same
plaintext under the same field always produces the same token — but the token reveals nothing about the
plaintext and cannot be reversed.

The asymmetry is the whole trick:

- **Write path** (`GenerateSearchToken`) computes **exactly one** token, using the keyset's **primary**
  PRF key, and records the key version alongside it. A newly written record is therefore indexed with
  precisely the current primary key.
- **Read path** (`GenerateSearchTokenCandidates`) computes a token for **every enabled** PRF key in the
  org's keyset (and, for a migrated org, appends the legacy-hex token). The repository queries them with
  a Mongo `$in` (`appendEncryptedFilters` in `instrument_query.mongodb.go`;
  `holder_query.mongodb.go` for the holder document).

### Worked example

An org's keyset has PRF key **A** (primary). Records written today are indexed with `token_A(value)`.

1. A key is later added so the keyset has **A** and **B**, with **B** now primary.
2. New writes index with `token_B(value)` only (write = single primary key).
3. A search for `value` computes candidates `{token_A(value), token_B(value)}` and issues
   `document_token ∈ {token_A, token_B}`.
4. Both the old (A-indexed) and new (B-indexed) records match — **without re-indexing the old rows**.

The same mechanism absorbs legacy→envelope migration: a migrated org's read candidates include the
legacy-hex token, so records written before migration still match. This is what lets encrypted equality
search and key rotation coexist: the reader fans out; the writer never has to.

---

## 4. Key management — one shared Transit engine, tenant scope in the key *name*

**The counterintuitive, load-bearing decision:** there is a **single, mode-derived, shared** Vault
Transit engine per process, and **tenant isolation lives entirely in the KEK key name — not in
per-tenant mounts.** A reader who assumes "multi-tenant means a mount per tenant" will be wrong; state
it plainly so nobody re-derives that assumption.

**Mounts** (`config.crm.encryption.go`, `defaultMountPath`):

- Multi-tenant: `transit-mt`
- Single-tenant: `transit-st`

That is the entire mount taxonomy — two constants, chosen by effective tenant mode, shared across every
tenant and org.

**KEK key name** carries the scope (`provisioningService.buildKEKPath`, `provisioning.go`):

- Multi-tenant: `{tenantID}_org-{orgID}`
- Single-tenant: `org-{orgID}`

The tenant and org segments are joined with an **underscore**, not a slash, because Vault Transit key
names cannot contain `/`. A Vault operation therefore addresses e.g. `transit-mt/encrypt/{tenant}_org-{id}`.

**Envelope DEK/KEK mechanics** (`pkg/crypto/tink/`):

1. `KeysetFactory` generates a Tink keyset — the **DEK** (an AEAD keyset for field ciphertext, a PRF
   keyset for search tokens). `GenerateAEADKeyset` / `GeneratePRFKeyset` mint fresh keysets;
   `GenerateMixedAEADKeyset` / `GenerateMixedPRFKeyset` compose a fresh primary with an imported legacy
   key for migration (§8).
2. `KeysetWrapper.WrapKeyset` serializes the DEK and calls Vault Transit **Encrypt** — the **KEK** wraps
   the DEK. Only the wrapped blob and the non-secret `KeysetInfo` are persisted; the cleartext keyset
   (`KeysetBundle.RawKeyset`) is never stored or logged.
3. At use time, `KeysetManager` (`keyset_manager.go`) reads the wrapped keyset, calls Vault Transit
   **Decrypt** to unwrap the DEK, and constructs the Tink primitives. It **caches** unwrapped primitives
   keyed on `tenantID:organizationID[:version]` for a short TTL, guarding each key with a **per-org
   mutex** so concurrent first-access on the same org does not stampede Vault (`getOrgLock`,
   `getOrUnwrap`).

`OrganizationKeyset.SafeView` (`pkg/mmodel/organization_keyset.go`) redacts both wrapped keysets to
`[REDACTED]` for any logging or API response.

---

## 5. Fail-closed matrix

Every gate below is a distinct code site that refuses rather than degrades. There is no silent fallback
anywhere in the decrypt or key-access path.

| Gate | Trigger | Enforced by |
|---|---|---|
| Invalid auth method | `KMS_VAULT_AUTH_METHOD` unset or not `approle`/`token` | `vault.ParseAuthMethod` (`pkg/crypto/kms/vault/auth.go`) returns an error → boot fails via `resolveVaultAuth`. |
| Token auth outside local | `KMS_VAULT_AUTH_METHOD=token` while `DEPLOYMENT_MODE` ≠ `local` | `resolveVaultAuth` + `isLocalDeployment` (`config.crm.encryption.go`) reject; any unset/typo'd deployment mode can never fall through to the dev token. |
| Missing Transit mount | The org's mount does not exist in Vault | `checkMountExists` returns typed `vault.ErrMountNotFound` (`pkg/crypto/kms/vault/errors.go`, `transit.go`); callers `errors.Is` and fail closed — no fallback mount. |
| Reserved `"default"` tenant | A caller supplies tenant `"default"` at the provisioning boundary | `ResolveProvisionTenantID` (`field_encryptor.go`) returns `ErrReservedTenantID`; `"default"` is the reserved single-tenant sentinel and may not be provisioned externally. |
| Unreadable marker version | Decrypt sees `tink:vN:...` where `N ∉ state.ReadableVersions` | `versionIsReadable` in `decryptEnvelope` (`encryption.go`) returns `ErrEnvelopeDecryptFailed` — **no** fallback to legacy. An empty/nil `ReadableVersions` set is never readable. |
| Legacy read disallowed | Unmarked ciphertext when `state.CanReadLegacy == false` | `decryptLegacy` (`encryption.go`) returns `ErrLegacyReadNotAllowed`. |

Two boot-time postures reinforce the matrix:

- **Envelope mode refuses to boot without a valid Vault config** — `validateVaultConfig`
  (`config.crm.encryption.go`) short-circuits legacy mode but requires a valid, `Validate()`-passing
  config for envelope.
- **AppRole logs in eagerly at startup** (`initVaultClient` → `client.Login`), so a broken AppRole fails
  boot rather than the first transaction. Token auth defers validation to first use. Either way, a Vault
  op that returns HTTP 403 triggers a single automatic re-authentication and retry (`transit.go`); a
  second failure propagates.

---

## 6. Provisioning and domain models

`ProvisioningService.Provision` (`provisioning.go`) is **idempotent** and self-healing. On a fresh org
it: builds the KEK key name (`buildKEKPath`, §4) → generates the AEAD + PRF bundles (fresh, or **mixed**
with imported legacy material when `importLegacy` is set, §8) → persists the `OrganizationKeyset` at
`Version: 1, Revision: 1` with `KEKPath` and `KEKMountPath` → creates the `OrganizationRegistryRecord`.
Partial-failure recovery reconstructs a missing registry from an already-persisted keyset, so a crash
between the two writes is repairable rather than fatal.

**`OrganizationKeyset`** (`pkg/mmodel/organization_keyset.go`) — the wrapped key material:

- Fields: `TenantID`, `OrganizationID`, `Version`, `KEKPath`, `KEKMountPath`, `WrappedKeyset` (AEAD),
  `KeysetInfo`, `WrappedHMACKeyset` (PRF), `HMACKeysetInfo`, `Revision`, `CreatedAt`, `RotatedAt`.
- `Validate` requires: non-empty org id; **`Version >= 1`**; `KEKPath`; `KEKMountPath` (required on write;
  the read/unwrap path tolerates a legacy record lacking it via fallback); `WrappedKeyset`; and
  `KeysetInfo.PrimaryKeyID != 0`.
- A **compound unique index `(tenant_id, organization_id, version)`** — each version is an independent
  document, which is what makes the read-all-versions decrypt path (§3, §5) possible.
- `SafeView` redacts both wrapped keysets.

**`OrganizationRegistryRecord`** (`pkg/mmodel/organization_registry.go`) — the protection state of an org.
`NewOrganizationRegistryRecord` seeds: `Status = active`, `ProtectionModel = envelope`,
`CurrentVersion = 1`, `ReadableVersions = [1]`, `Revision = 1`, and **`LegacyReadable = true`** — so a
freshly provisioned or migrated org can still read its pre-migration legacy ciphertext. A unique index
`(tenant_id, organization_id)` keeps it one-per-org. Updates are a `ReplaceOne` filtered on
`{organization_id, revision}`; zero matches map to `ErrRegistryRevisionConflict` (optimistic
concurrency).

---

## 7. Protection state resolution

`ProtectionStateResolver.Resolve` (`protection_state.go`) decides an org's state and caches it for
**5 minutes** (`protectionStateCacheTTL`), keyed on `tenantID:organizationID`, so a multi-field search
consults the registry once per window rather than once per field. Successful provisioning calls
`Invalidate`, making a legacy→envelope transition visible immediately.

| Situation | Resolved state |
|---|---|
| Nil registry repo (`KMS_VENDOR=none`), registry-not-found, or nil record | `Mode = legacy`, `CanReadLegacy = true`, `CurrentKeysetVersion = 0`, empty `ReadableVersions`, status label `none`. |
| Active record | `Mode = envelope`, `CanReadLegacy = record.LegacyReadable`, `CurrentKeysetVersion = record.CurrentVersion`, `ReadableVersions = record.ReadableVersions`, status label `active`. |
| Any other (unknown) status | Hard error, `status=unknown` counter, and a `Warn` log naming only the status string. |

The fail-closed spine of §5 rides on this table: a legacy or unprovisioned org resolves to an **empty**
`ReadableVersions`, and an empty set is never readable — so such an org can never decrypt a marked
ciphertext.

---

## 8. Lazy legacy→envelope migration

Migration is **lazy provisioning with legacy import**, triggered on the first encrypted-field access to
an as-yet-unprovisioned org in envelope mode. `KeysetManager.GetActivePrimitives` hits
`ErrKeysetNotFound`, calls `autoProvision` (`keyset_manager.go`) with actor `system:auto-provision` and
**`importLegacy: true`**, which produces a **mixed keyset**: a fresh envelope **primary** key plus the
imported legacy key as an **enabled, non-primary** key — all wrapped by the KEK as one keyset.

The consequences fall out cleanly:

- New writes use the fresh primary and are marked (`tink:v1:...`).
- Old **unmarked** legacy bytes still decrypt — `decryptLegacyFromKeyset` (`encryption.go`) runs them
  through the org's per-org composite AEAD (with nil AAD, mirroring how legacy data was written), not the
  process-global crypto.
- Legacy equality search still matches, because the read candidates include the appended legacy-hex token
  (§3).

> **NOTE — `holder_backfill.go` is NOT encryption backfill.** It lives outside this package entirely
> (`components/ledger/internal/services/backfill/holder_backfill.go`, wired from `internal/bootstrap/`).
> It provisions deterministic self-holders and materializes `account.holder_id` in PostgreSQL (a
> `squirrel.Update("account")` over NULL, non-external, non-deleted rows). It touches encryption
> only incidentally: self-holders it writes through the repository get encrypted on write like any other
> record. Do not read it as a batch re-encryption or key-rotation job — there is no such job (§11).

---

## 9. Telemetry and audit

**Metrics** (declared in `pkg/utils/metrics.go`; emitted via the nil-safe `protectionMetrics` seam in
`metrics.go`). All labels are a closed vocabulary; none carry PII, values, or secrets:

| Metric | Labels |
|---|---|
| `crm_protection_mode_resolution_total` | `mode` |
| `crm_protection_status_total` | `status` |
| `crm_protection_encrypt_decrypt_total` | `path`, `outcome`, `error_type` |
| `crm_protection_provider_operation_ms` | `operation`, `provider` |
| `crm_protection_provider_operation_failures_total` | `operation`, `error_code` |
| `crm_protection_legacy_read_total` | `organization_status` |
| `crm_protection_cache_total` | `operation`, `result` |
| `crm_protection_registry_conflict_total` | — (declared, **not yet emitted**) |

**Spans.** Encrypt/decrypt/resolve open `service.protection.encrypt_field`,
`service.protection.decrypt_field`, and `service.protection.resolve_mode`. Inputs use the
`app.request.*` namespace (`app.request.organization_id`, `app.request.field` — the field **name**,
never its value); the chosen route is recorded as `app.protection.path`.

**Audit.** Each terminal `Provision` outcome emits **exactly one** `ProtectionAuditEvent`
(`pkg/mmodel/protection_audit_event.go`) via `auditWriter.EmitAsync` (`audit.go`). Emission is
best-effort and **detached** — `context.WithoutCancel(ctx)` + a 5s timeout, run under
`libRuntime.SafeGoWithContextAndComponent` — so it survives the request's cancellation and **never
affects the provisioning result**. Events carry no PII and no secret material (only static reason
phrases, outcome, actor, and primary key IDs).

**HTTP surface** (envelope mode only; `RegisterCRMRoutesToApp` in `crm_routes.go` registers these solely
when the handlers are non-nil, which they are only in envelope mode):

| Route | Auth resource (namespace `midaz`) |
|---|---|
| `POST /organizations/:organization_id/encryption/provision` | `encryption` |
| `GET /organizations/:organization_id/encryption/status` | `encryption` |
| `GET /organizations/:organization_id/protection/audit` (cursor-paged) | `protection` |

---

## 10. Configuration

Wired in `initCRMEncryption` / `buildVaultConfig` (`config.crm.encryption.go`); mode from
`pkg/crypto/mode.go`.

| Env var | Config field | Role |
|---|---|---|
| `KMS_VENDOR` | `KMSVendor` | Mode selector: unset/`none` → legacy; `hashicorp-vault` → envelope; else boot fails. |
| `KMS_VAULT_ADDR` | `VaultAddr` | Vault server address (envelope only). |
| `KMS_VAULT_ROLE_ID` / `KMS_VAULT_SECRET_ID` | `VaultRoleID` / `VaultSecretID` | AppRole credentials. |
| `KMS_VAULT_AUTH_METHOD` | `VaultAuthMethod` | `approle` \| `token`. Sole driver of `resolveVaultAuth`; fails closed if unset/invalid. |
| `DEPLOYMENT_MODE` | `DeploymentMode` | Gates the dev root token to `local` only. |
| `MULTI_TENANT_ENABLED` | `MultiTenantEnabled` | Selects `transit-mt` vs `transit-st` and the MT key-naming / reserved-tenant rules. |
| `LCRYPTO_ENCRYPT_SECRET_KEY` | `CrmEncryptSecretKey` | Legacy AES key. Live cipher in legacy mode; imported for legacy reads in envelope mode. |
| `LCRYPTO_HASH_SECRET_KEY` | `CrmHashSecretKey` | Legacy HMAC secret. Same dual role. |

---

## 11. Known limitations and reserved surfaces

- **Key rotation is scaffolded but inert.** The read path is built for multiple readable versions and
  the marker carries a version, but **no code path writes a keyset at `Version >= 2` or advances a
  registry's `CurrentVersion`**. Rotation is a designed-for capability, not a shipped one.
- **`RotatedAt` is reserved.** The field exists on `OrganizationKeyset` and is copied by `SafeView`, but
  **no current code assigns it.** Treat it as reserved for the rotation path above.
- **The dev root token is guarded to `local`.** Token auth returns the hardcoded dev token
  `DefaultVaultDevToken = "root"` and is permitted **only** when `DEPLOYMENT_MODE=local`
  (`resolveVaultAuth`). State plainly: **do not wire token auth in saas/byoc** — use AppRole. The guard
  rejects any non-local deployment, including an unset or typo'd mode.
- **Two base64 alphabets, on purpose.** Envelope markers encode their payload with base64 **URL-safe**
  (`FormatEnvelopeMarker` / `ParseEnvelopeMarker`, consistent with the PRF search tokens), while legacy
  on-disk bytes use base64 **Std** for lib-commons compatibility. These alphabets are intentionally
  different and **must not be unified** — the marker code comments say so.

---

## Related

- [`docs/architecture/ledger-tracer-topology.md`](./ledger-tracer-topology.md) — deployment topology of
  the ledger/tracer products this subsystem runs inside.
- [`docs/auth/RBAC-NAMESPACES.md`](../auth/RBAC-NAMESPACES.md) — the `midaz` authz namespace the
  `encryption` / `protection` resources register under.
- [`docs/api/SCOPING.md`](../api/SCOPING.md) — path-scoped organization model the encryption routes
  follow.
