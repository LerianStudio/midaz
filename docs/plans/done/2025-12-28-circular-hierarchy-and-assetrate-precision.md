# Implementation Plan: Circular Hierarchy Detection & AssetRate Precision Fix (Revised)

## Goal
Fix two architectural issues:
1. **Circular Hierarchy Detection** - Prevent account parent-child cycles (A→B→C→A)
2. **AssetRate Precision** - Use `int64` instead of `float64` to preserve precision for large values

## Revision Notes
This plan was revised based on code review feedback addressing:
- TOCTOU race condition mitigation
- Depth limit for DoS prevention
- Safe UUID parsing
- Correct understanding of scaled-integer semantic model
- JSON/API backward compatibility

**Update 2025-12-28:** Corrected error codes (0133→0134, 0134→0135) and migration number (000009→000018) to reflect current codebase state.

**Update 2025-12-28 (Code Review Fixes):**
- Fixed Task 1.2: Changed from switch-case to map-based error pattern (matches actual `pkg/errors.go` implementation)
- Fixed Task 2.2: Made data migration **mandatory** (prevents 100x rate corruption)
- Added NULL scale handling in migration (defensive data cleanup)
- Added comprehensive API Consumer Migration Guide with code examples

## Architecture Overview

```
Issue 1: Circular Hierarchy
┌─────────────────────────────────────────────────────────────┐
│ components/onboarding/internal/services/command/            │
│   create-account.go                                         │
│     └── validateAccountPrerequisites()                      │
│           └── NEW: detectCycleInHierarchy() [with limits]   │
└─────────────────────────────────────────────────────────────┘

Issue 2: AssetRate Precision (Decimal Model)
┌─────────────────────────────────────────────────────────────┐
│ Current: rate=525 (float64), scale=2 → precision loss       │
│ Problem: float64 loses precision for values > 2^53          │
│ Fix: Use decimal.Decimal to match Balance/Operation pattern │
│ Change: DB BIGINT → NUMERIC, JSON number → string           │
└─────────────────────────────────────────────────────────────┘
```

## Tech Stack
- Go 1.24+
- `github.com/shopspring/decimal` - High-precision decimal arithmetic
- PostgreSQL (migration: BIGINT → NUMERIC for rate column)

## Prerequisites
- [ ] Run `make test` - ensure all tests pass before changes
- [ ] Backup database or work on dev environment

---

## Batch 1: Circular Hierarchy Detection (Tasks 1.1-1.5)

### Task 1.1: Add Error Codes for Circular Hierarchy
**File:** `pkg/constant/errors.go`
**Estimated Time:** 2 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Add new error codes for circular hierarchy and depth limit.

**Find this line (around line 143):**
```go
	ErrInvalidCodeLength                        = errors.New("0133")
```

**Add after it:**
```go
	ErrCircularAccountHierarchy                 = errors.New("0134")
	ErrAccountHierarchyTooDeep                  = errors.New("0135")
```

**Verification:**
```bash
grep -n "ErrCircularAccountHierarchy\|ErrAccountHierarchyTooDeep" pkg/constant/errors.go
# Expected: Two lines with error codes 0134 and 0135
```

---

### Task 1.2: Add Error Mappings for New Error Codes
**File:** `pkg/errors.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Add the business error mappings for the new error codes. Note: This file uses a **map-based pattern**, not switch-case.

**Find this section in the `errorMap` inside `ValidateBusinessError` function (search for `constant.ErrInvalidParentAccountID`):**
```go
		constant.ErrInvalidParentAccountID: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrInvalidParentAccountID.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		},
```

**Add these map entries after it:**
```go
		constant.ErrCircularAccountHierarchy: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrCircularAccountHierarchy.Error(),
			Title:      "Circular Account Hierarchy Detected",
			Message:    "Setting this parent account would create a circular reference in the account hierarchy.",
		},
		constant.ErrAccountHierarchyTooDeep: ValidationError{
			EntityType: entityType,
			Code:       constant.ErrAccountHierarchyTooDeep.Error(),
			Title:      "Account Hierarchy Too Deep",
			Message:    "The account hierarchy exceeds the maximum allowed depth of 100 levels.",
		},
