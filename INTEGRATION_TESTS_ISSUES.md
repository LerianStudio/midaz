# Integration Test Failures - Issue Map (Zero-Knowledge Context)

Date: 2025-12-28
Run: `make test-integration` (local hot-reload stack running)
Result: 227 tests, 3 skipped, 53 failures

This document explains each failing group, why it fails, where to look, and the most direct fix paths. It is intended for a new developer with no prior project context.

## How to Reproduce

1. Ensure the local stack is running (as you normally do for integration tests).
2. From repo root, run:
   ```
   make test-integration
   ```
3. Compare failures with the sections below.

## Auth-Security Tests (NoAuthHeader / InvalidToken)

### Symptom
Requests without auth or with invalid tokens return 200/201 instead of 401. Tests fail:
- `TestIntegration_Security_NoAuthHeader/*`
- `TestIntegration_Security_InvalidToken/*`

### Root Cause
The auth middleware bypasses authorization when the auth plugin is disabled. Local `.env` defaults to `PLUGIN_AUTH_ENABLED=false`.

### Key Files
- `components/onboarding/.env`
- `components/transaction/.env`
- `components/crm/.env`
- `../lib-auth/auth/middleware/middleware.go` (Authorize() returns `c.Next()` when disabled)
- `tests/integration/security_boundary_test.go`

### Fix Options
- Make these tests conditional on auth enabled (similar to cross-tenant tests), or
- Enable auth plugin in the stack and supply `TEST_AUTH_*` credentials.

## Cross-Tenant Security Tests

### Status
Now skipped if `PLUGIN_AUTH_ENABLED` is not found or false in `.env`.

### Gate Logic
`tests/integration/security_boundary_test.go` reads:
- `components/onboarding/.env`
- `components/transaction/.env`
- `components/crm/.env`

If any file sets `PLUGIN_AUTH_ENABLED=true`, cross-tenant tests run. If all found are false, or key missing in all files, tests are skipped.

## Asset Code Validation (Asset_Update / Idempotency_DuplicateAssetCreate)

### Symptom
Asset creation fails with code 400 and error code `0033` (Invalid Code Format).

### Root Cause
Asset codes are validated to be **uppercase letters only**. Tests generate alphanumeric codes using `RandString`, which includes digits.

### Key Files
- `pkg/utils/utils.go` (ValidateCode)
- `tests/helpers/random.go` (RandString includes digits)
- `tests/integration/patch_delete_operations_test.go`

### Fix Options
- Update tests to generate codes with uppercase letters only, or
- Update validator to allow digits (if API contract should allow alphanumeric).

## Operation Route Validation

### Invalid operationType -> 500
#### Symptom
Invalid `operationType` returns 500 instead of 400.

#### Root Cause
Input struct validates only `required` and not enum. Invalid values reach DB CHECK constraint and return 500.

#### Files
- `pkg/mmodel/operation-route.go` (CreateOperationRouteInput)
- `components/transaction/migrations/000007_create_operation_route_table.up.sql`
- `tests/integration/routing_operation_route_test.go`
- `tests/integration/operation_routes_test.go`

#### Fix
Add validation `oneof=source destination` to input, or map DB constraint error to 400.

### Empty validIf Accepted
#### Symptom
`account.validIf` empty string succeeds, expected failure.

#### Root Cause
Validation checks for nil but not empty string / empty array.

#### Files
- `components/transaction/internal/adapters/http/in/operation-route.go` (validateAccountRule)
- `tests/integration/routing_operation_route_test.go`

#### Fix
Treat empty string/empty array as invalid.

### Delete Non-Existent Route Returns 204
#### Symptom
DELETE non-existent route returns 204 instead of 404.

#### Root Cause
Delete query does not check rows affected.

#### Files
- `components/transaction/internal/adapters/postgres/operationroute/operationroute.postgresql.go`
- `tests/integration/operation_routes_test.go`

#### Fix
Check `RowsAffected()` and return ErrNotFound when 0.

## GET After DELETE Still Succeeds (OperationRoute / TransactionRoute / AccountType)

### Symptom
After delete, GET still returns entity instead of 404.

### Likely Causes
- Soft delete does not confirm row update, or
- Read replica / caching returns stale data.

### Files to Inspect
- OperationRoute delete: `components/transaction/internal/adapters/postgres/operationroute/operationroute.postgresql.go`
- TransactionRoute delete: `components/transaction/internal/adapters/postgres/transactionroute/transactionroute.postgresql.go`
- AccountType delete: `components/onboarding/internal/adapters/postgres/accounttype/accounttype.postgresql.go`
- Replica wiring: `components/*/internal/bootstrap/config.go`

