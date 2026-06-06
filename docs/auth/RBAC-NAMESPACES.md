# Auth / RBAC Namespaces (R9)

After the monorepo consolidation, the holder/instrument (CRM) and fee/billing routes are served
by the **unified ledger binary** on `:3002` alongside the onboarding/transaction/routing routes.
The HTTP routes were merged into one Fiber app. As part of the v4 Instruments rename (D-10), the
CRM routes were **flipped from the `plugin-crm` namespace into the host ledger's `midaz`
namespace** in code; the fee/billing routes still authorize under their original `plugin-fees`
namespace string.

> **Route merge ≠ authz merge.** The tenant-manager RBAC policies key on these literal namespace
> strings. Flipping a namespace (collapsing `plugin-crm` into `midaz`) orphans every tenant's
> existing `plugin-crm:*` grant against the old key. The in-code flip lands at v4; the
> coordinated tenant-manager policy migration is **X1 — Fred-owned, executed with the plugin-auth
> team at v4 finalization (a release/deploy gate, NOT a merge gate)**. See "X1 — policy migration"
> below. The fee namespace (`plugin-fees`) is unchanged and unaffected.

> **Route shape ≠ authz key.** Separately from the namespace flip, the CRM and composition routes
> moved from header-scoped organization (`X-Organization-Id`) to path-scoped
> (`/v1/organizations/{organization_id}/...`; composition adds `/ledgers/{ledger_id}`). That change
> is purely URL shape — the `namespace:resource:action` keys below are byte-identical to before, so
> the X1 grant migration is unaffected by it. See `docs/api/SCOPING.md` (R22 reversed).

## The three namespaces

The merged binary calls `auth.Authorize(<namespace>, <resource>, <action>)` under three distinct
namespace literals:

| Namespace | Owner / code | Resources | Source |
|-----------|--------------|-----------|--------|
| `midaz` | ledger — `midazName` const; CRM (collapsed package) — `ApplicationName` const | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings`, `holders`, `instruments` | `components/ledger/internal/adapters/http/in/routes.go` (`midazName = "midaz"`, helper `protectedMidaz`); `components/ledger/internal/adapters/http/in/crm_routes.go` (`const ApplicationName = "midaz"`) for the `holders`/`instruments` resources |
| `routing` | ledger — `routingName` const | `account-types`, `operation-routes`, `transaction-routes` | `components/ledger/internal/adapters/http/in/routes.go` (`routingName = "routing"`, helper `protectedRouting`) |
| `plugin-fees` | fees (embedded in ledger) | `packages`, `estimates`, `billing-packages`, `billing-calculate` | `components/ledger/internal/adapters/http/in/fees_routes.go` (`feesApplicationName = "plugin-fees"`); also `pkg/constant.ModuleFees = "plugin-fees"` and `components/ledger/pkg/feeshared/constant/app.go` |

The `(<action>)` dimension is the HTTP verb mapped to `get` / `post` / `patch` / `delete`. The CRM
`related-parties` DELETE authorizes under the `instruments` resource (sub-resource maintenance,
verb `delete`), not its own resource.

## Tenant-manager policy-key coupling

The three namespace strings above are the **policy keys** that tenant-manager RBAC policies are
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
  ledger route registrar, `pkg/constant.ModuleFees`, and `components/ledger/pkg/feeshared` so the
  embedded fee code, its Mongo tenant client, and its authz all key on the same string.

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

The remaining namespaces (`midaz`, `routing`, `plugin-fees`) are the authoritative authorization
contract for the unified binary. Risk **R9** (the original namespace divergence) is closed for
CRM by this flip; the fee namespace stays distinct by design.

---

# Cross-monorepo namespace strategy (Epic 3.3 — auth-stabilization)

The section above scopes R9/X1 to the **ledger binary** and the CRM flip. This section widens the
lens to the **whole monorepo**: after consolidation, five authz namespaces ship across the four Go
deploy units. This is a **decision document only** — it records options + a recommendation for the
owner and **defers all execution to the X1 gate**. No namespace literal is changed by this doc.

## 1. Current state — five namespaces across four deploy units

`auth.Authorize(<namespace>, <resource>, <action>)` (lib-auth v2.8.0, global RBAC check) is called
under five distinct namespace literals across the monorepo:

| Namespace | Deploy unit | Resources (verified) | Source (file:line) |
|-----------|-------------|----------------------|--------------------|
| `midaz` | ledger (`:3002`) | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings`, `holders`, `instruments` | `components/ledger/internal/adapters/http/in/routes.go:26` (`midazName = "midaz"`); `crm_routes.go:20` (`const ApplicationName = "midaz"`) for `holders`/`instruments` |
| `routing` | ledger (`:3002`, same binary) | `account-types`, `operation-routes`, `transaction-routes` | `components/ledger/internal/adapters/http/in/routes.go:27` (`routingName = "routing"`, helper `protectedRouting` at `:231`) |
| `plugin-fees` | ledger (`:3002`, same binary) | `packages`, `estimates`, `billing-packages`, `billing-calculate` | `components/ledger/internal/adapters/http/in/fees_routes.go:19` (`feesApplicationName = "plugin-fees"`) |
| `tracer` | tracer (`:4020`) | `reservations`, `audit-events` | `components/tracer/pkg/constant/app.go:7` (`const ApplicationName = "tracer"`); wired via `bootstrap/config.go:1127` → `AppName`, consumed at `middleware/auth_guard.go:86` |
| `reporter` | reporter-manager (`:4005`) | `templates`, `reports`, `deadlines`, `data-source`, `metrics` | `pkg/reporter/constant/app.go:7` (`const ApplicationName = "reporter"`); routes at `components/reporter-manager/internal/adapters/http/in/routes.go:71+` |

