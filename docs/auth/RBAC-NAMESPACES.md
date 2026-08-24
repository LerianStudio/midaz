# Auth / RBAC Namespaces (R9)

After the monorepo consolidation, the holder/instrument (CRM) and fee/billing routes are served
by the **unified ledger binary** on `:3002` alongside the onboarding/transaction/routing routes.
The HTTP routes were merged into one Fiber app. As part of the v4 Instruments rename (D-10), the
CRM routes were **flipped from the `plugin-crm` namespace into the host ledger's `midaz`
namespace** in code; as of the B2 reversal (see §4), the **fee/billing routes were likewise
flipped from `plugin-fees` into `midaz`** in the embedded ledger binary, and the ledger's
`plugin-fees` manifest/embed/publisher were removed. The standalone `plugin-fees` and
`plugin-crm` services keep their own slugs in their own deploy units — the flip is scoped to the
**embedded** ledger binary only.

> **Route merge ≠ authz merge.** The tenant-manager RBAC policies key on these literal namespace
> strings. Flipping a namespace (collapsing `plugin-crm` **or `plugin-fees`** into `midaz`) orphans
> every tenant's existing `plugin-crm:*` / `plugin-fees:*` grant against the old key. The in-code
> flip lands at v4; the coordinated tenant-manager policy migration is **X1 — Fred-owned, executed
> with the plugin-auth team at v4 finalization (a release/deploy gate, NOT a merge gate)**. See
> "X1 — policy migration" below.

> **Driver — BOLA one-identity-one-slug.** The receiver in plugin-identity (`:4001`) enforces BOLA:
> the publishing app's `ClientId` MUST equal the caller's `callerClientID`. The embedded ledger app
> publishes under a single identity (`midaz`), so it CANNOT also publish a `plugin-fees` manifest —
> that request is rejected `403`. One identity → one slug. This is why fees fold into `midaz` in the
> embedded binary (B2, §4); it does NOT affect the standalone `plugin-fees` deploy, which publishes
> under its own identity.

> **Route shape ≠ authz key.** Separately from the namespace flip, the CRM and composition routes
> moved from header-scoped organization (`X-Organization-Id`) to path-scoped
> (`/v1/organizations/{organization_id}/...`; composition adds `/ledgers/{ledger_id}`), and the
> fee/billing routes followed on 2026-06-07. Both changes are purely URL shape — the
> `namespace:resource:action` keys below are byte-identical to before, so the X1 grant migration
> is unaffected and no `plugin-fees` policy migration exists. See `docs/api/SCOPING.md`
> (R22 reversed, now exception-free).
>
> The same holds for the **ledger-scoped fee/billing surface on `/v2`** (2026-08-01). The twelve
> operations are served at two scopes — organization-scoped on `/v1`, ledger-scoped on `/v2` — and
> both attach the identical guard chain from one shared table (`feeGuardRoutes` in
> `fees_routes.go`) with the same `(resource, action)` tuples.
> A grant that authorizes a `/v1` fee call authorizes its `/v2` twin, and vice versa.
>
> _Superseded by B2 (§4):_ the shared guard table now keys on the `midaz` namespace (was
> `plugin-fees`). The URL-shape claim above is unchanged; what changed is the namespace literal, so
> the fee routes now DO ride the X1 grant migration (see §5) instead of "no policy surface to
> migrate." The `(resource, action)` tuples are still byte-identical across `/v1` and `/v2`.

## The namespaces

In the embedded binary the merged app calls `auth.Authorize(<namespace>, <resource>, <action>)`
under a **single** namespace literal — `midaz` — after the B2 reversal (§4) folded fees in
alongside CRM:

