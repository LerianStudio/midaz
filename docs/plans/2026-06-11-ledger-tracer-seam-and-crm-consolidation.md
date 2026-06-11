# Ledger↔Tracer gRPC+mTLS Seam + CRM Consolidation Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Move the ledger→tracer reservation seam to gRPC secured by mutual TLS (behind a gRPC×REST transport toggle), unblock multi-tenant reservations via trusted tenant propagation, publish a canonical infra topology doc, fold the CRM package tree into the ledger, and sweep the general docs (README, llms, AGENTS, env) to match the new reality.

**Architecture:** Tracer and reporter are separately-sellable **products** (separate builds = product/license boundary) — embedding the tracer into the ledger is therefore off the table. The reserve call stays a synchronous network call (double-entry correctness requires limit enforcement *before* commit); latency is recovered by **deployment co-scheduling** (separate Deployments, `podAffinity`-preferred onto shared nodes), not by collapsing the seam. The seam runs over **gRPC** (greenfield: `buf` codegen) with a **REST fallback behind a toggle** (the `TracerReserver` interface already abstracts transport). **Service identity is mutual TLS** — the connection itself proves "this caller is the ledger", so there is **no static shared secret in app config**. The tenant is propagated as a trusted `X-Tenant-Id` (gRPC metadata / HTTP header), trusted *because* the mTLS peer is a verified service, consistent with the durable "tenant is the trust boundary" decision. CRM, a ledger feature (not a product), moves from `components/crm/` to `components/ledger/internal/crm/`.

**Tech Stack:** Go 1.26.x, gRPC + `buf` (greenfield), Go crypto/tls mutual TLS (app-loaded certs, with a `mesh` mode for environments where a service mesh terminates mTLS), Fiber v2 (REST fallback), lib-commons v5 (`tenant-manager/{core,middleware}`, per-tenant PG managers), MongoDB (CRM), PostgreSQL (tracer per-tenant pools), otelgrpc for tracing.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Single-tenant reserve works over gRPC+mTLS, toggle-selectable against the retained REST path; proto+codegen wired; `TracerReserver` transport-agnostic | 1.1, 1.2, 1.3 | Detailed |
| 2 | Multi-tenant reservations work end-to-end: trusted `X-Tenant-Id` propagation + tracer-side tenant resolution; boot-guard replaced by an mTLS-required assertion | 2.1, 2.2 | Epic-level |
| 3 | Canonical infra topology doc published (deployment matrix, co-scheduling, mTLS model, ports/toggle) — no Helm | 3.1 | Epic-level |
| 4 | CRM lives in `components/ledger/internal/crm/`; `components/crm/` deleted; build + routes green | 4.1, 4.2 | Epic-level |
| 5 | General docs reflect the new seam, transport, topology, CRM placement, and env surface (README, llms, AGENTS, env reference) | 5.1 | Epic-level |

> **Scope note (skill: scope check).** Phase 4 (CRM consolidation) and Phase 5 (general docs) are **independent** of the seam work (Phases 1–3) and of each other; they could be separate plans or parallel tracks. They are kept here per the requesting decision; execute in any order relative to 1–3. **Boundary with the in-flight OpenAPI doc-quality campaign:** Phase 5 covers README/llms/AGENTS/env and architecture prose only — it does NOT touch the OpenAPI annotation sweep, which is a separate ongoing effort.

---

## Seam Contract (shared across Phases 1–2 — both transports must agree)

**Identity = mutual TLS.** Both the gRPC and the REST listeners on the tracer require and verify a client certificate; the ledger client presents one and verifies the tracer's server certificate. There is **no `Authorization` header and no `X-API-Key`** on this seam — the verified mTLS peer *is* the credential.