> **Audit-ref check:** every ref in the Epic 3.3 brief is accurate as written — `tracer` at
> `pkg/constant/app.go:7`, `reporter` at `pkg/reporter/constant/app.go:7`, `routing` splitting
> `account-types`/`operation-routes`/`transaction-routes` from their `midaz` siblings inside the
> **same ledger binary**. No corrections needed. (reporter-worker has health-only `:4006`, no REST
> API, so it contributes no namespace.)

## 2. Consequence — silent 403 across the platform

The five literals are independent **policy keys** in tenant-manager. A grant under one key is
invisible to the others. So a tenant provisioned with `midaz:*` (a natural "give me everything"
grant) **silently 403s** every `routing`, `plugin-fees`, `tracer`, and `reporter` resource:

```
midaz:transactions:post     → 200   (granted)
routing:operation-routes:post → 403  (no routing grant — silent)
plugin-fees:packages:post     → 403  (no plugin-fees grant — silent)
tracer:audit-events:get       → 403  (no tracer grant — silent)
reporter:reports:post         → 403  (no reporter grant — silent)
```

Failure mode is the worst kind: **silent 403, no hint that the answer is "wrong namespace".** To
authorize one logical platform, an integrator must provision grants in **five namespaces** and
discover the boundaries by hitting 403s. Three of those boundaries (`midaz`/`routing`/`plugin-fees`)
live in a single binary, which makes the split especially non-obvious.

## 3. Trust-model context (owner decision, 2026-06-06)

Recorded verbatim from the owner, resolving Epic 2.2 (fees `X-Organization-Id` org-claim
cross-check) as **no-action**:

> "não existe risco. o tenant owner é responsável efetivamente por todas as orgs embaixo dele."

