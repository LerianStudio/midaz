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
> Related design record: the seam plan that motivates the gRPC+mTLS reservation channel and the
> CRM consolidation into ledger lived at `docs/plans/2026-06-11-ledger-tracer-seam-and-crm-consolidation.md`
> and has since been removed — it survives in git history.
>
> **Citation convention.** Unprefixed file citations (`config.go`, `tls_seam.go`, `routes.go`) refer to
> the component under discussion in that section. Where a filename exists in more than one component, a
> `ledger/` or `tracer/` prefix disambiguates. The ledger reservation seam itself lives in the
> transaction create use cases (`components/ledger/internal/services/command/`:
> `create_transaction_v2.go`, `revert_transaction.go`, `commit_transaction.go`,
> `tracer_context_anchor.go`, `transaction_reservation_anchor.go`); the tracer
> client it depends on is injected at bootstrap through the narrow `command.ContextTracerClient` port
> (wrapped by `command.ContextTracerCoordinator`), so the use case never learns the transport. The
> cited function/const symbols are the durable anchors.

---

For migration, identity provisioning, completion, TTL expiry and deployment
reversal, follow the [reservation rollout procedure](ledger-tracer-rollout.md).

## Reservation settings

`tracer.validationMode` is independent of `tracer.mode`, `failPosture`, timeout
and authorized per-call skips. Missing/null settings retain the `limits` default;
`rules-and-limits` is explicit. Both create-ledger and settings PATCH validate the
same values, and PATCH checks activation against the atomically merged settings.

Combined controls can be configured while `mode=off`. Enabling advisory/enforce
requires the context integration's activation verifier; absent readiness returns
canonical `0534`/503 before settings are persisted. Switching a ledger back to
`mode=off` remains possible when that verifier is absent. The verifier belongs to the complete
integration composition; its presence is not inferred from `/version`. It must
perform a local readiness check, not a financial Reserve probe under the
settings database lock.

The HTTP/gRPC clients independently reject replies without the expected
contract revision, transaction identity and completed controls. Bootstrap
installs the coordinator and activation verifier together when `TRACER_BASE_URL`
is set; adding the settings field alone does not enable the integration. The
Tracer must map the Ledger's credential for the chosen transport (see §5), and
both sides need aligned resource bounds. Accounts and entries carry the Ledger's stored asset code;
Tracer limits require codes following the Ledger asset code rule (uppercase
letters, 1–100) and match them by exact code, so a non-conforming stored code
is never limited.

The coordinator is connected to engine-backed v2 creation (including the
shared revert path, PENDING creation/termination and atomic batches). It projects
fee-inclusive logical entries, preserves pending credit destinations, loads
official facts and calls Reserve immediately before the engine runs. It keeps no
state between admission and completion: nothing is persisted on the Ledger side
for the reservation.

In enforce, `DENY` retains code `0177`/422; its explanation covers rules and
usage limits. `REVIEW` returns `0535`/422 and creates neither accounting entries
nor a pending hold. Advisory observes both decisions. When admission is rejected
after Reserve was sent, the Ledger releases by transaction before answering.
Off/authorized skip precedes context loading.

Completion is inline and addressed by transaction ID. After the engine answers,
a direct APPROVED transaction confirms and a transaction the engine provably did
not apply releases; a PENDING transaction confirms on `/v2` commit and releases
on `/v2` cancel. A ledger in mode off sends no completion, so a PENDING
transaction committed or canceled after its ledger was switched off is left to
the TTL. An engine outcome that stays unknown makes no completion call; a
singular create and an atomic batch also log a Warn naming the transaction. An
admission that failed for availability skips the inline completion and hands it
straight to the retrier. An operation conflict (`0530`, for example a confirm after TTL expiry), a
refusal before evaluation (`0043`, `0487`, `0527`, recognized only by that canonical code) or a
response that contradicts the request is terminal and is not retried: one Error
log names the transaction and the consequence. A release answered with `0530` is
already settled and is logged at Info only.
Any other completion the Tracer does not acknowledge, such as one that fails for unavailability
(transport failure, timeout, a persistent HTTP 401, 429, 5xx, a 3xx or 4xx without a
recognized code, a gRPC status without a recognized message, `0161`, `0330`, `0422`), goes to the in-memory retrier
(`transaction_reservation_retry.go`): bounded attempts, backoff with jitter, a
wall-clock budget sized just above the direct reservation TTL, and a
process-wide cap on concurrent sequences. The retrier never blocks the request
and never fails it, because accounting has already run; a restart loses the
retries in flight, and graceful shutdown logs the ones it abandons.

