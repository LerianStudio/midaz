# Runbook: v4 Release — X1 RBAC Namespace Migration & Rollback

> **Scope.** Two linked operations for the `v4` / monorepo-consolidation release of midaz
> (module `github.com/LerianStudio/midaz/v4`):
> **(A)** the X1 RBAC namespace migration (CRM holders/instruments authz flip `plugin-crm` → `midaz`), and
> **(B)** rolling back a v4 release.
>
> These two operations are coupled: the authz namespace is enforced **fail-closed** (403 on
> missing/mismatched grants), so the order of code deploy vs. tenant-manager policy migration is
> load-bearing in BOTH directions (forward and rollback).
>
> Related docs: `docs/auth/RBAC-NAMESPACES.md` (authoritative for X1),
> `docs/architecture/ledger-tracer-topology.md`, `CLAUDE.md` (architecture),
> `docs/standards/telemetry.md`, `docs/standards/error-handling.md`.

---

## Status / Ownership

| Field | Value |
|-------|-------|
| Runbook status | **Draft — operational, contains explicit gaps (see "Known Gaps").** |
| X1 policy migration owner | **Fred**, executed **with the plugin-auth team at v4 finalization** (`RBAC-NAMESPACES.md:13, 78`). |
| Policy-applying system | **`tenant-manager`** — the namespace strings ARE the policy keys it looks up. |
| v4 code / deploy owner | Release operator (deploy + Helm/secrets). Coordinates with X1 owner on lockstep timing. |
| Enforcement model | **FAIL-CLOSED.** Missing/mismatched grant → **HTTP 403 Forbidden**, silently (`RBAC-NAMESPACES.md:12, 51-68, 116-128`). |
| Affected surface | CRM holder / instrument / related-parties routes only. Ledger core + fees authz unchanged. |

---

## Gate Classification: X1 is a RELEASE/DEPLOY gate, NOT a merge gate

**X1 is a release/deploy gate, NOT a merge gate** (`RBAC-NAMESPACES.md:12-14, 78-79`).

- Merging the v4 code flip does **not** require the tenant-manager policies to be migrated first.
  Route merge ≠ authz merge. The in-code flip lands at v4; the coordinated policy migration is X1,
  executed at v4 finalization.
- **Local / dev deployments with auth disabled are unaffected** — no policy migration needed there.
- **Any environment with auth enabled** MUST have its tenant-manager policies migrated **in lockstep
  with the v4 release.** Deploying v4 code to an auth-enabled environment without the matching
  `midaz:*` grants → every CRM holder/instrument request returns **403**.

---

## What the flip actually is

- **File:** `components/ledger/internal/adapters/http/in/crm_routes.go:20`
- **Constant:** `const ApplicationName = "midaz"` (changed from the removed `plugin-crm`).
- **Routes affected:** all CRM `holders`, `instruments`, and `related-parties` routes.
  Each is wired via `protectedMidaz(auth, resource, action, ...)` → `auth.Authorize(ApplicationName, resource, action)` (`crm_routes.go:62-79`).
- **Net effect:** the authz namespace for these routes moves from `plugin-crm` → `midaz`.

Namespace layout in v4 (for context — only the CRM rows are migrating):

| Namespace | Resources | Source |
|-----------|-----------|--------|
| `midaz` | organizations, ledgers, assets, asset-rates, portfolios, segments, accounts, balances, transactions, operations, settings | `routes.go:15` (`midazName`), `protectedMidaz` `routes.go:307-309` |
| `midaz` (flipped in v4) | **holders, instruments, related-parties** | `crm_routes.go:20`, `crm_routes.go:62-79` |
| `routing` | account-types, operation-routes, transaction-routes | `routes.go:16` (`routingName`), `protectedRouting` `routes.go:311-313` |
| `plugin-fees` (**UNCHANGED**) | packages, estimates, billing-packages, billing-calculate | `fees_routes.go:18` (`feesApplicationName`), `pkg/constant/module.go:24` |

> **Fees do NOT migrate.** `plugin-fees:*` is intentionally preserved with no migration
> (`RBAC-NAMESPACES.md:7-8, 15, 84-86`). Do not touch fees grants during X1.
>
> **Routing does NOT migrate either (this build).** `account-types`, `operation-routes`, and
> `transaction-routes` stay under the `routing` namespace — `protectedRouting` still calls
> `auth.Authorize(routingName, ...)` (`routes.go:312`; `routingName = "routing"` at `routes.go:16`).
> X1 touches CRM holders/instruments only. `RBAC-NAMESPACES.md` Epic 3.3 *proposes* folding
> `routing:* → midaz:*` into a later release; if a future build flips `protectedRouting` to `midazName`,
> routing grants must migrate too and the matrix + smoke tests below must add the routing routes.
> **Re-verify `routes.go:312` before each release** to confirm routing has not been folded into X1.

