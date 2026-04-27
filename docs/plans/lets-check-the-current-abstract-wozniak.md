# Phases 3 and 4 — CRM/Ledger Abstraction Layer Implementation

## Context

The CRM/Ledger abstraction layer (see `docs/CRM_LEDGER_ABSTRACTION_LAYER.md`) makes account
creation a single business-atomic operation across two services: Ledger must guarantee that a
ledger account is not operational until its CRM holder alias exists and is consistent. Phases 1
and 2 of the plan are mostly shipped:

- Saga data model, Postgres migration (`000011`), and repository with `Upsert/Attach/MarkCompleted/MarkFailed`.
- Account eligibility gate (`VerifyAccountsTransactable`) that blocks transactions on `PENDING_CRM_LINK`,
  `FAILED_CRM_LINK`, and `blocked=true`.
- `CreateAccount` internal path that forces `PENDING_CRM_LINK` + `blocked=true` when `opts.PendingCRMLink` is set.
- `ActivateAccount` command that atomically flips status and blocked under `SELECT … FOR UPDATE`.
- Dormant CRM HTTP client adapter with the `CRMAccountRelationshipPort` interface and code-constant
  timeouts + circuit-breaker (every method returns `ErrCRMInternalRouteNotImplemented` today).
- `CRM_BASE_URL` config loaded by bootstrap; client instantiated but not consumed.

This plan covers Phases 3 and 4 only: hardening CRM so service-to-service orchestration is safe
(Phase 3), and wiring the public `POST /account-registrations` saga in Ledger that orchestrates
end-to-end account creation across both services (Phase 4).

Phases 5–7 (recovery worker, query APIs, hardening) are explicitly out of scope.

## Directional Decisions (confirmed this pass)

1. **CRM routing**: harden the existing `/v1/*` routes; Ledger calls them with a service JWT.
   No `/internal/v1/*` namespace is introduced. The port's doc comment
   (`components/ledger/internal/adapters/crm/http/account_relationship.port.go:10-12`) referencing
   `/internal/v1/*` must be updated to match.
2. **Close alias semantics**: add a new explicit `POST /v1/holders/:holder_id/aliases/:id/close`
   endpoint distinct from the existing `DELETE` soft-delete. Sets `banking_details.closing_date`
   and `deleted_at`, and is idempotent.
3. **Failure scope in Phase 4**: on CRM failure, mark `FAILED_RETRYABLE`/`FAILED_TERMINAL` and
   return. No inline compensation, no inline retry. The Ledger account stays blocked; Phase 5's
   recovery worker handles everything else.

---

## Phase 3 — CRM Orchestration Safety

### 3.1 Idempotency store (Mongo-backed)

CRM has zero idempotency support today and no Redis client wired up. A Mongo collection is the
natural fit.

**New files:**

- `components/crm/internal/adapters/mongodb/idempotency/idempotency.mongodb.go` — repository
  storing `{tenant_id, idempotency_key, request_hash, response_document, created_at, expires_at}`.
  Unique index on `(tenant_id, idempotency_key)`. TTL index on `expires_at` (24h default).
- `components/crm/internal/services/command/idempotency_guard.go` — thin orchestration helper used
  by `CreateAlias` and `CloseAlias` command services: takes `(ctx, idempotencyKey, requestHash, fn)`,
  returns cached response on key match, errors with `ErrIdempotencyKey` on hash mismatch, invokes
  `fn` and stores the result otherwise.

**Key constant:** reuse the header name `libConstants.IdempotencyKey` already used by Ledger.
Do not invent a CRM-specific header.

**Request hash canonicalization:** reuse the helper created in §4.2 below (place it in
`pkg/utils/canonical_hash.go` so both Ledger and CRM import from `pkg/`).

### 3.2 Harden `POST /v1/holders/:holder_id/aliases` for idempotency

**Files to touch:**

- `components/crm/internal/adapters/http/in/alias.go` (`AliasHandler.CreateAlias`) — read
  `Idempotency-Key` header, compute canonical hash over the body, pass both into the command
  service.
- `components/crm/internal/services/command/create-alias.go` — wrap the existing create logic in
  the idempotency guard from §3.1. Return the cached alias on key replay with matching hash;
  return `ErrIdempotencyKey` on hash mismatch.
- `components/crm/internal/adapters/http/in/routes.go:58` — no route change, but confirm the
  middleware chain exposes the body to the handler for hashing. Route stays JWT-protected.