The Tracer TTL is the backstop for every completion the Ledger does not deliver.
A direct reservation expires after 5 minutes; a PENDING one, sent with
`longLived`, after `RESERVATION_LONG_LIVED_TTL_HOURS` (default 720 hours). The
Tracer reaper then marks the operation and its reservations `EXPIRED`, returns
the held capacity and appends one `RESERVE_OPERATION_EXPIRED` audit event; a
later confirm or release answers `0530`. Expiry does not prove that accounting
failed, so under `enforce` a lost confirm frees limit capacity at expiry. Runtime
construction tests are not proof of a completed production rollout.

### Coordination diagnostics

The Ledger reservation path emits `tracer_coordination_total` and
`tracer_coordination_duration_ms` through the existing metrics factory. Labels
are restricted to `operation` (`admission`, `confirm`, `release`) and `result`
(`allow`, `deny`, `review`, `fail_open`, `unavailable`, `context_invalid`,
`delivered`, `failed`); unexpected labels become `unknown`. No account, asset,
transaction, policy, amount or error text is attached as an application metric
label.

Admission duration includes logical projection, official facts and Reserve. It
excludes accounting execution. DENY/REVIEW measure the Tracer decision even in
advisory mode; `fail_open` measures an admission error that the Ledger permits
through, including advisory availability failures. Off and honored skip produce
no admission metric or downstream call. Completion metrics measure the inline
attempt only: `failed` means the transition was handed to the retrier or
rejected as terminal, which the Error log distinguishes. Track expiries on the Tracer through `RESERVE_OPERATION_EXPIRED`
audit events.

## 1. Product segregation matrix

The two components are **separately-sellable products**, each shipping as its own OCI image under
the **Elastic License 2.0** (`LICENSE:1`). This product boundary — not a technical limitation — is
*why* the tracer is not embedded into the ledger binary: a customer can license and run the
ledger alone, or add the tracer as a distinct product.

| Tier | Image / build context | Binary source | Base image | Port(s) | License | Notes |
|---|---|---|---|---|---|---|
| **Ledger** (unified) | `components/ledger/Dockerfile` | `components/ledger/cmd/app/main.go` (`Dockerfile:19`) | distroless `static-debian12` (default tag), run as nonroot via `USER nonroot:nonroot` (`Dockerfile:28`), static `CGO_ENABLED=0 -tags netgo` (`Dockerfile:13-31`) | `:3002` (`EXPOSE 3002`, `Dockerfile:26`) | Elastic-2.0 | One binary serving onboarding + transaction + CRM (holders/instruments) + fees; routes register under the `midaz` authz namespace via `protectedMidaz(...)` (`routes.go`). No embedded `HEALTHCHECK` — relies on orchestrator probes. |
| **Tracer** | `components/tracer/Dockerfile` | `components/tracer/cmd/app/main.go` | distroless `static-debian12:nonroot`, `GOMEMLIMIT=1800MiB` baked (`Dockerfile`) | `:4020` REST seam (`EXPOSE 4020`); optional gRPC seam on a separate port (see §6) | Elastic-2.0 | A separate `Dockerfile.dev` uses `alpine:3.23` with a `wget` `HEALTHCHECK` against `/readyz` on `SERVER_PORT` (`Dockerfile.dev`). |

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

