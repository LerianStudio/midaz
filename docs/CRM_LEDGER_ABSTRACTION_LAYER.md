# CRM and Ledger Abstraction Layer Plan

## Purpose

Create a Ledger-owned abstraction API that lets clients open and manage account relationships as one coherent business operation across Ledger and CRM.

The goal is not database-level atomicity across Postgres and MongoDB. The goal is business-level atomicity:

```text
An account must not become operational unless its CRM holder relationship exists and is consistent.
```

## Current Relationship

Ledger owns the financial account, balance creation, account status, and transaction eligibility.

CRM owns holders, aliases, banking details, regulatory fields, related parties, and the account-to-holder relationship.

The current bridge is the CRM alias document:

```text
CRM Holder -> CRM Alias -> Ledger Account
```

The CRM alias stores:

```text
holder_id
ledger_id
account_id
```

The Ledger account does not store `holder_id` or a CRM alias reference.

## Design Decision

Avoid a third deployable component.

Ledger will own the abstraction API and orchestration workflow because Ledger controls whether an account can transact.

```text
Client
  -> Ledger abstraction API
      -> Ledger local account use cases and repositories
      -> CRM remote API or gRPC client
      -> Ledger orchestration state table
      -> Ledger recovery worker
```

CRM remains a separate deployable service. Ledger crosses the service boundary only through an explicit CRM relationship port.

## Core Architecture

```text
components/ledger
  internal/adapters/http/in/account_registration.go
  internal/adapters/crm/http/account_relationship.go
  internal/adapters/postgres/accountregistration/account_registration.postgresql.go
  internal/services/command/create_account_registration.go
  internal/services/command/get_account_registration.go
  internal/services/command/account_registration_recovery.go
```

The Ledger abstraction layer should call Ledger application code directly. It should not call Ledger's own HTTP endpoints.

Ledger should call CRM through an interface:

```go
type CRMAccountRelationshipPort interface {
    GetHolder(ctx context.Context, organizationID string, holderID uuid.UUID) (*mmodel.Holder, error)
    CreateAccountAlias(ctx context.Context, organizationID string, holderID uuid.UUID, input *mmodel.CreateAliasInput, idempotencyKey string) (*mmodel.Alias, error)
    GetAliasByAccount(ctx context.Context, organizationID string, ledgerID string, accountID string) (*mmodel.Alias, error)
    CloseAlias(ctx context.Context, organizationID string, holderID uuid.UUID, aliasID uuid.UUID, idempotencyKey string) error
}
```

The first implementation can be HTTP-based because CRM and Ledger are separate deployable entities.

## Multi-Tenant Context Propagation

Tenant propagation is a hard requirement for Ledger-to-CRM calls.

CRM resolves tenant database context through tenant middleware. Ledger service-to-service calls must therefore include a tenant identity that CRM can trust and use to route the request to the correct tenant database.

Preferred approach:

```text
Ledger receives end-user request with tenant-aware JWT
Ledger extracts tenant_id from validated request context
Ledger calls CRM with M2M authentication plus tenant context
CRM accepts tenant context only from trusted Ledger service identity
CRM injects tenant-specific MongoDB context before handler execution
```

Acceptable tenant propagation mechanisms:

```text
M2M JWT containing tenant_id and service identity claims
X-Tenant-Id header accepted only over mTLS/service mesh from Ledger
Signed internal header containing tenant_id, timestamp, and request hash
```

The implementation must not rely on the public end-user token being forwarded blindly to CRM.

Required safeguards:

```text
CRM must reject tenant headers from untrusted callers
CRM must reject internal calls with missing tenant context when multi-tenancy is enabled
Ledger must log tenant_id hash or safe tenant identifier on every CRM call
Ledger and CRM traces must include the same tenant context attributes
```

## Public Ledger APIs

### Create Account Registration

```http
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations
Idempotency-Key: <required>
```

Creates a Ledger account and a CRM alias as one orchestrated operation.

Request body:

```json
{
  "holderId": "0197...",
  "account": {
    "name": "Main Checking",
    "assetCode": "BRL",
    "type": "deposit",
    "alias": "@customer_main_brl",
    "metadata": {
      "product": "checking"
    }
  },
  "crmAlias": {
    "bankingDetails": {
      "branch": "0001",
      "account": "12345",
      "type": "CACC",
      "countryCode": "BR"
    },
    "regulatoryFields": {
      "participantDocument": "12345678000199"
    },
    "relatedParties": []
  }
}
```

Successful response:

```json
{
  "id": "0197...",
  "status": "COMPLETED",
  "account": {},
  "holder": {},
  "alias": {}
}
```

### Get Account Registration

```http
GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations/{registration_id}
```

Returns the current orchestration state and known resource IDs.

### Get Account CRM Profile

```http
GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/crm-profile
```

Returns the Ledger account enriched with CRM holder and alias data.

### List Holder Accounts

```http
GET /v1/organizations/{organization_id}/holders/{holder_id}/accounts
```

Returns all Ledger accounts associated with a CRM holder through CRM aliases.

### Close Account Relationship

```http
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/close
Idempotency-Key: <required>
```

Coordinates account closure across Ledger and CRM.

### Retry Account Registration

```http
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations/{registration_id}/retry
Idempotency-Key: <required>
```

Manually retries a registration in `FAILED_RETRYABLE`, `COMPENSATING`, or another recoverable state.

This endpoint is intended for operations and support workflows. It must require elevated permissions.

### Cancel Account Registration

```http
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/account-registrations/{registration_id}/cancel
Idempotency-Key: <required>
```

Manually forces compensation for a registration that cannot be completed.

This endpoint must only work for registrations that have not completed successfully.

## CRM Service Contracts

Ledger needs CRM endpoints that are safe for service-to-service orchestration.

Preferred internal CRM contracts:

```http
GET  /internal/v1/organizations/{organization_id}/holders/{holder_id}
POST /internal/v1/organizations/{organization_id}/holders/{holder_id}/account-aliases
GET  /internal/v1/organizations/{organization_id}/account-aliases/by-account?ledger_id={ledger_id}&account_id={account_id}
POST /internal/v1/organizations/{organization_id}/holders/{holder_id}/aliases/{alias_id}/close
```

If internal routes are not added immediately, Ledger can initially call existing CRM routes, but those routes must be hardened for idempotency and clear duplicate semantics.

## Required CRM Guarantees

CRM alias creation must be idempotent.

Same idempotency key and same request body returns the original alias.

Same idempotency key and different request body returns conflict.

Same `ledger_id` and `account_id` for a different active holder returns conflict.

Same `ledger_id`, `account_id`, and holder for an equivalent request returns the existing active alias.

CRM should continue enforcing one active alias per Ledger account.

CRM must expose deterministic duplicate semantics:

```text
same idempotency key + same request hash -> return original alias
same idempotency key + different request hash -> 409 conflict
same ledger_id/account_id + same holder + same payload -> return existing alias
same ledger_id/account_id + different holder -> 409 account already associated
same ledger_id/account_id + same holder + different payload -> 409 conflict
```

CRM must support explicit request timeouts from Ledger and return clear retryable vs terminal error mappings.

## Ledger Workflow State

Add an orchestration table in Ledger's Postgres database.

Suggested table name:

```text
account_registration
```

Suggested columns:

```text
id uuid primary key
organization_id uuid not null
ledger_id uuid not null
holder_id uuid not null
idempotency_key text not null
request_hash text not null
account_id uuid null
crm_alias_id uuid null
status text not null
failure_code text null
failure_message text null
created_at timestamptz not null
updated_at timestamptz not null
completed_at timestamptz null
retry_count int not null default 0
next_retry_at timestamptz null
claimed_by text null
claimed_at timestamptz null
last_recovered_at timestamptz null
```

Suggested indexes:

```text
unique (organization_id, ledger_id, idempotency_key)
index (organization_id, ledger_id, status)
index (status, next_retry_at)
index (claimed_at)
index (account_id)
index (holder_id)
```