```

**Verification:**
```bash
grep -n "ErrCircularAccountHierarchy\|ErrAccountHierarchyTooDeep" pkg/errors.go
# Expected: Both map entries found
```

---

### Task 1.3: Add Cycle Detection Helper Function (Revised)
**File:** `components/onboarding/internal/services/command/create-account.go`
**Estimated Time:** 5 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Add a helper function to detect cycles with depth limit and safe error handling.

**Add this constant after the imports:**
```go
const (
	// MaxAccountHierarchyDepth limits the depth of parent-child account chains
	// to prevent DoS attacks via deep hierarchies and ensure reasonable traversal time.
	MaxAccountHierarchyDepth = 100
)
```

**Add this function at the end of the file (after `applyAccountingValidations`):**
```go
// detectCycleInHierarchy checks if setting parentAccountID as parent of the new account
// would create a circular reference. It traverses up the hierarchy from parentAccountID
// with a depth limit to prevent DoS attacks.
//
// Returns:
// - nil: No cycle detected, safe to proceed
// - ErrCircularAccountHierarchy: Cycle would be created
// - ErrAccountHierarchyTooDeep: Hierarchy exceeds max depth
// - Other errors: Database or validation errors
func (uc *UseCase) detectCycleInHierarchy(ctx context.Context, organizationID, ledgerID uuid.UUID, parentAccountID string, newAccountID *string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.detect_cycle_in_hierarchy")
	defer span.End()

	visited := make(map[string]bool)
	currentID := parentAccountID
	depth := 0

	for currentID != "" {
		depth++

		// Depth limit check - prevents DoS via deep hierarchies
		if depth > MaxAccountHierarchyDepth {
			err := pkg.ValidateBusinessError(constant.ErrAccountHierarchyTooDeep, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Hierarchy depth limit exceeded", err)
			logger.Warnf("Account hierarchy depth limit exceeded: %d levels", depth)
			return err
		}

		// Cycle detection - check if we've visited this node
		if visited[currentID] {
			err := pkg.ValidateBusinessError(constant.ErrCircularAccountHierarchy, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Circular hierarchy detected in existing data", err)
			logger.Warnf("Circular hierarchy detected: already visited account %s", currentID)
			return err
		}

		// Would-create-cycle check - new account would be in its own ancestry
		if newAccountID != nil && currentID == *newAccountID {
			err := pkg.ValidateBusinessError(constant.ErrCircularAccountHierarchy, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Circular hierarchy would be created", err)
			logger.Warnf("Circular hierarchy would be created: parent chain leads back to new account %s", *newAccountID)
			return err
		}

		visited[currentID] = true

		// Safe UUID parsing - handles corrupted database data gracefully
		parsedID, parseErr := uuid.Parse(currentID)
		if parseErr != nil {
			logger.Errorf("Invalid UUID in parent account chain: %s, error: %v", currentID, parseErr)
			// Treat as end of chain - defensive termination for corrupted data
			return nil
		}

		// Fetch the current account to get its parent
		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, parsedID)
		if err != nil {
			// Check if it's a "not found" error using the correct error type
			// AccountRepo.Find returns pkg.EntityNotFoundError, not services.ErrDatabaseItemNotFound
			var entityNotFoundErr pkg.EntityNotFoundError
			if errors.As(err, &entityNotFoundErr) {
				logger.Infof("Account %s not found, end of hierarchy chain", currentID)
				return nil
			}
			// Propagate real database errors
			libOpentelemetry.HandleSpanError(&span, "Database error during hierarchy check", err)
			logger.Errorf("Error fetching account %s during hierarchy check: %v", currentID, err)
			return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
		}

		// Move to parent
		if acc.ParentAccountID == nil || *acc.ParentAccountID == "" {
			// Reached root, no cycle
			return nil
		}

		currentID = *acc.ParentAccountID
	}

	return nil
}
```

**Verification:**
```bash
grep -n "detectCycleInHierarchy\|MaxAccountHierarchyDepth" components/onboarding/internal/services/command/create-account.go
# Expected: Constant and function definition found
```

---

### Task 1.4: Integrate Cycle Detection into Account Creation
**File:** `components/onboarding/internal/services/command/create-account.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Call the cycle detection function during account creation.

**Find this code in `validateAccountPrerequisites` (around line 56-78):**
```go
	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		assert.That(assert.ValidUUID(*cai.ParentAccountID),
			"parent account ID must be valid UUID",
			"parent_account_id", *cai.ParentAccountID)

		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, uuid.MustParse(*cai.ParentAccountID))
		if err != nil {
			businessErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", businessErr)

			return uuid.Nil, businessErr
		}

		assert.NotNil(acc, "parent account must exist after successful Find",
			"parent_account_id", *cai.ParentAccountID)

		if acc.AssetCode != cai.AssetCode {
			businessErr := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", businessErr)

			return uuid.Nil, businessErr
		}
	}
```

**Replace with:**
```go
	if !libCommons.IsNilOrEmpty(cai.ParentAccountID) {
		// Safe UUID parsing - avoids panic on malformed input
		parsedParentID, parseErr := uuid.Parse(*cai.ParentAccountID)
		if parseErr != nil {
			businessErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid parent account ID format", businessErr)
			return uuid.Nil, businessErr
		}

		acc, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, &portfolioUUID, parsedParentID)
		if err != nil {
			businessErr := pkg.ValidateBusinessError(constant.ErrInvalidParentAccountID, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find parent account", businessErr)

			return uuid.Nil, businessErr
		}

		assert.NotNil(acc, "parent account must exist after successful Find",
			"parent_account_id", *cai.ParentAccountID)

		if acc.AssetCode != cai.AssetCode {
			businessErr := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", businessErr)

			return uuid.Nil, businessErr
		}

		// Check for circular hierarchy before allowing account creation
		// Note: This check is not atomic with account creation. For strict prevention,
		// consider adding a database trigger or constraint. See TOCTOU note in docs.
		if err := uc.detectCycleInHierarchy(ctx, organizationID, ledgerID, *cai.ParentAccountID, nil); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Circular hierarchy check failed", err)
			return uuid.Nil, err
		}
	}
```

**Verification:**
```bash
grep -n "detectCycleInHierarchy" components/onboarding/internal/services/command/create-account.go
# Expected: Two matches - function definition and call site
```

---

### Task 1.5: Verify Required Imports
**File:** `components/onboarding/internal/services/command/create-account.go`
**Estimated Time:** 1 minute
**Recommended Agent:** `backend-engineer-golang`

**Description:** Ensure all required imports are present. The `pkg` package is needed for `EntityNotFoundError`.

**Verify these imports exist (they should already be present):**
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg"  // For EntityNotFoundError, ValidateBusinessError, etc.
	// ... rest of imports
)
```

**Verification:**
```bash
grep -n '"github.com/LerianStudio/midaz/v3/pkg"' components/onboarding/internal/services/command/create-account.go
# Expected: Import statement found
```

---

### Task 1.6: Add Unit Tests for Cycle Detection
**File:** `components/onboarding/internal/services/command/create-account_test.go`
**Estimated Time:** 10 minutes
**Recommended Agent:** `qa-analyst`

**Description:** Add comprehensive unit tests for the `detectCycleInHierarchy` function covering all code paths.

**Test cases to add:**
```go
func TestDetectCycleInHierarchy_DepthLimitExceeded(t *testing.T) {
	// Setup: Create mock with 101+ level deep hierarchy
	// Assert: Returns ErrAccountHierarchyTooDeep
}

func TestDetectCycleInHierarchy_CycleInExistingData(t *testing.T) {
	// Setup: Create mock with A->B->C->A cycle
	// Assert: Returns ErrCircularAccountHierarchy
}

func TestDetectCycleInHierarchy_WouldCreateCycle(t *testing.T) {
	// Setup: newAccountID is in the parent chain
	// Assert: Returns ErrCircularAccountHierarchy
}

func TestDetectCycleInHierarchy_InvalidUUID(t *testing.T) {
	// Setup: Parent chain contains invalid UUID
	// Assert: Returns nil (defensive termination)
}

func TestDetectCycleInHierarchy_AccountNotFound(t *testing.T) {
	// Setup: Mock returns EntityNotFoundError
	// Assert: Returns nil (end of chain)
}

func TestDetectCycleInHierarchy_DatabaseError(t *testing.T) {
	// Setup: Mock returns other database error
	// Assert: Returns InternalError
}

func TestDetectCycleInHierarchy_NormalTraversal(t *testing.T) {
	// Setup: Valid 3-level hierarchy with no cycles
	// Assert: Returns nil
}
```

**Verification:**
```bash
go test -v -race ./components/onboarding/internal/services/command/... -run TestDetectCycleInHierarchy
# Expected: All tests pass
```

---

### Code Review Checkpoint 1
Run tests to verify circular hierarchy detection:
```bash
go test -v -race ./components/onboarding/internal/services/command/... -run TestCreateAccount
make test-property
```

**Expected:** All tests pass.

---

## Batch 2: AssetRate Precision Fix (Tasks 2.1-2.6)

### Understanding the Semantic Model Change

**Current Model (Scaled-Integer):**
- `rate = 525` (stored as BIGINT)
- `scale = 2` (stored as NUMERIC)
- Actual rate = `rate / 10^scale` = `525 / 100` = `5.25`
- JSON output: `{"rate": 525.0, "scale": 2.0}` (numbers)

**New Model (Direct Decimal - Matching Transaction API):**
- `rate = 5.25` (stored as NUMERIC, direct value)
- `scale = 2` (stored as NUMERIC, optional for display)
- Actual rate = `rate` directly (no division needed)
- JSON output: `{"rate": "5.25", "scale": 2}` (rate as string, matching Balance/Operation pattern)

**Why This Change:**
- ✅ Matches existing transaction API (Balance.Available, Operation.Value use decimal.Decimal)
- ✅ Full precision for any value (no 2^53 limit)
- ✅ Consistent decimal arithmetic across the codebase
- ⚠️ **BREAKING CHANGE**: Rate serializes as string (like all decimals in Midaz)
- 📝 Requires database migration

---

### Task 2.1: Update AssetRate Model to Use decimal.Decimal
**File:** `pkg/mmodel/assetrate.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Change Rate from float64 to decimal.Decimal, matching the pattern used for Balance and Operation.

**Add import:**
```go
import (
	"time"

	"github.com/shopspring/decimal"
)
```

**Find and replace the Rate and Scale fields (around lines 46-51):**

**Old:**
```go
	// Conversion rate value
	// example: 100
	Rate float64 `json:"rate" example:"100"`

	// Decimal places for the rate
	// example: 2
	// minimum: 0
	Scale *float64 `json:"scale" example:"2" minimum:"0"`
```

**New:**
```go
	// Conversion rate value (direct decimal value, matching Balance/Operation pattern)
	// example: 5.25
	// Note: Serializes to JSON as string: {"rate": "5.25"}
	Rate decimal.Decimal `json:"rate" example:"5.25" swaggertype:"string"`

	// Decimal places for display/calculation (optional)
	// example: 2
	// minimum: 0
	Scale int `json:"scale" example:"2" minimum:"0"`
```

**Verification:**
```bash
grep -n "decimal.Decimal.*json" pkg/mmodel/assetrate.go
# Expected: Rate uses decimal.Decimal
```

---

### Task 2.2: Create Database Migration for AssetRate
**File:** `components/transaction/migrations/000018_alter_asset_rate_to_decimal.up.sql` (new file)
**Estimated Time:** 5 minutes
**Recommended Agent:** `devops-engineer`

**Description:** Create migration to change rate column from BIGINT to NUMERIC **with mandatory data conversion**.

> ⚠️ **CRITICAL**: Data migration is REQUIRED, not optional. Without it, existing rates like `525` (meaning 5.25 with scale=2) will become `525.0` instead of `5.25`, causing 100x errors in currency conversions.

**Complete Code:**
```sql
-- Migration: Change asset_rate.rate from BIGINT to NUMERIC for decimal precision
-- CRITICAL: This migration includes mandatory data conversion
--
-- Semantic change:
--   BEFORE: rate=525, scale=2 → calculated as 525/10^2 = 5.25
--   AFTER:  rate=5.25 (direct decimal value)

-- Step 1: Handle NULL scale values (defensive - set to 0 if NULL)
UPDATE asset_rate
SET rate_scale = 0
WHERE rate_scale IS NULL;

-- Step 2: Convert scaled integers to direct decimal values BEFORE type change
-- This MUST happen before ALTER to avoid data corruption
UPDATE asset_rate
SET rate = rate / POWER(10, rate_scale)
WHERE rate_scale > 0;

-- Step 3: Now safe to alter column type
ALTER TABLE asset_rate
    ALTER COLUMN rate TYPE NUMERIC(38, 18);

-- Step 4: Add comment explaining the semantic change
COMMENT ON COLUMN asset_rate.rate IS 'Direct decimal rate value (e.g., 5.25). Migrated from scaled integer on 2025-12-28.';
```

**Create down migration:**
**File:** `components/transaction/migrations/000018_alter_asset_rate_to_decimal.down.sql`
```sql
-- Rollback: Change asset_rate.rate from NUMERIC back to BIGINT
-- WARNING: This will truncate decimal values and requires re-scaling
-- This rollback is LOSSY - decimal precision will be lost

-- Step 1: Convert back to scaled integers (multiply by 10^scale)
UPDATE asset_rate
SET rate = rate * POWER(10, rate_scale)
WHERE rate_scale > 0;

-- Step 2: Alter column type back to BIGINT
ALTER TABLE asset_rate
    ALTER COLUMN rate TYPE BIGINT USING ROUND(rate)::BIGINT;

-- NOTE: After rollback, verify data integrity manually
```

**Verification:**
```bash
ls -la components/transaction/migrations/000018_alter_asset_rate_to_decimal.*
# Expected: Both .up.sql and .down.sql files exist
```

---

### Task 2.3: Update PostgreSQL Model
**File:** `components/transaction/internal/adapters/postgres/assetrate/assetrate.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Update the database model to use decimal.Decimal for Rate.

**Add import:**
```go
import (
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)
```

**Find and update the struct fields (around lines 32-33):**

**Old:**
```go
	Rate           float64        // Conversion rate value
	RateScale      float64        // Decimal places for the rate
```

**New:**
```go
	Rate           decimal.Decimal // Conversion rate value (direct decimal, matches Balance pattern)
	RateScale      int             // Decimal places for display/calculation
```

**Update ToEntity method (around lines 42-58):**

**Old:**
```go
func (a *AssetRatePostgreSQLModel) ToEntity() *mmodel.AssetRate {
	assetRate := &mmodel.AssetRate{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		ExternalID:     a.ExternalID,
		From:           a.From,
		To:             a.To,
		Rate:           a.Rate,
		Scale:          &a.RateScale,
		Source:         a.Source,
		TTL:            a.TTL,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}

	return assetRate
}
```

**New:**
```go
func (a *AssetRatePostgreSQLModel) ToEntity() *mmodel.AssetRate {
	assetRate := &mmodel.AssetRate{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		ExternalID:     a.ExternalID,
		From:           a.From,
		To:             a.To,
		Rate:           a.Rate,
		Scale:          a.RateScale,
		Source:         a.Source,
		TTL:            a.TTL,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}

	return assetRate
}
```

**Update FromEntity method (around lines 61-77):**

**Old:**
```go
func (a *AssetRatePostgreSQLModel) FromEntity(assetRate *mmodel.AssetRate) {
	*a = AssetRatePostgreSQLModel{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: assetRate.OrganizationID,
		LedgerID:       assetRate.LedgerID,
		ExternalID:     assetRate.ExternalID,
		From:           assetRate.From,
		To:             assetRate.To,
		Rate:           assetRate.Rate,
		RateScale:      *assetRate.Scale,
		Source:         assetRate.Source,
		TTL:            assetRate.TTL,
		CreatedAt:      assetRate.CreatedAt,
		UpdatedAt:      assetRate.UpdatedAt,
	}
}
```

**New:**
```go
func (a *AssetRatePostgreSQLModel) FromEntity(assetRate *mmodel.AssetRate) {
	*a = AssetRatePostgreSQLModel{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: assetRate.OrganizationID,
		LedgerID:       assetRate.LedgerID,
		ExternalID:     assetRate.ExternalID,
		From:           assetRate.From,
		To:             assetRate.To,
		Rate:           assetRate.Rate,
		RateScale:      assetRate.Scale,
		Source:         assetRate.Source,
		TTL:            assetRate.TTL,
		CreatedAt:      assetRate.CreatedAt,
		UpdatedAt:      assetRate.UpdatedAt,
	}
}
```

**Verification:**
```bash
grep -n "Rate.*int64\|RateScale.*int" components/transaction/internal/adapters/postgres/assetrate/assetrate.go
# Expected: Fields use int64 and int types
```

**Update ToEntity method - No pointer dereference for Scale:**

Update line 51 from:
```go
Scale: &a.RateScale,
```

To:
```go
Scale: a.RateScale,
```

**Update FromEntity method - No pointer dereference for Scale:**

Update line 71 from:
```go
RateScale: *assetRate.Scale,
```

To:
```go
RateScale: assetRate.Scale,
```

**Verification:**
```bash
grep -n "decimal.Decimal" components/transaction/internal/adapters/postgres/assetrate/assetrate.go
# Expected: Rate field uses decimal.Decimal
```

---

### Task 2.4: Update CreateAssetRateInput
**File:** `pkg/mmodel/assetrate.go`
**Estimated Time:** 2 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Update the input struct to accept decimal.Decimal.

**Find these fields in CreateAssetRateInput (around lines 99-105):**

**Old:**
```go
	// Conversion rate value (required)
	// example: 100
	// required: true
	Rate int `json:"rate" validate:"required" example:"100"`

	// Decimal places for the rate (optional)
	// example: 2
	// minimum: 0
	Scale int `json:"scale,omitempty" validate:"gte=0" example:"2" minimum:"0"`
```

**New:**
```go
	// Conversion rate value (required, accepts string or number in JSON)
	// example: 5.25
	// required: true
	Rate decimal.Decimal `json:"rate" validate:"required" example:"5.25" swaggertype:"string"`

	// Decimal places for display/calculation (optional)
	// example: 2
	// minimum: 0
	Scale int `json:"scale,omitempty" validate:"gte=0" example:"2" minimum:"0"`
```

**Verification:**
```bash
grep -A2 "Rate.*decimal.Decimal" pkg/mmodel/assetrate.go
# Expected: Rate decimal.Decimal in both AssetRate and CreateAssetRateInput
```

---

### Task 2.5: Update Create/Update AssetRate Command
**File:** `components/transaction/internal/services/command/create-assetrate.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `backend-engineer-golang`

**Description:** Remove the float64 conversion - types now match directly.

**Find and update `updateAssetRateFields` (around lines 109-126):**

**Old:**
```go
func (uc *UseCase) updateAssetRateFields(arFound *mmodel.AssetRate, cari *mmodel.CreateAssetRateInput) {
	// WARNING: Converting int64 to float64 loses precision for values > 2^53 (9007199254740992)
	// TODO(review): Refactor AssetRate.Rate to use decimal.Decimal for full precision
	// See: tests/fuzzy/assetrate_precision_fuzz_test.go for demonstration of the issue
	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	arFound.Rate = rate
	arFound.Scale = &scale
	arFound.Source = cari.Source
	arFound.TTL = *cari.TTL
	arFound.UpdatedAt = time.Now()

	if !libCommons.IsNilOrEmpty(cari.ExternalID) {
		arFound.ExternalID = *cari.ExternalID
	}
}
```

**New:**
```go
func (uc *UseCase) updateAssetRateFields(arFound *mmodel.AssetRate, cari *mmodel.CreateAssetRateInput) {
	// Direct decimal assignment - matches Balance/Operation pattern, full precision
	arFound.Rate = cari.Rate
	arFound.Scale = cari.Scale
	arFound.Source = cari.Source

	if cari.TTL != nil {
		arFound.TTL = *cari.TTL
	}

	arFound.UpdatedAt = time.Now()

	if !libCommons.IsNilOrEmpty(cari.ExternalID) {
		arFound.ExternalID = *cari.ExternalID
	}
}
```

**Find and update `createNewAssetRate` (around lines 138-157):**

**Old:**
```go
	// WARNING: Converting int64 to float64 loses precision for values > 2^53 (9007199254740992)
	// TODO(review): Refactor AssetRate.Rate to use decimal.Decimal for full precision
	// See: tests/fuzzy/assetrate_precision_fuzz_test.go for demonstration of the issue
	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	assetRateDB := &mmodel.AssetRate{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     *externalID,
		From:           cari.From,
		To:             cari.To,
		Rate:           rate,
		Scale:          &scale,
		Source:         cari.Source,
		TTL:            *cari.TTL,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
```

**New:**
```go
	// Direct decimal assignment - full precision, matches Balance/Operation pattern
	ttl := 0
	if cari.TTL != nil {
		ttl = *cari.TTL
	}

	assetRateDB := &mmodel.AssetRate{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     *externalID,
		From:           cari.From,
		To:             cari.To,
		Rate:           cari.Rate,
		Scale:          cari.Scale,
		Source:         cari.Source,
		TTL:            ttl,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
```

**Verification:**
```bash
grep -n "float64" components/transaction/internal/services/command/create-assetrate.go
# Expected: No matches (all float64 conversions removed)
grep -n "cari.Rate" components/transaction/internal/services/command/create-assetrate.go
# Expected: Direct assignment, no type conversion
```

---

### Task 2.6: Run Database Migration
**File:** N/A (database operation)
**Estimated Time:** 2 minutes
**Recommended Agent:** `devops-engineer`

**Description:** Apply the migration to change the rate column type.

**Command:**
```bash
# Apply the migration
make migrate-up
```

**Verification:**
```bash
# Check the column type
psql -d midaz -c "\d asset_rate"
# Expected: rate column shows type NUMERIC(38,18)
```

**Note:** If there is existing data in asset_rate table, you may need a data migration script to convert scaled integers to decimals:
```sql
-- Data migration (if needed)
UPDATE asset_rate
SET rate = rate / POWER(10, rate_scale)
WHERE rate_scale > 0;
```

---

### Task 2.7: Update Property Tests for AssetRate
**File:** `tests/property/asset_rate_test.go`
**Estimated Time:** 3 minutes
**Recommended Agent:** `qa-analyst`

**Description:** Update property tests to verify decimal.Decimal precision is preserved and matches Balance pattern.

**Update existing tests to use decimal.Decimal** (if needed) and **add this test:**
```go
// Property: Decimal rate values maintain full precision (no float64 truncation)
func TestProperty_AssetRateDecimalPrecision_Model(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Generate values with decimal places that would lose precision in float64
		intPart := rng.Int63n(1000000)
		decPart := rng.Intn(100) // 0-99 cents

		// Create decimal rate: e.g., 5.25
		rateStr := fmt.Sprintf("%d.%02d", intPart, decPart)
		rate, err := decimal.NewFromString(rateStr)
		if err != nil {
			t.Logf("Failed to create decimal: %v", err)
			return false
		}

		// Simulate storage and retrieval (JSON roundtrip)
		// Note: decimal.Decimal marshals to JSON as string: "5.25"
		jsonBytes, err := json.Marshal(map[string]decimal.Decimal{"rate": rate})
		if err != nil {
			t.Logf("Failed to marshal: %v", err)
			return false
		}

		var result map[string]decimal.Decimal
		err = json.Unmarshal(jsonBytes, &result)
		if err != nil {
			t.Logf("Failed to unmarshal: %v", err)
			return false
		}

		retrieved := result["rate"]

		// Property: exact equality after JSON roundtrip
		if !rate.Equal(retrieved) {
			t.Logf("Precision loss after roundtrip: original=%s retrieved=%s", rate, retrieved)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 500}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("AssetRate decimal precision property failed: %v", err)
	}
}
```

**Required imports:**
```go
import (
	"encoding/json"
	"fmt"
	// ... existing imports
)
```

**Verification:**
```bash
go test -v -race ./tests/property/... -run TestProperty_AssetRateDecimalPrecision
# Expected: PASS
```

---

### Code Review Checkpoint 2
Run full test suite:
```bash
make test
make test-property
```

**Expected:** All tests pass with the int64 changes.

---

## Summary

| Batch | Files Modified | Changes |
|-------|----------------|---------|
| 1 | 3 files + tests | Add circular hierarchy detection with depth limit |
| 2 | 5 files + migration | Convert AssetRate from float64 to decimal.Decimal |

## Key Design Decisions

### Circular Hierarchy Detection
- **Depth Limit:** 100 levels maximum (configurable via constant)
- **Algorithm:** Visited-set traversal with early termination
- **Error Handling:** Safe UUID parsing, proper error type checking
- **Observability:** OpenTelemetry span integration
- **TOCTOU Note:** Race condition exists but is low-risk; document suggests DB constraint for strict prevention

### AssetRate Precision
- **Semantic Model:** Changed from scaled-integer to direct decimal (rate=5.25 directly)
- **Type Change:** `float64` → `decimal.Decimal` (matches Balance/Operation pattern)
- **API Compatibility:** ⚠️ **BREAKING CHANGE** - JSON format changes from number to string
  - Before: `{"rate": 5.25}` (number)
  - After: `{"rate": "5.25"}` (string, matching Balance.Available pattern)
- **DB Compatibility:** 📝 Requires migration (BIGINT → NUMERIC)

## TOCTOU Race Condition Note

The circular hierarchy check and account creation are not atomic. Two concurrent requests could theoretically create a cycle:

```
T1: Request A checks B→A cycle (NO - B doesn't exist)
T2: Request B checks A→B cycle (NO - A doesn't exist)
T3: Request A creates account with parent=B
T4: Request B creates account with parent=A
Result: A→B→A cycle exists
```

**Mitigation Options (not implemented in this plan):**
1. **Database trigger** - Check cycle on INSERT/UPDATE at DB level
2. **SELECT FOR UPDATE** - Lock parent chain during check
3. **Serializable isolation** - Transaction-level isolation

The current implementation provides reasonable protection for normal usage. The race window is small and requires precise timing.

## Failure Recovery

### If circular hierarchy test fails:
1. Check `MaxAccountHierarchyDepth` constant value
2. Verify `pkg.EntityNotFoundError` is used with `errors.As()` pattern
3. Ensure error code 0134/0135 are properly mapped

### If AssetRate test fails:
1. Verify `Rate decimal.Decimal` type in both model and DB model
2. Check JSON marshaling produces string: `{"rate": "5.25"}` (matching Balance pattern)
3. Ensure no float64 conversions remain in create-assetrate.go
4. Verify database migration ran successfully (rate column is NUMERIC)

### Breaking Changes for API Consumers:
1. **Rate field now outputs as string:** `{"rate": "5.25"}` instead of `{"rate": 5.25}`
   - Consumers must parse string to number: `parseFloat(response.rate)`
   - This matches the existing Balance API pattern
2. **Scale field changed from nullable to non-nullable:** `0` instead of `null`
3. **Semantic change:** Rate is now direct decimal value, not scaled integer
   - Before: rate=525, scale=2 meant 5.25 (calculated)
   - After: rate=5.25 (direct value)

---

## API Consumer Migration Guide

### AssetRate Response Changes (v3.x Breaking Change)

**Before (current):**
```json
{
  "id": "uuid",
  "from": "USD",
  "to": "BRL",
  "rate": 5.25,
  "scale": 2
}
```

**After (this release):**
```json
{
  "id": "uuid",
  "from": "USD",
  "to": "BRL",
  "rate": "5.25",
  "scale": 2
}
```

### Client Code Migration Examples

**TypeScript/JavaScript:**
```typescript
// BEFORE
interface AssetRate {
  rate: number;
  scale: number | null;
}
const actualRate = response.rate / Math.pow(10, response.scale ?? 0);

// AFTER
interface AssetRate {
  rate: string;  // Changed to string
  scale: number; // No longer nullable
}
const actualRate = parseFloat(response.rate); // Direct value, no calculation needed
// Or use a decimal library for precision:
import Decimal from 'decimal.js';
const actualRate = new Decimal(response.rate);
```

**Go:**
```go
// BEFORE
type AssetRate struct {
    Rate  float64  `json:"rate"`
    Scale *float64 `json:"scale"`
}
actualRate := rate.Rate / math.Pow(10, *rate.Scale)

// AFTER
import "github.com/shopspring/decimal"
type AssetRate struct {
    Rate  decimal.Decimal `json:"rate"`  // Direct decimal
    Scale int             `json:"scale"` // Non-nullable
}
actualRate := rate.Rate // Direct value, no calculation
```

**Python:**
```python
# BEFORE
rate = response["rate"] / (10 ** response.get("scale", 0))

# AFTER
from decimal import Decimal
rate = Decimal(response["rate"])  # Direct value
```

### Input Changes

**Creating/Updating AssetRate:**
```json
// BEFORE: Send scaled integer
{
  "from": "USD",
  "to": "BRL",
  "rate": 525,
  "scale": 2
}

// AFTER: Send direct decimal value as string or number
{
  "from": "USD",
  "to": "BRL",
  "rate": "5.25",
  "scale": 2
}
// Note: decimal.Decimal accepts both "5.25" (string) and 5.25 (number) on input
```

### Migration Checklist for API Consumers

- [ ] Update type definitions (`rate: number` → `rate: string`)
- [ ] Remove rate calculation logic (`rate / 10^scale` → use rate directly)
- [ ] Update scale type if using nullable (`scale?: number` → `scale: number`)
- [ ] Test with decimal precision edge cases
- [ ] Consider using a decimal library for financial calculations

---

### Handling New Account Hierarchy Error Responses

This release introduces two new error responses for account creation/update operations when invalid parent account relationships are detected:

#### Error Payload Shape

Both errors return HTTP **400 Bad Request** with the following JSON structure:

```json
{
  "code": "0134",
  "title": "Circular Account Hierarchy Detected",
  "message": "Setting this parent account would create a circular reference in the account hierarchy."
}
```

```json
{
  "code": "0135",
  "title": "Account Hierarchy Too Deep",
  "message": "The account hierarchy exceeds the maximum allowed depth of 100 levels."
}
```

| Error Code | HTTP Status | Title | When It Occurs |
|------------|-------------|-------|----------------|
| `0134` | 400 | Circular Account Hierarchy Detected | Setting a parent would create A→B→C→A cycle |
| `0135` | 400 | Account Hierarchy Too Deep | Parent chain exceeds 100 levels |

#### Client Code Error Handling Examples

**TypeScript/JavaScript:**
```typescript
// Define typed errors for account hierarchy issues
interface MidazApiError {
  code: string;
  title: string;
  message: string;
}

const ERROR_CODES = {
  CIRCULAR_HIERARCHY: '0134',
  HIERARCHY_TOO_DEEP: '0135',
} as const;

async function createAccount(payload: CreateAccountRequest): Promise<Account> {
  const response = await fetch('/api/accounts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });

  if (!response.ok) {
    const error: MidazApiError = await response.json();
    
    switch (error.code) {
      case ERROR_CODES.CIRCULAR_HIERARCHY:
        // User-actionable: suggest choosing a different parent
        throw new CircularHierarchyError(
          'Cannot set this parent account - it would create a circular reference. ' +
          'Please choose a different parent account.'
        );
      
      case ERROR_CODES.HIERARCHY_TOO_DEEP:
        // User-actionable: suggest restructuring hierarchy
        throw new HierarchyTooDeepError(
          'Account hierarchy limit reached (100 levels). ' +
          'Consider restructuring your account hierarchy.'
        );
      
      default:
        throw new ApiError(error.message);
    }
  }

  return response.json();
}

// Custom error classes for type-safe handling
class CircularHierarchyError extends Error {
  readonly code = '0134';
  constructor(message: string) {
    super(message);
    this.name = 'CircularHierarchyError';
  }
}

class HierarchyTooDeepError extends Error {
  readonly code = '0135';
  constructor(message: string) {
    super(message);
    this.name = 'HierarchyTooDeepError';
  }
}
```

**Go:**
```go
package midaz

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Error codes for account hierarchy validation
const (
	ErrCodeCircularHierarchy = "0134"
	ErrCodeHierarchyTooDeep  = "0135"
)

// APIError represents a Midaz API error response
type APIError struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

func (e APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Title, e.Message)
}

// Sentinel errors for type checking
var (
	ErrCircularHierarchy = errors.New("circular account hierarchy detected")
	ErrHierarchyTooDeep  = errors.New("account hierarchy too deep")
)

func CreateAccount(client *http.Client, payload CreateAccountRequest) (*Account, error) {
	// ... request setup omitted for brevity ...
	
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var apiErr APIError
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
			return nil, fmt.Errorf("failed to decode error: %w", err)
		}

		switch apiErr.Code {
		case ErrCodeCircularHierarchy:
			return nil, fmt.Errorf("%w: %s", ErrCircularHierarchy, apiErr.Message)
		case ErrCodeHierarchyTooDeep:
			return nil, fmt.Errorf("%w: %s", ErrHierarchyTooDeep, apiErr.Message)
		default:
			return nil, &apiErr
		}
	}

	var account Account
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &account, nil
}

// Usage with errors.Is for type-safe checking
func handleAccountCreation() {
	account, err := CreateAccount(client, payload)
	if err != nil {
		if errors.Is(err, ErrCircularHierarchy) {
			// Handle circular hierarchy - prompt user to select different parent
			log.Warn("Circular hierarchy detected, user needs to select different parent")
			return
		}
		if errors.Is(err, ErrHierarchyTooDeep) {
			// Handle depth limit - suggest hierarchy restructuring
			log.Warn("Hierarchy too deep, suggest restructuring")
			return
		}
		// Handle other errors
		log.Errorf("Account creation failed: %v", err)
		return
	}
	// Success
}
```

**Python:**
```python
from dataclasses import dataclass
from typing import Optional
import requests

# Error codes
ERR_CIRCULAR_HIERARCHY = "0134"
ERR_HIERARCHY_TOO_DEEP = "0135"


@dataclass
class MidazAPIError(Exception):
    """Base exception for Midaz API errors."""
    code: str
    title: str
    message: str

    def __str__(self) -> str:
        return f"{self.title}: {self.message}"


class CircularHierarchyError(MidazAPIError):
    """Raised when setting a parent would create a circular reference."""
    pass


class HierarchyTooDeepError(MidazAPIError):
    """Raised when account hierarchy exceeds maximum depth."""
    pass


def create_account(payload: dict) -> dict:
    """Create an account with hierarchy error handling."""
    response = requests.post(
        "https://api.midaz.io/v1/accounts",
        json=payload,
        headers={"Content-Type": "application/json"}
    )

    if not response.ok:
        error_data = response.json()
        code = error_data.get("code")
        title = error_data.get("title", "Unknown Error")
        message = error_data.get("message", "An error occurred")

        if code == ERR_CIRCULAR_HIERARCHY:
            raise CircularHierarchyError(
                code=code,
                title=title,
                message="Cannot use this parent account - would create circular reference."
            )
        elif code == ERR_HIERARCHY_TOO_DEEP:
            raise HierarchyTooDeepError(
                code=code,
                title=title,
                message="Account hierarchy limit (100 levels) exceeded."
            )
        else:
            raise MidazAPIError(code=code, title=title, message=message)

    return response.json()


# Usage example
def handle_account_creation():
    try:
        account = create_account({
            "name": "New Account",
            "parent_account_id": "some-parent-uuid"
        })
        print(f"Account created: {account['id']}")
    except CircularHierarchyError as e:
        # User-actionable: display message to select different parent
        print(f"Please select a different parent account: {e.message}")
    except HierarchyTooDeepError as e:
        # User-actionable: suggest hierarchy restructuring  
        print(f"Consider restructuring your account hierarchy: {e.message}")
    except MidazAPIError as e:
        # Generic API error handling
        print(f"API error ({e.code}): {e.message}")
```

#### Retry and Backoff Recommendations

| Error Code | Retryable? | Recommended Action |
|------------|------------|-------------------|
| `0134` (Circular Hierarchy) | **No** | User intervention required - display error message and prompt selection of a different parent account |
| `0135` (Hierarchy Too Deep) | **No** | User intervention required - display error message suggesting account hierarchy restructuring |

**Important:** These are **validation errors**, not transient failures. Do not implement automatic retry with backoff for these error codes. Instead:

1. **Display clear user-facing messages** explaining why the operation failed
2. **Provide actionable guidance** (e.g., "Choose a different parent account")
3. **Log the error** for debugging with the full error payload
4. **Consider UI hints** - disable parent account options that would create cycles if detectable client-side

#### Testing Guidance

To properly test your error handling for these new error cases:

**1. Circular Hierarchy Test Case:**
```bash
# Create account A, then B with parent A, then try to update A with parent B
curl -X POST /accounts -d '{"name": "A"}' # Returns id: "aaa..."
curl -X POST /accounts -d '{"name": "B", "parent_account_id": "aaa..."}' # Returns id: "bbb..."
curl -X PATCH /accounts/aaa... -d '{"parent_account_id": "bbb..."}'
# Expected: 400 with code "0134"
```

**2. Hierarchy Depth Test Case:**
```bash
# Create a chain of 100+ accounts, then try to add one more level
# This requires programmatic setup - see integration tests for examples
# Expected: 400 with code "0135"
```

**3. Mock Server Responses for Unit Tests:**
```typescript
// Jest/Vitest example
const mockCircularError = {
  status: 400,
  body: {
    code: '0134',
    title: 'Circular Account Hierarchy Detected',
    message: 'Setting this parent account would create a circular reference in the account hierarchy.'
  }
};

const mockDepthError = {
  status: 400,
  body: {
    code: '0135', 
    title: 'Account Hierarchy Too Deep',
    message: 'The account hierarchy exceeds the maximum allowed depth of 100 levels.'
  }
};

// Test your error handling maps these to the correct typed exceptions
```

**4. Integration Test Checklist:**
- [ ] Verify circular hierarchy error is returned when creating cycle
- [ ] Verify depth limit error is returned at 101+ levels
- [ ] Verify error response structure matches expected schema
- [ ] Verify your client correctly maps error codes to typed errors/exceptions
- [ ] Verify user-facing error messages are displayed correctly