- **Reserve is synchronous, pre-commit, hot-path.** `reservePreparedTransaction` is called inline right before
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

  The budget deliberately outlasts the tracer's own five-minute hold, but expiry is terminal: the
  operation is `EXPIRED` and a late confirm or release answers `0530` on every attempt, so the retrier
  stops and the spend is never counted.

  Two failures remain and both are reported at Error naming the transaction, the reservation and the
  amount: a sequence that exhausts its budget, and a transition turned away because the concurrency
  cap is full. The expiry sweep is the backstop for the CAPACITY only, and it returns capacity
  without ever counting the spend.

  The retry is in-process, so a ledger restart with sequences in flight loses them; the Tracer TTL
  then returns their capacity at expiry.

Net: the tracer can stay a small replica set and tolerate occasional saturation on the post-commit path,
while the pre-commit reserve path is the only latency-sensitive RPC — which is what co-scheduling (§2)
addresses.

**Back-pressure constraint (grounded, prescriptive support):** when the per-tenant pool cap is reached
the tracer supervisor stops spawning workers and returns **503 with `Retry-After`**
(`TENANT_CAP_RETRY_AFTER_SECONDS`, default 5s) plus a canonical error envelope
(`tracer/.env.example:283-292`). This is the documented graceful back-pressure behavior any scaling
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
opt-in via `TRACER_BASE_URL`. When empty (the documented default), `buildContextTracer`
(`bootstrap/tracer_context_runtime.go`) wires no coordinator and logs *"Tracer reservation integration
disabled (TRACER_BASE_URL unset)"*. The transaction paths then never build a reservation request or
dial the tracer; the call sites guard on the absent coordinator (`tracer_context_anchor.go`,
`transaction_reservation_anchor.go`).

**Layer 2 — tracer wired but unreachable (`failPosture` governs).** When a tracer *is* wired but
unreachable, behavior is governed by **per-ledger tracer settings** (`mode` + `failPosture`), not a
global flag (`contextTracerDisposition`, `tracer_context_anchor.go`):

- `mode = off` skips the integration. `advisory` proceeds after a Tracer
  `DENY`/`REVIEW` decision or identified availability failure, but rejects
  deterministic contract, policy, limit and context failures.
- `mode = enforce` + `failPosture = open` (**default**) → record span attribute
  `app.tracer.reservation_skipped=true` and proceed. A degraded tracer cannot block *all* transactions
  (`tracer_context_anchor.go`).
- `mode = enforce` + `failPosture = closed` → reject the transaction with
  `ErrTransactionReservationUnavailable` **before any balance move**, and do **not** set the skipped
  marker.

This open-vs-closed contract is locked by the proof tests in
`transaction_reservation_failposture_test.go`.

**How "unreachable" is detected.** Transport/availability failures normalize to `ErrTracerUnavailable`
at the transport boundary so `failPosture` can branch on them (`seam_errors.go`). Both clients read
the canonical code first: the problem `code` on REST (`contextHTTPResponseError`), the gRPC status
message on gRPC (`mapGRPCError`). Only a recognized code is deterministic, whatever the HTTP status or
gRPC code: `0043`, `0487` and `0527` are refusals before evaluation, and `0094`, `0143`, `0342`,
`0343`, `0530`, `0531` and `0534` keep their own class. Everything else is `ErrTracerUnavailable`:
transport errors, timeouts and context deadline/cancellation, HTTP 401 (after at most one token renewal), 429,
5xx, any 3xx or 4xx without a recognized code (a mesh RBAC denial, an ingress default backend, a 404
from an older Tracer pod, a redirect, which is never followed), and every gRPC status without a
recognized message (`PermissionDenied`, `InvalidArgument`, `NotFound`, `FailedPrecondition`,
`Unimplemented`, `Unavailable` and the rest). So a 503 or `Unavailable` carrying `0527` is a
rejection, while `0161`, `0330` and `0422` stay unavailability. A refusal before evaluation rejects
the Reserve in every mode, with `0527` for a missing policy and `0534` otherwise, and sends no
release. Over REST, a Ledger that holds no token the
Tracer accepts fails internally with `0536` (`ErrTracerTokenUnavailable`), classified as the same
unavailability: under `enforce` + `failPosture=closed` the API client receives `0178`/503, and the span
attribute `app.tracer.failure_cause=token_unavailable` tells it apart from an outage. A business
**DENY** decision is a *successful result*, not an error. `contextTracerDisposition` lets only
identified availability failures follow posture. Invalid context, unusable policies/limits, CEL
failures and unknown errors reject in both advisory and enforce modes. They are recorded as
`context_invalid`, never as `fail_open`.