Retention policy:

```text
COMPLETED records remain queryable for at least 90 days
COMPENSATED and FAILED_TERMINAL records remain queryable for at least 180 days
records older than retention move to archive storage before deletion
idempotency keys cannot be reused while the operation record remains active or retained
```

## Workflow Statuses

```text
RECEIVED
HOLDER_VALIDATED
LEDGER_ACCOUNT_CREATED
CRM_ALIAS_CREATED
ACCOUNT_ACTIVATED
COMPLETED
COMPENSATING
COMPENSATED
FAILED_RETRYABLE
FAILED_TERMINAL
```

## Account Operational State

Ledger must support a non-operational account state during orchestration.

`status` is the primary state machine. `blocked` is a derived operational guard that must be updated in the same database transaction as `status`.

Invalid combinations must be prevented by service logic and, where possible, database constraints.

Recommended initial state:

```text
status = PENDING_CRM_LINK
blocked = true
```

Recommended final state:

```text
status = ACTIVE
blocked = false
```

Transaction paths must reject blocked accounts and accounts with non-active status.

This is the core safety mechanism that prevents orphaned Ledger accounts from transacting.

Allowed state combinations:

```text
PENDING_CRM_LINK + blocked = true
ACTIVE + blocked = false
CLOSED + blocked = true
FAILED_CRM_LINK + blocked = true
```

Disallowed state combinations:

```text
PENDING_CRM_LINK + blocked = false
ACTIVE + blocked = true, except explicit manual/compliance block flows
FAILED_CRM_LINK + blocked = false
```

Activation must update `status` and `blocked` atomically in one Ledger database transaction.

## Create Account Registration Flow

```text
1. Validate path parameters, body, and Idempotency-Key.
2. Hash normalized request body.
3. Insert or load account_registration by organization, ledger, and idempotency key.
4. If existing completed operation has same hash, return stored result.
5. If existing operation has different hash, return conflict.
6. Call CRM GetHolder.
7. Store status HOLDER_VALIDATED.
8. Create Ledger account as blocked and PENDING_CRM_LINK.
9. Store account_id and status LEDGER_ACCOUNT_CREATED.
10. Call CRM CreateAccountAlias with ledger_id and account_id.
11. Store crm_alias_id and status CRM_ALIAS_CREATED.
12. Activate and unblock Ledger account.
13. Store status ACCOUNT_ACTIVATED.
14. Store status COMPLETED and completed_at.
15. Return account, holder, alias, and registration status.
```

`ACCOUNT_ACTIVATED` is a durable checkpoint between Ledger activation and final completion. If the process crashes after activation but before completion, recovery only needs to mark the operation as completed after verifying the account and CRM alias remain consistent.

## Failure Handling

If holder validation fails, mark `FAILED_TERMINAL` unless the error is clearly transient.

If Ledger account creation fails, mark `FAILED_TERMINAL` or `FAILED_RETRYABLE` based on the error type.

If CRM alias creation fails after account creation, keep the Ledger account blocked and mark `COMPENSATING` or `FAILED_RETRYABLE`.

If compensation succeeds, mark `COMPENSATED`.

If CRM alias creation succeeds but Ledger activation fails, keep the account blocked and mark `FAILED_RETRYABLE`.

If the process crashes, the recovery worker resumes from the stored status.

Retry policy:

```text
retry transient CRM and Ledger failures with exponential backoff
cap automatic retries with max_retry_count
after max_retry_count, move to FAILED_RETRYABLE for manual retry or COMPENSATING if safe compensation is deterministic
CRM_ALIAS_CREATED must not remain retryable forever; after timeout or max retries, escalate to COMPENSATING or manual intervention
```

Suggested defaults:

```text
max_retry_count = 10
initial_backoff = 5 seconds
max_backoff = 15 minutes
crm_alias_created_activation_timeout = 60 minutes
```

## Compensation Rules

Before CRM alias exists:

```text
soft-delete or cancel the pending Ledger account
delete or neutralize the default balance if needed
mark operation COMPENSATED
```

After CRM alias exists:

```text
first retry Ledger activation
if business rules require cancellation, close CRM alias and cancel pending Ledger account
mark operation COMPENSATED only after both sides are safe
```

Never mark an operation as failed cleanly if the system cannot prove compensation completed.

If CRM alias exists but Ledger activation repeatedly fails, the system must choose one of two explicit outcomes:

```text
complete by fixing/retrying Ledger activation
compensate by closing CRM alias and canceling the pending Ledger account
```

The operation must not remain indefinitely in `CRM_ALIAS_CREATED` with an account blocked forever unless it is escalated to an operational queue.

## Recovery Worker

Ledger should run a recovery worker for account registrations stuck in non-terminal states.

Recovery candidates:

```text
RECEIVED older than N minutes
HOLDER_VALIDATED older than N minutes
LEDGER_ACCOUNT_CREATED older than N minutes
CRM_ALIAS_CREATED older than N minutes
COMPENSATING older than N minutes
FAILED_RETRYABLE older than N minutes
```

The worker must use a multi-replica-safe claim mechanism.

Preferred Postgres pattern:

```sql
SELECT id
FROM account_registration
WHERE status IN (...)
  AND (next_retry_at IS NULL OR next_retry_at <= now())
  AND (claimed_at IS NULL OR claimed_at < now() - interval '5 minutes')
ORDER BY updated_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;
```

After selecting rows, the worker updates:

```text
claimed_by
claimed_at
retry_count
last_recovered_at
```

This avoids multiple Ledger replicas blocking on the same rows or recovering the same operation concurrently.

Recovery behavior:

```text
LEDGER_ACCOUNT_CREATED -> retry CRM alias creation or compensate
CRM_ALIAS_CREATED -> retry Ledger activation
ACCOUNT_ACTIVATED -> verify resources and mark COMPLETED
COMPENSATING -> retry compensation
FAILED_RETRYABLE -> inspect known resource IDs and resume deterministically
```

## Idempotency

Idempotency is mandatory for all orchestrated command APIs.

Ledger public abstraction APIs require `Idempotency-Key`.

Ledger calls CRM with derived keys:

```text
account-registration:{organization_id}:{ledger_id}:{client_idempotency_key}:crm-create-alias
account-registration:{organization_id}:{ledger_id}:{client_idempotency_key}:crm-close-alias
```

Ledger local operations should also be safe under retry by checking the orchestration record before creating additional resources.

Request hash normalization:

```text
parse JSON body into typed request model
apply defaults exactly as execution will use them
canonicalize to JSON with stable field names and sorted map keys
represent absent optional fields consistently
hash canonical bytes with SHA-256
store request_hash as lowercase hex
```

The hash must not be computed from raw request bytes because equivalent JSON payloads can differ by whitespace or key order.

The canonicalization implementation should be covered by tests for:

```text
same body with different key order produces same hash
same body with whitespace differences produces same hash
nil and empty values follow the documented normalization policy
different business values produce different hashes
```

## CRM Client Resilience

Ledger's CRM client must define explicit timeouts and circuit breaker behavior.

Suggested defaults:

```text
GetHolder timeout = 5 seconds
CreateAccountAlias timeout = 10 seconds
GetAliasByAccount timeout = 5 seconds
CloseAlias timeout = 10 seconds
circuit breaker opens after 5 consecutive failures or high failure ratio over a rolling window
circuit breaker half-open probe interval = 30 seconds
```

CRM timeout and circuit breaker failures should map to retryable orchestration errors.

CRM validation, holder not found, account already associated with another holder, and malformed request errors should map to terminal orchestration errors.

## Security

Ledger-to-CRM calls require service-to-service authentication.

Acceptable options:

```text
mTLS
M2M JWT client credentials
signed internal request headers
service mesh identity
```

Internal CRM orchestration routes must not be callable by public clients.

## Observability

Each account registration should emit structured logs and OpenTelemetry spans with:

```text
registration_id
organization_id
ledger_id
holder_id
account_id
crm_alias_id
status
idempotency_key_hash
failure_code
```

Metrics:

```text
account_registration_started_total
account_registration_completed_total
account_registration_failed_total
account_registration_compensated_total
account_registration_recovery_attempts_total
account_registration_duration_seconds
account_registration_stuck_total
```

Status transition events:

```text
account_registration.received
account_registration.ledger_account_created
account_registration.crm_alias_created
account_registration.account_activated
account_registration.completed
account_registration.failed_retryable
account_registration.failed_terminal
account_registration.compensated
```

Phase 1 can rely on polling through `GET /account-registrations/{id}`.

Phase 2 should emit events through RabbitMQ or webhooks so clients and operations tooling do not depend exclusively on polling.

## Testing Strategy

Unit tests:

```text
idempotency same request returns same result
idempotency same key different request returns conflict
CRM holder not found stops before account creation
CRM alias failure leaves account blocked and marks retryable or compensating
Ledger activation failure leaves account blocked and marks retryable
duplicate CRM alias maps to conflict when owned by another holder
activation updates status and blocked atomically
invalid status and blocked combinations are rejected
request hash canonicalization is stable across equivalent JSON payloads
```

Integration tests:

```text
successful account registration creates Ledger account and CRM alias
account is blocked before CRM alias exists
account is active only after CRM alias creation
recovery resumes from LEDGER_ACCOUNT_CREATED
recovery resumes from CRM_ALIAS_CREATED
recovery resumes from ACCOUNT_ACTIVATED
compensation cancels pending account when CRM alias cannot be created
multi-replica recovery uses SKIP LOCKED or lease claim without duplicate recovery
```

Contract tests:

```text
Ledger CRM client handles CRM success responses
Ledger CRM client handles CRM conflict responses
Ledger CRM client handles CRM retryable failures
CRM account-alias creation is idempotent
CRM account-alias creation enforces one active alias per account
tenant context is propagated and rejected when missing
CRM client maps retryable and terminal errors correctly
```

## Implementation Phases

### Phase 1: Contracts and Data Model

Create request and response models for account registration.

Add Ledger `account_registration` migration.

Add repository interface and Postgres implementation for orchestration state.

Define `CRMAccountRelationshipPort`.

Add CRM HTTP client adapter skeleton.

Define tenant propagation contract for Ledger-to-CRM calls.

Define canonical request hash algorithm.

Define orchestration record retention policy.

### Phase 2: Ledger Pending Account Support

Add or standardize `PENDING_CRM_LINK` account status.

Ensure account creation can create blocked pending accounts.

Add activation operation that moves pending account to active and unblocked.

Ensure transaction paths reject blocked or non-active accounts.

Deploy and soak this phase before the account registration API starts writing `PENDING_CRM_LINK` accounts.

Activation must update account `status` and `blocked` atomically.

### Phase 3: CRM Orchestration Safety

Add or harden CRM account-alias creation for idempotency.

Add lookup by `ledger_id` and `account_id`.

Add alias close operation if missing.

Return semantic conflicts for account already associated.

Add CRM idempotency support for account-alias creation and close operations.

Add CRM tenant-context enforcement for internal Ledger calls.

### Phase 4: Create Account Registration API

Add Ledger HTTP handler and route.

Implement create account registration use case.

Persist status transitions after every durable step.

Return completed resources only when account is active and CRM alias exists.

Add explicit CRM client timeout and circuit breaker configuration.

### Phase 5: Recovery and Compensation

Add recovery worker in Ledger bootstrap.

Implement deterministic resume by status.

Add retry and compensation policies.

Add stuck-operation metrics and logs.

Use `FOR UPDATE SKIP LOCKED` or lease-based row claiming for multi-replica safety.

Add manual retry and cancel endpoints.

### Phase 6: Query APIs

Add get account registration endpoint.

Add account CRM profile endpoint.

Add holder accounts endpoint.

Use batch account lookup for holder accounts to avoid N+1 queries.

