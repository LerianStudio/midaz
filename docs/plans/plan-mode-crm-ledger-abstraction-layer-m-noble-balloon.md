# CRM/Ledger Abstraction Layer — Phase 1 & 2 Implementation Plan

## Context

Today, a "customer opens an account" business operation spans two services: Ledger creates the financial account; CRM creates the holder-to-account alias. No coordinator exists — if one side succeeds and the other fails, the system is left in an inconsistent state (orphaned account, dangling alias, or a Ledger account that is transactable before CRM confirms the holder relationship).

The [CRM_LEDGER_ABSTRACTION_LAYER.md](../../CRM_LEDGER_ABSTRACTION_LAYER.md) design document defines the full remediation: a Ledger-owned saga orchestrator that guarantees **business-level atomicity** (an account must not transact unless its CRM alias exists and is consistent), with durable state, recovery, and compensation.

This plan covers **Phase 1 (Contracts + Data Model)** and **Phase 2 (Ledger Pending Account Support)** combined, because the two are deeply coupled: the orchestration state table only makes sense alongside the `PENDING_CRM_LINK` account state it manages. Phases 3-7 (CRM hardening, public orchestration API, recovery worker, queries, hardening) will be separate plans.

**Exit state after this plan**:
- Ledger can create accounts in a non-operational `PENDING_CRM_LINK + blocked=true` state.
- An `account_registration` table exists in the Ledger transaction DB to durably record orchestration attempts.
- A `CRMAccountRelationshipPort` interface and an HTTP-based adapter skeleton exist, compiling but not yet wired into an end-user API.
- Canonical request hashing utility exists with tests.
- Activation path updates `status` and `blocked` atomically in one PG transaction.
- Transaction eligibility checks reject `PENDING_CRM_LINK` and any `blocked=true` account.

## Design Decisions (Locked)

| Decision | Choice | Reason |
|---|---|---|
| Tenant propagation | M2M JWT with `tenant_id` claim | Reuses existing Casdoor + lib-auth v2 stack; cryptographically signed; no new infra. |
| CRM routes | New `/internal/v1/*` routes (Phase 3) | Isolates orchestration contract from public API. This plan defines the client; CRM-side routes land in Phase 3. |
| Orchestration state store | Ledger `transaction` Postgres database | Already has the schema migrator and worker host; keeps Ledger-owned state colocated with transaction authority. |
| Orchestration claiming | `FOR UPDATE SKIP LOCKED` | Standard Postgres saga idiom; new to this codebase (BalanceSyncWorker uses Redis Lua), but the right tool for durable row-level state. Worker itself ships in Phase 5. |
| REST vs gRPC for CRM client | REST / HTTP+JSON | Matches existing `tmclient` pattern; no gRPC infrastructure in the repo today. Revisit in Phase 7 if latency warrants. |

## Architecture Overview

```
Ledger (components/ledger)
├── adapters/
│   ├── http/in/                      [Phase 4 — out of scope]
│   ├── postgres/accountregistration/ ← NEW (Phase 1)
│   │     account_registration.go           (repo interface + Postgres impl)
│   │     account_registration.sql.go       (squirrel builders)
│   └── crm/http/                     ← NEW (Phase 1)
│         account_relationship.port.go      (CRMAccountRelationshipPort interface)
│         account_relationship.client.go    (HTTP client skeleton, no auth)
│         [m2m_auth.go deferred to Phase 4]
├── services/
│   └── command/
│         create_account.go           ← MODIFIED (Phase 2 — honor pending state)
│         activate_account.go         ← NEW (Phase 2 — atomic status+blocked)
├── migrations/transaction/
│         NNNNNN_create_account_registration_table.{up,down}.sql  ← NEW (Phase 1)
└── bootstrap/
         config.go                    ← MODIFIED (single CRM_BASE_URL env var + DI wiring)

pkg/
├── mmodel/
│     account_registration.go         ← NEW (domain model)
│     account.go                      ← MODIFIED (new status constants)
├── constant/
│     errors.go                       ← MODIFIED (new error sentinels)
│     account_status.go               ← MODIFIED or NEW (PENDING_CRM_LINK, FAILED_CRM_LINK)
└── canonicaljson/                    ← NEW
      canonical.go                    (canonical JSON + SHA-256 request hash)
      canonical_test.go
```