**Boot-time graceful absence even when configured.** The gRPC client uses one persistent lazy
connection — `grpc.NewClient` does not dial until the first RPC — so wiring the client never blocks on
tracer reachability at boot (`grpc_client.go`). An empty target fails *construction* at boot rather than
at first transaction. `Close()` drains on SIGTERM when registered with the composition root. The REST
client mints its first M2M token at boot on a best-effort basis (five-second bound): a failure logs a
Warn and the first reservation retries the mint.

---

## 5. Producer identity and tenant authorization

Every reservation caller is a **platform producer** from a fixed roster, `{ledger}`
(`tracer/internal/adapters/producerauth`). The producer's service name is also its integration ID
and the service the tenant-manager associates with a tenant. Each transport has its own credential;
both resolve to the same `Producer{Service}` value, and everything after identity is shared.

| Transport | Ledger presents | Tracer verifies | Mapped by |
|---|---|---|---|
| gRPC (`TRACER_TRANSPORT=grpc`, default) | its client certificate over native mTLS; tenant in `x-tenant-id` metadata; no token | `RequireAndVerifyClientCert` against `TRACER_TLS_CLIENT_CA_FILE`; the verified leaf must carry exactly one URI subject alternative name | `certUri` in `TRACER_PLATFORM_PRODUCERS` |
| REST (`TRACER_TRANSPORT=rest`) | `Authorization: Bearer <M2M token>` from the Access Manager plus `X-Tenant-Id` | lib-auth `RequireM2M`, locally against the cached JWKS at `CONTEXT_M2M_JWKS_URL` and the issuer `CONTEXT_M2M_ISSUER` | the token's `azp` as `clientId` in `TRACER_PLATFORM_PRODUCERS` |