**Behavior required by the spec:**

| Scenario | Result |
|---|---|
| same key + same hash | return stored alias, HTTP 200 |
| same key + different hash | `ErrIdempotencyKey` → 409 |
| same `(ledger_id, account_id)` + same holder + same payload, no key | return existing alias, HTTP 200 (via §3.5) |
| same `(ledger_id, account_id)` + different holder | `ErrAliasHolderConflict` (code `0170` already defined, unused today) → 409 |
| same `(ledger_id, account_id)` + same holder + different payload | `ErrIdempotencyKey` → 409 |

### 3.3 New: lookup by `(ledger_id, account_id)` query

**Repository (`components/crm/internal/adapters/mongodb/alias/alias_query.mongodb.go`):** add
`FindByLedgerAndAccount(ctx, ledgerID, accountID string) (*mmodel.Alias, error)`. Returns
`ErrAliasNotFound` if none exists. Use the existing `buildAliasFilter` helper with a single-result
projection.

**Route:** add `GET /v1/aliases/by-account?ledger_id=&account_id=` in
`components/crm/internal/adapters/http/in/routes.go` (currently only `GET /v1/aliases` exists
at line 57). Register a new query-side handler in `alias.go`. Returns 200 with the alias, 404
with `ErrAliasNotFound` otherwise.

This is what the Ledger port's `GetAliasByAccount` will call.

### 3.4 New: explicit alias close endpoint

**Route:** `POST /v1/holders/:holder_id/aliases/:id/close`. Register in `routes.go` between the
existing DELETE (line 61) and DeleteRelatedParty (line 62).

**Handler:** `AliasHandler.CloseAlias` in `components/crm/internal/adapters/http/in/alias.go`.
Accepts an empty body and an `Idempotency-Key` header. Returns 200 with the closed alias.

**Command service:** new file `components/crm/internal/services/command/close-alias.go`:
1. Wrap in idempotency guard (§3.1).
2. Load alias; return `ErrAliasNotFound` if missing.
3. Return 200 no-op if `alias.banking_details.closing_date` is already set (natural idempotency).
4. Set `banking_details.closing_date = now().UTC()` and `deleted_at = now().UTC()` in a single
   `UpdateOne`.
5. Emit telemetry span and audit log.