**Flow (Phase 4 will call this; not yet wired)**:

```
                   [Phase 4 orchestrator — NOT in this plan]
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│ 1. Hash canonical request body (pkg/canonicaljson)      │
│ 2. Upsert account_registration by idempotency key       │
│ 3. Call CRMClient.GetHolder(ctx, orgID, holderID)       │
│ 4. CreateAccount(..., pending=true) → blocked, PENDING  │
│ 5. CRMClient.CreateAccountAlias(...)                    │
│ 6. ActivateAccount(accountID) → atomic status+blocked   │
│ 7. Mark registration COMPLETED                          │
└─────────────────────────────────────────────────────────┘
```

## Phase 1: Contracts and Data Model

### 1.1 Domain Model — `pkg/mmodel/account_registration.go` (NEW)

Define the orchestration entity. Mirror the column list in the design doc. Use `uuid.UUID` types (per CLAUDE.md rule 9). Statuses as string constants defined here.

```go
type AccountRegistrationStatus string

const (
    AccountRegistrationReceived             AccountRegistrationStatus = "RECEIVED"
    AccountRegistrationHolderValidated      AccountRegistrationStatus = "HOLDER_VALIDATED"
    AccountRegistrationLedgerAccountCreated AccountRegistrationStatus = "LEDGER_ACCOUNT_CREATED"
    AccountRegistrationCRMAliasCreated      AccountRegistrationStatus = "CRM_ALIAS_CREATED"
    AccountRegistrationAccountActivated     AccountRegistrationStatus = "ACCOUNT_ACTIVATED"
    AccountRegistrationCompleted            AccountRegistrationStatus = "COMPLETED"
    AccountRegistrationCompensating         AccountRegistrationStatus = "COMPENSATING"
    AccountRegistrationCompensated          AccountRegistrationStatus = "COMPENSATED"
    AccountRegistrationFailedRetryable      AccountRegistrationStatus = "FAILED_RETRYABLE"
    AccountRegistrationFailedTerminal       AccountRegistrationStatus = "FAILED_TERMINAL"
)

type AccountRegistration struct {
    ID                uuid.UUID
    OrganizationID    uuid.UUID
    LedgerID          uuid.UUID
    HolderID          uuid.UUID
    IdempotencyKey    string
    RequestHash       string        // lowercase hex SHA-256
    AccountID         *uuid.UUID
    CRMAliasID        *uuid.UUID
    Status            AccountRegistrationStatus
    FailureCode       *string
    FailureMessage    *string
    RetryCount        int
    NextRetryAt       *time.Time
    ClaimedBy         *string
    ClaimedAt         *time.Time
    LastRecoveredAt   *time.Time
    CreatedAt         time.Time
    UpdatedAt         time.Time
    CompletedAt       *time.Time
}
```

**Follow CLAUDE.md rule 17** (declaration order): exported types at top, no business logic (mmodel is a data package).

### 1.2 Migration — `components/ledger/migrations/transaction/NNNNNN_create_account_registration_table.{up,down}.sql` (NEW)

Use `make migrate-create COMPONENT=transaction NAME=create_account_registration_table`. Verify the next sequence number against existing files in `components/ledger/migrations/transaction/`.

**Columns** (from design doc line 272-294): `id`, `organization_id`, `ledger_id`, `holder_id`, `idempotency_key`, `request_hash`, `account_id`, `crm_alias_id`, `status`, `failure_code`, `failure_message`, `created_at`, `updated_at`, `completed_at`, `retry_count`, `next_retry_at`, `claimed_by`, `claimed_at`, `last_recovered_at`.

**Indexes** (from design doc line 296-305):
```sql
UNIQUE (organization_id, ledger_id, idempotency_key)
INDEX  (organization_id, ledger_id, status)
INDEX  (status, next_retry_at)  -- used by Phase 5 recovery worker
INDEX  (claimed_at)
INDEX  (account_id)
INDEX  (holder_id)
```

