# Ledger / Tracer — Deployment Topology

> **Status: canonical topology reference.** This document is the single source of truth for
> how the ledger and tracer products relate at deploy time, where the seams are, and
> what the runtime contracts demand of an operator.
>
> **Helm charts and Kubernetes manifests are intentionally NOT in this repository** — they are owned
> by the infra team in a downstream repo. This repo has no `helm/`, `k8s/`, or `deploy/` directory
> (confirmed: `components/` holds only `infra`, `ledger`, and `tracer`). Everything below is either (a) a **fact** grounded in code, config,
> Dockerfiles, or compose files in *this* repo — cited inline — or (b) a **RECOMMENDATION** for the
> infra team, explicitly labeled, grounded in the real runtime constraints rather than in any
> existing manifest. Where a constraint is inferred rather than directly proven, that is called out.
>
> Related design record: the seam plan that motivates the gRPC reservation channel and the
> CRM consolidation into ledger lived at `docs/plans/2026-06-11-ledger-tracer-seam-and-crm-consolidation.md`
> and has since been removed — it survives in git history.
>
> **Citation convention.** Unprefixed file citations (`config.go`, `tls_seam.go`, `routes.go`) refer to
> the component under discussion in that section. Where a filename exists in more than one component, a
> `ledger/` or `tracer/` prefix disambiguates. The ledger reservation seam itself lives in the
> transaction create use cases (`components/ledger/internal/services/command/`:
> `create_transaction_v2.go`, `revert_transaction.go`, `commit_transaction.go`,
> `transaction_control_ports.go`, `transaction_reservation_anchor.go`); the tracer
> client it depends on is injected at bootstrap through the narrow `command.TracerReserver` port, so
> the use case never learns the transport. Line ranges are accurate at time of writing but rot; the
> cited function/const symbols are the durable anchors.

---

## 1. Product segregation matrix

The two components are **separately-sellable products**, each shipping as its own OCI image under
the **Elastic License 2.0** (`LICENSE:1`). This product boundary — not a technical limitation — is
*why* the tracer is not embedded into the ledger binary: a customer can license and run the
ledger alone, or add the tracer as a distinct product.

| Tier | Image / build context | Binary source | Base image | Port(s) | License | Notes |
|---|---|---|---|---|---|---|
| **Ledger** (unified) | `components/ledger/Dockerfile` | `components/ledger/cmd/app/main.go` (`Dockerfile:19`) | distroless `static-debian12` (default tag), run as nonroot via `USER nonroot:nonroot` (`Dockerfile:28`), static `CGO_ENABLED=0 -tags netgo` (`Dockerfile:13-31`) | `:3002` (`EXPOSE 3002`, `Dockerfile:26`) | Elastic-2.0 | One binary serving onboarding + transaction + CRM (holders/instruments) + fees; routes register under the `midaz` authz namespace via `protectedMidaz(...)` (`routes.go`). No embedded `HEALTHCHECK` — relies on orchestrator probes. |
| **Tracer** | `components/tracer/Dockerfile` | `components/tracer/cmd/app/main.go` | distroless `static-debian12:nonroot`, `GOMEMLIMIT=1800MiB` baked (`Dockerfile`) | `:4020` HTTP API (`EXPOSE 4020`); `:4021` gRPC reservation seam, always on (see §6) | Elastic-2.0 | A separate `Dockerfile.dev` uses `alpine:3.23` with a `wget` `HEALTHCHECK` against `/readyz` on `SERVER_PORT` (`Dockerfile.dev`). |

---

## 2. Co-scheduling (separate Deployments, NOT a sidecar)

**Deployment fact:** ledger and tracer run as **separate processes on distinct ports**, each
with its own component `docker-compose.yml` declaring only its own app service and joining the shared
external `infra-network` (`ledger/docker-compose.yml:1-21`, `tracer/docker-compose.yml:1-37`;
both composes declare `infra-network` with `external: true`).
This separate-process boundary is the deployment expression of the "separately-sellable products"
framing in §1.

> **RECOMMENDATION (infra-owned, NOT an existing manifest):** deploy ledger and tracer as **separate
> Kubernetes Deployments** and prefer co-scheduling them onto shared nodes via
> `podAffinity: preferredDuringSchedulingIgnoredDuringExecution` (soft affinity), rather than packing
> the tracer into the ledger pod as a sidecar / same-pod container. The repo contains no k8s manifests,
> no `podAffinity`, and no sidecar config — this is guidance, not deployed fact.

**Why co-schedule (soft affinity):** the reservation reserve RPC is **synchronous and on the hot path**
— called by `createTransactionWithEngine` immediately before `ExecutePreparedEngine` on the
default transaction path (`services/command/create_transaction_engine.go`; reservation mechanics at
`transaction_reservation_anchor.go`).
Co-locating ledger and tracer on the same node trims that round-trip's network latency without
collapsing the two into one failure/scale unit.

**Why REJECT sidecar / same-pod:**

1. **Heavier scale unit + connection fan-out.** In multi-tenant mode the ledger holds independent
   per-tenant PostgreSQL managers (onboarding + transaction), opening and closing a connection pool
   *per tenant* (`config.go:103-113`, `config.go:648-660`; per-manager sizing via
   `DB_ONBOARDING_MAX_OPEN_CONNS` / `DB_TRANSACTION_MAX_OPEN_CONNS` at `config.go:130-131,148-149`).
   Total ledger→Postgres demand scales as roughly `ledger_replicas × tenants × per-pool-max`
   (the fan-out formula and the ~80% `pg.max_connections` headroom rule are written out in
   `tracer/.env.example:244-281`). Bundling the tracer into the ledger pod means every ledger replica
   you add to satisfy connection or throughput demand drags a tracer replica along — multiplying the
   *heavier* unit and its connection demand for no reason.

2. **Coupled startup / readiness.** A same-pod tracer would fold the tracer's warmup into ledger pod
   startup and couple ledger readiness to tracer cache warmup. *(This warmup-coupling point is inferred
   from the tracer's "readiness reports cache health" design referenced in the tracer component
   `CLAUDE.md`; it is reasoning, not a measured fact or a manifest assertion.)*