| Namespace | Owner / code | Resources | Source |
|-----------|--------------|-----------|--------|
| `midaz` | ledger — `midazName` const; CRM (collapsed package) — `ApplicationName` const; fees (embedded) — shared `midazName` const via `protectedFees` | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings`, `account-types`, `operation-routes`, `transaction-routes`, `holders`, `instruments`, `packages`, `estimates`, `billing-packages`, `billing-calculate` | `components/ledger/internal/adapters/http/in/routes.go` (`midazName = "midaz"`, helper `protectedMidaz`); `crm_routes.go` (`const ApplicationName = "midaz"`) for the `holders`/`instruments` resources; `fees_routes.go` (table `feeGuardRoutes`, helper `protectedFees`, which uses the shared `midazName` const from `routes.go`) for the `packages`/`estimates`/`billing-packages`/`billing-calculate` resources — there is no `feeshared` authz const. The ledger-scoped `/v2` fee twins in `fees_v2_register.go` (`RegisterFeesV2RoutesToApp`) attach the same `feeGuardRoutes` table through the same helper — same namespace, same tuples |

> **Mongo module ≠ authz slug.** The fee tenant-manager MODULE name is a SEPARATE literal
> (`pkg/constant.ModuleFees = "fees-api"`) and carries no authz meaning — it did NOT move when the
> fee authz slug flipped to `midaz`. Authz namespace and tenant module are independent: fees having
> its own Mongo tenant client never justified a separate authz namespace (see §4, B2).

> **Standalone deploys keep their slugs.** This single-namespace picture is the **embedded ledger
> binary**. The standalone `plugin-fees` and `plugin-crm` services publish under their own
> identities in their own deploy units and retain `plugin-fees` / `plugin-crm` as their authz
> slugs.

The `account-types`, `operation-routes`, and `transaction-routes` resources authorize under the
`midaz` namespace (helper `protectedMidaz`), parity with `main` — there is no separate `routing`
namespace in the ledger binary. The tenant-manager grant re-key for any environment that still holds
`routing:*` grants remains gated to X1 (below).

The `(<action>)` dimension is the HTTP verb mapped to `get` / `post` / `patch` / `delete`. The CRM
`related-parties` DELETE authorizes under the `instruments` resource (sub-resource maintenance,
verb `delete`), not its own resource.

## Tenant-manager policy-key coupling

The two namespace strings above are the **policy keys** that tenant-manager RBAC policies are
written against. Authorization for a request resolves as:

```
auth.Authorize(<namespace>, <resource>, <action>)  →  tenant-manager policy lookup keyed on <namespace>
```

Consequences:

- A tenant granted `midaz:holders:post` can create holders. After the v4 flip, the CRM
  `holders`/`instruments` resources share the `midaz` namespace surface with the ledger's own
  resources; `plugin-fees:*` remains an independent policy surface.
- Flipping a namespace literal in code (here `plugin-crm` → `midaz`) without a coordinated
  tenant-manager policy migration orphans every existing grant under the old key. That migration
  is X1 (below) — the in-code flip is intentional and lands at v4; the policy migration is gated
  to release, not to merge.
- The fee/billing namespace literal is intentionally identical (`plugin-fees`) across the
  ledger route registrar and `components/ledger/pkg/feeshared`, so the embedded fee code and its
  authz key on the same string. Its Mongo tenant client keys on a different string:
  `pkg/constant.ModuleFees` (`fees-api`) is the tenant-manager MODULE name, matching what
  tenant-manager provisions for fees. Authz namespace and tenant module are independent — renaming
  one does not move the other.

## X1 — policy migration (`plugin-crm` → `midaz`)

The v4 in-code flip removes `plugin-crm` from the codebase and repoints the CRM routes to the
`midaz` namespace (resource `aliases` → `instruments`; `holders` unchanged; related-parties under
the `instruments` resource). Because tenant-manager RBAC policies key on the literal namespace
string, this **orphans every tenant's `plugin-crm:*` grant** until the corresponding policies are
re-issued under the new keys.

The grant matrix tenants migrate **to**:

```
plugin-crm:holders:{get,post,patch,delete}    →  midaz:holders:{get,post,patch,delete}      (resource unchanged)
plugin-crm:aliases:{get,post,patch,delete}    →  midaz:instruments:{get,post,patch,delete}  (resource renamed)
   (related-parties DELETE)                    →  midaz:instruments:delete                    (sub-resource)