**CHECK constraint** on `status` restricting to the ten defined values.

Down migration: `DROP TABLE account_registration`.

Run `make migrate-lint` after creation.

### 1.3 Repository — `components/ledger/internal/adapters/postgres/accountregistration/` (NEW)

Two files, following the pattern in `components/ledger/internal/adapters/postgres/account/`:

**`account_registration.go`** — interface + Postgres impl. Per CLAUDE.md rule 17, interface first.

```go
type Repository interface {
    // UpsertByIdempotencyKey is the saga entry point.
    // Returns (registration, wasCreated).
    // If an existing row exists with the SAME hash: returns it (for replay).
    // If an existing row exists with a DIFFERENT hash: returns ErrIdempotencyKeyConflict.
    UpsertByIdempotencyKey(ctx context.Context, reg *mmodel.AccountRegistration) (*mmodel.AccountRegistration, bool, error)

    FindByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountRegistration, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, mutators ...StatusMutator) error
    AttachAccount(ctx context.Context, id, accountID uuid.UUID) error
    AttachCRMAlias(ctx context.Context, id, aliasID uuid.UUID) error
    MarkCompleted(ctx context.Context, id uuid.UUID, completedAt time.Time) error
    MarkFailed(ctx context.Context, id uuid.UUID, status mmodel.AccountRegistrationStatus, code, message string) error
}
```

**Squirrel (CLAUDE.md rule 14)** — all SQL built via `squirrel` query builder. No raw string concatenation.

**Multi-tenant**: Read PG from context via `tmcore.GetPGContext(ctx, constant.ModuleTransaction)` (pattern from `balance_sync.worker.go:252-267`).

**Observability**: Every method opens a child span via `tracer.Start` using `_, span := tracer.Start(...)` (not overwriting ctx — CLAUDE.md rule on span lifecycle). Errors wrapped with `%w`; business errors returned direct.

### 1.4 Canonical Request Hash — `pkg/canonicaljson/canonical.go` (NEW)

No canonical JSON utility exists in the repo. Build a minimal one:

```go
// Canonicalize returns the canonical JSON byte encoding of v:
// - map keys sorted lexicographically
// - struct fields emitted in JSON tag order
// - no insignificant whitespace
// - nil and omitempty handled consistently
func Canonicalize(v any) ([]byte, error)

// Hash returns the lowercase hex SHA-256 of Canonicalize(v).
func Hash(v any) (string, error)
```

**Implementation approach**: Marshal to `any` via `json.Marshal` + `json.Unmarshal`, then custom recursive encoder that sorts map keys. Alternative: use `github.com/gibson042/canonicaljson-go` if already in `go.sum` (check first; don't add deps we don't need).

**Tests** (per design doc line 540-545):
- Same body with different key order produces same hash.
- Same body with whitespace differences produces same hash.
- nil and empty values follow documented policy.
- Different business values produce different hashes.

### 1.5 CRM Client — `components/ledger/internal/adapters/crm/http/` (NEW)

**`account_relationship.port.go`** — interface per design doc line 69:

```go
type CRMAccountRelationshipPort interface {
    GetHolder(ctx context.Context, organizationID string, holderID uuid.UUID) (*mmodel.Holder, error)
    CreateAccountAlias(ctx context.Context, organizationID string, holderID uuid.UUID, input *mmodel.CreateAliasInput, idempotencyKey string) (*mmodel.Alias, error)
    GetAliasByAccount(ctx context.Context, organizationID, ledgerID, accountID string) (*mmodel.Alias, error)
    CloseAlias(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID, idempotencyKey string) error
}
```