3. **Independent lifecycle.** Separate Deployments let tracer roll, scale, and fail independently of
   ledger — which the seam is explicitly built to tolerate (see §3 and §4).

---

## 3. Independent scaling

> **RECOMMENDATION (infra-owned):** give the tracer its **own** autoscaler
> (HPA / KEDA) and let it scale independently of ledger. No HPA objects exist in this repo; concrete
> replica counts, CPU thresholds, and target numbers are infra-owned and are **not** invented here.

**What is grounded — the hot-path vs. off-path split that makes independent scaling safe:**

- **Reserve is synchronous, pre-commit, hot-path.** `reserveTransaction` is called inline right before
  `ExecutePreparedEngine`; a reject returns *before* any balance moves
  (`create_transaction_engine.go`).
  The tracer must therefore be **low-latency**, but each reservation's work is bounded per transaction.

- **Confirm / Release are post-decision and non-blocking.** After a confirmed engine
  result, `confirmReservations` runs for non-PENDING transactions. Only a known
  precommit rejection, or a request that never reached the engine, releases the
  reservation; an indeterminate engine outcome does not assume that no balance moved
  (`services/command/create_transaction_engine.go`). PENDING defers confirm to
  `/commit` and release to `/cancel`, which the versioned transition use case answers
  (`services/command/commit_transaction.go`, `transitionPendingV2`, which enables
  `transitionPendingWithEngine` to call `confirmReservationsByTransaction` /
  `releaseReservationsByTransaction`).
  Transport failures on confirm/release are logged at Warn, span-recorded and **never propagated**,
  so a tracer outage during this window never fails a transaction whose balances already moved.
  They are **not dropped**, though: the failed transition is handed to
  `transaction_reservation_retry.go`, which redelivers it off the request path with exponential
  backoff and jitter — about six minutes across twenty attempts, capped at 256 concurrent sequences
  process-wide. The retry is safe because the tracer, not the ledger, guarantees the repeat is free:
  a confirm settles a reservation only while it is still settleable and the row flip is guarded on
  the status read under the lock, so re-delivering a confirm whose response was lost moves no counter
  twice.

  The budget deliberately outlasts the tracer's own five-minute hold, because a confirm that arrives
  after the hold expired is still worth delivering — the tracer counts the spend without disturbing
  the capacity its expiry sweep already returned.

  Two failures remain and both are reported at Error naming the transaction, the reservation and the
  amount: a sequence that exhausts its budget, and a transition turned away because the concurrency
  cap is full. The expiry sweep is not the durability story for a confirm; it is the backstop
  for the CAPACITY only, and it returns capacity without ever counting the spend.

  Residual, and deliberately not solved here: the retry is in-process, so a ledger restart with
  sequences in flight loses them. Closing that needs durable state for the pending transition (the
  `outbox` primitives in lib-commons, or reservation state on the transaction row) plus a sweeper —
  a persistence decision, not a defect fix.

  A confirm also reports what it found: `ConfirmByTransaction` returns `confirmed` (rows it moved to
  CONFIRMED) and `already_released` (rows of the transaction that were already RELEASED), and
  `ConfirmById` returns `already_released`. A released row's spend is never counted, so the ledger
  records a non-zero `already_released` as a divergence without failing the transaction: a Warn log, a
  span event `tracer.reservation.confirm_already_released`, and the counter
  `tracer_reservation_confirm_already_released_total{operation}` (`operation` ∈ `confirm`,
  `confirm_by_transaction`; `transaction_reservation_telemetry.go`).

Net: the tracer can stay a small replica set and tolerate occasional saturation on the post-commit path,
while the pre-commit reserve path is the only latency-sensitive RPC — which is what co-scheduling (§2)
addresses.

**Back-pressure constraint (grounded, prescriptive support):** when the per-tenant pool cap is reached
the tracer supervisor stops spawning workers. The HTTP API returns **503 with `Retry-After`**
(`TENANT_CAP_RETRY_AFTER_SECONDS`, default 5s) plus a canonical error envelope; the reservation seam
returns gRPC `Unavailable` with code `0445`, which the ledger routes through `failPosture`
(`tracer/.env.example`). This is the documented graceful back-pressure behavior any scaling
policy must respect; it is not a manifest.

> **RECOMMENDATION caveat:** resource limits in the component composes (e.g. tracer `cpus:1`/`512M`;
> ledger sets none) are **dev docker-compose
> values** (`tracer/docker-compose.yml:19-35`,
> `ledger/docker-compose.yml:1-15`), **not** production requests/limits. Do not treat them as
> production sizing. The tracer compose does set a `/readyz` healthcheck with `stop_grace_period:25s`,
> which exceeds `READYZ_DRAIN_GRACE_SECONDS` (12s default) for a clean drain.

---

## 4. Graceful absence (ledger-only)

A ledger product can ship and run **without** any tracer. There are two layers of graceful absence.

**Layer 1 — integration not wired at all (`TRACER_BASE_URL` unset).** The whole tracer integration is
opt-in via `TRACER_BASE_URL`. When empty (the documented default), `buildTracerReserver` returns a
genuine `nil` `TracerReserver` interface and logs *"Tracer reservation integration disabled
(TRACER_BASE_URL unset)"* (`config.go:1548-1554`; struct doc at `config.go:285-290`). The
transaction-create path is then byte-for-byte unchanged.

A `nil` reserver is treated as "tracer disabled" at every call site via explicit nil guards, mirroring
the streaming nil-emitter pattern: `reserveTransaction` returns *proceed* with an empty handle, and
confirm/release are no-ops (`transaction_control_ports.go`,
`transaction_reservation_anchor.go:98-101, 252-254, 266-268`). So the create path runs identically with
or without a tracer wired.

**Layer 2 — tracer wired but unreachable (`failPosture` governs).** When a tracer *is* wired but
unreachable, behavior is governed by **per-ledger tracer settings** (`mode` + `failPosture`), not a
global flag (`transaction_reservation_anchor.go:149-189`):

