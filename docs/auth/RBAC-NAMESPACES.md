# Auth / RBAC Namespaces (R9)

After the monorepo consolidation, the holder/alias (CRM) and fee/billing routes are served by
the **unified ledger binary** on `:3002` alongside the onboarding/transaction/routing routes.
The HTTP routes were merged into one Fiber app, but the **authorization namespaces were NOT
merged**. Each domain still authorizes under its original namespace string.

> **Route merge ≠ authz merge.** The tenant-manager RBAC policies key on these literal namespace
> strings. Renaming a namespace (e.g. collapsing `plugin-crm` into `midaz`) would silently break
> authorization for every tenant whose policies reference the old string. Per the locked Phase 9
> decision, the four namespaces are **preserved as-is**. A coordinated policy migration is a
> separate funded effort and is **out of Phase 9 scope** (deferred — see "Deferred unification").

## The four namespaces

The merged binary calls `auth.Authorize(<namespace>, <resource>, <action>)` under four distinct
namespace literals:

| Namespace | Owner / code | Resources | Source |
|-----------|--------------|-----------|--------|
| `midaz` | ledger — `midazName` const | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings` | `components/ledger/internal/adapters/http/in/routes.go` (`midazName = "midaz"`, helper `protectedMidaz`) |
| `routing` | ledger — `routingName` const | `account-types`, `operation-routes`, `transaction-routes` | `components/ledger/internal/adapters/http/in/routes.go` (`routingName = "routing"`, helper `protectedRouting`) |
| `plugin-crm` | crm (collapsed package) | `holders`, `aliases` | `components/crm/adapters/http/in/routes.go` (`const ApplicationName = "plugin-crm"`) |
| `plugin-fees` | fees (embedded in ledger) | `packages`, `estimates`, `billing-packages`, `billing-calculate` | `components/ledger/internal/adapters/http/in/fees_routes.go` (`feesApplicationName = "plugin-fees"`); also `pkg/constant.ModuleFees = "plugin-fees"` and `components/ledger/pkg/feeshared/constant/app.go` |

The `(<action>)` dimension is the HTTP verb mapped to `get` / `post` / `patch` / `delete`.

## Tenant-manager policy-key coupling

The four namespace strings above are the **policy keys** that tenant-manager RBAC policies are
written against. Authorization for a request resolves as:

```
auth.Authorize(<namespace>, <resource>, <action>)  →  tenant-manager policy lookup keyed on <namespace>
```

Consequences:

- A tenant granted `plugin-crm:holders:post` can create holders. That grant does **not** carry
  over to `midaz:*` or `plugin-fees:*` — the namespaces are independent policy surfaces.
- Renaming any namespace literal in code without a coordinated tenant-manager policy migration
  would orphan every existing grant under the old key. This is why the rename is deferred, not
  done opportunistically during the route merge.
- The fee/billing namespace literal is intentionally identical (`plugin-fees`) across the
  ledger route registrar, `pkg/constant.ModuleFees`, and `components/ledger/pkg/feeshared` so the
  embedded fee code, its Mongo tenant client, and its authz all key on the same string.

## Deferred unification

Unifying these into a single namespace (or a coherent namespace scheme) is **out of Phase 9
scope** and tracked as risk **R9**. Any future harmonization MUST be a coordinated change that
migrates tenant-manager policies in lockstep with the code rename — it cannot be a unilateral
code edit. Until then, the four namespaces above are the authoritative authorization contract
for the unified binary.
