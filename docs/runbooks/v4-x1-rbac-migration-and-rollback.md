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
> Related docs: `docs/auth/RBAC-NAMESPACES.md` (authoritative for X1), `docs/plans/reporter-engine-migration.md`,
> `CLAUDE.md` (architecture), `docs/standards/telemetry.md`, `docs/standards/error-handling.md`.

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
  Each calls `auth.Authorize(ApplicationName, resource, action)` (`crm_routes.go:36-53`).
- **Net effect:** the authz namespace for these routes moves from `plugin-crm` → `midaz`.

Namespace layout in v4 (for context — only the CRM rows are migrating):

| Namespace | Resources | Source |
|-----------|-----------|--------|
| `midaz` | organizations, ledgers, assets, asset-rates, portfolios, segments, accounts, balances, transactions, operations, settings | `routes.go:19`, `protectedMidaz` `routes.go:183-185` |
| `midaz` (flipped in v4) | **holders, instruments, related-parties** | `crm_routes.go:20`, `crm_routes.go:36-53` |
| `routing` | account-types, operation-routes, transaction-routes | `routes.go:20`, `protectedRouting` `routes.go:187-189` |
| `plugin-fees` (**UNCHANGED**) | packages, estimates, billing-packages, billing-calculate | `fees_routes.go:19`, `pkg/constant/module.go:24` |

> **Fees do NOT migrate.** `plugin-fees:*` is intentionally preserved with no migration
> (`RBAC-NAMESPACES.md:7-8, 15, 84-86`). Do not touch fees grants during X1.
>
> **Routing does NOT migrate either (this build).** `account-types`, `operation-routes`, and
> `transaction-routes` stay under the `routing` namespace — `protectedRouting` still calls
> `auth.Authorize(routingName, ...)` (`routes.go:188`; `routingName = "routing"` at `routes.go:20`).
> X1 touches CRM holders/instruments only. `RBAC-NAMESPACES.md` Epic 3.3 *proposes* folding
> `routing:* → midaz:*` into a later release; if a future build flips `protectedRouting` to `midazName`,
> routing grants must migrate too and the matrix + smoke tests below must add the routing routes.
> **Re-verify `routes.go:188` before each release** to confirm routing has not been folded into X1.

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
- [ ] **Reporter / secrets readiness** (these ship in the same v4 release and have their own one-way
      manual steps — see "v4 deploy residuals" below): the renamed secrets
      `CRYPTO_HASH_SECRET_KEY_CRM`, `CRYPTO_ENCRYPT_SECRET_KEY_CRM`, the `DATASOURCE_CRM_*` vars, and
      `RUN_MODE` per reporter Deployment are all staged.
- [ ] Fees grants left **untouched** (no `plugin-fees` changes in this release).
- [ ] Local/dev with auth disabled confirmed unaffected (no policy work needed there).

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

2. **Deploy v4 code** to the environment (ledger unified binary + reporter, per the deploy-residuals
   section). With both grant sets present, CRM routes authorize the moment the binary starts checking
   `midaz:*`.

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
> confirmed gap). The steps below are derived from the fail-closed enforcement model and the deploy
> residuals; treat sequencing details marked **VERIFY** as unconfirmed.

Rolling back v4 → pre-v4 re-introduces the old `plugin-crm` namespace literals in `auth.Authorize`.
If tenants are now on `midaz:*` grants, those grants **orphan again** and all CRM routes 403. So the
policy state must be reverted alongside the code.

### Rollback decision — when, and the point of no return

- **Roll back if:** authorized tenants see sustained CRM `403`s after the X1 cutover that the migration
  matrix cannot explain; reporter `/readyz` stays red; or reporter logs show secret-decrypt failures.
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
   restore: module import path, reporter binary file structure (git handles the file moves),
   fetcher HTTP client code, notification consumer, reconciler, and the env-var **names** the old code reads.

3. **Reverse the v4 deploy residuals that code rollback does NOT undo** — see the next section. These
   are manual operator steps.

4. **Decommission the new `midaz:*` CRM grants** once the rollback is confirmed healthy (optional —
   may park them for a re-roll-forward lane).

5. **Smoke test** CRM authz on the rolled-back (pre-v4) binary, plus the reporter/ledger health checks.

---

## ONE-WAY DOORS — what a code rollback does NOT undo

A `git revert` restores code, but the following require explicit operator action. **Validate each
before declaring rollback complete.**