Evaluate whether the holder-centric route should eventually move to CRM with a Ledger lookup behind a service contract.

### Phase 7: Hardening

Add integration tests with CRM test server or containers as appropriate.

Add contract tests for CRM client behavior.

Add dashboard metrics and alerts for stuck registrations.

Document operational runbooks for retry and compensation.

Add status transition events through RabbitMQ or webhook callbacks.

Add dashboards and alerts for CRM circuit breaker open state.

## Close Account Relationship Flow

Closing an account relationship must be specified with the same rigor as creation.

Public API:

```http
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/close
Idempotency-Key: <required>
```

Preconditions:

```text
account exists and belongs to organization_id/ledger_id
CRM alias exists for ledger_id/account_id
account is not already closed
balance state satisfies close policy
```

Balance close policy must be explicit. Recommended default:

```text
reject close if available, on_hold, or pending balances are non-zero
allow close only after balances are zeroed or transferred by a separate transaction
```

Recommended close sequence:

```text
1. Insert or load close orchestration record by idempotency key.
2. Validate Ledger account and current balance state.
3. Resolve CRM alias by ledger_id/account_id.
4. Mark Ledger account as CLOSING and blocked atomically.
5. Call CRM CloseAlias and set banking_details.closing_date where applicable.
6. Mark Ledger account as CLOSED and blocked atomically.
7. Mark close orchestration as COMPLETED.
```

Failure handling:

```text
if CRM close fails after Ledger is CLOSING, keep account blocked and retry CRM close
if CRM close succeeds but Ledger CLOSED update fails, retry Ledger close
do not reopen CRM alias automatically unless an explicit compensation decision is made
closed CRM alias + CLOSING Ledger account is recoverable by completing Ledger close
```

Close flow should use its own orchestration status table or a generalized lifecycle operation table. Do not overload account registration statuses unless the model is explicitly generalized.

## Open Decisions

Choose REST or gRPC for Ledger-to-CRM service calls.

Decide whether CRM internal orchestration routes are new routes or hardened existing routes.

Decide exact account status constants and database constraints for valid `status` and `blocked` combinations.

Decide whether failed CRM alias creation should immediately compensate or retry first.

Decide retention period for completed and compensated orchestration records.

Decide if direct public CRM alias creation should remain allowed for account binding.

Decide tenant propagation mechanism for Ledger-to-CRM calls.

Decide max retry count and retry backoff values per operation.

Decide whether status transition events use RabbitMQ, webhooks, or both.

Decide whether close orchestration gets a separate table or a generalized lifecycle table.

## Recommended Defaults

Use Ledger-owned orchestration with CRM HTTP client initially.

Use mandatory idempotency keys.

Use `PENDING_CRM_LINK` plus `blocked = true` before CRM succeeds, with atomic status and blocked updates.

Prefer retry before compensation for transient CRM failures.

Expose direct CRM alias creation as advanced/manual only, or restrict it for account binding after the abstraction API is stable.

Store orchestration state in Ledger Postgres.

Do not add a third deployable component.

Use `FOR UPDATE SKIP LOCKED` for recovery claims.

Use explicit CRM client timeouts and circuit breaker protection.

Treat tenant propagation as a Phase 1 blocker, not an implementation detail.

## Gandalf Review Addendum

Gandalf reviewed this plan on 2026-04-23 and confirmed the core direction: Ledger-owned saga orchestration is the right model because Ledger controls transaction eligibility.

The review identified these hardening requirements, now incorporated into this plan:

```text
tenant context propagation is mandatory for Ledger-to-CRM calls
status and blocked must be treated as one atomic operational state
recovery must be multi-replica-safe with SKIP LOCKED or leases
CRM calls need explicit timeouts and circuit breaker protection
CRM_ALIAS_CREATED cannot stay retryable forever without escalation
close-account flow needs its own precise orchestration semantics
request hash canonicalization must be specified and tested
orchestration records require retention and archival policy
manual retry and cancel operations are needed for supportability
holder account listing must avoid N+1 account lookups
```