```

This policy migration is **X1 — Fred-owned, executed with the plugin-auth team at v4
finalization**. It is a **release/deploy gate, NOT a merge gate**: merging the v4 code flip does
not require the policies to be migrated first. Local/dev deployments with auth disabled are
unaffected. Until the migration runs, environments with auth enabled must have their
tenant-manager policies updated in lockstep with the v4 release.

The remaining namespaces (`midaz`, `plugin-fees`) are the authoritative authorization
contract for the unified binary. Risk **R9** (the original namespace divergence) is closed for
CRM by this flip; the fee namespace stays distinct by design.

---

# Cross-monorepo namespace strategy (Epic 3.3 — auth-stabilization)

The section above scopes R9/X1 to the **ledger binary** and the CRM flip. This section widens the
lens to the **whole monorepo**: after consolidation, three authz namespaces ship across the two Go
deploy units. This is a **decision document only** — it records options + a recommendation for the
owner and **defers all execution to the X1 gate**. No namespace literal is changed by this doc.

## 1. Current state — three namespaces across two deploy units

`auth.Authorize(<namespace>, <resource>, <action>)` (lib-auth v3.0.0, global RBAC check) is called
under three distinct namespace literals across the monorepo:

Refs below are anchored on **symbol names**, not line numbers: line numbers rot silently on the
next edit to the file, and four of the eight that used to sit in this table had already drifted.

| Namespace | Deploy unit | Resources (verified) | Source (file + symbol) |
|-----------|-------------|----------------------|------------------------|
| `midaz` | ledger (`:3002`) | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings`, `account-types`, `operation-routes`, `transaction-routes`, `holders`, `instruments` | `components/ledger/internal/adapters/http/in/routes.go` (`midazName`, helper `protectedMidaz`); `crm_routes.go` (`ApplicationName`) for `holders`/`instruments` |
| `plugin-fees` | ledger (`:3002`, same binary) | `packages`, `estimates`, `billing-packages`, `billing-calculate` | `components/ledger/internal/adapters/http/in/fees_routes.go` (`feesApplicationName`, table `feeGuardRoutes`, helpers `attachFeeGuards`/`protectedFees`); the ledger-scoped `/v2` twins in `fees_v2_register.go` (`RegisterFeesV2RoutesToApp`) attach the same table |
| `tracer` | tracer (`:4020`) | `reservations`, `audit-events` | `components/tracer/pkg/constant/app.go` (`ApplicationName`); wired via `components/tracer/internal/bootstrap/config.go` (`AppName:`), consumed at `middleware/auth_guard.go` (`(*AuthGuard).Protect`) |

> **Audit-ref check:** every symbol above resolves in the tree as written. `account-types`,
> `operation-routes`, and `transaction-routes` authorize under `midaz` (helper `protectedMidaz`),
> parity with `main`; the ledger binary carries no separate `routing` namespace. Two of the three
> namespaces (`midaz`/`plugin-fees`) ship from the one ledger binary; only `tracer` is a separate
> deploy unit. Serving fees at a second scope (`/v2`) added no namespace: the count stays three.

## 2. Consequence — silent 403 across the platform

The three literals are independent **policy keys** in tenant-manager. A grant under one key is
invisible to the others. So a tenant provisioned with `midaz:*` (a natural "give me everything"
grant) **silently 403s** every `plugin-fees` and `tracer` resource:

```
midaz:transactions:post     → 200   (granted)
plugin-fees:packages:post     → 403  (no plugin-fees grant — silent)
tracer:audit-events:get       → 403  (no tracer grant — silent)
```

Failure mode is the worst kind: **silent 403, no hint that the answer is "wrong namespace".** To
authorize one logical platform, an integrator must provision grants in **three namespaces** and
discover the boundaries by hitting 403s. Two of those boundaries (`midaz`/`plugin-fees`)
live in a single binary, which makes the split especially non-obvious.

## 3. Trust-model context (owner decision, 2026-06-06)

Recorded verbatim from the owner, resolving Epic 2.2 (fees `X-Organization-Id` org-claim
cross-check) as **no-action** [the header itself was since removed — fees moved to path-scoped
`organization_id` on 2026-06-07; the decision carries over unchanged to the path parameter]:

> "não existe risco. o tenant owner é responsável efetivamente por todas as orgs embaixo dele."

The **tenant** is the platform's principal and trust boundary. There is **no organization
dimension in authz**: lib-auth v3.0.0 `Authorize(sub, resource, action)`
(`auth/middleware/middleware.go:216`) is a global RBAC check — no org argument exists. `tenantId`
(seeded by `MarkTrustedAuthAssertion`, `pkg/net/http/protected_routes.go:40-69`) is a tenant-DB
selector, and one tenant holds many organizations. Intra-tenant org targeting is **by design, not a
gap**: a caller authorized for a resource can target any org under their tenant, exactly as midaz's
path-based `organization_id` routes already work. This closed Epic 2.2 with no code change and no
upstream org-dimension issue.