**`account_relationship.client.go`** — HTTP client skeleton. Model the lifecycle on `tmclient` (referenced from `components/ledger/internal/bootstrap/config.go:919`):
- `net/http.Client` with explicit `Timeout` per operation — **code constants**, not env vars: GetHolder 5s, CreateAccountAlias 10s, GetAliasByAccount 5s, CloseAlias 10s (from design doc).
- Wrap with `libCircuitBreaker` (per `circuitbreaker.go:58-99` pattern). **Code constants**: 5 consecutive failures to open; 30s half-open probe. Promote to env vars only when ops proves a need.
- Base URL from config (`CRM_BASE_URL`). Default: `http://midaz-crm:4003` (matches `components/crm/docker-compose.yml`).
- All calls target `/internal/v1/*` paths — CRM-side routes land in Phase 3. The client compiles and has a mock-friendly interface; it will return `ErrCRMInternalRouteNotImplemented` until Phase 3 completes.
- Structured logging via `libLog` (CLAUDE.md Observability — no `fmt.Sprintf` inside logger calls).

**Error mapping** (design doc line 560-564):
- HTTP 404 on GetHolder → `ErrHolderNotFound` (terminal).
- HTTP 409 on CreateAccountAlias with conflicting holder → `ErrAliasHolderConflict` (terminal).
- HTTP 409 on CreateAccountAlias with same payload → treat as success (replay).
- HTTP 5xx, timeout, connection refused → `ErrCRMTransient` (retryable).
- Circuit breaker open → `ErrCRMTransient` (retryable).

**`m2m_auth.go`** — **DEFERRED TO PHASE 4.** The client skeleton does not need auth wiring: in Phase 1-2 it returns `ErrCRMInternalRouteNotImplemented` for every method, because CRM-side routes don't exist yet (Phase 3) and no public API calls the client yet (Phase 4). Phase 4 will add:
- `ClientCredentials` flow against Casdoor (reuses existing auth stack).
- Claim: `{"service": "ledger", "tenant_id": "<from tmcore.GetTenantIDContext>"}`.
- Token URL derived from existing `PLUGIN_AUTH_HOST` + Casdoor's well-known path (no new env var).
- Audience as a code constant (`"crm"`).
- Client ID + Secret as env vars at that point: `CRM_M2M_CLIENT_ID`, `CRM_M2M_CLIENT_SECRET`.

**Config additions** — `components/ledger/internal/bootstrap/config.go`:
- **One** new field: `CRMBaseURL string` (default `http://midaz-crm:4003`). That is it.
- Wire the client into `InitServersWithOptions` after `initTenantClient()` and before handler construction.

### 1.6 Error Sentinels — `pkg/constant/errors.go` (MODIFIED)

Add (see CLAUDE.md rule: one sentinel per code, in `pkg/constant/errors.go` only):

```go
ErrAccountRegistrationNotFound       = errors.New("0162")
ErrAccountRegistrationIdempotencyConflict = errors.New("0163")
ErrCRMTransient                      = errors.New("0164")
ErrHolderNotFound                    = errors.New("0165")
ErrAliasHolderConflict               = errors.New("0166")
ErrCRMInternalRouteNotImplemented    = errors.New("0167")
ErrInvalidAccountActivationState     = errors.New("0168")
```

Map each to an HTTP error type via `pkg.ValidateBusinessError` (match existing patterns in the file).

## Phase 2: Ledger Pending Account Support

### 2.1 Account Status Constants — `pkg/constant/` (MODIFIED or NEW)

Inspect whether status codes currently live in `pkg/mmodel/account.go` or `pkg/constant/`. The existing `determineStatus()` at `components/ledger/internal/services/command/create_account.go:209-211` defaults to `"ACTIVE"`. Add constants (if not present):

```go
const (
    AccountStatusActive          = "ACTIVE"
    AccountStatusInactive        = "INACTIVE"
    AccountStatusClosed          = "CLOSED"
    AccountStatusPendingCRMLink  = "PENDING_CRM_LINK"  // NEW
    AccountStatusFailedCRMLink   = "FAILED_CRM_LINK"   // NEW
)
```

Validate the allowed `(status, blocked)` combinations from the design doc (line 358-372) in the account command layer.

### 2.2 Create Account — Honor Pending State

**File**: `components/ledger/internal/services/command/create_account.go`

