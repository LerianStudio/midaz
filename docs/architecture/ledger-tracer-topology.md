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
`failPosture`. `handleReserveError` treats **any other** reserve error as fail-posture-gated, so a
tracer defect cannot let an `enforce`+`closed` ledger commit unchecked. The full outcome table lives in
`docs/api/SCOPING.md`.

**Boot-time graceful absence even when configured.** The gRPC client uses one persistent lazy
connection — `grpc.NewClient` does not dial until the first RPC — so wiring the client never blocks on
tracer reachability at boot (`grpc_client.go:79-123`). An empty target fails *construction* at boot
rather than at first transaction. `Close()` drains on SIGTERM when registered with the composition root
(`grpc_client.go:125-129`).

---

## 5. mTLS model (identity, not a shared secret)

**Seam identity is mutual TLS — the verified mTLS peer IS the credential. There is no shared secret and
no static key.** This is stated explicitly in code: *"identity on the reservation seam is mutual TLS
(the verified peer IS the credential — no shared secret)"* (`config.go:1556-1564`).

`TRACER_TLS_MODE` selects the posture, with a fail-fast typed error on an invalid value
(`ledger/tls_seam.go:51-62`, server-side mirror `tracer/tls_seam.go:48-59`):

- **`mtls`** — the app presents and verifies certificates directly.
- **`mesh`** — the app dials/listens **plaintext** and delegates mTLS origination/termination to a
  local **Istio/Linkerd** service-mesh sidecar.
- **empty** — plaintext with no verified peer. The tracer accepts it only when
  `DEPLOYMENT_MODE=local` is set explicitly; an unset `DEPLOYMENT_MODE` or any other deployment mode
  refuses boot (`ValidateSeamTransportPosture`, `tracer/seam_posture.go`).

**`mtls` mode, ledger (client) side** (`ledger/tls_seam.go:82-103`): presents its leaf via
`GetClientCertificate`, verifies the tracer's server leaf against `RootCAs` loaded from
`TRACER_TLS_CA_FILE`, and pins `ServerName` to the host parsed from `TRACER_BASE_URL`
(`seamServerName`, `config.go:1564, 1656-1669`). The injected mTLS dial credentials are appended **last**
and the insecure default is gated off (`len(conf.dialOptions)==0`) so it cannot clobber them
(`grpc_client.go:100-111`, injection at `config.go:1633-1635`).

**`mtls` mode, tracer (server) side** (`tracer/tls_seam.go`, `buildMTLSConfig`): presents its own leaf via
`GetCertificate` and enforces `tls.RequireAndVerifyClientCert` against the client CA pool from
`TRACER_TLS_CLIENT_CA_FILE`. **The reservation seam is unreachable without a verified client cert.**

**Client identity allowlist (gRPC listener only).** A CA-signed certificate is not enough on the seam:
`TRACER_TLS_CLIENT_ALLOWED_NAMES` (comma-separated) names the client identities the gRPC listener
accepts. The handshake passes only when one of the leaf's DNS SANs or URI SANs (e.g. `spiffe://...`)
equals an entry, or, on a leaf that carries no SAN, its Subject CN does, compared exactly after
trimming and case-folding (`buildGRPCSeamTLSConfig` → `verifyClientAllowedName` / `clientCertAllowed`).
A refused handshake reaches the ledger as gRPC `Unavailable`, so it follows `failPosture`. An empty
allowlist accepts any certificate the client CA signed: under `DEPLOYMENT_MODE=saas` the tracer refuses
to boot (`ValidateSeamTransportPosture`); elsewhere it logs one Warn at boot
(`warnGRPCSeamAcceptsAnyClient`). The list is ignored in `mesh`/empty mode and never applied to the
HTTP listener, which keeps the base `buildSeamTLSConfig`.

**CA env-var asymmetry (deliberate and correct):** each side names the CA var by what it verifies on the
*other* end. Ledger `TRACER_TLS_CA_FILE` holds the CA that verifies the **tracer's** server leaf
(`config.go:310`, `tls_seam.go:87-90`). Tracer `TRACER_TLS_CLIENT_CA_FILE` holds the CA that verifies the
**ledger's** client leaf (`tracer/config.go:68-72`, `tls_seam.go:83-86`).

**Rotation / hot-reload:** both sides load their cert/key through the lib-commons
`certificate.Manager` (`libCert "github.com/LerianStudio/lib-commons/v7/commons/certificate"`, both
`tls_seam.go:14`). The seam therefore inherits **cert rotation without restart**: the ledger serves the
latest cert via `GetClientCertificate → certManager.TLSCertificate()`
(`ledger/tls_seam.go:82-102`), the tracer via `GetCertificate → certManager.GetCertificateFunc()`
(`tracer/tls_seam.go:78-90`).

**Fail-fast on missing material:** in `mtls` mode each of `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE`,
and the respective CA file is required; a missing one fails boot with an error naming the exact missing
knob, on **both** sides (`ledger/tls_seam.go:67-77`, `tracer/tls_seam.go:63-73`).

### Trusted `x-tenant-id` — the rationale

Tenant crosses the seam as a **trusted `x-tenant-id` gRPC metadata key**, not a JWT claim and not a
shared secret. It is trusted **precisely because the mTLS peer is verified** (or sits behind a verified
mesh sidecar): mTLS replaces token identity, so there is no `Authorization` metadata. The ledger appends
the key on every RPC (`tenantMetadataKey` in the ledger `adapters/tracer` package, emitted via
`AppendToOutgoingContext`); the tracer reads the same key (`seamtenant.MetadataKey`).