---

## X1 Policy Migration Matrix (old key → new key)

Apply in `tenant-manager` for **every tenant** that holds the old keys
(`RBAC-NAMESPACES.md:70-76`):

| Old key (`plugin-crm`) | New key (`midaz`) | Note |
|------------------------|-------------------|------|
| `plugin-crm:holders:get` | `midaz:holders:get` | resource unchanged |
| `plugin-crm:holders:post` | `midaz:holders:post` | resource unchanged |
| `plugin-crm:holders:patch` | `midaz:holders:patch` | resource unchanged |
| `plugin-crm:holders:delete` | `midaz:holders:delete` | resource unchanged |
| `plugin-crm:aliases:get` | `midaz:instruments:get` | **resource RENAMED** `aliases` → `instruments` |
| `plugin-crm:aliases:post` | `midaz:instruments:post` | **resource RENAMED** |
| `plugin-crm:aliases:patch` | `midaz:instruments:patch` | **resource RENAMED** |
| `plugin-crm:aliases:delete` | `midaz:instruments:delete` | **resource RENAMED** |
| related-parties DELETE | `midaz:instruments:delete` | sub-resource of `instruments` |

> **Trap:** the resource name changes for instruments (`aliases` → `instruments`), not just the
> namespace prefix. A find-and-replace of only `plugin-crm` → `midaz` will leave
> `midaz:aliases:*` grants, which the v4 code never checks → silent 403. The grant must read
> `midaz:instruments:*`.

---

## Failure mode if the order is wrong (read before doing anything)

The enforcement is fail-closed and the failure is **silent**:

- Flipping the namespace literal in code without a coordinated policy migration
  **orphans every tenant's `plugin-crm:*` grant** — the old grants no longer match the new
  `midaz:*` requirement (`RBAC-NAMESPACES.md:51-68`).
- The orphaned request returns **plain HTTP 403**. There is **no** "wrong namespace" hint in the
  response. The symptom is indistinguishable from an ordinary authz denial
  (`RBAC-NAMESPACES.md:116-133`).
- Symmetric consequences:
  - **Deploy v4 code WITHOUT the policy migration** → all CRM holder/instrument routes 403.
  - **Roll back v4 code WITHOUT reverting the policy migration** → all CRM holder/instrument routes 403.

This is why both procedures below pin a strict order.

---

## PRECONDITIONS — before deploying v4 (auth-enabled environments)

Confirm ALL of the following. If any is unchecked, do not start the forward procedure.

- [ ] X1 owner (Fred) and the plugin-auth team are engaged and available for the deploy window.
- [ ] You have an inventory of every tenant currently holding `plugin-crm:holders:*` and
      `plugin-crm:aliases:*` grants in `tenant-manager`. (Needed to know who to migrate.)
- [ ] The new-key grant set per tenant is staged using the migration matrix above
      (`midaz:holders:*`, `midaz:instruments:*`), with the `aliases → instruments` rename applied.
- [ ] You have decided whether to **dual-provision** (issue new `midaz:*` keys WHILE keeping the
      old `plugin-crm:*` keys briefly) to avoid a hard cutover gap. **VERIFY whether tenant-manager
      supports holding both key sets simultaneously — the docs do not confirm this** (see Known Gaps).
- [ ] Rollback plan reviewed: you know how to revert grants `midaz:*` → `plugin-crm:*` if you roll back code.
- [ ] Fees grants left **untouched** (no `plugin-fees` changes in this release).
- [ ] Local/dev with auth disabled confirmed unaffected (no policy work needed there).
- [ ] **KMS mode determined.** Check `KMS_VENDOR`: unset/`none` → legacy CRM crypto (no KMS, no
      rollback data door); `hashicorp-vault` → envelope encryption of CRM PII, which is a **rollback
      one-way door** (see One-Way Doors). In envelope mode confirm the Vault surface is staged:
      `KMS_VAULT_ADDR`, `KMS_VAULT_ROLE_ID`, `KMS_VAULT_SECRET_ID`, `KMS_VAULT_AUTH_METHOD`
      (`approle`|`token`; `token` is accepted **only** when `DEPLOYMENT_MODE=local`), and
      `DEPLOYMENT_MODE`. (`components/ledger/internal/bootstrap/config.crm.encryption.go`.)

---

## FORWARD PROCEDURE — deploy v4 + migrate X1 policies (lockstep)