### Policy / authz state
- **Tenant-manager grants.** Code rollback re-introduces `plugin-crm:*` checks but does NOT touch
  `tenant-manager`. The `midaz:*` grants issued during X1 must be reverted to `plugin-crm:*` by
  Fred + plugin-auth, or CRM routes 403. (`RBAC-NAMESPACES.md` — no automated reverse path.)

### Secrets & environment (must be renamed/restored by hand)
- **CRM crypto secret keys.** v4 renamed `CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM` →
  `CRYPTO_HASH_SECRET_KEY_CRM` and `CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM` →
  `CRYPTO_ENCRYPT_SECRET_KEY_CRM` (commit `c367752e9`; vars `CRYPTO_HASH_SECRET_KEY_CRM` /
  `CRYPTO_ENCRYPT_SECRET_KEY_CRM` in `components/reporter/.env.example`).
  Pre-v4 code reads the `*_PLUGIN_CRM` names. **Rename the Kubernetes Secrets / env back** or the
  rolled-back reporter cannot decrypt.
- **Datasource vars.** `DATASOURCE_CRM_*` (config name `crm`) → revert to `DATASOURCE_PLUGIN_CRM_*`
  (config name `plugin_crm`).
- **Deleted fetcher vars.** v4 removed `FETCHER_URL`, `FETCHER_ENABLED`, `M2M_TOKEN_PROVIDER_*`,
  `FETCHER_RECONCILER_INTERVAL` (commit `9d244290d`). v4 has **no code path that reads `FETCHER_*` at
  all.** Pre-v4 reporter REQUIRES them and the **fetcher service must be running** (it was spun down
  in v4 infra consolidation). Restore the vars AND re-create the fetcher deployment, or the
  rolled-back reporter fails initializing its fetcher HTTP client.
- **`RUN_MODE`.** New in v4 only; pre-v4 binaries ignore it. Remove `RUN_MODE` from the rolled-back
  Deployments.

### Deploy topology
- **Reporter binary split.** v4 ships ONE binary at `components/reporter` selected by `RUN_MODE`
  (`api` :4005 distroless / `worker` :4006 alpine+Chromium / `all` = dev only). Pre-v4 is TWO separate
  binaries. On rollback, **restore two Deployment templates** (reporter-manager, reporter-worker).
  Git restores the two source trees from history automatically; the **Helm/Deployment manifests are
  manual.** (`reporter/cmd/app/main.go:50`, `reporter/internal/app/app.go:61-72`,
  reporter-{manager,worker}/Dockerfile anchors.)
- **Port `:4006` overlap (returning fetcher ↔ v4 worker).** The retired pre-v4 fetcher service bound
  `:4006` (`FETCHER_URL=http://fetcher:4006`), the SAME port the v4 reporter-worker uses for `HEALTH_PORT`
  (`reporter/internal/worker/bootstrap/config.go:23`). Separate pods don't hard-collide, but a stale v4
  reporter-worker Deployment left up while the fetcher returns can produce **Service/route overlap on
  `:4006`**. **Tear down the v4 reporter Deployments (or confirm no `:4006` Service overlap) BEFORE
  re-creating the fetcher deployment.**
- **Mode-aware config validation gap.** A v4 pod that ran `RUN_MODE=api` may have been deployed with
  partial config (e.g. no RabbitMQ/S3, which only the worker validates —
  `reporter/internal/manager/bootstrap/config.go:133-249`,
  `reporter/internal/worker/bootstrap/config_validation.go:18-76`). Pre-v4 used **unified validation**
  and will **fail to start** if that full config is absent. **Ensure FULL config is present before
  rolling back**, or migrate Deployment-by-Deployment.
- **Readyz probes.** v4 probes are mode-aware (`reporter/internal/manager/adapters/http/in/readyz_handler.go:98-114`,
  `reporter/internal/worker/bootstrap/health-server.go:145-156`). Pre-v4 unified probe expects full
  config; same mitigation as above.

### Stored data / templates
- **Report templates referencing `{{ crm.* }}`.** If any report template was authored/stored
  (MongoDB reporter-db) in the v4 era using the `crm` datasource token, pre-v4 code does NOT recognize
  `crm` and the template breaks. **Manually migrate stored templates** `{{ crm.* }}` → `{{ plugin_crm.* }}`.
  **VERIFY the template storage format** — if templates are code-generated rather than stored, this is
  moot (see Known Gaps).
- **Module path `/v4`.** `go.mod:1` is `github.com/LerianStudio/midaz/v4`. Internal deploy rollback
  works via git+rebuild, but any **external library consumer** importing `midaz/v4` sees a breaking
  import path on rollback to `v3` and must migrate imports. (Monorepo-wide bump; reporter/ledger/tracer
  do not publish independently.)