The **tenant** is the platform's principal and trust boundary. There is **no organization
dimension in authz**: lib-auth v2.8.0 `Authorize(sub, resource, action)`
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
| **B** | `plugin-fees` | **(B1) Keep separate** per the R9 closure recorded above ("the fee namespace stays distinct by design"). **(B2) Fold** into `midaz`. | B1 preserves the deliberate fee/billing separation already ratified under R9 — fees is a distinct product surface with its own Mongo tenant client keyed on the same literal. B2 buys one fewer namespace at the cost of reopening a settled R9 decision and entangling billing grants with ledger grants. |
| **C** | `tracer` / `reporter` | **(C1) Keep per-deploy-unit** namespaces with documented grant bundles. **(C2) Align** under `midaz`. | C1 keeps operational clarity: each separately-deployed service owns its policy surface, deployable/grantable in isolation. C2 creates one mega-namespace spanning four deploy units — a single `midaz:*` grant would then authorize the audit log and the report engine, widening blast radius and coupling unrelated release cadences. |

**Recommendation (one call, owner-gated):**

- **A1 — unify `routing` into `midaz`.** Same binary, same domain, smallest blast radius, kills the
  least-defensible split. This is the single highest-value change.
- **B1 — keep `plugin-fees` separate.** Honors the R9 closure; do not reopen a settled decision for
  marginal grant-count savings.
- **C1 — keep `tracer` and `reporter` per-deploy-unit**, but ship a **documented grant bundle** (the
  table in §1 plus a published "platform grant set") so integrators provision all five knowingly
  instead of discovering them via 403s. Per-deploy-unit namespaces match the deploy topology and
  keep release cadences decoupled; the silent-403 problem is solved by **documentation**, not by
  collapsing the boundary.

Net: the platform moves from **five → four** namespaces (`midaz` absorbs `routing`; `plugin-fees`,
`tracer`, `reporter` stay), and the residual split is documented as a published grant bundle. This
trades a small one-time `routing` migration for permanent removal of the most surprising footgun,
while keeping the operationally-meaningful boundaries that track deploy units.

## 5. Migration sketch — sequence with X1, one break not two

Any `routing → midaz` re-key is the **same class of breaking change** as the X1 `plugin-crm → midaz`
policy migration: it orphans existing grants under the old key until tenant-manager policies are
re-issued. Shipping them in **two separate releases doubles the integrator's grant-migration pain.**
The decision is therefore to **fold the `routing` re-key into the X1 gate** so integrators absorb
**one** coordinated breaking grant migration.

Combined X1 grant matrix (additive to the existing `plugin-crm → midaz` matrix above):

```
# X1 — CRM flip (already specified above)
plugin-crm:holders:{get,post,patch,delete}  →  midaz:holders:{get,post,patch,delete}
plugin-crm:aliases:{get,post,patch,delete}  →  midaz:instruments:{get,post,patch,delete}
   (related-parties DELETE)                  →  midaz:instruments:delete

# Epic 3.3 — routing unification (proposed, A1, if accepted at the gate)
routing:account-types:{get,post,patch,delete}      →  midaz:account-types:{...}
routing:operation-routes:{get,post,patch,delete}   →  midaz:operation-routes:{...}
routing:transaction-routes:{get,post,patch,delete} →  midaz:transaction-routes:{...}

# UNCHANGED — no migration
plugin-fees:*   (B1 — stays distinct)
tracer:*        (C1 — stays distinct; documented grant bundle)
reporter:*      (C1 — stays distinct; documented grant bundle)
```

Sequencing within the single X1 release:

1. **Code:** flip `routingName`/`protectedRouting` to register under `midazName` (one-line per
   helper at `routes.go:27,231`), landing in the same release as the CRM flip. Merge ≠ authz merge:
   the code change can merge ahead; grants migrate at the release gate.
2. **Policy:** tenant-manager re-issues `plugin-crm:*` **and** `routing:*` grants under `midaz` in
   one migration window, executed with the plugin-auth team.
3. **Docs:** publish the platform grant bundle (the §1 table, post-unification: `midaz` +
   `plugin-fees` + `tracer` + `reporter`) so integrators see the full four-namespace set up front.

**This document decides nothing unilaterally.** It records the current state, the consequence, the
trust model, and a recommended option set. Execution — including whether to accept A1 at all — is
**deferred to the X1 gate**, owner-decided with the plugin-auth team, so that the only namespace
break integrators ever absorb is the single coordinated X1 migration.
