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
> `ledger/` or `tracer/` prefix disambiguates. The ledger reservation seam itself lives in the HTTP
> **handler** layer (`components/ledger/internal/adapters/http/in/`: `transaction_create.go`,
> `transaction_reservation.go`, `transaction_reservation_anchor.go`) — it is a transport-boundary
> integration (call the tracer, branch on `failPosture`), not domain logic, so it sits at the boundary
> rather than in `internal/services/command/`. Line ranges are accurate at time of writing but rot;
> the cited function/const symbols are the durable anchors.

---

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
— called inline immediately before `ProcessBalanceOperations` in the transaction-create handler
(`transaction_create.go:1231-1239`; anchor doc at `transaction_reservation_anchor.go:72-85`).
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
  the balance commit; a reject returns *before* any balance moves (`transaction_create.go:1231-1239`).
  The tracer must therefore be **low-latency**, but each reservation's work is bounded per transaction.

- **V1 Confirm / Release are post-commit and best-effort (non-blocking).** After a successful balance
  commit, `confirmReservations` runs for non-PENDING transactions; on a commit failure
  `releaseReservations` runs; PENDING defers confirm to `/commit` and release to `/cancel`
  (`transaction_create.go:1241-1273`). Transport failures on confirm/release are logged at Warn,
  span-recorded, and **never propagated**. This is a known correctness gap, not a durability
  backstop: if confirm is lost after money moves, expiry later removes that amount from Tracer usage
  and reopens limit capacity (`transaction_reservation_anchor.go:246-261, 282-289`). Closing the gap
  requires the V2 durable Ledger-owned outcome handoff described in §7. The Tracer receiver exists
  additively; V1 remains unchanged until the Ledger explicitly reserves with
  `LEDGER_OUTCOME_V2` and runs its durable dispatcher.

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
opt-in via `TRACER_BASE_URL`. When empty (the documented default), `buildTracerReserver` returns a
genuine `nil` `TracerReserver` interface and logs *"Tracer reservation integration disabled
(TRACER_BASE_URL unset)"* (`config.go:1548-1554`; struct doc at `config.go:285-290`). The
transaction-create path is then byte-for-byte unchanged.