### Data that is SAFE (no action needed)
- **`extraction_mapping` MongoDB collection.** Deleted in v4, but v4's in-process synchronous
  extraction never writes it. Pre-v4 code sees an empty collection = same as a fresh start. No cleanup.
  (`docs/plans/reporter-engine-migration.md:124`.)
- **`aliases_<orgId>` / `holders_<orgId>` collections.** Org-scoped names are a pre-existing ledger
  pattern, NOT a v4 change. Reporter reads them in-process; rollback is read-safe.
  (`components/ledger/internal/crm/adapters/mongodb/instrument/instrument.mongodb.go`.)
- **Ledger fee collections.** Reporter only READS these; it never writes them. Fee-collection schema
  is a LEDGER concern — if ledger is rolled back, handle its fee schema separately. Reporter rollback
  alone is safe here. (`components/ledger/pkg/fee/`.)
- **Postgres migrations.** No v4-specific reporter migrations; reporter runs none (read-only on
  external datasources). Ledger/tracer own their own migration dirs independently.

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
- **Reporter readiness:** both rolled-back deployments report ready.
  ```
  curl -i https://<reporter-manager-host>:4005/readyz   # REST API surface
  curl -i https://<reporter-worker-host>:4006/readyz    # consumer + health surface
  ```
  A failing `/readyz` on the rolled-back reporter most likely means partial config carried over from
  v4 mode-aware validation — supply the full unified config.
- **Fetcher dependency:** confirm the fetcher service is running and the pre-v4 reporter initialized
  its fetcher HTTP client (no `FETCHER_URL` missing errors in logs). Logs are structured per
  `docs/standards/telemetry.md`; search by the immediate failing resource, not by broad scope IDs.
- **Reports render:** generate one report end-to-end and confirm the final artifact lands in S3.
- **No secret-decrypt errors:** confirm the `*_PLUGIN_CRM` crypto keys were restored (no decrypt
  failures in reporter logs).

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
   (RUN_MODE per Deployment, image keys, env) live in a separate infra repo. **VERIFY and update them
   there** for both forward and rollback.
4. **Fetcher retirement runbook.** v4 retired the fetcher service infra-side, but no fetcher
   spin-up/spin-down runbook was found. **VERIFY the fetcher can be re-created** before committing to
   a reporter rollback.
5. **Multi-tenant service availability.** v4 reporter multi-tenancy needs `MULTI_TENANT_URL` →
   tenant-manager service when `MULTI_TENANT_ENABLED=true`. **VERIFY** this service is reachable in the
   target environment (it may be staging/dev only). Multi-tenancy is config-driven and reversible
   (`MULTI_TENANT_ENABLED=false`) — no data one-way door.
6. **Report template storage format.** Whether report templates are stored blobs (then `{{ crm.* }}`
   migration is breaking) or code-generated (then safe) is **UNCERTAIN. VERIFY** the storage layer
   before assuming the template migration is or isn't required.
7. **Ledger fee-collection write schema.** If ledger v4 wrote fee collections with a schema differing
   from v3, a ledger rollback without a down-migration can fail. This is a **ledger** concern outside
   reporter; **VERIFY** with the ledger owner if ledger is rolled back in the same window.
8. **Exact CRM route paths/version prefix.** The curl examples are illustrative. **VERIFY** against
   `components/ledger/internal/adapters/http/in/routes.go` and `crm_routes.go` for the deployed build.

---

## References

- `docs/auth/RBAC-NAMESPACES.md` — X1 gate definition, migration matrix, fail-closed model (authoritative).
- `components/ledger/internal/adapters/http/in/crm_routes.go` (`:20`, `:36-53`) — the flip and authz calls.
- `components/ledger/internal/adapters/http/in/routes.go` (`:19-20`, `:183-189`) — namespace helpers.
- `components/ledger/internal/adapters/http/in/fees_routes.go` (`:19`), `pkg/constant/module.go` (`:24`) — fees namespace (unchanged).
- `docs/plans/reporter-engine-migration.md` — reporter in-process engine migration, deleted env/collections.
- Commits: `c367752e9` (plugin_crm → crm rename), `9d244290d` (retire fetcher HTTP path), `bfa9b4b69` (reporter binary consolidation).
- `components/reporter/.env.example` (vars `CRYPTO_*_SECRET_KEY_CRM`, `DATASOURCE_CRM_*`), `reporter/internal/{manager,worker}/bootstrap/` — config & validation.
- `docs/standards/telemetry.md`, `docs/standards/error-handling.md` — logging/error conventions for triage.