The policy migration must complete **in lockstep with the v4 release** (`RBAC-NAMESPACES.md:82`).
The window between "v4 code live" and "new grants live" is a hard-403 window for CRM routes; minimize it.

> **⛔ STOP — choose your path before touching `tenant-manager`.**
> The graceful **dual-provision** path below depends on `tenant-manager` being able to hold both
> `plugin-crm:*` and `midaz:*` grants for the same tenant at once. **That is NOT confirmed** (Known Gap #2).
> - **Default / always safe:** if you have not confirmed dual-grant support → use the **Hard-cutover
>   variant** further down. Do not run the dual-provision steps.
> - **Only** run the dual-provision path once dual-grant support is confirmed. Running its step 1 against a
>   tenant-manager that *overwrites* rather than *adds* grants can silently drop the old `plugin-crm:*`
>   grant and defeat the graceful intent — the worst of both worlds.

**Dual-provision order (minimizes the 403 window — ONLY if dual-grant support is confirmed, see STOP above):**

1. **Pre-stage new grants (before code deploy).** In `tenant-manager`, issue the new `midaz:holders:*`
   and `midaz:instruments:*` grants for every affected tenant **in addition to** their existing
   `plugin-crm:*` grants. This makes the cutover graceful: pre-v4 code still reads `plugin-crm:*`
   (still present), v4 code reads `midaz:*` (now present).
   - **VERIFY tenant-manager allows both key sets at once** before relying on this. If it does not,
     skip to the hard-cutover variant below.

2. **Deploy v4 code** to the environment (ledger unified binary). With both grant sets present, CRM
   routes authorize the moment the binary starts checking `midaz:*`.

3. **Smoke test CRM authz** (see Verification). Confirm holder/instrument GET/POST/PATCH/DELETE return
   non-403 for a tenant that should have access, and 403 for one that should not.

4. **Decommission old grants.** Once v4 is confirmed healthy and stable, remove the now-unused
   `plugin-crm:holders:*` and `plugin-crm:aliases:*` grants from each tenant. Do this only after you
   are confident you will not roll back, OR keep them parked as a fast rollback lane (see Rollback).

**Hard-cutover variant (if dual-provision is NOT supported — VERIFY):**

1. Schedule a coordinated maintenance window with Fred + plugin-auth.
2. Deploy v4 code and execute the grant migration `plugin-crm:*` → `midaz:*` as close to
   simultaneously as possible. Every second between the two is a 403 window for CRM routes.
3. Smoke test immediately (Verification section).

**If you see 403s on CRM routes after deploy:** the most likely cause is missing or mis-keyed
`midaz:*` grants (especially `midaz:aliases:*` left un-renamed instead of `midaz:instruments:*`).
Re-check the migration matrix. The 403 carries no namespace hint, so trust the matrix, not the error body.

---

## ROLLBACK PROCEDURE — reverting a v4 release

> **The X1 doc has NO documented reverse procedure** (`RBAC-NAMESPACES.md` has no "Rollback" section —
> confirmed gap). The steps below are derived from the fail-closed enforcement model; treat sequencing
> details marked **VERIFY** as unconfirmed.

Rolling back v4 → pre-v4 re-introduces the old `plugin-crm` namespace literals in `auth.Authorize`.
If tenants are now on `midaz:*` grants, those grants **orphan again** and all CRM routes 403. So the
policy state must be reverted alongside the code.

### Rollback decision — when, and the point of no return

- **Roll back if:** authorized tenants see sustained CRM `403`s after the X1 cutover that the migration
  matrix cannot explain.
- **Point of no return for fast rollback** is forward step 4 (decommissioning the old `plugin-crm:*` grants):
  - **Before** forward step 4 — if you parked the old grants — rollback is fast: roll the code back and the
    still-present `plugin-crm:*` grants satisfy the pre-v4 checks the instant the old binary starts.
  - **After** forward step 4 — the fast lane is closed; rollback now requires full re-provisioning of
    `plugin-crm:*` grants per the procedure below, coordinated with Fred + plugin-auth.

### Order (recommended)

1. **Re-provision old grants FIRST (before code rollback).** In `tenant-manager`, restore
   `plugin-crm:holders:*` and `plugin-crm:aliases:*` for every affected tenant (reverse the matrix:
   `midaz:holders:*` → `plugin-crm:holders:*`, `midaz:instruments:*` → `plugin-crm:aliases:*`, applying
   the `instruments → aliases` rename in reverse).
   - If you parked the old grants instead of decommissioning them in forward step 4, they are already
     present — verify and skip re-issuance.
   - **VERIFY the safe sequence** (re-provision-then-rollback vs. rollback-then-re-provision). The doc
     does not specify it; re-provisioning first keeps the rollback target's `plugin-crm:*` checks
     satisfied the instant the old binary starts, minimizing the 403 window.

2. **Roll back the code** (git revert / redeploy the pre-v4 image). Reverting code is sufficient to
   restore the module import path and the env-var **names** the old code reads.

3. **Review the one-way doors that code rollback does NOT undo** — see the next section. The
   load-bearing one is the tenant-manager grant revert (step 1 above); the module-path note is
   informational. Confirm none was missed before declaring rollback complete.

4. **Decommission the new `midaz:*` CRM grants** once the rollback is confirmed healthy (optional —
   may park them for a re-roll-forward lane).

5. **Smoke test** CRM authz on the rolled-back (pre-v4) binary, plus the ledger health checks.

---

## ONE-WAY DOORS — what a code rollback does NOT undo

A `git revert` restores code, but the following require explicit operator action. **Validate each
before declaring rollback complete.**

### Policy / authz state
- **Tenant-manager grants.** Code rollback re-introduces `plugin-crm:*` checks but does NOT touch
  `tenant-manager`. The `midaz:*` grants issued during X1 must be reverted to `plugin-crm:*` by
  Fred + plugin-auth, or CRM routes 403. (`RBAC-NAMESPACES.md` — no automated reverse path.)

### Module path
- **Module path `/v4`.** `go.mod:1` is `github.com/LerianStudio/midaz/v4`. Internal deploy rollback
  works via git+rebuild, but any **external library consumer** importing `midaz/v4` sees a breaking
  import path on rollback to `v3` and must migrate imports. (Monorepo-wide bump; ledger and tracer do
  not publish independently.)

### KMS envelope encryption (CRM PII at rest) — applies ONLY when `KMS_VENDOR=hashicorp-vault`
- **Envelope ciphertext is NOT readable by a pre-encryption binary.** In envelope mode CRM
  holder/instrument PII is written to the `holders_<orgId>` / `aliases_<orgId>` collections as Tink
  envelope ciphertext (`tink:v{version}:{base64}`, marker prefix `tink:v`), keyed by per-organization
  Tink DEKs that are themselves wrapped by a HashiCorp Vault Transit KEK (a single shared, mode-derived
  engine — `transit-mt`/`transit-st` — with tenant/org scope carried in the KEK **name**, not in
  separate mounts). Decryption **fails closed**: `decryptEnvelope`
  (`components/ledger/internal/crm/services/encryption/encryption.go:509-536`) refuses any marker whose
  version is not in the organization's readable set and never falls back to legacy crypto. A binary that
  predates the encryption feature has **no code path** to read these values, so a code rollback past the
  encryption feature leaves every envelope-encrypted CRM field **unreadable**. This is a hard one-way
  door: **do not roll back a `KMS_VENDOR=hashicorp-vault` deployment past the encryption feature without
  a decrypt/re-encrypt data plan** (and preserve the Vault Transit keysets — losing them is
  unrecoverable). Legacy mode (`KMS_VENDOR` unset/`none`) is not subject to this door.

### Data that is SAFE (no action needed)
- **`aliases_<orgId>` / `holders_<orgId>` collections — SAFE in legacy KMS mode only.** The org-scoped
  CRM collection NAMES are a pre-existing ledger pattern, NOT a v4 change; the `aliases_<orgId>` storage
  name predates the v4 instruments rename. Rollback is read-safe **only when `KMS_VENDOR` is unset or
  `none`** (legacy mode) — then no collection migration is required.
  (`components/ledger/internal/crm/adapters/mongodb/instrument/instrument.mongodb.go`.) **When
  `KMS_VENDOR=hashicorp-vault` (envelope mode) the field CONTENTS are encrypted and rollback is NOT
  read-safe — see the KMS envelope encryption one-way door above.**
- **Ledger fee collections.** Fee-collection schema is a LEDGER concern — if ledger is rolled back,
  handle its fee schema separately (see Known Gaps). (`components/ledger/pkg/fee/`.)
- **Postgres migrations.** Ledger and tracer own their own migration dirs independently; roll each
  back per its own migration history.

---

## Verification / Smoke Tests

### A. Confirm CRM routes authorize correctly post-X1 (forward)

For a tenant that SHOULD have CRM access (auth-enabled environment):

```
# Expect: NOT 403 (200/201/204/404-for-missing-id are all fine; 403 means a grant problem)
curl -i -H "Authorization: Bearer $TOKEN" \
  https://<ledger-host>:3002/v1/organizations/<orgId>/holders

curl -i -H "Authorization: Bearer $TOKEN" \
  https://<ledger-host>:3002/v1/organizations/<orgId>/instruments
```

> **VERIFY the exact route paths and version prefix** against `routes.go` /
> `crm_routes.go` for this build — the host/port `:3002` is the ledger unified binary, but the path
> shape above is illustrative. Do not rely on it verbatim.

Pass criteria:
- Authorized tenant: holder + instrument GET/POST/PATCH/DELETE return **non-403**.
- Unauthorized tenant: returns **403** (fail-closed working as intended).
- A 403 for the authorized tenant = grant problem. Re-check the migration matrix, especially that
  instruments grants read `midaz:instruments:*` and NOT `midaz:aliases:*`.

### B. Confirm rollback is healthy (pre-v4 binary)

- **CRM authz:** same curl checks as (A); authorized tenant must be non-403, proving the
  `plugin-crm:*` grants were restored in `tenant-manager`.
- **Ledger readiness:** the rolled-back ledger reports ready on its single port.
  ```
  curl -i https://<ledger-host>:3002/readyz
  ```
  Logs are structured per `docs/standards/telemetry.md`; search by the immediate failing resource, not
  by broad scope IDs.

### C. Observability probes (adjust labels to your deployment)

The org runs Grafana/Loki/Prometheus. A fail-closed `403` carries no namespace hint (see "Failure mode"),
so probe it directly rather than curl-by-hand. **The label/metric names below are TEMPLATES — verify them
against your actual dashboards/exporters; do not assume they exist verbatim.**

- **Loki — CRM 403s by tenant** (confirm the status-code/route/tenant labels your ledger emits):
  ```logql
  sum by (tenant) (count_over_time({app="ledger"} | json | status="403" | path=~`.*/(holders|instruments).*` [5m]))
  ```
- **Prometheus — 4xx rate on ledger `:3002` CRM paths** (confirm your HTTP metric + label names):
  ```promql
  sum by (path) (rate(http_requests_total{service="ledger",code=~"4..",path=~".*(holders|instruments).*"}[5m]))
  ```

A sustained non-zero series for an *authorized* tenant after the cutover = the 403 window is still open →
re-check the migration matrix. The series falling to zero confirms the window closed.

---

## Known Gaps / VERIFY Before Relying

1. **No documented reverse policy migration.** `RBAC-NAMESPACES.md` has no rollback section. The
   reverse-grant steps here are derived from the fail-closed model, not from documented procedure.
   **VERIFY with Fred + plugin-auth** who reverts policies and the exact sequence
   (re-provision-then-rollback vs. rollback-then-re-provision).
2. **Dual-provision support.** Whether `tenant-manager` can hold both `plugin-crm:*` and `midaz:*`
   grants for a tenant simultaneously (to avoid a hard-cutover 403 window) is **NOT confirmed in the
   facts. VERIFY** before choosing the graceful forward path; otherwise use the hard-cutover variant.
3. **Helm chart location.** No Helm manifests exist inside this repo; the deploy manifests
   (image keys, env) live in a separate infra repo. **VERIFY and update them there** for both forward
   and rollback.
4. **Multi-tenant service availability.** Multi-tenancy needs `MULTI_TENANT_URL` → tenant-manager
   service when `MULTI_TENANT_ENABLED=true`. **VERIFY** this service is reachable in the target
   environment (it may be staging/dev only). Multi-tenancy is config-driven and reversible
   (`MULTI_TENANT_ENABLED=false`) — no data one-way door.
5. **Ledger fee-collection write schema.** If ledger v4 wrote fee collections with a schema differing
   from v3, a ledger rollback without a down-migration can fail. This is a **ledger** concern;
   **VERIFY** with the ledger owner if ledger is rolled back in the same window.
6. **Exact CRM route paths/version prefix.** The curl examples are illustrative. **VERIFY** against
   `components/ledger/internal/adapters/http/in/routes.go` and `crm_routes.go` for the deployed build.

---

## References

- `docs/auth/RBAC-NAMESPACES.md` — X1 gate definition, migration matrix, fail-closed model (authoritative).
- `components/ledger/internal/adapters/http/in/crm_routes.go` (`:20`, `:62-79`) — the flip and authz calls.
- `components/ledger/internal/adapters/http/in/routes.go` (`:15-16` names, `:307-313` `protectedMidaz`/`protectedRouting`) — namespace helpers.
- `components/ledger/internal/adapters/http/in/fees_routes.go` (`:18`), `pkg/constant/module.go` (`:24`) — fees namespace (unchanged).
- `docs/standards/telemetry.md`, `docs/standards/error-handling.md` — logging/error conventions for triage.