The tenant resolver is wired **only** onto the reservation RPCs (`seamtenant.Resolver`,
`grpc/in/tenant_interceptor.go`); user-facing tracer routes keep their JWT-claim tenant path. The
header is only as trustworthy as the peer, so the deployment must verify it one of two ways:

- **`mtls`** with `TRACER_TLS_CLIENT_ALLOWED_NAMES` naming the ledger's certificate identity
  (required under `DEPLOYMENT_MODE=saas`).
- **`mesh`** with STRICT `PeerAuthentication` (or the Linkerd equivalent) on the tracer workload plus an
  `AuthorizationPolicy` or `NetworkPolicy` that admits only the ledger's identity to `:4021`. The app
  cannot check this itself and logs a boot Warn in `mesh` mode.

An empty `TRACER_TLS_MODE` serves the seam plaintext with no verified peer, so the tracer refuses to
boot with it unless `DEPLOYMENT_MODE=local` is set explicitly; an unset `DEPLOYMENT_MODE` refuses too
(`ValidateSeamTransportPosture`). Under multi-tenant mode a
missing/empty/invalid trusted tenant key is a **clean failure** (`ErrReservationTenantRequired` →
gRPC `InvalidArgument`) and never falls back to a default or wrong pool. A tenant the tenant manager
reports as not provisioned, suspended or purged answers gRPC `Unavailable` with
`ErrReservationTenantInactive` (`0534`), so the ledger routes it through `failPosture` instead of
reading it as a refusal. In single-tenant mode the resolver is a no-op pass-through and nothing is
appended.

This is the same trusted-tenant boundary the rest of the platform rides: `MULTI_TENANT_ENABLED=true` is
rejected at config validation unless `PLUGIN_AUTH_ENABLED=true` (`config.go:351-353`), and tenant IDs
derive from the JWT via lib-commons tenant managers + middleware (`config.go:483-486`).

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
| Tenant reached its per-tenant worker cap | `Unavailable` | `0445` | `failPosture` |

Under `failPosture`, `open` proceeds with a SKIPPED audit and `closed` rejects with `0178`. Expect the
cold cache window right after a tracer rollout, and size `failPosture` accordingly.

**Tracer migration `000025` needs a maintenance window.** Widening the asset columns rewrites
`transaction_validations` and rebuilds all of its indexes under an `ACCESS EXCLUSIVE` lock, so size
the window by that table's row count: every read and write of validations blocks for the whole
rewrite. Each `ALTER` runs with a 5-second lock timeout and fails fast with SQLSTATE `55P03` instead
of queueing behind live traffic (where it would block every later query on the table). Both `ALTER`s
are guarded on the column's current type, so a re-run in a quieter window is safe.

**Ports.** The ledger serves everything on a single port, default `:3002` (`SERVER_ADDRESS`). The tracer
serves its HTTP API and health on `:4020` (`SERVER_ADDRESS`) and the reservation **gRPC seam on
`:4021`** (`TRACER_GRPC_PORT`). The image's `EXPOSE` covers `:4020` only; the component compose
publishes `:4021` (`TRACER_GRPC_HOST_PORT`), and a cluster deploy must expose `:4021` as a service port
reachable from the ledger.

**Both tracer listeners share one base TLS posture.** `buildSeamTLSConfig` is the base config of both
the HTTP and the gRPC listener, so the two cannot drift: in `mtls` both require and verify a client
cert; in `mesh`/unset both listen plaintext. The gRPC listener alone layers the client-identity
allowlist on top (`buildGRPCSeamTLSConfig`, §5).

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
| `TRACER_TLS_MODE` | ledger | `mtls`\|`mesh`/empty | `buildSeamClientTLSConfig` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | ledger | client leaf material (mtls) | `buildClientMTLSConfig` |
| `TRACER_TLS_CA_FILE` | ledger | CA verifying the **tracer's** server leaf | `buildClientMTLSConfig` |
| `TRACER_GRPC_PORT` | tracer | gRPC seam listen address; empty → `:4021`; always on | `DefaultTracerGRPCPort`, `ApplyGRPCSeamDefaults` |
| `TRACER_TLS_MODE` | tracer | `mtls`\|`mesh`\|empty; empty (plaintext, no verified peer) boots only with an explicit `DEPLOYMENT_MODE=local` (unset refuses); `mesh` logs a boot Warn | `buildSeamTLSConfig`, `ValidateSeamTransportPosture` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | tracer | server leaf material (mtls) | `buildMTLSConfig` |
| `TRACER_TLS_CLIENT_CA_FILE` | tracer | CA verifying the **ledger's** client leaf | `buildMTLSConfig` |
| `TRACER_TLS_CLIENT_ALLOWED_NAMES` | tracer | comma-separated client identities (DNS SAN / URI SAN, or CN on a cert without SANs) the gRPC listener accepts under mtls; empty → any CA-signed cert plus a boot Warn, and a refused boot under `DEPLOYMENT_MODE=saas`; never applied to the HTTP listener | `buildGRPCSeamTLSConfig`, `clientCertAllowed`, `ValidateSeamTransportPosture` |
| `TENANT_CAP_RETRY_AFTER_SECONDS` | tracer | HTTP 503 `Retry-After` on tenant-pool cap (default 5s) | `tracer/.env.example` |