- `mode = off` / `advisory` → never blocks; proceed.
- `mode = enforce` + `failPosture = open` (**default**) → record span attribute
  `app.tracer.reservation_skipped=true` and proceed. A degraded tracer cannot block *all* transactions
  (the R20 rationale, `transaction_reservation_anchor.go:178-182`).
- `mode = enforce` + `failPosture = closed` → reject the transaction with
  `ErrTransactionReservationUnavailable` **before any balance move**, and do **not** set the skipped
  marker.

This open-vs-closed contract is locked by proof tests: `TestTracerFailOpenSkipped` asserts
commit + `reservation_skipped=true` (`transaction_reservation_failposture_test.go:92-108`);
`TestTracerFailClosedDoesNotMarkSkipped` asserts reject with `ErrTransactionReservationUnavailable` and
no marker (`:113-132`).

**How "unreachable" is detected.** Transport/availability failures normalize to `ErrTracerUnavailable`
at the transport boundary so `failPosture` can branch on them: gRPC `Unavailable` /
`DeadlineExceeded` / `Canceled` and context deadline/cancellation are folded into `ErrTracerUnavailable`
(`mapGRPCError` in `grpc_client.go`). A business **DENY** or **REVIEW** decision is a *successful
result*, not an error. A request the tracer **refused** — gRPC `InvalidArgument` / `FailedPrecondition`,
including a reserve replayed onto a settled reservation (`0533`) — is classified as `ErrTracerRejected`,
not as unavailability: the tracer answered, so under `enforce` it rejects with `0532` whatever the
`failPosture`. A rejected credential — gRPC `Unauthenticated` / `PermissionDenied` — is
`ErrTracerUnauthorized`: under `enforce` it follows `failPosture` but rejects with `0536` instead of
`0178` when closed, and it is logged at Error and counted in every mode (§5). `handleReserveError` treats **any other** reserve error as fail-posture-gated, so a
tracer defect cannot let an `enforce`+`closed` ledger commit unchecked. A deadline or cancellation on a
sent call is additionally marked `ErrTracerNoAnswer`; only such an unanswered reserve may still have
reserved capacity, so only it is settled by transaction once the accounting outcome is known (confirm
when the movement applied outside PENDING, release on `0178` or an engine abort), best-effort. The
settle runs on a dedicated queue (64 in flight, separate from the by-id retrier so an outage cannot
crowd out real confirms), each attempt after a fixed `3s` tracer lock wait plus `TRACER_TIMEOUT_MS`;
a confirm that fails or settles nothing gets one more attempt, then a Warn with the transaction id. An atomic batch or cross-ledger v2 commit
whose execution hand-off fails releases the reservations held for its items, because no movement follows.
A rule the tracer cannot evaluate does not stop evaluation: a matched `DENY` wins, otherwise the answer is
`REVIEW` with `reason=rule_evaluation_error`, on reserve and on `POST /v1/validations` alike.
Under `enforce` a limit `DENY` rejects with `0177`, a rule `DENY` with `0535` and a `REVIEW` with `0531`.
The full outcome table lives in `docs/api/SCOPING.md`.

**Boot-time graceful absence even when configured.** The gRPC client uses one persistent lazy
connection — `grpc.NewClient` does not dial until the first RPC — so wiring the client never blocks on
tracer reachability at boot (`grpc_client.go:79-123`). An empty target fails *construction* at boot
rather than at first transaction. `Close()` drains on SIGTERM when registered with the composition root
(`grpc_client.go:125-129`).

---

## 5. Seam identity and transport security

The ledger identifies itself on the seam with **at most one credential**, and each side selects it
from configuration. The Access Manager application token is the default identity of a deployment
that runs plugin auth; the tracer API key covers deployments without the Access Manager; mTLS, a
service mesh and a `NetworkPolicy` stay available as transport layers, and are the identity only
when neither credential is configured. A BYOC cluster therefore needs neither a mesh nor client
certificates.

### Identity ladder (tracer, server side)

The tracer enforces the first identity its configuration enables (`resolveSeamIdentity`,
`tracer/seam_posture.go`) and installs the matching unary interceptors after the otelgrpc stats
handler, in this order: identity interceptor(s), then the tenant interceptor (`seamUnaryInterceptors`,
`tracer/grpc_server.go`).

| Priority | Identity | Enabled by | What the ledger sends | Interceptors |
|---|---|---|---|---|
| 1 | **token** | `PLUGIN_AUTH_ENABLED=true` | `authorization: Bearer <application token>` | lib-auth `NewGRPCAuthUnaryPolicy` → `SeamPrincipalInterceptor` → tenant from the token claim |
| 2 | **API key** | `API_KEY_ENABLED=true` | `x-api-key: <API_KEY>` | `SeamAPIKeyInterceptor` → tenant from `x-tenant-id` |
| 3 | **transport** | `TRACER_TLS_MODE=mtls` or `mesh` | a verified client certificate, or a mesh-verified peer | tenant from `x-tenant-id` |
| 4 | **none** | nothing above | nothing | tenant from `x-tenant-id` |

**Token identity.** lib-auth authorizes every `ReservationService` RPC against the Access Manager
as product `tracer`, resource `reservations`, action `post` (`SeamAuthPolicyConfig`,
`grpc/in/seam_auth_interceptor.go`). The policy has no default, so an RPC added without a mapping is
refused. `components/tracer/permissions.yaml` declares the `reservations/post` grant on the
`editor`, `validator` and `audit-viewer` roles, mirroring the central Access Manager seed. A human
who holds that grant still cannot use the seam: `SeamPrincipalInterceptor` reads the authorized
token's claims and refuses every token whose `type` is not `application`. Then:

- single-tenant: the token `azp` (the application client id) or `sub`
  (`<owner>/<application id>`) must be listed in `TRACER_SEAM_ALLOWED_CLIENTS`;
- multi-tenant: the token must carry a `tenantId` claim and its `name` claim must be that same
  tenant's ledger→tracer client (`ledger-m2m-tracer-{tenant}`, the tenant compared canonically with
  the claim's), so another application of the same tenant
  cannot drive the seam; the allowlist is ignored with a boot Warn, because every tenant has its own
  ledger client;