**Tenant propagation (multi-tenant only):**
```
gRPC:  metadata key  "x-tenant-id" = <tenantID>
REST:  HTTP header   "X-Tenant-Id" = <tenantID>
```
- Value is `tmcore.GetTenantIDContext(ctx)` on the ledger request context (already resolved by the ledger's own ingress auth).
- The tracer accepts it **only on an mTLS-verified connection**; on an unverified connection the request never reaches tenant resolution.
- Single-tenant: omit the tenant key/header (tracer uses its static DB).

**Transport selection:** a ledger config value (`TRACER_TRANSPORT = grpc | rest`) picks the implementation behind the `TracerReserver` interface. Both are mTLS-secured. Initial default and rollout disposition are a checkpoint decision (see Epic 1.2).

**TLS mode:** a config value (`TRACER_TLS_MODE = mtls | mesh`) selects app-loaded certs (`mtls`: cert/key/CA paths required on both sides) vs. mesh-terminated mTLS (`mesh`: the app speaks plaintext to a local sidecar that does mTLS; the boot-guard trusts the operator's assertion). `mtls` is the portable default; `mesh` is for clusters running Istio/Linkerd.

---

## Phase 1 — gRPC+mTLS transport with REST fallback, single-tenant (DETAILED)

### Epic 1.1: Reservation proto contract + buf codegen toolchain

**Goal:** A versioned `.proto` mirroring the reserve/confirm/release contract, with `buf`-driven Go codegen wired into the build and CI, generating stubs both components import.
**Scope:** `proto/reservation/v1/`, `buf.yaml`/`buf.gen.yaml`, Makefile, generated `pkg/proto/reservation/v1/`
**Dependencies:** none
**Done when:** `make proto` regenerates deterministic stubs; CI fails if generated code is stale; the proto encodes exactly the fields of the REST `ReserveRequest`/`ReserveResult`.
**Status:** Pending

#### Task 1.1.1: Define `reservation.proto`

- [ ] Done

**Context:** The wire contract exists today as Go types in `components/ledger/internal/adapters/tracer/client.go`: `ReserveRequest` (`client.go:89-109`), `ReserveAccount` (`68-75`), `ReserveResult` (`115-119`). It maps to `POST /v1/reservations` (+ confirm/release by id and by transaction, `routes.go:354-364`). The repo has **no** gRPC today (no `.proto`, no `buf`; gRPC is transitive via otel only).

**Implementation vision:** Author `proto/reservation/v1/reservation.proto` (proto3). Define a `ReservationService` with RPCs `Reserve`, `ConfirmByTransaction`, `ReleaseByTransaction`, `ConfirmById`, `ReleaseById` mirroring the five REST routes. Messages mirror the Go types field-for-field, preserving semantics: `Reserve` request carries transactionId, requestId, amount (string — keep decimal-as-string, do NOT use double), currency, account{accountId}, optional segmentId/portfolioId/merchantId/transactionType, transactionTimestamp (RFC3339 string), longLived bool; `ReserveResult` carries transactionId, denied bool, reservationIds (repeated string/uuid). Set `option go_package = "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1;reservationv1";`. Confirm/release requests carry the id or transaction_id. Keep the proto a faithful subset so REST and gRPC stay interchangeable.

**Files:**
- Create: `proto/reservation/v1/reservation.proto`

**Verification:** `buf lint` passes (after 1.1.2 wires buf); proto compiles.

**Done when:** the proto fully expresses the reserve/confirm/release contract with decimal-safe types.

#### Task 1.1.2: Wire buf codegen + Makefile target + CI staleness check

- [ ] Done

**Context:** No protobuf toolchain exists. Lerian convention favors `buf`.

**Implementation vision:** Add `buf.yaml` (module/lint/breaking config) and `buf.gen.yaml` (Go + gRPC plugins → output under `pkg/proto/`). Add a `make proto` target (and a `make proto-check` that regenerates and fails on a dirty diff, wired into CI alongside existing lint). Pin plugin versions. Generate the stubs and commit them. Add `proto-check` to the CI workflow that runs `make lint`.

**Files:**
- Create: `buf.yaml`, `buf.gen.yaml`
- Create: `pkg/proto/reservation/v1/*.pb.go` (generated)
- Modify: `Makefile` (or `mk/*.mk` — locate where `lint`/`test` targets live), CI workflow under `.github/`

**Verification:** `make proto` is idempotent (second run = no diff); `make proto-check` passes clean and fails on a hand-edited stub.

**Done when:** codegen is reproducible and CI-guarded.

### Epic 1.2: gRPC server (tracer) + client (ledger) behind `TracerReserver`, with transport toggle

**Goal:** The tracer serves reservations over gRPC by delegating to the same use-case core the REST handler uses (no logic fork); the ledger has a gRPC client behind the existing `TracerReserver` interface; a config toggle selects gRPC vs REST.
**Scope:** tracer bootstrap + gRPC handler, `components/ledger/internal/adapters/tracer/`, `components/ledger/internal/bootstrap/`
**Dependencies:** Epic 1.1
**Done when:** with `TRACER_TRANSPORT=grpc`, reserve/confirm/release run over gRPC and produce identical results to REST; the reserve anchor (`transaction_reservation_anchor.go:115`) is unchanged (transport-agnostic).
**Status:** Pending

#### Task 1.2.1: gRPC server on the tracer, sharing the reservation use-case core

- [ ] Done

**Context:** The REST reservation handler (`reservationHandler.Reserve` etc., wired at `components/tracer/internal/adapters/http/in/routes.go:354-364`) delegates to a reservation use-case/service. The gRPC server must call that **same** use-case — no duplicated business logic.

**Implementation vision:** Add a gRPC `ReservationService` server implementation (new adapter package, e.g. `components/tracer/internal/adapters/grpc/in/`) that maps proto messages to the use-case's domain inputs and back, delegating to the identical service the REST handler uses (locate it from the REST handler's constructor). Stand up a `grpc.NewServer` in the tracer bootstrap on a dedicated port (`TRACER_GRPC_PORT`), registered as a lib-commons `RunApp` runnable so it drains on SIGTERM. Add otelgrpc server interceptor for tracing parity with REST. Do not touch the REST handler.

**Files:**
- Create: `components/tracer/internal/adapters/grpc/in/reservation_server.go`
- Modify: tracer bootstrap (locate the server/runnable wiring; search `RunApp` / Fiber listener in `components/tracer/internal/bootstrap/`)
- Test: `components/tracer/internal/adapters/grpc/in/reservation_server_test.go`

**Verification:** `go test ./components/tracer/internal/adapters/grpc/... -v` — server maps proto↔domain and returns the same decisions as the REST path for equivalent inputs (denied, reservationIds).

**Done when:** the gRPC server reserves/confirms/releases via the shared use-case.

#### Task 1.2.2: gRPC client on the ledger behind `TracerReserver` + transport toggle

- [ ] Done

**Context:** `TracerReserver` is the interface the reserve anchor depends on; today only the HTTP client (`components/ledger/internal/adapters/tracer/client.go`) implements it, wired by `buildTracerReserver` (`config.go:1529-1561`). The interface keeps the anchor transport-agnostic.

**Implementation vision:** Add a gRPC client implementing `TracerReserver` (new file alongside `client.go`, e.g. `grpc_client.go`), using the generated stubs + `grpc.NewClient` with a persistent connection, otelgrpc client interceptor, and error mapping to `ErrTracerUnavailable` (transport/unavailable → the fail-posture path; business denials come back as a normal result with `denied=true`). In `buildTracerReserver`, branch on `cfg.TracerTransport` (`grpc`|`rest`) to construct the gRPC or REST impl. **Initial default = `rest`** (the proven path; gRPC is opt-in during rollout) — flipping the default to `grpc` after the Phase-1 integration test + a soak is a one-constant change and a checkpoint decision. Keep the REST client as-is for now (mTLS added in Epic 1.3). Remove the now-superseded `M2MTokenProvider`/`WithM2MTokenProvider`/Bearer branch in `applyAuth` (`client.go:59-61,136-140,362-375`) — mTLS replaces token identity; retain only the header/metadata injection point for `X-Tenant-Id` (used in Phase 2).

**Files:**
- Create: `components/ledger/internal/adapters/tracer/grpc_client.go`
- Modify: `components/ledger/internal/adapters/tracer/client.go` (drop the dead M2M/Bearer seam), `components/ledger/internal/bootstrap/config.go` (`Config` tracer fields: add `TracerTransport`; `buildTracerReserver` `1529-1561`)
- Test: `components/ledger/internal/adapters/tracer/grpc_client_test.go`, bootstrap test for transport selection

**Verification:** `ALLOW_INSECURE_TLS=true go test ./components/ledger/internal/adapters/tracer/... ./components/ledger/internal/bootstrap/... -v` — `TRACER_TRANSPORT=grpc` builds the gRPC impl; `rest` builds the HTTP impl; unavailable upstream → `ErrTracerUnavailable`.

**Done when:** both transports satisfy `TracerReserver` and the toggle selects between them; dead M2M seam removed.

### Epic 1.3: Mutual TLS on both transports + boot-guard becomes an mTLS assertion

**Goal:** The tracer requires+verifies client certs and the ledger presents+verifies them, on both gRPC and REST; an unverified peer cannot reserve. `buildTracerReserver` refuses to wire an insecure client when the integration is on.
**Scope:** tracer gRPC + Fiber TLS config, ledger gRPC + HTTP client TLS config, both bootstraps
**Dependencies:** Epic 1.2
**Done when:** a reserve over either transport succeeds only with a valid client cert (or `TRACER_TLS_MODE=mesh`); a missing/invalid client cert is rejected at the TLS layer; an end-to-end single-tenant gRPC+mTLS integration test passes.
**Status:** Pending

#### Task 1.3.1: Tracer-side mTLS (server requires + verifies client certs)

- [ ] Done

**Context:** The tracer serves REST via Fiber and (after Epic 1.2) gRPC. Today the reservation seam has no transport security.

**Implementation vision:** Add a TLS config block to the tracer (`TRACER_TLS_MODE`, `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE`, `TRACER_TLS_CLIENT_CA_FILE`). In `mtls` mode: build a `*tls.Config` with the server cert/key, `ClientAuth: tls.RequireAndVerifyClientCert`, and `ClientCAs` = the loaded CA; apply it to the gRPC server (`grpc.Creds(credentials.NewTLS(...))`) and the Fiber listener (TLS listener). In `mesh` mode: listen plaintext (the sidecar terminates mTLS) and skip app TLS. Reservations specifically must not be reachable without mTLS in `mtls` mode.

**Files:**
- Modify: tracer bootstrap (TLS config + gRPC creds + Fiber TLS listener), tracer `Config`
- Test: tracer TLS integration test (valid cert passes, no/invalid cert rejected)

**Verification:** `go test ./components/tracer/... -run TestReservationMTLS -v` (or integration tag) — handshake succeeds with a CA-signed client cert, fails without.

**Done when:** the tracer enforces client-cert verification on the seam in `mtls` mode.

#### Task 1.3.2: Ledger-side mTLS (client presents cert + verifies server) + boot-guard rework

- [ ] Done

**Context:** `buildTracerReserver` (`config.go:1529-1561`) currently **hard-fails** under multi-tenancy with the integration on (`1545-1548`) because no auth existed. mTLS now provides identity; the guard's premise changes.

**Implementation vision:** Add ledger client TLS config (`TRACER_TLS_MODE`, `TRACER_TLS_CERT_FILE`, `TRACER_TLS_KEY_FILE`, `TRACER_TLS_CA_FILE`). Build a `*tls.Config` presenting the client cert and verifying the tracer's server cert against the CA; apply to the gRPC client (`credentials.NewTLS`) and the REST `http.Client` transport. Rework the guard: if `TRACER_BASE_URL`/endpoint set AND `TRACER_TLS_MODE=mtls` AND cert/key/CA paths are missing → **fail fast** ("reservation seam requires mTLS material"). If `mesh` → trust the operator (no cert check). **Delete** the `if cfg.MultiTenantEnabled { return nil,... }` block (`1545-1548`) — MT is no longer the discriminator; insecure transport is. This also closes the legacy single-tenant plaintext path: with the integration on, mTLS is required regardless of tenancy.

**Files:**
- Modify: `components/ledger/internal/adapters/tracer/grpc_client.go` + `client.go` (TLS on both), `components/ledger/internal/bootstrap/config.go` (`Config` TLS fields; `buildTracerReserver` `1529-1561`)
- Test: `components/ledger/internal/bootstrap/config_test.go` (guard cases)

**Verification:** `ALLOW_INSECURE_TLS=true go test ./components/ledger/internal/bootstrap/... -run TestBuildTracerReserver -v` — integration off → nil; on + `mtls` + missing certs → error; on + `mtls` + certs present → non-nil (both tenancy modes); on + `mesh` → non-nil without certs.

**Done when:** the ledger speaks mTLS on both transports and refuses to wire an insecure seam.

#### Task 1.3.3: End-to-end single-tenant gRPC+mTLS integration test

- [ ] Done

**Context:** No test exercises the secured transport end-to-end.

**Implementation vision:** Integration test (build tag `integration`, repo testcontainer pattern) standing up the tracer with gRPC+mTLS and the ledger client with a CA-signed cert; drive reserve→confirm over gRPC; assert success, then assert a client without a valid cert is rejected at handshake, and that `TRACER_TRANSPORT=rest` over mTLS also succeeds. Use fixed times (no `time.Now()` in tests); generate test certs via a fixture helper.

**Files:**
- Create: `components/ledger/internal/adapters/tracer/seam_mtls_integration_test.go`

**Verification:** `ALLOW_INSECURE_TLS=true go test -tags integration ./components/ledger/internal/adapters/tracer/... -run TestSeamMTLS -v` (Docker required)

**Done when:** the secured single-tenant seam passes over both transports; uncertified client is rejected.

---

## Phase 2 — Multi-tenant reservations (epic-level)

### Epic 2.1: Trusted tenant propagation + tracer-side resolution

**Goal:** The ledger sends `x-tenant-id` (gRPC metadata / HTTP header) from `tmcore.GetTenantIDContext(ctx)`; the tracer, on an mTLS-verified connection, resolves the per-tenant PG pool from it for the reservation path.
**Scope:** ledger client (metadata/header injection at the reserve call), tracer gRPC + REST tenant-resolution for the reservation surface
**Dependencies:** Phase 1
**Done when:** an MT reserve resolves the correct tenant pool from the trusted tenant key; a missing key under MT fails clean (4xx/INVALID_ARGUMENT), never a wrong/default pool; user routes on the tracer keep their JWT-claim tenant path (`routes.go:253-257`) unchanged. Tenant trust is conditioned on the mTLS peer being a verified service.
**Status:** Pending

### Epic 2.2: Multi-tenant end-to-end test + boot-guard final state

**Goal:** Prove tenant isolation over the secured seam and confirm the boot-guard's final behavior.
**Scope:** ledger↔tracer MT integration tests, bootstrap guard
**Dependencies:** Epic 2.1
**Done when:** an integration test shows tenant A's reservation state is invisible to tenant B over gRPC+mTLS; an MT ledger with the integration on + mTLS configured boots and reserves; the guard fails fast only on insecure transport, not on tenancy.
**Status:** Pending

---

## Phase 3 — Canonical infra topology doc (epic-level)

### Epic 3.1: Ledger↔tracer↔reporter deployment & seam reference for infra

**Goal:** A single canonical doc the infra team owns downstream — **no Helm charts** (that work is theirs).
**Scope:** `docs/architecture/ledger-tracer-reporter-topology.md` (new)
**Dependencies:** Phase 1 (transport + mTLS are real); reflects Phase 2 tenant model
**Done when:** the doc covers — (1) **product segregation matrix**: per-tier artifacts (core-banking = ledger+tracer+reporter; segregated subsets), each a separate image/license; (2) **co-scheduling**: separate Deployments + `podAffinity: preferred` onto shared nodes, explicitly **rejecting** sidecar/same-pod and why (heavier scale unit, pod-startup inheriting tracer warmup, per-tenant PG-pool fan-out = `ledger_replicas × tenants`); (3) **independent scaling**: tracer/reporter own HPA; tracer stays a small replica set; (4) **graceful absence**: ledger-only unsets the tracer endpoint, failPosture governs; (5) **mTLS model**: `TRACER_TLS_MODE=mtls|mesh`, cert material + rotation (cert-manager), the `mesh` option for Istio/Linkerd, and the trusted-`x-tenant-id` rationale; (6) **transport/ports**: gRPC port + REST port, the `TRACER_TRANSPORT` toggle, rollout/fallback guidance; (7) reporter as an async RabbitMQ consumer (broker-credential auth), off the hot path. Reviewed by ring:docs-reviewer.
**Status:** Pending

---

## Phase 4 — CRM consolidation into the ledger (independent, epic-level)

> CRM is a **feature of the ledger product**, not a separately-sold artifact, so it does not belong in `components/` (= "separately-buildable product"). It has no `cmd/`/`internal/` and is imported only by the ledger. ~35–40 files change (package moves + import rewrites); behavior must not change.

### Epic 4.1: Move the CRM package tree into `components/ledger/internal/crm/`

**Goal:** All CRM packages live under `components/ledger/internal/crm/` with rewritten import paths; repo builds; no behavior change.
**Scope:** `components/crm/{services,adapters/mongodb/{holder,instrument,dupkey}}` → `components/ledger/internal/crm/...`; import sites in ledger bootstrap + HTTP layer
**Dependencies:** none
**Done when:** `components/crm/` deleted; new path `github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/...`; `go build ./...` + `go vet ./...` green. Import sites to rewrite (verify exhaustively): `components/ledger/internal/bootstrap/config.mongo.crm.go:18-20`, `components/ledger/internal/adapters/http/in/holder.go:8`, `components/ledger/internal/adapters/http/in/instrument.go`, `components/ledger/internal/services/composition/ports.go`, plus intra-CRM imports. **Decision:** target `internal/crm` (Go internal visibility enforces ledger-only). `components/ledger/pkg/fee` uses a different convention; aligning fees is explicitly out of scope (kept a pure move).
**Status:** Pending

### Epic 4.2: Verify CRM wiring, routes, and MT module intact post-move

**Goal:** CRM holder/instrument routes and the per-tenant Mongo module behave exactly as before.
**Scope:** `config.go` (CRM DI `933`, unified server `984`), `crm_routes.go` (`18-20`, `34-54`), `config.mongo.crm.go` (`initCRM` `37-57`, MT `62-95`), CRM tests
**Dependencies:** Epic 4.1
**Done when:** the `midaz` authz namespace registration is unchanged, the `constant.ModuleCRM` tenant Mongo module still resolves per-tenant DBs, CRM unit+integration tests pass, holder/instrument endpoints respond. Placement only — no route paths or auth namespaces change.
**Status:** Pending

---

## Phase 5 — General documentation sweep (independent, epic-level)

### Epic 5.1: Update README / llms / AGENTS / env reference to the new reality

**Goal:** The repo's general docs reflect the gRPC+mTLS seam, the transport toggle, the deployment topology, the CRM relocation, and the new env surface.
**Scope:** root `README.md` (+ component READMEs touched), `llms.txt`/`llms-full.txt`, `AGENTS.md`, `CLAUDE.md` (the Key Files / component descriptions if they reference the seam or CRM path), env reference
**Dependencies:** Phases 1, 4 (the seam + CRM placement must be real); Phase 3 doc is cross-linked
**Done when:** docs describe — the ledger↔tracer seam as gRPC+mTLS with REST fallback (not "unauthenticated HTTP"); the new env vars (`TRACER_TRANSPORT`, `TRACER_GRPC_PORT`, `TRACER_TLS_MODE`, `TRACER_TLS_*`); the removal of the old MT boot-guard refusal; CRM's new path (`components/ledger/internal/crm/`) wherever the old `components/crm` path is documented; a cross-link to the Phase 3 topology doc. The OpenAPI annotation campaign is untouched. Reviewed by ring:docs-reviewer. **Note:** this is a doc-only phase; pair each claim with the code that makes it true (no aspirational docs).
**Status:** Pending

---

## Self-Review

- **Spec coverage:** gRPC+mTLS from the start → Phases 1–2; transport toggle → Epic 1.2; infra doc (no Helm) → Phase 3; CRM consolidation → Phase 4; general docs → Phase 5. All requested items covered; the static-API-key approach is fully removed in favor of mTLS.
- **Vagueness scan:** Phase 1 tasks name exact files/lines, the proto field semantics (decimal-as-string), the guard block to delete (`config.go:1545-1548`), per-mode TLS behavior (`mtls`|`mesh`), and the dead-seam removal (M2M/Bearer). Edge cases named: missing certs (fail-fast), `mesh` mode (trust operator), uncertified client (TLS-layer reject), transport default (`rest` initially, flip is a checkpoint call).
- **Contract consistency:** the Seam Contract (mTLS + `x-tenant-id`, no shared secret) is defined once and referenced by Phases 1–2; `TracerReserver` kept stable so the reserve anchor is transport-agnostic; proto mirrors the REST types field-for-field so the toggle is faithful.
- **Phase boundaries:** P1 ends with a working secured single-tenant seam over both transports; P2 adds MT; P3/P5 are standalone docs; P4 is a behavior-preserving move. No phase ends mid-refactor.
- **Verification plausibility:** commands use `ALLOW_INSECURE_TLS=true go test` + `-tags integration` + `make proto`/`make lint`, targeting real paths.
- **Open confirmations for checkpoint:** (1) reservations are service-only (so the REST fallback is transitional and may be removed post-rollout) vs. a public surface (REST stays permanent); (2) initial `TRACER_TRANSPORT` default (`rest` for safe rollout, per the plan) and when to flip to `grpc`.
