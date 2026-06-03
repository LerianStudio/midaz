# P4 Fees Collapse — 3-Way Environment Diff (P4-T17 / P4-T20)

Scope: fold fee-specific config into the unified ledger `Config` (`components/ledger/internal/bootstrap/config.go`), namespaced to avoid collision with ledger's existing surface. This is the env reconciliation artifact required by P4-T17/T20.

Three columns:
- **Fees standalone** — `plugin-fees/.env.example` + `plugin-fees/internal/bootstrap/config.go` env tags.
- **Ledger (pre-merge)** — `components/ledger/.env.example` + `config.go` env tags before P4.
- **Merged (post-P4)** — what the unified ledger binary reads after this chunk.

The fee config that survives the standalone-bootstrap deletion (P4-T19) is narrow: the fee Mongo connection (PD-7: fee collections live in ledger's Mongo, separate logical DB) and `DEFAULT_CURRENCY`. Everything else in the standalone fees env is provided by `UnifiedServer` / ledger's existing surface, or is dead after the in-process re-point (P4-T06/T07/T08).

## A. Fee config carried into the merged binary

| Standalone fees | Ledger pre-merge | Merged (post-P4) | Notes |
|---|---|---|---|
| `MONGO_URI` | — | `MONGO_FEES_URI` | Renamed/namespaced. Bare `MONGO_*` would collide with nothing in ledger (ledger uses `MONGO_ONBOARDING_*`/`MONGO_TRANSACTION_*`), but a bare name is still ambiguous in a multi-domain binary, so namespaced. |
| `MONGO_HOST` | — | `MONGO_FEES_HOST` | namespaced |
| `MONGO_NAME` | — | `MONGO_FEES_NAME` | namespaced. Default `fees` (separate logical DB in the shared Mongo, PD-7). |
| `MONGO_USER` | — | `MONGO_FEES_USER` | namespaced |
| `MONGO_PASSWORD` | — | `MONGO_FEES_PASSWORD` | namespaced |
| `MONGO_PORT` | — | `MONGO_FEES_PORT` | namespaced |
| `MONGO_PARAMETERS` | — | `MONGO_FEES_PARAMETERS` | namespaced |
| `MONGO_MAX_POOL_SIZE` | — | `MONGO_FEES_MAX_POOL_SIZE` | namespaced. Default 100 in `buildFeesMongoConnection`. |
| `MONGO_TLS_CA_CERT` (config.go tag) | — | `MONGO_FEES_TLS_CA_CERT` | namespaced |
| `MONGO_MIN_POOL_SIZE` | — | **DROPPED** | lib-commons mongo client (used by the `MongoConnection` wrapper) does not surface a min-pool knob; never wired. |
| `MONGO_MAX_CONN_IDLE_TIME_SECONDS` | — | **DROPPED** | not consumed by the moved `MongoConnection` wrapper. |
| `DEFAULT_CURRENCY` | — | `DEFAULT_CURRENCY` | **kept bare** (carried verbatim). Standalone shipped `BRL`; merged default is `USD` via `applyConfigDefaults` (standalone had NO hard default — it was required env). Document the value change for ops. |

## B. Standalone fee env that is GONE after the collapse (not merged)

| Standalone fees var | Why it disappears |
|---|---|
| `CLIENT_ID`, `CLIENT_SECRET` | outbound M2M auth to ledger — deleted with the HTTP client (P4-T08). |
| `MIDAZ_ONBOARDING_URL`, `MIDAZ_TRANSACTION_URL` | outbound ledger URLs — reads are now in-process via `query.UseCase` (P4-T06/T07). |
| `M2M_TARGET_SERVICE`, `M2M_CREDENTIAL_CACHE_TTL_SEC`, `AWS_REGION` | per-tenant M2M creds from AWS Secrets Manager — deleted with `internal/m2m` (P4-T08). |
| `PACKAGE_CACHE_*`, `BILLING_PACKAGE_CACHE_*`, `ACCOUNT_CACHE_*` | caches did NOT survive P4-T03 (default OFF, removed). No cache config in the merged binary. |
| `SERVER_PORT` (=4002), `SERVER_ADDRESS` | `UnifiedServer` owns the port (`:3002`). Fee routes mount on the ledger app (P4-T10). |
| `SWAGGER_*` | `UnifiedServer` owns `/swagger`; fee annotations merge into ledger's spec (P4-T17). |
| `CORS_ALLOW_ORIGINS`, `RATE_LIMIT_*`, `TRUSTED_PROXIES` | `UnifiedServer` provides CORS/limiter/proxy handling (P4-T10). |
| `READYZ_*`, `DEPLOYMENT_MODE` | ledger's readyz/drain machinery owns these. |
| `LICENSE_KEY`, `ORGANIZATION_IDS` | license middleware deleted in P4-T19; `lib-license-go` direct require dropped (P4-T04). |
| `OTEL_*`, `LOG_LEVEL`, `ENV_NAME`, `VERSION`, `ENABLE_TELEMETRY` | ledger already provides these (see collision table below). |
| `PLUGIN_AUTH_ADDRESS`, `PLUGIN_AUTH_ENABLED` | ledger's `AUTH_*` surface + shared auth client (the fee routes reuse it; auth NAMESPACE `plugin-fees` is preserved per-route, R9/P4-T22). |
| `MULTI_TENANT_*` | ledger already owns the full `MULTI_TENANT_*` surface; the fee tenant Mongo manager reuses ledger's tenant client. |
| `APPLICATION_NAME` (=`plugin-fees`) | **NOT carried as an env var.** See collision/footgun note below — it becomes the `constant.ModuleFees` provisioning + auth namespace, NOT the app name. App name stays `ledger`. |

## C. Collision analysis (vars present in BOTH standalone fees AND ledger)

A silent missing var passes CI build but fails at runtime (R17). These are the shared names where merge intent matters:

| Var | Standalone fees | Ledger | Resolution |
|---|---|---|---|
| `SERVER_PORT` | 4002 | 3002 | Ledger wins (`:3002`). Fee standalone port retired with the binary. **No conflict** — single value survives. |
| `MULTI_TENANT_ENABLED` / `MULTI_TENANT_URL` / `MULTI_TENANT_SERVICE_API_KEY` / `MULTI_TENANT_REDIS_*` / `MULTI_TENANT_CACHE_TTL_SEC` / `MULTI_TENANT_CONNECTIONS_CHECK_INTERVAL_SEC` | present | present | **Same semantics, ledger owns.** Fee tenant Mongo manager reuses ledger's tenant client + cache/loader. No second set of MT vars. |
| `LOG_LEVEL` | debug | debug | identical — ledger wins. |
| `ENV_NAME` | local | local | identical — ledger wins. |
| `VERSION` | v3.0.8 | v3.8.0 | Ledger wins (single binary version). Fee `VERSION` retired. |
| `OTEL_RESOURCE_SERVICE_NAME` | `plugin-fees` | `ledger` | **Ledger wins (`ledger`).** Fee telemetry now reports under the ledger service name. This is a deliberate observability namespace change — call out to dashboards/alerts keyed on `plugin-fees` OTEL service name (cross-ref P4-T21 APIDog/observability lockstep). |
| `APPLICATION_NAME` | `plugin-fees` | (absent; ledger uses OTEL service name) | **Footgun — see below.** Does NOT become a merged env var. |

### Footgun: `APPLICATION_NAME` / `plugin-fees` (mirrors the CRM `crm→crm-api` precedent)

In the standalone fees service, `constant.ApplicationName = "plugin-fees"` is overloaded onto THREE roles:
1. The auth/RBAC namespace in `auth.Authorize(applicationName, <resource>, <verb>)` (R9).
2. The tenant-manager **service name** passed positionally to `tmmongo.NewManager(tmClient, constant.ApplicationName, ...)` with **NO `WithModule`** — the source comment literally says *"unnamed module = single-module mode"*.
3. The OTEL service name + event-listener service.

Post-merge:
- The auth namespace `plugin-fees` is **preserved verbatim** on the fee routes (R9, P4-T22) — it is NOT the merged app name.
- `constant.ModuleFees = "plugin-fees"` is introduced for tenant-manager DB resolution via `tmmongo.WithModule(ModuleFees)`. **This value MUST match what tenant-manager provisioning expects** for tenants already served fee DBs. Because the standalone service ran *single-module* (service name = `plugin-fees`, no module), the provisioning shape that exists in prod is keyed on the SERVICE name `plugin-fees`, not on a sub-module. Cross-team confirmation of the exact provisioning key is a P4-T22 dependency (the same class of risk that produced the CRM `crm` vs `crm-api` mismatch). **Flag: if provisioning is keyed differently than `plugin-fees`, `WithModule` must be changed to match — do not assume.**
- The OTEL service name collapses to `ledger`. Any `plugin-fees`-keyed dashboard/alert must be repointed (P4-T21).

## D. Pre-existing gap surfaced (not introduced by P4)

The P3 CRM collapse added the `CrmPrefixed*` Mongo + bare `LCRYPTO_*` fields to the ledger `Config` struct but did **not** add a `MONGO_CRM_*` block to `components/ledger/.env.example`. The fee collapse does add the `MONGO_FEES_*` + `DEFAULT_CURRENCY` block to `.env.example` (this chunk). The CRM `.env.example` omission is pre-existing and out of scope here — surfaced so it is not mistaken for a P4 regression.

## E. Net merged-binary additions (this chunk)

`config.go` Config struct gains: `FeesPrefixedMongoURI`, `FeesPrefixedMongoDBHost`, `FeesPrefixedMongoDBName`, `FeesPrefixedMongoDBUser`, `FeesPrefixedMongoDBPassword`, `FeesPrefixedMongoDBPort`, `FeesPrefixedMongoDBParameters`, `FeesPrefixedMaxPoolSize`, `FeesPrefixedMongoTLSCACert`, `FeesDefaultCurrency`.

`applyConfigDefaults` gains: `FeesDefaultCurrency` → `USD` when unset.

`.env.example` gains: the `MONGO_FEES_*` block + `DEFAULT_CURRENCY=USD`.