- whenever the token carries `tenantId`, an `x-tenant-id` (or `md-tenant-id`) value naming another
  tenant is refused. Tenant ids compare by value, not spelling: two UUIDs compare as UUIDs, anything
  else trimmed, lower-cased and without dashes, because the tenant-manager writes the claim as a
  dashless UUID.

Application tokens authorize under their own `sub` only with `AUTH_M2M_INVERSION_ENABLED=true` (the
raw value `true`), which is recommended; without it the tracer boots with a Warn and lib-auth
authorizes every application token under the shared `admin/tracer-editor-role` subject, so the
Access Manager checks only that the token is valid, not the ledger client's own grant, and the
principal guard above alone restricts who may reserve. In multi-tenant mode lib-auth copies the `tenantId` claim into the
incoming `md-tenant-id` metadata only when it sees `MULTI_TENANT_ENABLED` as the raw value `true`,
and the tenant interceptor resolves the tenant from that value (`TokenTenantUnaryInterceptor`),
never from `x-tenant-id`. The tracer reads the claims unverified after the Access Manager
authorized the token; `AUTH_JWT_VERIFY_CERT` adds local signature verification.

**API key identity.** `SeamAPIKeyInterceptor` compares `x-api-key` with `API_KEY` using the same
constant-time check the HTTP listener applies to `X-API-Key` (`adapters/apikey`). An admitted call is
attributed to `API_KEY_LABEL` in the audit trail, as on HTTP.

**Status codes of a refused credential.**

| Condition | gRPC code |
|---|---|
| missing or unreadable token, token the Access Manager rejects as invalid, missing or wrong API key | `Unauthenticated` |
| Access Manager denies `reservations/post`; principal guard refuses the token (not an application token, client outside the allowlist, no `tenantId` claim or a client other than the tenant's ledger→tracer client in multi-tenant mode, tenant mismatch) | `PermissionDenied` |
| Access Manager unreachable | `Unavailable` |

The identity interceptors run before any handler, so a refused call holds nothing.

### Boot gate (tracer)

`ValidateSeamPosture` (`tracer/seam_posture.go`) runs before any listener binds. It reads
`DEPLOYMENT_MODE` raw: an unset value is not `local`.

| Identity | Condition | Result |
|---|---|---|
| any but token | `MULTI_TENANT_ENABLED=true` | refuse: the tenant must come from the caller's token |
| token | `AUTH_M2M_INVERSION_ENABLED` is not `true` | one Warn in every deployment mode, single- and multi-tenant: the principal guard alone binds the caller |
| token, multi-tenant | `MULTI_TENANT_ENABLED` is not the raw value `true` | refuse |
| token, single-tenant | `TRACER_SEAM_ALLOWED_CLIENTS` empty | refuse outside `DEPLOYMENT_MODE=local`; one Warn under `local` |
| token | `AUTH_CACHE_TTL` unset or not greater than zero | refuse under `DEPLOYMENT_MODE=saas` (every reservation would cost an Access Manager round trip); one Warn elsewhere |
| token or API key | `TRACER_TLS_MODE` empty | refuse under `DEPLOYMENT_MODE=saas` (a secret must not travel in clear); one Warn elsewhere |
| transport | `mtls` under `DEPLOYMENT_MODE=saas` without `TRACER_TLS_CLIENT_ALLOWED_NAMES` | refuse |
| transport | `mesh` | one Warn: the mesh must enforce STRICT mTLS and admit only the ledger |
| none | `DEPLOYMENT_MODE=saas` | refuse |
| none | BYOC, `local` or unset | boots with one Warn naming the exposure: any workload reaching `:4021` can release reservations |

The HTTP listener's gate follows the same rule (`ValidateAuthPresence`,
`tracer/auth_presence.go`): with neither `API_KEY_ENABLED` nor `PLUGIN_AUTH_ENABLED` it refuses to
boot under `DEPLOYMENT_MODE=saas` or `MULTI_TENANT_ENABLED=true` and boots with one Warn everywhere
else. A BYOC tracer without the Access Manager therefore boots with no auth configuration on either
listener, and the API key is the recommended identity there.

### Ledger side (client)

`buildTracerSeamIdentity` (`ledger/tracer_seam_identity.go`) selects the one credential every seam
call carries and logs the choice at boot (`identity`: `m2m-static`, `m2m-tenant`, `api-key` or
`none`, never secret material):

