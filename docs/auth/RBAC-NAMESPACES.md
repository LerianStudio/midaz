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

## The three namespaces

The merged binary calls `auth.Authorize(<namespace>, <resource>, <action>)` under three distinct
namespace literals:

| Namespace | Owner / code | Resources | Source |
|-----------|--------------|-----------|--------|
| `midaz` | ledger — `midazName` const; CRM (collapsed package) — `ApplicationName` const | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings`, `holders`, `instruments` | `components/ledger/internal/adapters/http/in/routes.go` (`midazName = "midaz"`, helper `protectedMidaz`); `components/crm/adapters/http/in/routes.go` (`const ApplicationName = "midaz"`) for the `holders`/`instruments` resources |
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