### Fix Options
- Ensure delete checks rows affected and updates `deleted_at`.
- Read-after-write consistency: route reads to primary or delay test/assertion.

## Asset Rates 500 on Create/Update

### Symptom
Asset rate creation fails with 500.

### Root Cause
DB column `external_id` is UUID. Tests send non-UUID strings like `ext-...`. No input validation, so DB error returns 500.

### Files
- DB schema: `components/transaction/migrations/000002_create_asset_rate_table.up.sql`
- Input model: `pkg/mmodel/assetrate.go`
- Tests: `tests/integration/asset_rate_test.go`, `tests/integration/asset_rates_test.go`

### Fix Options
- Update tests to pass a UUID in `externalId`, or
- Change schema to text, or
- Add validation to reject non-UUIDs with 400.

## Balance Tests (externalCode + metadata)

### Symptoms
- `externalCode` in account create rejected as unexpected field.
- Metadata on balance create/update rejected as unexpected field.

### Root Causes
- Account create input has no `externalCode` field.
- Balance create/update models do not accept `metadata`.
- `/accounts/external/:code/balances` uses alias `@external/<code>`, not account externalCode.

### Files
- `pkg/mmodel/account.go`
- `pkg/mmodel/balance.go`
- `components/transaction/internal/adapters/http/in/balance.go`
- `tests/integration/balance_mutations_test.go`

### Fix Options
- Update tests to use supported inputs, or
- Extend API models to include externalCode/metadata as intended.

## CRM Holder / Alias Payload Shape Issues

### Holder: addresses/contact/extended fields
#### Symptoms
- Addresses array rejected (expects object).
- Contact fields (`email`, `phone`, `mobile`) rejected (expects `primaryEmail`, etc.).
- Natural/Legal person extended fields rejected as unexpected.

#### Root Cause
Test payloads don’t match the current request models.

#### Files
- Model: `pkg/mmodel/holder.go`
- Tests: `tests/integration/crm_holder_lifecycle_test.go`

#### Fix
Align tests with model schema, or update model to accept current payloads.

### Holder: invalid type accepted
#### Symptom
`type=INVALID_TYPE` returns 201; expected 400.

#### Root Cause
Missing `oneof` validation on `CreateHolderInput.Type`.

#### Files
- `pkg/mmodel/holder.go`
- `tests/integration/crm_holder_lifecycle_test.go`

#### Fix
Add `oneof=NATURAL_PERSON LEGAL_PERSON` validation.

### Holder Update 500
#### Symptom
PATCH holder fails with 500.

#### Root Cause
Update payload omits `document`; MongoDB `FromEntity` encrypts `Document` unconditionally, fails on nil.

#### Files
- `components/crm/internal/services/update-holder.go`
- `components/crm/internal/adapters/mongodb/holder/holder.go`

#### Fix
Make `FromEntity` tolerate nil on update or provide document on update.

### Alias Create 500
#### Symptom
Alias creation fails with 500.

#### Root Cause
MongoDB `Alias.FromEntity` encrypts `ParticipantDocument` without nil check.

#### Files
- `components/crm/internal/adapters/mongodb/alias/alias.go`
- `tests/integration/crm_alias_lifecycle_test.go`

#### Fix
Guard nil before encryption or default empty string.

### Alias Banking Details Rejected
#### Symptom
Banking details fields rejected as unexpected.

#### Root Cause
Test payload uses fields not in model (`accountNumber`, `bankCode`, etc.). Model expects `branch`, `account`, `type`, `iban`, `countryCode`, `bankId`.

#### Files
- `pkg/mmodel/alias.go`
- `tests/integration/crm_alias_lifecycle_test.go`

#### Fix
Align test payload to model or update model to accept test fields.

## Suggested Next Steps (Pick One Track)

1. **Auth/ Security Track**
   - Gate NoAuthHeader/InvalidToken tests on `PLUGIN_AUTH_ENABLED`.
   - Optionally wire real auth for local dev.

2. **Routing Track**
   - Add validation for operationType.
   - Reject empty validIf.
   - Fix delete rows affected.
   - Investigate read-after-delete consistency.

3. **CRM Track**
   - Align tests with models or adjust models.
   - Fix holder update nil document handling.
   - Fix alias nil participant document handling.

4. **Asset/Balances Track**
   - Align asset code generation with validation.
   - Adjust balance tests to current schema.
   - Fix asset rate externalId to be UUID or validate.

## Notes
- Cross-tenant tests now skip when auth is disabled.
- No environment files were modified.