- **`PLUGIN_AUTH_ENABLED=true`** → an Access Manager application token minted through the ledger's
  Access Manager client (`PLUGIN_AUTH_HOST`) with a credential dedicated to the tracer. The
  `IDP_M2M_CLIENT_ID`/`IDP_M2M_CLIENT_SECRET` pair belongs to the manifest publisher and is not
  reused.
  - single-tenant: the static `TRACER_M2M_CLIENT_ID`/`TRACER_M2M_CLIENT_SECRET` pair; either one
    empty refuses boot while `TRACER_BASE_URL` is set.
  - multi-tenant: the calling tenant's own credential, read with lib-commons
    `secretsmanager.GetM2MCredentials` from the custody backend `M2M_SECRETS_BACKEND` selects (`aws`,
    the default: AWS Secrets Manager; `vault`: HashiCorp Vault KV v2 under `M2M_VAULT_MOUNT`, default
    `secret`, connected through `VAULT_ADDR`/`VAULT_TOKEN`/`VAULT_CACERT`/`VAULT_NAMESPACE`; any other
    value refuses boot, a selected backend never falls back to the other, and the path is the same on
    both) at
    `tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/tracer/credentials` (`ledger` is the
    ledger's tenant-manager service name, `APPLICATION_NAME`). Its token carries the tenant's
    `tenantId` claim. The static pair is never a fallback, because its token carries no tenant.
- **`PLUGIN_AUTH_ENABLED=false` and `TRACER_API_KEY` set** → `x-api-key` on every call (the tracer
  runs with `API_KEY_ENABLED=true` and the same key).
- **both** → boot refusal: a deployment has one seam identity.
- **neither** → no credential; identity is left to the transport.

`x-tenant-id` is still sent whenever the call's context carries a tenant. Under token identity the
tracer only cross-checks it against the claim.

**Token cache** (`M2MTokenSource`, `ledger/adapters/tracer/m2m_token_source.go`; credentials in
`m2m_credential_provider.go`): one entry per credential (per tenant in multi-tenant mode, one in
single-tenant mode). The single-tenant token is warmed at boot; a multi-tenant token is minted on the
tenant's first call on each pod. The refresh point is 80% of the token's lifetime, and the token is
served until `exp` − 30s, or until 90% of its lifetime when that margin would not leave it valid past
the refresh point (a token without a readable `exp` is given 5 minutes). Past the refresh point the
still-valid token keeps being served while one background mint replaces it; each failed background
mint is logged at Warn (with the tenant id) and the cached token stays served until it expires. Every
stored token also schedules its own refresh at its refresh point, so a tenant that stops calling
normally finds a valid token. A refresh that fails, or that is deferred to a mint in flight or to an
open failure window, is retried while the token is still valid: the backoff starts at 1 second,
doubles per consecutive failure, is capped at a quarter of the span between the refresh point and the
expiry, never fires inside the failure window, and never runs past the expiry, so an Access Manager outage longer than that span still lets
the token expire. A tenant idle for 30 minutes is no longer refreshed: its still-valid token stays
served until it expires, and the tenant is then forgotten, so its next call mints. Concurrent mints
for one tenant collapse into one, bounded at 5 seconds and detached from the callers. Callers wait for a mint only
when no valid token is cached, and only within their own deadline: a caller whose deadline or
cancellation strikes while it waits sent nothing and fails as plain `ErrTracerUnavailable` (the
ordinary Warn, not counted as a credential failure), while the mint completes for the next caller. A
failed mint opens a per-tenant failure window that every caller and every scheduled refresh honours:
it is answered from memory for 1 second after the first failure in a row, doubling per consecutive
failure up to 15 seconds, each window widened at random by up to 25% so tenants failing together
spread out; a successful mint resets it. An empty token, or one whose `exp` is not after its mint, is
a mint failure and is never sent; a lifetime read from `exp` is capped at 24 hours.

Tenant credentials read from the secret store are cached until the Access Manager refuses one (a
non-2xx answer) or mints an empty token with it; a transient mint failure (transport error, timeout)
keeps the credential. lib-auth reports a 5xx or 429 as a non-2xx refusal too, so a refusal drops the
credential at most once per tenant every 30 seconds; later refusals inside that interval keep it. A tenant with no secret is answered from memory for 5 seconds, so a secret the
tenant-manager writes moments later is picked up quickly; a malformed secret (incomplete, unreadable,
invalid path) for 30 seconds; a transient read failure is never cached.

When the tracer answers `Unauthenticated`, the client invalidates only the token that was rejected
(a token another call already refreshed stays cached), mints a new one and retries the call once. A
token younger than 5 seconds is not replaced: the call is not retried and the rejection is returned as
`ErrTracerUnauthorized`, so a tracer that rejects every token costs at most one mint per tenant every 5
seconds. `PermissionDenied` is never retried, and neither is an API key. A failed mint, an unreachable
Access Manager or a tenant with no credential fail the call as `ErrTracerUnavailable` with nothing
sent, so the call is never treated as unanswered; the ledger logs it once at Error and counts it on
`tracer_reservation_credential_rejected_total{operation,reason="not_sent"}`.

**Credential rejection on the ledger.** `mapGRPCError` maps `Unauthenticated` and
`PermissionDenied` to `ErrTracerUnauthorized`, which is neither `ErrTracerUnavailable`,
`ErrTracerRejected` nor `ErrTracerNoAnswer`. It is a configuration error that does not heal on its
own: a reserve under `advisory` proceeds; under `enforce` it follows `failPosture` (`closed` rejects
with `0536`, HTTP 503; `open` proceeds with `app.tracer.reservation_skipped=true`). Every branch
marks the span, logs once at Error with the transaction id (never the token) and increments
`tracer_reservation_credential_rejected_total{operation,reason="rejected"}`. A confirm or release that gets it is
logged at Error, counted and handed to the existing retry transport. An Access Manager outage on
the tracer side answers `Unavailable` and follows the ordinary `failPosture` (`0178`).

### Transport security

`TRACER_TLS_MODE` selects the transport posture, with a fail-fast typed error on an invalid value
(`ledger/tls_seam.go`, server-side mirror `tracer/tls_seam.go`):

- **`mtls`** — the app presents and verifies certificates directly.
- **`server`** — the tracer's gRPC listener presents its certificate and asks for no client
  certificate; the ledger verifies it against `TRACER_TLS_CA_FILE` and presents none. It encrypts
  the token or API key without client certificates or a mesh. The tracer's HTTP listener stays
  plaintext under `server`.
- **`mesh`** — the app dials/listens **plaintext** and delegates mTLS origination/termination to a
  local **Istio/Linkerd** service-mesh sidecar.
- **empty** — plaintext.

**`mtls` mode, ledger (client) side** (`ledger/tls_seam.go`, `buildClientMTLSConfig`): presents its
leaf via `GetClientCertificate`, verifies the tracer's server leaf against `RootCAs` loaded from
`TRACER_TLS_CA_FILE`, and pins `ServerName` to the host parsed from `TRACER_BASE_URL`
(`seamServerName`). `server` mode (`buildClientServerTLSConfig`) does the same without a client
certificate and refuses boot without `TRACER_TLS_CA_FILE`. The injected TLS dial credentials are
appended **last** and the insecure default is gated off so it cannot clobber them (`grpc_client.go`).

**`mtls` mode, tracer (server) side** (`tracer/tls_seam.go`, `buildMTLSConfig`): presents its own leaf via
`GetCertificate` and enforces `tls.RequireAndVerifyClientCert` against the client CA pool from
`TRACER_TLS_CLIENT_CA_FILE`.

**Client identity allowlist (gRPC listener, `mtls` only).** `TRACER_TLS_CLIENT_ALLOWED_NAMES`
(comma-separated) names the client identities the gRPC listener accepts. The handshake passes only
when one of the leaf's DNS SANs or URI SANs (e.g. `spiffe://...`) equals an entry, or, on a leaf
that carries no SAN, its Subject CN does, compared exactly after trimming and case-folding
(`buildGRPCSeamTLSConfig` → `verifyClientAllowedName` / `clientCertAllowed`). A refused handshake
reaches the ledger as gRPC `Unavailable`, so it follows `failPosture`. An empty allowlist accepts any
certificate the client CA signed: when the transport is the seam identity, `DEPLOYMENT_MODE=saas`
refuses to boot (`ValidateSeamPosture`); elsewhere the tracer logs one Warn at boot
(`warnGRPCSeamAcceptsAnyClient`). The list is ignored in `server`/`mesh`/empty mode and never
applied to the HTTP listener.

**CA env-var asymmetry (deliberate and correct):** each side names the CA var by what it verifies on the
*other* end. Ledger `TRACER_TLS_CA_FILE` holds the CA that verifies the **tracer's** server leaf.
Tracer `TRACER_TLS_CLIENT_CA_FILE` holds the CA that verifies the **ledger's** client leaf.

**Rotation / hot-reload:** both sides load their cert/key through the lib-commons
`certificate.Manager` (`libCert "github.com/LerianStudio/lib-commons/v7/commons/certificate"`), so the
seam rotates certificates without a restart: the ledger serves the latest cert via
`GetClientCertificate → certManager.TLSCertificate()`, the tracer via
`GetCertificate → certManager.GetCertificateFunc()`.

**Fail-fast on missing material:** in `mtls` mode each of `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE`,
and the respective CA file is required, and in `server` mode the tracer's cert/key and the ledger's
CA are; a missing one fails boot with an error naming the exact missing knob.

### Tenant on the seam

The tenant resolver is wired **only** onto the reservation RPCs (`seamtenant.Resolver`,
`grpc/in/tenant_interceptor.go`); user-facing tracer routes keep their JWT-claim tenant path.

- Under **token** identity the tenant is the token's `tenantId` claim (as `md-tenant-id`). A
  tenant's credential reaches only that tenant's reservations, and `x-tenant-id` is a cross-check.
- Under every other identity the tenant is the `x-tenant-id` metadata the ledger appends
  (`tenantMetadataKey` in the ledger `adapters/tracer` package; `seamtenant.MetadataKey` on the
  tracer). Those identities run single-tenant only, where the resolver is a no-op pass-through.

Under multi-tenant mode a missing, empty or invalid tenant is a **clean failure**
(`ErrReservationTenantRequired` → gRPC `InvalidArgument`) and never falls back to a default or wrong
pool. A tenant the tenant manager reports as not provisioned, suspended or purged answers gRPC
`Unavailable` with `ErrReservationTenantInactive` (`0534`), so the ledger routes it through
`failPosture` instead of reading it as a refusal.

### Operator checklist

**Token identity (recommended wherever the Access Manager runs):**

1. **Access Manager grant.** Every ledger client that calls the seam (the static single-tenant
   client and each tenant's client) needs `tracer/reservations:post`. The central seed carries it on
   the tracer `editor`, `validator` and `audit-viewer` roles (`tracer-reservation-permission`), and
   `components/tracer/permissions.yaml` mirrors it; the ledger manifest declares the edge with
   `m2m.needs: [tracer]`.
2. **Ledger credential.** Single-tenant: create a dedicated application client for the ledger and
   set `TRACER_M2M_CLIENT_ID`/`TRACER_M2M_CLIENT_SECRET`. Multi-tenant: the tenant-manager provisions
   each tenant's `ledger → tracer` credential at
   `tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/tracer/credentials` (client
   `ledger-m2m-tracer-{tenant}`); give the ledger read access to those secrets on the backend
   `M2M_SECRETS_BACKEND` selects (`aws`, the default: `AWS_REGION` and an IAM read grant; `vault`:
   `M2M_VAULT_MOUNT` plus `VAULT_ADDR`/`VAULT_TOKEN`/`VAULT_CACERT`/`VAULT_NAMESPACE` and a read
   policy on the mount). A tenant without the secret cannot reserve: its calls fail as unavailable and
   the fail posture decides.
3. **Tracer.** `PLUGIN_AUTH_ENABLED=true`, `PLUGIN_AUTH_ADDRESS`, and `AUTH_M2M_INVERSION_ENABLED=true`
   (recommended; without it the tracer boots with a Warn and the principal guard alone binds the
   caller). Single-tenant: `TRACER_SEAM_ALLOWED_CLIENTS` set to the ledger's client id. Multi-tenant:
   `MULTI_TENANT_ENABLED=true` exactly.
4. **lib-auth knobs on the tracer.** `AUTH_CACHE_TTL` greater than zero, e.g. `60s` (one Access
   Manager round trip per decision window instead of per reservation; required under
   `DEPLOYMENT_MODE=saas`, a boot Warn elsewhere), and recommended `AUTH_BREAKER_ENABLED=true` (stops
   hammering an unavailable Access Manager) and `AUTH_JWT_VERIFY_CERT` (local signature
   verification).
5. **Encryption.** `TRACER_TLS_MODE=server` on both sides (tracer cert/key, ledger CA), or `mtls` or
   `mesh`. `DEPLOYMENT_MODE=saas` refuses a credential in clear.

**Upgrade order (enabling token identity on a running pair).** Provision the ledger's credential
first — `TRACER_M2M_CLIENT_ID`/`TRACER_M2M_CLIENT_SECRET` in single-tenant mode, the tenant-manager's
ledger→tracer credential for every tenant in multi-tenant mode — and deploy the ledger with
`PLUGIN_AUTH_ENABLED=true`, so it sends tokens. Only then enable token identity on the tracer:
`PLUGIN_AUTH_ENABLED=true`, `AUTH_M2M_INVERSION_ENABLED=true` (recommended),
`TRACER_SEAM_ALLOWED_CLIENTS` in single-tenant mode, and `AUTH_CACHE_TTL`. A tracer that enables token identity before the ledger sends
tokens answers every reservation call `Unauthenticated`: under `enforce` + `closed` the ledger rejects
transactions with `0536`, and under `open` or `advisory` it proceeds without reservations, so limits
stop counting until the ledger catches up.

**BYOC options**, from strongest:

- Access Manager available: token identity as above.
- No Access Manager: API key — tracer `API_KEY_ENABLED=true` + `API_KEY`, ledger `TRACER_API_KEY`
  with the same value, plus `TRACER_TLS_MODE=server` to keep the key off the wire in clear.
- Certificates or a mesh already in place: `mtls` with `TRACER_TLS_CLIENT_ALLOWED_NAMES`, or `mesh`
  with STRICT `PeerAuthentication` plus an `AuthorizationPolicy` or `NetworkPolicy` admitting only
  the ledger to `:4021`.
- Nothing: the seam boots with a Warn and any workload that reaches `:4021` can reserve, confirm and
  release. Restrict `:4021` with a `NetworkPolicy` at least.

A `NetworkPolicy` that admits only the ledger to `:4021` is a useful additional layer under every
option.

---

## 6. Transport & ports

The reservation seam is a single `TracerReserver` port with one transport: the gRPC service
`lerian.midaz.reservation.v1.ReservationService` (`proto/reservation/v1`). The composition root builds
the gRPC client (`buildTracerReserver` → `buildTracerGRPCReserver`); the reserve anchor stays
transport-agnostic behind the port. The tracer's HTTP API serves no reservation route, so the gRPC seam
is the only surface that drives the reservation lifecycle, and the ledger is its only caller.

| Knob | Value | Behavior | Evidence |
|---|---|---|---|
| `TRACER_BASE_URL` | seam address | the authority is what gRPC dials; an `http://`/`https://` scheme is tolerated and stripped to `host:port` | `stripURLScheme`, `seamServerName` |
| Ledger `SERVER_ADDRESS` | `:3002` (default) | unified ledger binary, all APIs on one port | `ledger/.env.example` |
| Tracer `SERVER_ADDRESS` | `:4020` | HTTP API (rules, limits, validations, audit) + health | `tracer/.env.example` |
| `TRACER_GRPC_PORT` | `:4021` (**default**) | gRPC seam server, always started; empty resolves to the default | `DefaultTracerGRPCPort`, `ApplyGRPCSeamDefaults` |

**Wire contract evolves additively.** `proto/reservation/v1` grows only by new field numbers —
`ReserveAccount.type`, `ReserveRequest.metadata`, `ReserveRequest.revert` (field 13),
`ReserveResult.decision` / `reason` / `matched_rule_ids`, `ConfirmByTransactionResponse.confirmed` /
`already_released`, `ConfirmByIdResponse.already_released`. `denied` remains the field every ledger
gates on: a tracer that sends `decision` answers a `REVIEW` with `denied=true`, which a ledger that
predates it reads as a denial, and a ledger reading an older tracer sees an empty `decision` and gates
on `denied` alone.

**Deploy order is HARD: the tracer first, then the ledger, and no tracer rollback below this version
while a ledger that sends the new fields is running.** Additive fields do not make the pair
order-independent: an older tracer drops the unknown fields (`account.type`, `metadata`, `revert`)
silently. Nothing is refused, but account-type-scoped limits and rules never match, and a revert is not
recognised as one.

**Status codes the seam answers.** Beyond the reserve decision itself:

| Condition | gRPC code | Tracer code | Ledger under `enforce` |
|---|---|---|---|
| Malformed request, missing tenant under multi-tenant | `InvalidArgument` | request-specific, `ErrReservationTenantRequired` | `0532` |
| Reserve replayed onto a transaction whose reservation is already released, expired or confirmed; no counter moves | `FailedPrecondition` | `0533` | `0532` |
| Tenant not provisioned, suspended or purged | `Unavailable` | `0534` | `failPosture` |
| Tenant rule cache not loaded yet (cold start, or a tenant first seen after boot) | `Unavailable` | — | `failPosture` |
| Ledger's first call for a tenant on a pod (multi-tenant): its token is minted cold, and a mint that outlasts the call's `timeoutMs` leaves the call unsent | — (not sent) | — | `failPosture` |
| Tenant reached its per-tenant worker cap | `Unavailable` | `0445` | `failPosture` |
| Missing or invalid token, missing or wrong API key | `Unauthenticated` | — | `failPosture`, `0536` when closed |
| Access Manager denial, or the principal guard refuses the token | `PermissionDenied` | — | `failPosture`, `0536` when closed |
| Access Manager unreachable | `Unavailable` | — | `failPosture` |

Under `failPosture`, `open` proceeds with a SKIPPED audit and `closed` rejects with `0178` (`0536` for
a rejected credential). Expect the cold rule cache window right after a tracer rollout and the cold
first token mint per tenant after a ledger pod starts, and size `failPosture` accordingly.

**Tracer migration `000025` needs a maintenance window.** Widening the asset columns rewrites
`transaction_validations` and rebuilds all of its indexes under an `ACCESS EXCLUSIVE` lock, so size
the window by that table's row count: every read and write of validations blocks for the whole
rewrite. Each `ALTER` runs with a 5-second lock timeout and fails fast with SQLSTATE `55P03` instead
of queueing behind live traffic (where it would block every later query on the table). Both `ALTER`s
are guarded on the column's current type, so a re-run in a quieter window is safe.

**Tracer migration `000026` repairs schema-isolated tenants.** It converts `limits.max_amount`,
`usage_counters.current_usage` and `transaction_validations.amount` to `DECIMAL` (values kept as
currency units) wherever they are still `BIGINT` in the tenant's own schema; it is a no-op on a
single-tenant `public` schema. Where it converts, it rewrites those tables under the same 5-second
lock timeout as `000025`.

**Ports.** The ledger serves everything on a single port, default `:3002` (`SERVER_ADDRESS`). The tracer
serves its HTTP API and health on `:4020` (`SERVER_ADDRESS`) and the reservation **gRPC seam on
`:4021`** (`TRACER_GRPC_PORT`). The image's `EXPOSE` covers `:4020` only; the component compose
publishes `:4021` (`TRACER_GRPC_HOST_PORT`), and a cluster deploy must expose `:4021` as a service port
reachable from the ledger.

**Both tracer listeners share one base TLS posture.** `buildSeamTLSConfig` is the base config of both
the HTTP and the gRPC listener, so the two cannot drift: in `mtls` both require and verify a client
cert; in `mesh`/unset both listen plaintext. The gRPC listener alone layers its own posture on top
(`buildGRPCSeamTLSConfig`, §5): the client-identity allowlist under `mtls`, and the server-only
certificate under `server`, which leaves the HTTP listener plaintext.

The tracer's HTTP API on `:4020` is an **operations/configuration surface** (rules, limits, validations,
audit — operator-facing, internal), which is why a single `mtls` posture across the whole `:4020`
listener is acceptable: there is no direct end-customer access to demote.

---

## Appendix — Operator env surface

The seam's env knobs are defined as Go struct tags, validated in code and surfaced with their semantics
in `components/ledger/.env.example` and `components/tracer/.env.example`.

| Var | Side | Meaning | Evidence |
|---|---|---|---|
| `TRACER_BASE_URL` | ledger | opt-in switch for the whole integration and the seam address (`host:port`; an `http://`/`https://` scheme is stripped); empty → disabled | `buildTracerReserver`, `stripURLScheme` |
| `TRACER_TIMEOUT_MS` | ledger | client ceiling on every seam RPC; the per-ledger `tracer.timeoutMs` bounds the reserve beneath it | `buildTracerGRPCReserver` |
| `TRACER_TLS_MODE` | ledger | `mtls`\|`server`\|`mesh`/empty; `server` verifies the tracer's certificate and presents none | `buildSeamClientTLSConfig` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | ledger | client leaf material (mtls) | `buildClientMTLSConfig` |
| `TRACER_TLS_CA_FILE` | ledger | CA verifying the **tracer's** server leaf (mtls, server) | `buildClientMTLSConfig`, `buildClientServerTLSConfig` |
| `TRACER_M2M_CLIENT_ID` / `_SECRET` | ledger | single-tenant Access Manager credential dedicated to the seam; both required when `PLUGIN_AUTH_ENABLED=true` and `TRACER_BASE_URL` is set; ignored in multi-tenant mode, where each tenant's credential is read from `tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/tracer/credentials` | `buildTracerSeamIdentity`, `M2MTokenSource` |
| `M2M_SECRETS_BACKEND` | ledger | multi-tenant custody backend of the seam credentials: `aws` (default when empty; `AWS_REGION` and the default AWS credential chain) or `vault` (Vault KV v2 via `VAULT_ADDR`, `VAULT_TOKEN`, `VAULT_CACERT`, `VAULT_NAMESPACE`); any other value refuses boot; no fallback between backends; unused single-tenant | `buildM2MSecretsReader` |
| `M2M_VAULT_MOUNT` | ledger | Vault KV v2 mount holding the credentials when `M2M_SECRETS_BACKEND=vault`; empty → `secret` | `buildM2MSecretsReader` |
| `TRACER_API_KEY` | ledger | the tracer's `API_KEY`, sent as `x-api-key` when `PLUGIN_AUTH_ENABLED=false`; set together with plugin auth it refuses boot | `buildTracerSeamIdentity`, `WithAPIKey` |
| `TRACER_GRPC_PORT` | tracer | gRPC seam listen address; empty → `:4021`; always on | `DefaultTracerGRPCPort`, `ApplyGRPCSeamDefaults` |
| `TRACER_TLS_MODE` | tracer | `mtls`\|`server`\|`mesh`\|empty; `server` serves TLS on the gRPC listener only, with no client certificate; the boot gate is §5 | `buildSeamTLSConfig`, `buildGRPCSeamTLSConfig`, `ValidateSeamPosture` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | tracer | server leaf material (mtls, server) | `buildMTLSConfig`, `buildServerTLSConfig` |
| `TRACER_TLS_CLIENT_CA_FILE` | tracer | CA verifying the **ledger's** client leaf | `buildMTLSConfig` |
| `TRACER_TLS_CLIENT_ALLOWED_NAMES` | tracer | comma-separated client identities (DNS SAN / URI SAN, or CN on a cert without SANs) the gRPC listener accepts under mtls; empty → any CA-signed cert plus a boot Warn, and a refused boot under `DEPLOYMENT_MODE=saas` when the transport is the seam identity; never applied to the HTTP listener | `buildGRPCSeamTLSConfig`, `clientCertAllowed`, `ValidateSeamPosture` |
| `TRACER_SEAM_ALLOWED_CLIENTS` | tracer | comma-separated Access Manager application client ids (token `azp`; the token `sub`, `<owner>/<application id>`, is also accepted) admitted under token identity in single-tenant mode; required outside `DEPLOYMENT_MODE=local`; ignored with a Warn in multi-tenant mode | `SeamPrincipalInterceptor`, `ValidateSeamPosture` |
| `PLUGIN_AUTH_ENABLED` / `API_KEY_ENABLED` | tracer | select the seam identity (token first, then API key); `API_KEY`/`API_KEY_LABEL` are shared with the HTTP listener | `resolveSeamIdentity` |
| `AUTH_M2M_INVERSION_ENABLED` | tracer | recommended `true` under token identity; without it the tracer boots with a Warn and the Access Manager authorizes application tokens under a shared editor role, so only the principal guard restricts who may reserve | `ValidateSeamPosture` |
| `AUTH_CACHE_TTL` | tracer | lib-auth decision cache (e.g. `60s`); under token identity a value not greater than zero refuses boot under `DEPLOYMENT_MODE=saas` and logs a boot Warn elsewhere | `ValidateSeamPosture`, lib-auth |
| `AUTH_BREAKER_ENABLED` / `AUTH_JWT_VERIFY_CERT` | tracer | recommended under token identity: Access Manager circuit breaker, local signature verification | lib-auth |
| `TENANT_CAP_RETRY_AFTER_SECONDS` | tracer | HTTP 503 `Retry-After` on tenant-pool cap (default 5s) | `tracer/.env.example` |