**Why it matters here:** the namespace question is purely about *grant ergonomics within a tenant* —
not about isolation. Isolation is the tenant boundary (DB-level), already enforced. Collapsing or
keeping namespaces changes how many grants an integrator provisions; it does **not** change the
security model.

## 4. Decision options + recommendation

Three independent sub-decisions:

| # | Decision | Options | Trade-off |
|---|----------|---------|-----------|
| **A** | `routing` vs `midaz` | **(A1) Unify** `routing` into `midaz` — they share one binary and one midaz domain. **(A2) Keep split.** | A1 erases a same-binary footgun (the least defensible split — `account-types` is a sibling of `assets`). Cost: one-time policy re-key for `routing:*` grants. A2 costs nothing now but leaves the most surprising boundary in place. |
| **B** | `plugin-fees` | **(B1) Keep separate** per the R9 closure recorded above ("the fee namespace stays distinct by design"). **(B2) Fold** into `midaz`. **← DECISION (2026-08-21), reverses the B1 recommendation below.** | B1 preserves the deliberate fee/billing separation already ratified under R9 — fees is a distinct product surface with its own Mongo tenant client. **But that rationale does NOT hold for authz:** the Mongo tenant client keys on a SEPARATE literal (`pkg/constant.ModuleFees = "fees-api"`), not on the authz slug, so "fees has its own Mongo tenant client" never supported an authz-namespace split. B2 was forced by the **BOLA one-identity-one-slug** driver (plugin-identity `:4001` requires `app.ClientId == callerClientID`): the embedded ledger app publishes under one identity, so an app named `midaz` publishing a `plugin-fees` manifest is rejected `403`. B2 costs one coordinated grant re-key (folded into X1, §5); it applies to the **embedded** binary only — the standalone `plugin-fees` deploy keeps its slug. |
| **C** | `tracer` | **(C1) Keep per-deploy-unit** namespace with a documented grant bundle. **(C2) Align** under `midaz`. | C1 keeps operational clarity: the separately-deployed tracer owns its policy surface, deployable/grantable in isolation. C2 would fold a separate deploy unit's policy surface into `midaz` — a single `midaz:*` grant would then authorize the audit log too, widening blast radius and coupling unrelated release cadences. |

**Recommendation (one call, owner-gated):**

- **A1 — unify `routing` into `midaz`. Landed in code.** `account-types`, `operation-routes`, and
  `transaction-routes` now register under `midaz` (helper `protectedMidaz`), parity with `main`;
  the ledger binary no longer defines a `routing` namespace. Same binary, same domain, smallest
  blast radius, kills the least-defensible split. The tenant-manager grant re-key for any
  environment that still holds `routing:*` grants remains gated to X1 (§5).
- ~~**B1 — keep `plugin-fees` separate.** Honors the R9 closure; do not reopen a settled decision for
  marginal grant-count savings.~~ **REVERSED to B2 (2026-08-21) — fold `plugin-fees` into `midaz`
  in the embedded ledger binary.** The B1 rationale rested on R9's "distinct product surface with
  its own Mongo tenant client," but the Mongo module (`ModuleFees = "fees-api"`) is a separate
  literal from the authz slug and never bound the authz namespace. The forcing driver is **BOLA
  one-identity-one-slug** in plugin-identity `:4001` (`app.ClientId == callerClientID`): the
  embedded ledger publishes under one identity, so it cannot also publish a `plugin-fees` manifest —
  the request 403s. Fees now authorize under `midaz` (routes already flipped; the ledger's
  `plugin-fees` manifest/embed/publisher were removed). The grant re-key rides the single X1 window
  (§5). Scope: **embedded binary only** — the standalone `plugin-fees` service keeps its slug.
- **C1 — keep `tracer` per-deploy-unit**, but ship a **documented grant bundle** (the
  table in §1 plus a published "platform grant set") so integrators provision all of them knowingly
  instead of discovering them via 403s. A per-deploy-unit namespace matches the deploy topology and
  keeps release cadences decoupled; the silent-403 problem is solved by **documentation**, not by
  collapsing the boundary.