Current behavior (line 108-124): creates Account via repo, then synchronously creates default balance at line 135-148 with `AllowSending: true, AllowReceiving: true`.

**Changes**:
1. Add an internal parameter (not exposed on the public API yet — Phase 4 does that) that indicates "pending CRM link". Suggested approach: a new unexported constructor variant `createAccountWithOptions(ctx, ..., opts accountCreateOptions)` where `accountCreateOptions.PendingCRMLink bool`.
2. When `PendingCRMLink` is true:
   - Status defaults to `PENDING_CRM_LINK` (not `ACTIVE`).
   - `Blocked = true`.
   - Default balance created with `AllowSending: false, AllowReceiving: false`. (Blocking at balance level is the defense-in-depth — transaction eligibility also enforces it, see 2.4.)
3. Refactor the existing `CreateAccount` entry point to call the new internal with `PendingCRMLink: false`. No public API change.

**Test coverage**:
- Unit test: `PendingCRMLink: true` produces account with status `PENDING_CRM_LINK`, `blocked: true`, balance with `AllowSending: false, AllowReceiving: false`.
- Unit test: `PendingCRMLink: false` behaves identically to current `CreateAccount` (regression guard).

### 2.3 Activate Account — Atomic Status + Blocked

**File**: `components/ledger/internal/services/command/activate_account.go` (NEW)

The design doc (line 374) demands atomic `status` + `blocked` update. Implement as a single parameterized UPDATE:

```go
// ActivateAccount transitions a PENDING_CRM_LINK account to ACTIVE and unblocks it.
// Returns ErrInvalidAccountActivationState if the account is not in PENDING_CRM_LINK.
func (uc *UseCase) ActivateAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error
```

Implementation:
1. Open PG transaction from `tmcore.GetPGContext(ctx, constant.ModuleOnboarding)`.
2. `SELECT ... FOR UPDATE` the account row — verify current state is `PENDING_CRM_LINK + blocked=true`.
3. If mismatch: return `ErrInvalidAccountActivationState`.
4. Build squirrel UPDATE setting `status='ACTIVE'`, `blocked=false`, `updated_at=NOW()` in one statement.
5. Update the default balance: `allow_sending=true`, `allow_receiving=true` for the same account in the same PG transaction. (Balances live in the `transaction` DB — this crosses DB boundaries, so see note below.)
6. Commit.

**Cross-database concern**: Account lives in `onboarding` PG DB; balance lives in `transaction` PG DB. These are separate PG instances in some deployments. Options:
- **(A)** Update only the account's `status`/`blocked` in the onboarding DB transaction, then update balance sending/receiving in a second transaction against the transaction DB. Acceptable because transaction eligibility checks account status FIRST (see 2.4), so the balance flags are defense-in-depth, not the primary gate.
- **(B)** Accept that activation is not fully atomic across the two DBs; document and rely on eligibility-check ordering.

**Recommend (A)**: update account atomically (single statement, single DB), then update balance as a best-effort second step with retry. The transaction eligibility check is the real safety gate.

### 2.4 Transaction Eligibility — Reject PENDING_CRM_LINK and Blocked

**Investigation required before writing this code**: The exploration did not find an explicit `status == ACTIVE` check in `components/ledger/internal/services/command/write_transaction.go`. Before implementing, grep for account-status validation in:
- `components/ledger/internal/services/command/create-transaction*.go`
- `components/ledger/internal/services/command/validate-*.go`
- `components/ledger/internal/adapters/postgres/balance/` (balance may carry the check)

**Hypothesis**: Either validation happens against the balance's `allow_sending`/`allow_receiving` flags, or there is no current check and every account is transactable by default. Either way:

**Required change**: Add an explicit account-status check in the transaction creation path. Reject with a terminal business error if:
- `account.Status.Code == "PENDING_CRM_LINK"` OR
- `account.Status.Code == "FAILED_CRM_LINK"` OR
- `account.Blocked == true`

Error sentinel: use existing `constant.ErrAccountStatusInvalid` if present, otherwise add a new one.