A `nil` reserver is treated as "tracer disabled" at every call site via explicit nil guards, mirroring
the streaming nil-emitter pattern: `reserveTransaction` returns *proceed* with an empty handle, and
confirm/release are no-ops (`transaction_reservation.go:24-26`,
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
(`grpc_client.go:343-358`), and the REST client wraps transport errors equivalently
(`client.go:53-60, 351-354`). A business **DENIED** decision is a *successful result*, not an error.
`handleReserveError` additionally treats **any** non-availability reserve error as fail-posture-gated,
so a tracer defect cannot let an `enforce`+`closed` ledger commit unchecked
(`transaction_reservation_anchor.go:143-148`).

**Boot-time graceful absence even when configured.** The gRPC client uses one persistent lazy
connection — `grpc.NewClient` does not dial until the first RPC — so wiring the client never blocks on
tracer reachability at boot (`grpc_client.go:79-123`). An empty target fails *construction* at boot
rather than at first transaction. `Close()` drains on SIGTERM when registered with the composition root
(`grpc_client.go:125-129`).

---

## 5. Transport identity and REST application authentication

**Transport identity is mutual TLS; REST additionally authenticates with a dedicated Tracer API key.**
The Ledger sends `TRACER_API_KEY` only as `X-API-Key`. The Tracer reservation routes deliberately use
the API-key guard even when user-facing multi-tenant routes use plugin/JWT auth. The key is distinct
from `MULTI_TENANT_SERVICE_API_KEY`; reusing the Tenant Manager credential would collapse two trust
boundaries. `API_KEY_ENABLED` controls only the optional API-key fallback on Tracer's ordinary REST
routes: setting it to `false` never disables authentication on reserve, confirm, release, or durable
outcome operations. Those financial routes always require the configured `API_KEY`; a missing or
mismatched credential returns `401` before tenant resolution or handler execution. gRPC carries no API
key because its reservation listener authenticates the verified mTLS peer.

`TRACER_TLS_MODE` selects the posture, with a fail-fast typed error on an invalid value
(`ledger/tls_seam.go:51-62`, server-side mirror `tracer/tls_seam.go:48-59`):

- **`mtls`** — the app presents and verifies certificates directly.
- **`mesh` / empty** — the app dials/listens **plaintext** and delegates mTLS origination/termination to
  a local **Istio/Linkerd** service-mesh sidecar.

**`mtls` mode, ledger (client) side** (`ledger/tls_seam.go:82-103`): presents its leaf via
`GetClientCertificate`, verifies the tracer's server leaf against `RootCAs` loaded from
`TRACER_TLS_CA_FILE`, and pins `ServerName` to the host parsed from `TRACER_BASE_URL`
(`seamServerName`, `config.go:1564, 1656-1669`). The injected mTLS dial credentials are appended **last**
and the insecure default is gated off (`len(conf.dialOptions)==0`) so it cannot clobber them
(`grpc_client.go:100-111`, injection at `config.go:1633-1635`).

**`mtls` mode, tracer (server) side** (`tracer/tls_seam.go:62-94`): presents its own leaf via
`GetCertificate` and enforces `tls.RequireAndVerifyClientCert` against the client CA pool from
`TRACER_TLS_CLIENT_CA_FILE`. **The reservation seam is unreachable without a verified client cert.**

**CA env-var asymmetry (deliberate and correct):** each side names the CA var by what it verifies on the
*other* end. Ledger `TRACER_TLS_CA_FILE` holds the CA that verifies the **tracer's** server leaf
(`config.go:310`, `tls_seam.go:87-90`). Tracer `TRACER_TLS_CLIENT_CA_FILE` holds the CA that verifies the
**ledger's** client leaf (`tracer/config.go:68-72`, `tls_seam.go:83-86`).

**Rotation / hot-reload:** both sides load their cert/key through the lib-commons
`certificate.Manager` (`libCert "github.com/LerianStudio/lib-commons/v6/commons/certificate"`, both
`tls_seam.go:14`). The seam therefore inherits **cert rotation without restart**: the ledger serves the
latest cert via `GetClientCertificate → certManager.TLSCertificate()`
(`ledger/tls_seam.go:82-102`), the tracer via `GetCertificate → certManager.GetCertificateFunc()`
(`tracer/tls_seam.go:78-90`).

**Fail-fast on missing material:** in `mtls` mode each of `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE`,
and the respective CA file is required; a missing one fails boot with an error naming the exact missing
knob, on **both** sides (`ledger/tls_seam.go:67-77`, `tracer/tls_seam.go:63-73`).

### Trusted `x-tenant-id` — the rationale

Tenant crosses the seam as a **trusted `x-tenant-id`** — a REST header / gRPC outgoing metadata key, not
a JWT claim and not the application credential. It is trusted because the transport peer is verified
directly by mTLS or by the mesh; on REST, `X-API-Key` independently authenticates the calling application.
The key derives from one constant —
`TenantHeader = "X-Tenant-Id"` with the gRPC metadata key as its lower-cased form — so REST and gRPC
cannot drift (`client.go:359-368`; tracer-side resolver `seamtenant/resolver.go:33-39`).

The tenant resolver is wired **only** onto the reservation routes/RPCs (`resolver.go:5-16`,
`grpc/in/tenant_interceptor.go:20-29`); user-facing tracer routes keep their JWT-claim tenant path,
so there is no header-trust path reachable without the verified peer. Under multi-tenant mode a
missing/empty/invalid trusted tenant key is a **clean failure** (`ErrReservationTenantRequired` →
gRPC `InvalidArgument` / REST business error) and never falls back to a default or wrong pool; in
single-tenant mode the resolver is a no-op pass-through and nothing is appended
(`resolver.go:107-125`, `tenant_interceptor.go:36-43`, `reservation_tenant_middleware.go:47-52`).

This is the same trusted-tenant boundary the rest of the platform rides: `MULTI_TENANT_ENABLED=true` is
rejected at config validation unless `PLUGIN_AUTH_ENABLED=true` (`config.go:351-353`), and tenant IDs
derive from the JWT via lib-commons tenant managers + middleware (`config.go:483-486`).

---

## 6. Transport & ports

The reservation seam is a single `TracerReserver` port with **two interchangeable transports** —
gRPC and REST — selected by `TRACER_TRANSPORT`. The composition root picks the concrete client; the
reserve anchor stays transport-agnostic (`config.go:1574-1582` switches to
`buildTracerGRPCReserver` / `buildTracerRESTReserver`; `grpc_client.go:29-46` implements the same port
as the REST client).

| Knob | Value | Behavior | Evidence |
|---|---|---|---|
| `TRACER_TRANSPORT` | `grpc` (**default**) | empty → `tracerTransportGRPC` | `config.go:1569-1572`, consts `:1585-1588` |
| `TRACER_TRANSPORT` | `rest` | retained fallback, selectable explicitly | `config.go:1574-1582` |
| `TRACER_TRANSPORT` | any other | **fails boot** with a typed error | `config.go:1569-1581` |
| `TRACER_BASE_URL` | full URL | feeds both transports; REST uses it directly, gRPC strips scheme to `host:port` via `stripURLScheme` | `config.go:1607, 1626, 1645-1654` |
| Ledger `SERVER_ADDRESS` | `:3002` (default) | unified ledger binary, all APIs on one port | `config.go:62-63`, `ledger/.env.example:34-35` |
| Tracer `SERVER_ADDRESS` | `:4020` | REST seam + health | `tracer/.env.example:14-15` |
| `TRACER_GRPC_PORT` | **empty by default** | gRPC seam server **not started** unless set | `tracer/config.go:49-54`, `initGRPCServer` returns `nil,nil` when empty `:1230-1232` |

**Ports.** The ledger serves everything on a single port, default `:3002` (`SERVER_ADDRESS`). The tracer
serves REST/health on `:4020` (`SERVER_ADDRESS`); the reservation **gRPC seam listens on a separate
port** via `TRACER_GRPC_PORT`, which is **empty by default**, so the gRPC server is off unless an
operator configures it.

---

## 7. Durable reservation outcomes (V2)

V2 closes the lost post-commit confirmation gap without making the Tracer infer Ledger state. The
ownership boundary is strict: **the Ledger owns the terminal accounting decision and delivery; the
Tracer owns reservation capacity and the durable receipt. There is no Tracer-to-Ledger pull RPC.**

### Protocol

1. V1 continues to use `POST /v1/reservations` / `Reserve`. V2 uses the distinct
   `POST /v1/reservations/ledger-outcome-v2` / `ReserveV2` operation and carries
   `delivery_mode=LEDGER_OUTCOME_V2`. A pre-V2 server therefore returns `404` / `UNIMPLEMENTED`
   **before creating any hold**; the Ledger never falls back to the legacy operation. Every successful
   reserve response echoes the delivery mode the Tracer actually accepted. A V2 Ledger requires an
   explicit V2 echo and rejects a legacy or omitted echo before the balance path. On the V1 operation,
   omitted / `UNSPECIFIED` / `LEGACY` continues to select legacy confirm/release plus autonomous TTL.
2. After its own durable decision, the Ledger delivers
   `ApplyOutcome(transactionId, outcomeId, COMMITTED|ABORTED)` over REST or gRPC. `COMMITTED` moves
   every reservation from held capacity to consumed usage; `ABORTED` returns every hold. The Tracer
   uses the coordinates already stored at reserve time and never re-resolves the current limit set.
3. Receipt claim, every counter move, every reservation transition, and every reservation audit row
   commit in one Tracer PostgreSQL transaction. A transaction with zero counter-backed limits still
   gets a receipt, making delivery retryable even when there was no capacity row to transition.
4. The exact `(transactionId, outcomeId, outcome)` replay returns the stored receipt without another
   counter move or audit. Any different terminal decision for that transaction is a conflict. A
   different outcome ID cannot replace the transaction's receipt.
5. Reserve and ApplyOutcome serialize on the transaction ID. Once a receipt exists, no later reserve
   can enter for that transaction; mixing V1 and V2 reservations under one transaction is also a
   conflict. This prevents a zero-limit outcome racing a late reserve into a permanent hold.

### Expiry and observability

V2 reservations persist `reservation_expires_at=NULL`; legacy reservations are constrained to a
non-null expiry. That representation is deliberate rolling-deploy compatibility: the fixed predicate
in a pre-V2 reaper (`status='RESERVED' AND reservation_expires_at < now`) cannot discover a V2 row.
Database triggers also reject any live V2 status change or deletion without the matching durable
outcome receipt, and silently prevent deletion of a counter that backs a live V2 hold. Therefore an
old reaper's counter decrement is rolled back with its rejected row transition, while an old cleanup
worker skips the protected counter. The new reaper additionally selects `LEGACY` reservations only.
A V2 reservation stays `RESERVED` until the Ledger
delivers `COMMITTED` or `ABORTED`, including PENDING reservations older than 30 days. This deliberately
prefers held capacity over silently reopening a limit after money may have moved. The reaper reports
the V2 outstanding count and oldest age as gauges; observing a stale backlog never releases it. The
observer runs even when `RESERVATION_REAPER_ENABLED=false`; that switch controls only autonomous V1
expiry. In multi-tenant deployments, both gauges carry `tenant_id` so tenants cannot overwrite each
other's series, and stopping a tenant worker resets its series instead of freezing stale values. The
existing worker safety cap still applies: `MULTI_TENANT_MAX_TENANT_POOLS` must cover every active
tenant assigned to the instance; an over-cap tenant is rejected by the instance and has no local
observer. Counter retention also skips every counter referenced by a live reservation and skips
counter rows locked by concurrent reserve/outcome work, so a V2 outcome remains applicable after the
normal counter-retention horizon.

### Tenant boundary

REST and gRPC use the existing verified seam identity and trusted tenant metadata from §5. The receipt
and reservation tables live inside the resolved tenant database. A transaction ID presented under a
different tenant can at most create that tenant's zero-limit receipt; it cannot discover, transition,
or audit the original tenant's reservation.

### Rollout and rollback

1. Deploy migration `000021` first. Existing binaries remain safe: their inserts receive the `LEGACY`
   default, their reaper cannot see NULL-expiry V2 rows, and database triggers prevent their cleanup
   paths from mutating live V2 state.
2. Deploy the V2 Tracer across the fleet. Mixed Tracer fleets are safe: a V2 reserve routed to an old
   pod fails at the distinct operation before a hold exists, and an outcome routed to an old pod fails
   for retry. Neither failure can reach the Ledger money path or reopen held capacity.
3. Deploy the Ledger-side durable outcome dispatcher, then enable
   `TRACER_OUTCOME_MODE=ledger_outcome_v2` reservation traffic. The Ledger persists `PREPARED` before
   Reserve and the balance Lua projects its exact economic proof into a dedicated outcome outbox;
   the Ledger must durably retain that outcome until the Tracer
   acknowledges its receipt; process-local retries are not a correctness mechanism.
   In multi-tenant deployments, PREPARED, its delivery schedule, the tenant's active-outcome index, and the
   deployment-global tenant inventory are written by one Lua command in the same Redis Cluster
   `{transactions}` slot. An acknowledged outcome therefore cannot survive a shard failure or AOF recovery
   without its discovery pointer. The worker reloads inventory tenants through Tenant Manager after a restart
   instead of enumerating the expiring process cache; `PENDING_HELD` remains in the active index while
   intentionally absent from delivery. Retirement atomically requires an empty active index and an unchanged
   inventory generation in that same slot. An absent record leaves a false-positive discovery pointer until
   the explicit missing-record quarantine path removes its indexes, so a concurrent prepare or tenant deletion
   cannot make real backlog undiscoverable.
4. During rollback, set `TRACER_OUTCOME_MODE=legacy` to stop new V2 reserves but keep
   `TRACER_OUTCOME_WORKER_ENABLED=true`; keep a Tracer version that understands V2 and the dispatcher running
   until the V2 outstanding gauge reaches zero. Disabling new V2 reserves does not make the existing
   backlog safe to forget.
5. The migration down path takes exclusive locks on receipts, reservations, and counters, then refuses
   to remove V2 support while any V2 reservation or outcome receipt exists. Drain/archive that state
   explicitly before schema rollback; a forced rollback would erase the proof that money-path
   outcomes were applied exactly once.

### Redis durability and RPO

Outcome V2 treats Redis as authoritative before PostgreSQL receives the final transaction. Enabling V2,
or keeping only its drain worker enabled during rollback, therefore fails boot, readiness, and per-request
admission unless every Redis shard proves `maxmemory-policy=noeviction`, `appendonly=yes`, a healthy last
AOF write, and `appendfsync=always|everysec`. `appendfsync=everysec` deliberately accepts an acknowledged
persistence RPO of approximately one second; `always` is required where even that loss window is not
acceptable. These checks are shared with revert durability but are not coupled to the revert rollout state.

An old application binary cannot deliver an existing V2 outcome. Rolling it back while V2 state is
live is therefore availability-fail-closed, not data-loss-safe progress: V2 capacity remains held and
the Ledger keeps retrying until a V2 Tracer returns. The database invariants prevent that old binary
from expiring or deleting the hold, and the down migration prevents removing those invariants while
the backlog or its receipts exist.

> **NOTE — `:4021` is illustrative, not canonical.** `TRACER_GRPC_PORT`'s doc comment says *"e.g.
> :4021"* (`tracer/config.go:51`). There is no default value and no `EXPOSE 4021` anywhere. `:4021` is an
> example in a comment, not a configured or bound port.

**Both tracer listeners share one TLS posture.** `buildSeamTLSConfig` is called by both the REST and the
gRPC listener (`tracer/config.go:1202, 1241`), so the two transports cannot drift: in `mtls` both use
mTLS; in `mesh`/unset both listen plaintext (`grpc_server.go:74-76`, `http_server.go:75-93`).

### Rollout / fallback posture

**gRPC is the default and the production transport** (`config.go:1571` defaults empty →
`tracerTransportGRPC`); REST is **retained as a fallback**, selectable by setting
`TRACER_TRANSPORT=rest`. The tracer's REST surface on `:4020` is an **operations/configuration
surface** (rules, limits, validations — operator-facing, internal), which is why a single `mtls` posture
across the whole `:4020` listener is acceptable: there is no direct end-customer access to demote.

> **MIGRATION — wiring the tracer now defaults to gRPC.** A deploy that sets `TRACER_BASE_URL` without
> setting `TRACER_TRANSPORT` now speaks **gRPC**, not REST. Such a deploy must therefore (1) expose the
> tracer's gRPC seam by setting `TRACER_GRPC_PORT` on the tracer, and (2) under `TRACER_TLS_MODE=mtls`
> provision cert material on both ends. To keep the previous behavior, set `TRACER_TRANSPORT=rest`
> explicitly. **Soak pending:** gRPC+mTLS has not yet been exercised end-to-end in a live cluster — the
> default is set to the target transport ahead of that soak by deliberate decision (seam plan).

---

## Appendix — Operator env surface and a documented gap

The seam's env knobs are defined as Go struct tags and validated in code; this is the grounded source of
truth for their **existence and semantics**.

| Var | Side | Meaning | Evidence |
|---|---|---|---|
| `TRACER_BASE_URL` | ledger | opt-in switch for the whole integration; empty → disabled | `config.go:285-304, 1548-1554` |
| `TRACER_TRANSPORT` | ledger | `grpc`\|`rest`; empty → `grpc` | `config.go:294-297, 1569-1588` |
| `TRACER_API_KEY` | ledger | required `X-API-Key` credential when transport is REST | `config.go`, `client.go` |
| `TRACER_TIMEOUT_MS` | ledger | reserve RPC timeout | struct tag, `config.go:304-310` |
| `TRACER_TLS_MODE` | ledger | `mtls`\|`mesh`/empty | `config.go:298-303` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | ledger | client leaf material (mtls) | `tls_seam.go:67-77` |
| `TRACER_TLS_CA_FILE` | ledger | CA verifying the **tracer's** server leaf | `config.go:310`, `tls_seam.go:87-90` |
| `TRACER_GRPC_PORT` | tracer | gRPC seam listen addr; empty → off | `tracer/config.go:49-54, 1230-1232` |
| `TRACER_TLS_MODE` | tracer | `mtls`\|`mesh`/empty (mirrors ledger) | `tracer/tls_seam.go:48-59` |
| `TRACER_TLS_CERT_FILE` / `_KEY_FILE` | tracer | server leaf material (mtls) | `tracer/tls_seam.go:63-73` |
| `TRACER_TLS_CLIENT_CA_FILE` | tracer | CA verifying the **ledger's** client leaf | `tracer/config.go:68-72`, `tls_seam.go:83-86` |
| `TENANT_CAP_RETRY_AFTER_SECONDS` | tracer | 503 `Retry-After` on tenant-pool cap (default 5s) | `tracer/.env.example:283-292` |