Net (as originally recommended): the platform moves from **four → three** namespaces (`midaz`
absorbs `routing`; `plugin-fees` and `tracer` stay). **Under the B2 reversal it is four → two in the
embedded binary** (`midaz` absorbs both `routing` and `plugin-fees`; only `tracer` stays as a
separate deploy unit), with the residual split documented as a published grant bundle. This trades a
small one-time `routing` + `plugin-fees` migration for permanent removal of the most surprising
footguns, while keeping the one operationally-meaningful boundary that tracks a separate deploy unit.

## 5. Migration sketch — sequence with X1, one break not two

Any `routing → midaz` **or `plugin-fees → midaz` (B2)** re-key is the **same class of breaking
change** as the X1 `plugin-crm → midaz` policy migration: it orphans existing grants under the old
key until tenant-manager policies are re-issued. Shipping them in **separate releases multiplies the
integrator's grant-migration pain.** The decision is therefore to **fold the `routing` and
`plugin-fees` re-keys into the single X1 gate** so integrators absorb **one** coordinated breaking
grant migration. X1 is a **release/deploy gate, NOT a merge gate**: the code flips (CRM, routing,
fees) merge ahead; grants migrate at the release window. The standalone `plugin-fees` / `plugin-crm`
deploys keep their slugs and are NOT part of this migration.

Combined X1 grant matrix (additive to the existing `plugin-crm → midaz` matrix above):

```
# X1 — CRM flip (already specified above)
plugin-crm:holders:{get,post,patch,delete}  →  midaz:holders:{get,post,patch,delete}
plugin-crm:aliases:{get,post,patch,delete}  →  midaz:instruments:{get,post,patch,delete}
   (related-parties DELETE)                  →  midaz:instruments:delete

# Epic 3.3 — routing unification (A1 landed in code; grant re-key at the gate)
routing:account-types:{get,post,patch,delete}      →  midaz:account-types:{...}
routing:operation-routes:{get,post,patch,delete}   →  midaz:operation-routes:{...}
routing:transaction-routes:{get,post,patch,delete} →  midaz:transaction-routes:{...}

# B2 — fees fold (embedded binary; routes flipped in code, grant re-key at the gate)
plugin-fees:packages:{get,post,patch,delete}          →  midaz:packages:{...}
plugin-fees:estimates:{get,post,patch,delete}         →  midaz:estimates:{...}
plugin-fees:billing-packages:{get,post,patch,delete}  →  midaz:billing-packages:{...}
plugin-fees:billing-calculate:{get,post,patch,delete} →  midaz:billing-calculate:{...}

# UNCHANGED — no migration
tracer:*        (C1 — stays distinct; documented grant bundle)

# NOT migrated — standalone deploys keep their own slugs
plugin-fees:*   (standalone plugin-fees service — its own identity/deploy unit)
plugin-crm:*    (standalone plugin-crm service — its own identity/deploy unit)
```

Sequencing within the single X1 release:

1. **Code — landed.** `account-types`, `operation-routes`, and `transaction-routes` register under
   `midazName` via `protectedMidaz`; the `routing` const and helper are removed from `routes.go`.
   Merge ≠ authz merge: the code change merges ahead; grants migrate at the release gate.
2. **Policy:** tenant-manager re-issues `plugin-crm:*`, `routing:*` **and `plugin-fees:*` (B2)**
   grants under `midaz` in one migration window, executed with the plugin-auth team.
3. **Docs:** publish the platform grant bundle (the §1 table, post-unification: `midaz` + `tracer`
   for the embedded binary; the standalone `plugin-fees` / `plugin-crm` deploys retain their own
   slugs) so integrators see the full namespace set up front.

**This document decides nothing unilaterally.** It records the current state, the consequence, the
trust model, and the option set (with the B1→B2 reversal annotated in §4, not erased). The **code**
decisions have landed — A1 (`account-types`, `operation-routes`, `transaction-routes`) and B2 (fee
routes flipped to `midaz`; the ledger's `plugin-fees` manifest/embed/publisher removed) are folded
into `midaz` in the embedded ledger binary (§4, §5). What remains **deferred to the X1 gate** is the
**execution** of the tenant-manager policy migration and the re-key of any environment still holding
`routing:*` or `plugin-fees:*` grants — owner-decided with the plugin-auth team, so that the only
namespace break integrators ever absorb is the single coordinated X1 migration. The standalone
`plugin-fees` / `plugin-crm` services keep their slugs and are outside this migration.