### 2.5 Config & Bootstrap Wiring

**File**: `components/ledger/internal/bootstrap/config.go`

Add exactly **one** field to the `Config` struct:

```go
CRMBaseURL string `env:"CRM_BASE_URL,default=http://midaz-crm:4003"`
```

Timeouts and circuit-breaker thresholds are code constants in the CRM client package (see 1.5). M2M auth credentials are Phase 4.

Instantiate CRM client after `initTenantClient()` in `InitServersWithOptions`. Pass into use-case constructors via the DI chain. Do NOT wire into any public HTTP route yet (Phase 4).

Update `.env.example` at repo root and `components/ledger/.env.example` with the single new var.

## Critical Files — Modification Index

| Path | Action | Purpose |
|---|---|---|
| `pkg/mmodel/account_registration.go` | NEW | Domain model + status constants |
| `pkg/canonicaljson/canonical.go` | NEW | Canonical JSON + SHA-256 hash utility |
| `pkg/canonicaljson/canonical_test.go` | NEW | Tests for hash stability |
| `pkg/constant/errors.go` | MODIFY | New error sentinels 0162-0168 |
| `pkg/constant/account_status.go` | NEW or MODIFY | `PENDING_CRM_LINK`, `FAILED_CRM_LINK` |
| `components/ledger/internal/adapters/postgres/accountregistration/account_registration.go` | NEW | Repo interface + Postgres impl |
| `components/ledger/internal/adapters/postgres/accountregistration/account_registration.sql.go` | NEW | Squirrel query builders |
| `components/ledger/internal/adapters/postgres/accountregistration/account_registration_integration_test.go` | NEW | testcontainers coverage (CLAUDE.md rule 18) |
| `components/ledger/internal/adapters/crm/http/account_relationship.port.go` | NEW | Interface |
| `components/ledger/internal/adapters/crm/http/account_relationship.client.go` | NEW | HTTP client skeleton (no auth — Phase 4) |
| `components/ledger/internal/services/command/create_account.go` | MODIFY | Honor `PendingCRMLink` option |
| `components/ledger/internal/services/command/activate_account.go` | NEW | Atomic status + blocked transition |
| `components/ledger/internal/services/command/create_account_test.go` | MODIFY | Cover pending-mode path |
| `components/ledger/internal/services/command/activate_account_test.go` | NEW | Cover atomic activation + invalid state rejection |
| `components/ledger/internal/services/command/write_transaction.go` (or call chain) | MODIFY | Reject PENDING_CRM_LINK / blocked |
| `components/ledger/internal/bootstrap/config.go` | MODIFY | Add `CRM_BASE_URL` + DI wiring |
| `components/ledger/migrations/transaction/NNNNNN_create_account_registration_table.up.sql` | NEW | Orchestration table |
| `components/ledger/migrations/transaction/NNNNNN_create_account_registration_table.down.sql` | NEW | Reversal |
| `.env.example` | MODIFY | Add `CRM_BASE_URL` |
| `components/ledger/.env.example` | MODIFY | Add `CRM_BASE_URL` |

## Reused Patterns and Utilities

- **Repository structure**: Mirror `components/ledger/internal/adapters/postgres/account/` for `accountregistration/`.
- **Multi-tenant DB access**: `tmcore.GetPGContext(ctx, constant.ModuleTransaction)` — pattern from `components/ledger/internal/bootstrap/balance_sync.worker.go:252-267`.
- **Circuit breaker**: `NewCircuitBreakerManager` pattern from `components/ledger/internal/bootstrap/circuitbreaker.go:58-99`.
- **HTTP client lifecycle**: `tmclient` initialization pattern from `config.go:906-923`.
- **Span lifecycle**: `_, span := tracer.Start(...)` + `defer span.End()` (CLAUDE.md observability rule).
- **Structured logging**: `libLog.Err(err)`, `libLog.String(...)` — no `fmt.Sprintf` inside logger calls (CLAUDE.md rule 12).
- **Error sentinels**: Single source in `pkg/constant/errors.go`; use `pkg.ValidateBusinessError(constant.Err..., constant.EntityAccountRegistration)` with a new `EntityAccountRegistration` added to `pkg/constant/entity.go`.
- **Timestamps**: Capture `time.Now().UTC()` once and reuse for `CreatedAt` + `UpdatedAt` (CLAUDE.md rule 16).
- **Squirrel**: Use for all SQL building (CLAUDE.md rule 14). See any existing Postgres adapter for examples.