**Why not reuse DELETE:** the close-account flow in the design doc (§"Close Account Relationship
Flow") explicitly relies on a `closing_date` field for downstream banking-integration semantics.
Soft-delete alone does not carry that signal. Keeping the two endpoints distinct preserves the
Phase 7 close-account orchestration contract.

### 3.5 Semantic holder-mismatch detection on create

Current CRM relies on the unique Mongo index on `(ledger_id, account_id) WHERE deleted_at IS NULL`
to collide. That collision surfaces as `ErrAccountAlreadyAssociated` (CRM-0013) with no
information about *which holder* owns the conflicting alias.

**Change in `components/crm/internal/adapters/mongodb/alias/alias.mongodb.go:88` (Create):**
before insert, probe `FindByLedgerAndAccount` (§3.3). If a row exists and
`existing.holder_id != input.holder_id`, return `ErrAliasHolderConflict` (code `0170`, already
defined in `pkg/constant/errors.go` but unused). If `existing.holder_id == input.holder_id` and
the payload hashes match, return the existing alias (idempotent). Otherwise proceed with insert
and let the unique index catch the race.

This closes the spec's duplicate-semantics table without a schema change.

### 3.6 Tenant enforcement on the Ledger-originated path

CRM multi-tenant middleware already extracts tenant from JWT (`config.tenant.go:79`) and routes
Mongo queries accordingly. Ledger will call CRM with its service JWT, which must carry the same
tenant claim the end-user request had.

**No new middleware.** What Phase 3 must add:

- A bootstrap assertion in Ledger (`components/ledger/internal/bootstrap/config.go`) that when
  `MULTI_TENANT_ENABLED=true`, the CRM client is configured with a service auth mode that
  forwards the tenant claim. This becomes a real requirement when §4.1 wires the client; for
  Phase 3 we document the contract and add a `CRM_SERVICE_JWT_ISSUER` placeholder in the config
  struct (no-op if empty, error if multi-tenant is on and it is empty).
- A test in CRM confirming that requests without a tenant claim are rejected by the existing
  middleware when multi-tenant is enabled. If this test already passes, no change is needed.

---

## Phase 4 — Create Account Registration API

### 4.1 Replace CRM client stubs with real HTTP calls

**File:** `components/ledger/internal/adapters/crm/http/account_relationship.client.go`
(currently lines 57–258, all four methods return `ErrCRMInternalRouteNotImplemented`).

Each method:
- Constructs the URL against `baseURL` using the hardened `/v1/*` paths from Phase 3
  (`/v1/holders/:id`, `POST /v1/holders/:holder_id/aliases`, `GET /v1/aliases/by-account`,
  `POST /v1/holders/:holder_id/aliases/:id/close`).
- Attaches `Idempotency-Key` header on mutating calls (from method argument).
- Attaches `Authorization: Bearer <service JWT>` (see §4.6 for how the token is obtained).
- Attaches the tenant header (or forwards the inbound tenant claim via the service JWT).
- Runs inside the existing per-operation `context.WithTimeout` (lines 34–38) and circuit breaker
  (lines 40–45) — those are already wired; just remove the stub and keep the surrounding structure.

**HTTP → error mapping:**

| HTTP | Error sentinel | Classification |
|---|---|---|
| 200 / 201 | nil | success |
| 404 on GetHolder | `ErrHolderNotFound` | terminal |
| 404 on GetAliasByAccount | return `nil, ErrAliasNotFound` | saga-specific (maps to "no prior attempt") |
| 409 from CRM | `ErrCRMConflict` → map by CRM error code to `ErrIdempotencyKey` / `ErrAliasHolderConflict` | terminal |
| 4xx other | `ErrCRMBadRequest` | terminal |
| 5xx / timeout / CB-open / connection-refused | `ErrCRMTransient` | retryable |

Add the new sentinels (`ErrCRMConflict`, `ErrCRMBadRequest`) to `pkg/constant/errors.go`
alongside the existing `ErrCRMTransient` / `ErrCRMInternalRouteNotImplemented`. One sentinel per
code, per project rule.

**Delete** `ErrCRMInternalRouteNotImplemented` from the sentinel list once all four methods are
implemented — it becomes dead code.

**Update** the package doc in `account_relationship.port.go:10-12` to say `/v1/*` and remove the
"every method returns not-implemented" note.

### 4.2 Canonical request hash helper

**New file:** `pkg/utils/canonical_hash.go`. Exports:

```go
// CanonicalHashJSON returns the lowercase hex SHA-256 of the canonical JSON form of v.
// Canonical form: typed model → default application → json.Marshal with sorted map keys →
// SHA-256. Used by both the CRM idempotency guard (Phase 3) and the account-registration
// saga (Phase 4) so equivalent payloads collide on the same hash.
func CanonicalHashJSON(v any) (string, error)
```

Test cases required (per the design doc §Idempotency):

1. Same body, different key order → same hash.
2. Same body, whitespace differences in source → same hash.
3. Nil vs empty string distinction is preserved.
4. Different business values → different hashes.

### 4.3 Saga command service — `CreateAccountRegistration`

**New file:** `components/ledger/internal/services/command/create_account_registration.go`.

**Signature:**

```go
func (uc *UseCase) CreateAccountRegistration(
    ctx context.Context,
    organizationID, ledgerID uuid.UUID,
    input *mmodel.CreateAccountRegistrationInput,
    idempotencyKey string,
    token string,
) (*mmodel.AccountRegistration, *mmodel.Account, *mmodel.Alias, error)
```

**Steps** (each step persists status before the next runs, per the "durable checkpoint" discipline):

1. Validate input; compute `requestHash = CanonicalHashJSON(input)`.
2. Call `AccountRegistrationRepo.UpsertByIdempotencyKey(ctx, reg)` with `Status=RECEIVED`. If an
   existing row has a *different* hash → return `ErrIdempotencyKey`. If an existing row has the
   same hash and `Status=COMPLETED` → load and return the stored result (replay).
3. Call `crmClient.GetHolder(ctx, orgID.String(), input.HolderID)`. On `ErrHolderNotFound` →
   `repo.MarkFailed(FAILED_TERMINAL, "HOLDER_NOT_FOUND", …)`. On `ErrCRMTransient` →
   `repo.MarkFailed(FAILED_RETRYABLE, "CRM_TRANSIENT", …)` with `next_retry_at = now+5s`. Return.
4. `repo.UpdateStatus(HOLDER_VALIDATED)`.
5. Call `CreateAccount` with `accountCreateOptions{PendingCRMLink: true}` — **reuse the existing
   internal entry point**, do not call the public `CreateAccount` command. The existing code at
   `components/ledger/internal/services/command/create_account.go:42` and its internal
   `createAccountWithOptions` path already honor the flag; confirm the function is exported
   within the `command` package or extract a helper. Returns `account` with
   `Status=PENDING_CRM_LINK`, `Blocked=true`, `balance.AllowSending/Receiving=false`.
6. `repo.AttachAccount(id, account.ID)` then `repo.UpdateStatus(LEDGER_ACCOUNT_CREATED)`.
7. Build `CreateAliasInput` from `input.CRMAlias` and known `(ledger_id, account_id)`. Derive the
   per-operation idempotency key: `fmt.Sprintf("account-registration:%s:%s:%s:crm-create-alias",
   orgID, ledgerID, idempotencyKey)`. Call `crmClient.CreateAccountAlias(…)`. Terminal-error path
   → `FAILED_TERMINAL`. Transient-error path → `FAILED_RETRYABLE`. Account stays blocked.
8. `repo.AttachCRMAlias(id, alias.ID)` then `repo.UpdateStatus(CRM_ALIAS_CREATED)`.
9. Call `ActivateAccount(ctx, orgID, ledgerID, account.ID)` — reuses
   `components/ledger/internal/services/command/activate_account.go:39-81`. On failure →
   `FAILED_RETRYABLE` with `next_retry_at=now+5s`. Account stays blocked (by definition of the
   activation failure).
10. `repo.UpdateStatus(ACCOUNT_ACTIVATED)`.
11. `repo.MarkCompleted(id, now.UTC())`. Return `(registration, account, alias, nil)`.

Note: between step 10 and step 11, a crash leaves the record in `ACCOUNT_ACTIVATED` with the
account already fully usable — the recovery worker in Phase 5 only needs to verify and flip to
`COMPLETED`. This is the "durable checkpoint" discipline the design doc calls out.

**Telemetry:** wrap each step in a child span via `libCommons.NewTrackingFromContext(ctx)`
following the pattern in `create_account.go:49-52`. Emit the metrics listed in the design doc
(`account_registration_started_total`, `account_registration_completed_total`,
`account_registration_failed_total`).

### 4.4 `GetAccountRegistration` query service

**New file:** `components/ledger/internal/services/query/get_account_registration.go`. Thin
wrapper over `AccountRegistrationRepo.FindByID`. Used by the GET endpoint.

### 4.5 HTTP layer

**New input type:** `pkg/mmodel/account_registration_input.go`:

```go
type CreateAccountRegistrationInput struct {
    HolderID  uuid.UUID                `json:"holderId" validate:"required"`
    Account   CreateAccountInput       `json:"account" validate:"required"`
    CRMAlias  CreateAliasInput         `json:"crmAlias" validate:"required"`
}
```

Reuse the existing `CreateAccountInput` (`pkg/mmodel/account.go`) and `CreateAliasInput`
(`pkg/mmodel/alias.go`) — do not duplicate fields.

**New handler file:** `components/ledger/internal/adapters/http/in/account_registration.go`.
Follows the pattern at `components/ledger/internal/adapters/http/in/account.go:24-98`:

```go
type AccountRegistrationHandler struct {
    Command *command.UseCase
    Query   *query.UseCase
}

func (h *AccountRegistrationHandler) CreateAccountRegistration(i any, c *fiber.Ctx) error
func (h *AccountRegistrationHandler) GetAccountRegistration(c *fiber.Ctx) error
```

- `CreateAccountRegistration`: extract `organization_id`, `ledger_id` from `c.Locals`; read the
  `Idempotency-Key` header via `http.GetIdempotencyKeyAndTTL(c)` at
  `pkg/net/http/httputils.go:398-412` (enforce presence — return
  `constant.ErrIdempotencyKeyRequired` if missing); call the command; return `http.Created(c, result)`.
- `GetAccountRegistration`: extract IDs, call the query, return `http.OK(c, registration)` or
  404.

**Route registration** in
`components/ledger/internal/adapters/http/in/routes.go:99-161` inside `RegisterOnboardingRoutesToApp`,
after the existing account routes (line 146):

```go
f.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/account-registrations",
    protectedMidaz(auth, "account-registrations", "post", routeOptions,
        http.ParseUUIDPathParameters("account-registration"),
        http.WithBody(new(mmodel.CreateAccountRegistrationInput), arh.CreateAccountRegistration))...)

f.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/account-registrations/:id",
    protectedMidaz(auth, "account-registrations", "get", routeOptions,
        http.ParseUUIDPathParameters("account-registration"),
        arh.GetAccountRegistration)...)
```

The `retry` and `cancel` endpoints from the design doc are Phase 5 — omit here.

### 4.6 Bootstrap wiring

**File:** `components/ledger/internal/bootstrap/config.go`.

- Add `arh := &in.AccountRegistrationHandler{Command: commandUC, Query: queryUC}` next to
  the existing handler instantiations (roughly line ~500s where handlers are built).
- Pass `arh` into `RegisterOnboardingRoutesToApp`.
- Inject the existing `crmClient` (already instantiated for Phase 1 but blank-assigned) into
  `commandUC` so the saga can call it. Add a `CRMClient crmhttp.CRMAccountRelationshipPort`
  field to `command.UseCase` if it doesn't exist yet.
- Service auth for Ledger→CRM: pass the existing Casdoor JWK machinery through to the client.
  If a service-account JWT issuer is needed, add `CRM_SERVICE_JWT_ISSUER` env var and load it
  via the standard `SetConfigFromEnvVars` pattern. Error out at startup when
  `MULTI_TENANT_ENABLED=true` and the issuer is empty (the tenant claim must be verifiable on
  the CRM side).

**Config additions** (env-tagged fields on the `Config` struct):

```go
CRMServiceJWTIssuer string `env:"CRM_SERVICE_JWT_ISSUER"`
```

Timeouts and circuit-breaker values are already code constants in `account_relationship.client.go`
— the design doc explicitly says they become config only after ops proves a need. Keep them as-is.

---

## Cross-cutting considerations

- **No duplicate error sentinels.** All new error codes defined once in
  `pkg/constant/errors.go`. Delete `ErrCRMInternalRouteNotImplemented` once unused.
- **No `reflect.TypeOf(mmodel.Foo{}).Name()`.** Use `constant.EntityAccountRegistration` etc.
  (add to `pkg/constant/entity.go` if missing).
- **Structured logging.** No `fmt.Sprintf` inside `logger.Log` calls. Use `libLog.Err`,
  `libLog.String`. Warn on business failures (holder not found, idempotency mismatch); Error
  only on infrastructure failures (Postgres unreachable, CRM transport broken).
- **UTC timestamps.** `time.Now().UTC()` captured once per insert/update per rule 16. This
  is enforced by existing repo code — just propagate.
- **Span discipline.** One span per durable saga step. Child spans for I/O only. Do not shadow
  `ctx` with child spans (use `_, span := tracer.Start(...)`).
- **Multi-tenant DB resolution is already handled** by the middleware → `tmcore.GetPGContext(ctx)`
  path inside the repo layer. The new saga code does not need to thread tenant IDs explicitly.

## Critical files (reference map)

| Purpose | File |
|---|---|
| Saga state machine | `pkg/mmodel/account_registration.go:13-64` |
| Saga Postgres repo | `components/ledger/internal/adapters/postgres/accountregistration/account_registration.postgresql.go:66-102` |
| Saga migration | `components/ledger/migrations/onboarding/000011_create_account_registration_table.up.sql` |
| CRM port (to update doc + implement) | `components/ledger/internal/adapters/crm/http/account_relationship.port.go:29-64` |
| CRM client (to implement) | `components/ledger/internal/adapters/crm/http/account_relationship.client.go:57-258` |
| CreateAccount w/ PENDING_CRM_LINK | `components/ledger/internal/services/command/create_account.go:40` |
| ActivateAccount | `components/ledger/internal/services/command/activate_account.go:39-81` |
| Transaction eligibility gate | `components/ledger/internal/services/query/verify_accounts_transactable.go:39-80` |
| Ledger route registration | `components/ledger/internal/adapters/http/in/routes.go:99-161` |
| Ledger bootstrap | `components/ledger/internal/bootstrap/config.go:156-157` (CRM_BASE_URL) |
| Idempotency-Key helper | `pkg/net/http/httputils.go:398-412` |
| Redis idempotency pattern (for inspiration) | `components/ledger/internal/services/command/create_transaction_idempotency.go:35-98` |
| CRM alias Mongo repo | `components/crm/internal/adapters/mongodb/alias/alias.mongodb.go:88` (Create), `:167` (Find) |
| CRM alias query | `components/crm/internal/adapters/mongodb/alias/alias_query.mongodb.go:22` |
| CRM alias routes | `components/crm/internal/adapters/http/in/routes.go:57-62` |
| CRM alias model | `pkg/mmodel/alias.go:75-93` |
| CRM holder-conflict error code (unused, to activate) | `pkg/constant/errors.go` → `ErrAliasHolderConflict` (0170) |
| CRM handler pattern to mimic | `components/ledger/internal/adapters/http/in/account.go:24-98` |
| Command service pattern | `components/ledger/internal/services/command/create_account.go:40-150` |
| Postgres repo pattern (squirrel, getDB) | `components/ledger/internal/adapters/postgres/account/account.postgresql.go:104-197` |

---

## Verification

### Unit tests (ring:dev-cycle Gate 3)

- **CRM idempotency guard:** same key + same hash → cache hit; same key + different hash →
  409 error; new key → passthrough.
- **CRM create-alias holder conflict:** existing alias with different holder → `ErrAliasHolderConflict`.
- **CRM close-alias idempotency:** second close is a no-op returning 200.
- **Canonical hash helper:** the four cases listed in §4.2.
- **Saga state transitions:** each step persists status before the next is attempted; mocked
  CRM client returns terminal/transient errors and the saga records the expected failure state.
- **Saga replay:** `UpsertByIdempotencyKey` returning a `COMPLETED` record short-circuits and
  returns the stored result.
- **Saga hash mismatch:** `UpsertByIdempotencyKey` returning an existing row with different hash
  surfaces `ErrIdempotencyKey`.

### Integration tests (ring:dev-cycle Gate 6)

- End-to-end happy path: `POST /account-registrations` creates a Ledger account
  (`PENDING_CRM_LINK`, blocked), a CRM alias, activates the account (`ACTIVE`, unblocked), and
  returns `COMPLETED`.
- **Eligibility gate assertion:** while status is `PENDING_CRM_LINK`, a `POST /transactions/json`
  targeting that account returns `ErrAccountStatusTransactionRestriction` (verified via the
  existing `VerifyAccountsTransactable` path).
- Replay: same request with same `Idempotency-Key` returns the same result without duplicating
  side effects.
- Conflict: same key with different body returns 409.
- CRM 404 on holder: saga terminates `FAILED_TERMINAL`, no Ledger account created.
- CRM 5xx on alias creation: saga ends `FAILED_RETRYABLE` with Ledger account in
  `PENDING_CRM_LINK` + blocked=true, `next_retry_at` populated.
- CRM conflict on alias (different holder owns the account): saga ends `FAILED_TERMINAL`.

### Manual verification

- `make up` → hit `POST /v1/organizations/:org/ledgers/:lg/account-registrations` with a valid
  holder → confirm 201, account shows `ACTIVE`/unblocked, CRM alias exists, registration record
  is `COMPLETED`.
- Kill CRM container mid-flow → confirm saga ends `FAILED_RETRYABLE`, account stays blocked,
  subsequent `POST /transactions/json` on that account fails with the eligibility gate error.
- Rerun `make test-integration PKG=./components/ledger/...` and ensure the existing
  `VerifyAccountsTransactable` tests still pass (defense-in-depth check).

### Lint / review gates

- `make lint` passes with the new depguard and observability rules from the 2026-03 uplift.
- `ring:codereview` on the final branch for business-logic, security, consequences, multi-tenant,
  nil-safety, performance reviewers.

---

## Out of scope (explicit)

- **Phase 5**: recovery worker, retry/cancel endpoints, compensation loop, `FOR UPDATE SKIP
  LOCKED` claim.
- **Phase 6**: `GET /accounts/:id/crm-profile`, `GET /holders/:id/accounts`.
- **Phase 7**: contract tests, RabbitMQ event emission, dashboards, runbooks.
- **Phase 3-only** close-account orchestration (§"Close Account Relationship Flow") — only the
  CRM-side close primitive is added; the Ledger-side orchestrator lands in Phase 7.
- **Schema evolution beyond migration 000011**: no new Postgres migration is needed for Phase 4
  — the existing table already has every required column.