`TRACER_PLATFORM_PRODUCERS` is a JSON array such as
`[{"service":"ledger","clientId":"<azp>","certUri":"spiffe://<trust-domain>/ledger"}]`; its boot
rules are in [Tracer invariants](../tracer/INVARIANTS.md#producer-identity-for-reservations). An
unmapped `azp` or certificate is 403 / `PermissionDenied` with `0043`. On REST the rejections are
`application/problem+json`: a missing token is 401 `0041`, an invalid or expired token is 401 `0042`
(also when the JWKS cannot be fetched), and a user token or unmapped `azp` is 403 `0043`. Under
`DEPLOYMENT_MODE=local` the Tracer skips token verification and attributes every HTTP reservation to
the ledger producer, logging a Warn at boot; `DEPLOYMENT_MODE=local` together with
`MULTI_TENANT_ENABLED=true` refuses boot. `TRACER_PLATFORM_PRODUCERS` (at most 64 KiB; an unknown entry key is rejected) enables
the reservation surface; empty, the Tracer serves validations only, mounts no reservation route,
refuses `TRACER_GRPC_PORT` and accepts limits of any scope. Once it is set, outside `local`,
`CONTEXT_M2M_JWKS_URL` and `CONTEXT_M2M_ISSUER` are required, whichever transports it serves. A multi-tenant Tracer that serves gRPC refuses boot
without its tenant authorizer and tenant pool manager.

**Ledger side.** The integration ID is the Ledger's `APPLICATION_NAME` (unset means `ledger`),
checked against the roster at boot. For REST the Ledger mints the token with the lib-auth client
(`PLUGIN_AUTH_HOST`, `IDP_M2M_CLIENT_ID`, `IDP_M2M_CLIENT_SECRET`) through
`adapters/tracer/m2m_token_source.go`, and never logs the secret or the token:

- The token is cached and renewed ahead of `exp` by the smaller of 60 seconds and half its lifetime.
  While the cached token is still valid the renewal runs in the background, one renewal shared by
  every caller; only a caller without a valid token waits for the mint.
- After a failed mint the Ledger mints nothing for 5 seconds, and a caller without a valid token fails
  fast with the internal cause `0536`.
- A Tracer 401 makes the REST client discard the cached token, only when it is at least 5 seconds old,
  and retry once with a fresh one. A 401 that persists costs about one mint per 5 seconds and is
  Tracer unavailability under the ledger's `failPosture`, as is a 429 or a 403 without a recognized
  code; a 403 carrying `0043` is a rejection.
- The REST client never follows redirects, so the bearer token never reaches a redirect target.

REST refuses boot when `PLUGIN_AUTH_ENABLED=false` or either credential is empty, and when neither
`PLUGIN_AUTH_HOST` nor service discovery provides the plugin-auth address. Under
`DEPLOYMENT_MODE=saas` it also refuses an `http://` `TRACER_BASE_URL` or plugin-auth address unless
`TRACER_TLS_MODE=mesh`.
gRPC refuses boot unless `TRACER_TLS_MODE=mtls` with `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE` and
`TRACER_TLS_CA_FILE`.

**Tenant authorization.** After identity, the Tracer reads the tenant from `X-Tenant-Id` (REST) or
`x-tenant-id` (gRPC), a single constant (`TenantHeader`, `ledger/adapters/tracer/client.go`) so the two
transports cannot drift. In multi-tenant mode it checks the tenant against a cached set of the
tenants active for the producer's service, read from the tenant-manager with
`GET /v1/tenants/active?service=ledger` (`tracer/bootstrap/tenant_association_set.go`), then resolves
the tenant's Tracer pool. The set holds tenant IDs only; the Tracer fetches no Ledger credentials or
connection settings. The Tracer's `MULTI_TENANT_SERVICE_API_KEY` must be allowed to list active
tenants for `service=ledger`. A 401 or 403 on that list logs a Warn with a permission hint; members of
a usable set are still admitted until it is 3 × `MULTI_TENANT_CACHE_TTL_SEC` old, and every other
reservation gets 503 `0161`.

| Situation | Result |
|---|---|
| Single-tenant | header ignored; no lookup |
| Tenant missing or malformed | 400 `0487` / `InvalidArgument` |
| Tenant absent from a fresh active set | 403 `0043` / `PermissionDenied` |
| Tenant in a stale set younger than 3 × `MULTI_TENANT_CACHE_TTL_SEC` | admitted; one background refresh |
| Refresh failed or in its 5-second backoff, tenant in a usable set | admitted |
| Refresh failed or in its 5-second backoff, any other case | 503 `0161` / `Unavailable` with message `0161` |
| Tracer pool not found or suspended | 403 `0043` / `PermissionDenied` |
| Tracer pool unavailable | 503 `0161` / `Unavailable` with message `0161` |
| Caller cancelled | 503 `0330` / `Canceled`, no code in the message |
| Deadline passed | 504 `0422` / `DeadlineExceeded`, no code in the message |
| Missing producer or inconsistent tenant wiring | 503 `0527` / `Unavailable` with message `0527` |

A set is fresh for `MULTI_TENANT_CACHE_TTL_SEC` and usable up to three times that. A fresh set answers
its members without a call; a stale but usable set admits them at once while one background refresh
runs (stale-while-revalidate). A tenant absent from the set starts at most one shared refresh every
5 seconds, so a newly associated tenant gets `0043` until a refresh sees it; only these miss lookups
start that window, never warm-up, background or expiry refreshes. For 5 seconds after a failed list
call the Tracer makes no call. A failed refresh never produces `0043`. An empty list while the
previous set is non-empty and usable is ignored: one Error is logged, the previous set keeps
answering, and non-members get `0161`. Each failed list call logs one Warn, with a permission hint
for a 4xx; the tenant-manager client can add its own Error for a non-200 answer. The Tracer warms
every set at boot in the background: a failure also logs a Warn, and requests refresh on demand.
Warm-up and background refreshes run under `SafeGo`.

`0161`, `0330` and `0422` are availability failures, so the Ledger applies its `failPosture`. `0043`,
`0487` and `0527` are refusals before evaluation: the Ledger rejects the Reserve in every posture and
treats the same answer on a completion as terminal.

The tenant resolver is wired **only** onto the reservation routes and RPCs; user-facing tracer routes
keep their JWT-claim tenant path, so no header-trust path is reachable without a verified producer.
Revoking an association or suspending a tenant takes effect once the cached set expires, up to
`MULTI_TENANT_CACHE_TTL_SEC` (default 120 seconds), and up to three times that while the
tenant-manager cannot answer.

**TLS per listener.** `TRACER_TLS_MODE` has a fail-fast typed error on an invalid value on both sides
(`ledger/bootstrap/tracer_context_config.go`, `tracer/bootstrap/tls_seam.go`):

- **`mtls`** — the Tracer presents its server certificate on both listeners. The HTTP listener
  (`buildHTTPTLSConfig`) serves server-only TLS: it neither requests nor verifies a client
  certificate, because HTTP producers authenticate with tokens, and console, load-balancer and probe
  traffic reach it without one. The gRPC listener (`buildGRPCTLSConfig`) enforces
  `tls.RequireAndVerifyClientCert` against `TRACER_TLS_CLIENT_CA_FILE`, which stays required in `mtls`.
- **`mesh` / empty** — HTTP listens plaintext behind a service-mesh sidecar. The gRPC seam cannot run
  in this mode: a non-empty `TRACER_GRPC_PORT` refuses boot, as it does when no `certUri` is mapped.
  Under `DEPLOYMENT_MODE=saas` an empty `TRACER_TLS_MODE` refuses boot; name `mtls` or `mesh`.

On the Ledger, `mtls` verifies the tracer's server leaf against `TRACER_TLS_CA_FILE` and pins
`ServerName` to the host of `TRACER_BASE_URL`. gRPC always presents the client leaf; REST presents it
only when `TRACER_TLS_CERT_FILE` or `TRACER_TLS_KEY_FILE` is set, and requires an `https`
`TRACER_BASE_URL`. `mesh`/empty is valid only with REST. Under `DEPLOYMENT_MODE=saas` the Ledger
refuses a REST `TRACER_BASE_URL` over `http://` unless `TRACER_TLS_MODE=mesh`.

**CA env-var asymmetry (deliberate):** each side names the CA var by what it verifies on the *other*
end. Ledger `TRACER_TLS_CA_FILE` verifies the **tracer's** server leaf; Tracer
`TRACER_TLS_CLIENT_CA_FILE` verifies the **ledger's** gRPC client leaf.

**Rotation / hot-reload:** both sides load their cert/key through the lib-commons
`certificate.Manager`, so certificates rotate without restart. JWKS keys refresh in the background
of the Tracer's key source; M2M tokens renew before they expire.

---

## 6. Transport & ports

The reservation seam is one `command.ContextTracerClient` port with **two interchangeable transports**
selected by `TRACER_TRANSPORT`. The composition root (`buildContextTracerClient`,
`ledger/bootstrap/tracer_context_runtime.go`) picks `ContextGRPCClient` or `ContextHTTPClient`; the
anchor stays transport-agnostic. Both carry the same contract and address completion by transaction
ID.

| Knob | Value | Behavior |
|---|---|---|
| `TRACER_TRANSPORT` | `grpc` (**default**) | empty selects gRPC; requires `TRACER_TLS_MODE=mtls` |
| `TRACER_TRANSPORT` | `rest` | M2M bearer token; any TLS mode |
| `TRACER_TRANSPORT` | any other | **fails boot** with a typed error |
| `TRACER_BASE_URL` | full URL | feeds both transports; REST uses it directly, gRPC strips the scheme to `host:port` |
| Ledger `SERVER_ADDRESS` | `:3002` (default) | unified ledger binary, all APIs on one port |
| Tracer `SERVER_ADDRESS` | `:4020` | HTTP API, `/v1/reservations` and health |
| `TRACER_GRPC_PORT` | **empty by default** | gRPC seam server **not started** unless set; requires `mtls` and a `certUri` mapping |

**Ports.** The ledger serves everything on a single port, default `:3002`. The tracer serves HTTP on
`:4020`; the reservation **gRPC seam listens on a separate port** via `TRACER_GRPC_PORT`, which is
**empty by default**.

> **NOTE — `:4021` is illustrative, not canonical.** `TRACER_GRPC_PORT`'s documentation uses
> *"e.g. :4021"*. There is no default value and no `EXPOSE 4021` anywhere.

**One Tracer, both transports.** A single `TRACER_PLATFORM_PRODUCERS` entry may carry both a
`clientId` and a `certUri`, so the same Tracer deployment serves a REST Ledger and a gRPC Ledger. An
unspecified `TRACER_TRANSPORT` selects gRPC: a deploy that sets `TRACER_BASE_URL` without it must
expose `TRACER_GRPC_PORT` on the tracer, run `mtls` on both ends and map the Ledger's certificate URI.
Behind a mesh without native `mtls`, select `TRACER_TRANSPORT=rest`.

---

## Appendix — Operator env surface

The seam's env knobs are defined as Go struct tags and validated in code; this is the grounded source of
truth for their **existence and semantics**.

| Var | Side | Meaning |
|---|---|---|
| `TRACER_BASE_URL` | ledger | set = integration on; empty = disabled |
| `TRACER_TRANSPORT` | ledger | `grpc`\|`rest`; empty → `grpc` |
| `TRACER_TIMEOUT_MS` | ledger | shared admission cap (facts and Reserve); client RPC cap |
| `TRACER_TLS_MODE` | ledger | `mtls`\|`mesh`/empty; gRPC requires `mtls`; REST over `http://` under `DEPLOYMENT_MODE=saas` requires `mesh` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | ledger | client leaf; required for gRPC, optional for REST |
| `TRACER_TLS_CA_FILE` | ledger | CA verifying the **tracer's** server leaf |
| `APPLICATION_NAME` | ledger | integration ID; must be in the roster `{ledger}` |
| `PLUGIN_AUTH_HOST` | ledger | Access Manager that mints the M2M token; required for REST |
| `IDP_M2M_CLIENT_ID` / `IDP_M2M_CLIENT_SECRET` | ledger | M2M application credentials for REST |
| `TRACER_GRPC_PORT` | tracer | gRPC seam listen addr; empty → off |
| `TRACER_TLS_MODE` | tracer | `mtls` (HTTP server-only TLS, gRPC mutual TLS)\|`mesh`/empty; empty refuses boot under `DEPLOYMENT_MODE=saas` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | tracer | server leaf material (mtls) |
| `TRACER_TLS_CLIENT_CA_FILE` | tracer | CA verifying the **ledger's** gRPC client leaf; required in `mtls` |
| `TRACER_PLATFORM_PRODUCERS` | tracer | roster service → `clientId` and/or `certUri`; set enables the reservation surface, empty is validations-only; at most 64 KiB, unknown keys rejected |
| `CONTEXT_M2M_JWKS_URL` | tracer | JWKS verifying producer tokens; required with producers set outside `DEPLOYMENT_MODE=local` |
| `CONTEXT_M2M_ISSUER` | tracer | expected token issuer; required unless `DEPLOYMENT_MODE=local` |
| `TENANT_CAP_RETRY_AFTER_SECONDS` | tracer | 503 `Retry-After` on tenant-pool cap (default 5s) |

The Ledger and Tracer `.env.example` files expose these variables. The Ledger
template additionally lists the coordinator resource settings; the Tracer
template lists producer identity and Reserve budgets. Absent resource keys use
the shared technical defaults, while empty rendered values fail validation.
Operators must verify and measure the chosen values before enabling the
integration; they are not workload SLOs.

Official fact loading uses a local deadline bounded by the per-ledger `timeoutMs`
and global `TRACER_TIMEOUT_MS` cap. A local fact timeout is a deterministic facts
failure and never follows the remote fail posture. After facts load, Reserve
receives a fresh admission deadline with the same bound; the caller deadline
still caps the whole operation. Completion ignores the caller's cancellation and
is bounded by the per-ledger `timeoutMs` instead.