## Verification

Run in this order:

```bash
# 1. Static + tests
make tidy                       # dependency cleanup
make format
make lint                       # golangci-lint v2.4.0 — must pass
make check-logs
make check-tests
make sec                        # gosec + govulncheck

# 2. Migrations
make migrate-lint               # validate SQL
# Apply forward + rollback to verify the down migration:
make ledger COMMAND="migrate-up"
make ledger COMMAND="migrate-down"
make ledger COMMAND="migrate-up"

# 3. Unit + integration
make test-unit PKG=./pkg/canonicaljson/...
make test-unit PKG=./components/ledger/internal/services/command/...
make test-integration PKG=./components/ledger/internal/adapters/postgres/accountregistration/...

# 4. Full suite
make test
make coverage-unit
```

### Manual verification scenarios

1. **Pending account creation (unit-testable)**: Call `CreateAccountWithOptions(..., {PendingCRMLink: true})` via a test harness. Assert:
   - Account row has `status='PENDING_CRM_LINK'`, `blocked=true`.
   - Default balance row has `allow_sending=false`, `allow_receiving=false`.

2. **Activation atomicity (integration)**: Use the test `activate_account_test.go`:
   - Create pending account.
   - Call `ActivateAccount`.
   - Assert account is `status='ACTIVE'`, `blocked=false`.
   - Assert balance has `allow_sending=true`, `allow_receiving=true`.
   - Assert a second `ActivateAccount` call returns `ErrInvalidAccountActivationState`.

3. **Transaction rejection (integration)**: Create a pending account. Attempt a transaction against it. Assert the request fails with a terminal business error (not a 500).

4. **Canonical hash stability (unit)**: Marshal the same logical payload with shuffled JSON key order twice. Assert hash bytes are identical.

5. **CRM client shape (compile-check)**: `go build ./...` succeeds with the new packages. Client can be instantiated; calls to its methods return `ErrCRMInternalRouteNotImplemented` (because CRM side is not yet built).

### Out-of-scope smoke tests (Phase 4+ territory)

These are explicitly NOT verified by this plan:
- End-to-end account-registration HTTP flow (Phase 4 adds the handler).
- CRM alias creation from Ledger (Phase 3 adds CRM-side routes).
- Recovery worker resumption (Phase 5).
- Compensation paths (Phase 5).

## Open Questions for Execution Time

These are acceptable to defer but flag during implementation:

1. **Transaction eligibility check location**: Needs a direct read of `components/ledger/internal/services/command/create-transaction*.go` before writing Phase 2.4. If no current check exists, the change is additive; if a check exists, extend it.
2. **Cross-DB activation**: Confirm whether a single deployment uses one PG instance for both `onboarding` and `transaction` DBs (default Midaz docker-compose: yes; production: may differ). If always same instance, a single PG transaction across both schemas is possible; if not, accept the documented two-step flow.
3. **CRM client Phase 1 failure mode**: The client is useful even without CRM-side routes if it returns a clear sentinel. Confirm that Phase 3 uses the same port URLs this skeleton targets.

## Blast Radius

- **Production behavior**: ZERO change. No new routes exposed; `CreateAccount` public API unchanged; new internal options default to non-pending; new tables/migrations are additive.
- **Database migrations**: One new table in `transaction` DB. Reversible via `.down.sql`.
- **Config surface**: 1 new env var (`CRM_BASE_URL`) with a sane default. M2M credentials (2 more) land in Phase 4 when they're actually used.
- **Dependency graph**: No new external deps required unless we elect to add `github.com/gibson042/canonicaljson-go`; prefer writing a thin canonical encoder in-repo.
