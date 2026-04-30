//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v3/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify the end-to-end snapshot round-trip:
// domain Operation.Snapshot → OperationPostgreSQLModel.Snapshot (JSONB bytes) →
// row in operation.snapshot column → scan → ToEntity → typed
// Balance.OverdraftUsed / BalanceAfter.OverdraftUsed.
//
// Companion files already cover pure-Go paths (ToEntity / FromEntity /
// parseDecimalOrZero) via unit tests; this file exercises the actual PG
// driver / JSONB column interaction to catch column-list drift, scan-site
// misalignment, and legacy row compatibility that unit tests cannot.
//
// Wire-shape contract: every operation row decodes to a fully populated
// Operation.Snapshot (value type, never nil) and Balance.OverdraftUsed /
// BalanceAfter.OverdraftUsed (decimal.Decimal, always set). Legacy rows
// (snapshot = '{}'), missing keys, and freshly-written non-overdraft ops all
// surface the same uniform wire shape.

// snapshotFixtureOperation builds an Operation suitable for Create() using the
// shared test dependencies. Snapshot is the only variant between scenarios;
// every other field is set to the same safe default as in
// operation.postgresql_integration_test.go.
func snapshotFixtureOperation(ids testIDs, now time.Time, snapshot mmodel.OperationSnapshot) *Operation {
	amount := decimal.NewFromInt(100)
	availableBefore := decimal.NewFromInt(1000)
	onHoldBefore := decimal.Zero
	availableAfter := decimal.NewFromInt(900)
	onHoldAfter := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)

	return &Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     "Snapshot round-trip test",
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amount},
		Balance: Balance{
			Available: &availableBefore,
			OnHold:    &onHoldBefore,
			Version:   &versionBefore,
		},
		BalanceAfter: Balance{
			Available: &availableAfter,
			OnHold:    &onHoldAfter,
			Version:   &versionAfter,
		},
		Status:          Status{Code: "APPROVED"},
		AccountID:       ids.AccountID.String(),
		AccountAlias:    "@test-account",
		BalanceKey:      "default",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		CreatedAt:       now,
		UpdatedAt:       now,
		Snapshot:        snapshot,
	}
}

// ============================================================================
// Create + Read Round-Trip — Active Overdraft (Both Fields Populated)
// ============================================================================

// TestIntegration_OperationSnapshot_CreateAndRead_NonZero verifies the happy
// path: an Operation with both overdraftUsedBefore and overdraftUsedAfter
// populated to non-zero values survives a PG round-trip with byte-level
// JSONB fidelity and typed decimal rehydration.
func TestIntegration_OperationSnapshot_CreateAndRead_NonZero(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	snapshot := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "50",
		OverdraftUsedAfter:  "130",
	}
	op := snapshotFixtureOperation(ids, now, snapshot)

	// Act: Create writes the snapshot as JSONB; Find reads it back.
	created, err := repo.Create(ctx, op)
	require.NoError(t, err, "Create should not return error")
	require.NotNil(t, created)

	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(created.ID))
	require.NoError(t, err, "Find should succeed")
	require.NotNil(t, found)

	// Assert: Snapshot struct round-trips with both fields intact.
	assert.Equal(t, "50", found.Snapshot.OverdraftUsedBefore, "OverdraftUsedBefore must preserve string literal")
	assert.Equal(t, "130", found.Snapshot.OverdraftUsedAfter, "OverdraftUsedAfter must preserve string literal")

	// Assert: typed decimal fields rehydrated from the snapshot strings.
	assert.True(t, decimal.NewFromInt(50).Equal(found.Balance.OverdraftUsed),
		"Balance.OverdraftUsed must equal decimal(50); got %s", found.Balance.OverdraftUsed.String())
	assert.True(t, decimal.NewFromInt(130).Equal(found.BalanceAfter.OverdraftUsed),
		"BalanceAfter.OverdraftUsed must equal decimal(130); got %s", found.BalanceAfter.OverdraftUsed.String())

	// Assert: the raw JSONB stored in the column matches the canonical shape.
	var rawSnapshot string
	err = container.DB.QueryRow(`SELECT snapshot::text FROM operation WHERE id = $1`, created.ID).Scan(&rawSnapshot)
	require.NoError(t, err, "direct JSONB read should succeed")
	assert.JSONEq(t, `{"overdraftUsedBefore":"50","overdraftUsedAfter":"130"}`, rawSnapshot,
		"raw JSONB column payload should match the canonical shape")
}

// ============================================================================
// Create + Read Round-Trip — Zero Shape (Non-Overdraft Op)
// ============================================================================

// TestIntegration_OperationSnapshot_CreateAndRead_ZeroShape verifies the
// dominant production case: a non-overdraft operation persists the always-
// populated zero shape. Under the new contract the JSONB column carries
// `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}` rather than `{}`,
// and the read path surfaces a fully populated Snapshot value with both
// typed Balance.OverdraftUsed / BalanceAfter.OverdraftUsed set to
// decimal.Zero.
func TestIntegration_OperationSnapshot_CreateAndRead_ZeroShape(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	zeroShape := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "0",
		OverdraftUsedAfter:  "0",
	}
	op := snapshotFixtureOperation(ids, now, zeroShape)

	// Act
	created, err := repo.Create(ctx, op)
	require.NoError(t, err)
	require.NotNil(t, created)

	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(created.ID))
	require.NoError(t, err)
	require.NotNil(t, found)

	// Assert: zero shape round-trips intact; both fields surface as "0".
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedBefore, "non-overdraft op carries '0' verbatim")
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedAfter)
	assert.True(t, found.Balance.OverdraftUsed.Equal(decimal.Zero), "no overdraft → Balance.OverdraftUsed is decimal.Zero")
	assert.True(t, found.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))

	// Assert: the JSONB column carries the always-populated zero shape (not '{}').
	var rawSnapshot string
	err = container.DB.QueryRow(`SELECT snapshot::text FROM operation WHERE id = $1`, created.ID).Scan(&rawSnapshot)
	require.NoError(t, err)
	assert.JSONEq(t, `{"overdraftUsedBefore":"0","overdraftUsedAfter":"0"}`, rawSnapshot,
		"non-overdraft snapshot must persist as the always-populated zero shape")
}

// ============================================================================
// Read-Many Consistency (FindAll)
// ============================================================================

// TestIntegration_OperationSnapshot_FindAll_MixedSnapshots verifies that
// reading multiple operations in a single FindAll call preserves each row's
// snapshot independently. Catches scan-site drift that would be hidden when
// every row happens to have the same snapshot.
func TestIntegration_OperationSnapshot_FindAll_MixedSnapshots(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	// Three operations: one with active overdraft, two with the zero shape.
	withOverdraft := snapshotFixtureOperation(ids, now, mmodel.OperationSnapshot{
		OverdraftUsedBefore: "25",
		OverdraftUsedAfter:  "75",
	})
	withOverdraft.Description = "has snapshot"

	zeroShape := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "0",
		OverdraftUsedAfter:  "0",
	}

	zeroA := snapshotFixtureOperation(ids, now.Add(time.Microsecond), zeroShape)
	zeroA.Description = "zero A"

	zeroB := snapshotFixtureOperation(ids, now.Add(2*time.Microsecond), zeroShape)
	zeroB.Description = "zero B"

	for _, toCreate := range []*Operation{withOverdraft, zeroA, zeroB} {
		_, err := repo.Create(ctx, toCreate)
		require.NoError(t, err, "Create should succeed for %q", toCreate.Description)
	}

	// Act
	operations, _, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, defaultPagination())
	require.NoError(t, err)
	assert.Len(t, operations, 3, "should return all three operations")

	// Assert: each operation's snapshot is correctly preserved after the scan.
	byDescription := make(map[string]*Operation, len(operations))
	for _, o := range operations {
		byDescription[o.Description] = o
	}

	require.Contains(t, byDescription, "has snapshot")
	gotWith := byDescription["has snapshot"]
	assert.Equal(t, "25", gotWith.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "75", gotWith.Snapshot.OverdraftUsedAfter)
	assert.True(t, decimal.NewFromInt(25).Equal(gotWith.Balance.OverdraftUsed))
	assert.True(t, decimal.NewFromInt(75).Equal(gotWith.BalanceAfter.OverdraftUsed))

	for _, description := range []string{"zero A", "zero B"} {
		require.Contains(t, byDescription, description)
		got := byDescription[description]
		assert.Equal(t, "0", got.Snapshot.OverdraftUsedBefore, "%q must carry zero shape", description)
		assert.Equal(t, "0", got.Snapshot.OverdraftUsedAfter)
		assert.True(t, got.Balance.OverdraftUsed.Equal(decimal.Zero), "%q Balance.OverdraftUsed must be zero", description)
		assert.True(t, got.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
	}
}

// ============================================================================
// Legacy Row Behavior
// ============================================================================

// TestIntegration_OperationSnapshot_LegacyRow verifies that a row inserted
// WITHOUT specifying the snapshot column — simulating an existing row from
// before the column was added — reads back with the always-populated zero
// shape on the entity. The column's NOT NULL DEFAULT '{}' ensures valid
// JSONB is always stored, and ToEntity's default-fill applies the zero
// shape when keys are absent.
//
// This is the critical backwards-compatibility guarantee — existing
// production rows from before the snapshot migration landed must produce
// exactly the same wire shape as freshly written non-overdraft ops.
func TestIntegration_OperationSnapshot_LegacyRow(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	// Use CreateTestOperation (which does NOT specify snapshot) to simulate a
	// row written before the snapshot column landed. The DB default
	// '{}'::jsonb applies.
	opParams := pgtestutil.OperationParams{
		TransactionID:         ids.TransactionID,
		Description:           "legacy row",
		Type:                  "DEBIT",
		AccountID:             ids.AccountID,
		AccountAlias:          "@test-account",
		BalanceID:             ids.BalanceID,
		AssetCode:             "USD",
		Amount:                decimal.NewFromInt(100),
		AvailableBalance:      decimal.NewFromInt(1000),
		OnHoldBalance:         decimal.Zero,
		AvailableBalanceAfter: decimal.NewFromInt(900),
		OnHoldBalanceAfter:    decimal.Zero,
		BalanceVersionBefore:  1,
		BalanceVersionAfter:   2,
		Status:                "APPROVED",
		BalanceAffected:       true,
	}
	opID := pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	// Sanity-check: the DB default actually produced '{}' in the column.
	var rawSnapshot string
	err := container.DB.QueryRow(`SELECT snapshot::text FROM operation WHERE id = $1`, opID).Scan(&rawSnapshot)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, rawSnapshot,
		"snapshot column NOT NULL DEFAULT should produce '{}' for rows without explicit snapshot")

	// Act
	found, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
	require.NoError(t, err)
	require.NotNil(t, found)

	// Assert: legacy '{}' decodes to the same shape as a fresh non-overdraft
	// op — uniform wire response regardless of row age.
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedBefore, "legacy row default-fills to '0'")
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedAfter)
	assert.True(t, found.Balance.OverdraftUsed.Equal(decimal.Zero))
	assert.True(t, found.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
}

// ============================================================================
// Point-in-Time Query
// ============================================================================

// TestIntegration_OperationSnapshot_PointInTime verifies that the point-in-time
// balance reconstruction path (FindLastOperationBeforeTimestamp, which uses
// the lean operationPointInTimeColumns scan list) correctly carries the
// snapshot through. The PIT model only populates BalanceAfter (the "after"
// view at the queried timestamp), so we only assert the After side.
//
// Without snapshot propagation on the PIT scan list, historical balance
// reconstruction would surface typed OverdraftUsed as decimal.Zero, defeating
// the audit-trail purpose of the snapshot column.
func TestIntegration_OperationSnapshot_PointInTime(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)

	snapshot := mmodel.OperationSnapshot{
		OverdraftUsedBefore: "100",
		OverdraftUsedAfter:  "250",
	}
	op := snapshotFixtureOperation(ids, now, snapshot)

	created, err := repo.Create(ctx, op)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Act: query the operation via the PIT path, using a timestamp after the
	// operation's createdAt. The PIT query orders by created_at DESC and
	// takes the latest matching row.
	found, err := repo.FindLastOperationBeforeTimestamp(
		ctx,
		ids.OrgID,
		ids.LedgerID,
		ids.AccountID,
		ids.BalanceID,
		now.Add(time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, found, "PIT query should find the operation")

	// Assert: the PIT model carries the snapshot through.
	assert.Equal(t, "250", found.Snapshot.OverdraftUsedAfter)
	// PIT path doesn't populate Before-side typed Balance.OverdraftUsed
	// (it reconstructs a single historical balance state, not a pre/post
	// pair). The snapshot string is still present because ToEntity decodes
	// it whole; it's just the typed field on Balance (not BalanceAfter)
	// that the PIT path leaves alone.
	assert.True(t, decimal.NewFromInt(250).Equal(found.BalanceAfter.OverdraftUsed),
		"PIT BalanceAfter.OverdraftUsed must equal decimal(250); got %s",
		found.BalanceAfter.OverdraftUsed.String())
}

// TestIntegration_OperationSnapshot_PointInTime_LegacyRow mirrors
// TestIntegration_OperationSnapshot_LegacyRow but through the PIT query
// path, guarding against scan-site drift on the narrower
// operationPointInTimeColumns list. Legacy rows decode to the always-
// populated zero shape on the PIT path too.
func TestIntegration_OperationSnapshot_PointInTime_LegacyRow(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	opParams := pgtestutil.OperationParams{
		TransactionID:         ids.TransactionID,
		Description:           "legacy PIT row",
		Type:                  "DEBIT",
		AccountID:             ids.AccountID,
		AccountAlias:          "@test-account",
		BalanceID:             ids.BalanceID,
		AssetCode:             "USD",
		Amount:                decimal.NewFromInt(100),
		AvailableBalance:      decimal.NewFromInt(1000),
		OnHoldBalance:         decimal.Zero,
		AvailableBalanceAfter: decimal.NewFromInt(900),
		OnHoldBalanceAfter:    decimal.Zero,
		BalanceVersionBefore:  1,
		BalanceVersionAfter:   2,
		Status:                "APPROVED",
		BalanceAffected:       true,
	}
	pgtestutil.CreateTestOperation(t, container.DB, ids.OrgID, ids.LedgerID, opParams)

	// Act
	found, err := repo.FindLastOperationBeforeTimestamp(
		ctx,
		ids.OrgID,
		ids.LedgerID,
		ids.AccountID,
		ids.BalanceID,
		time.Now().Add(time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, found, "PIT query should find the legacy row")

	// Assert: legacy row default-fills to the zero shape on the PIT path.
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedBefore)
	assert.Equal(t, "0", found.Snapshot.OverdraftUsedAfter)
	assert.True(t, found.BalanceAfter.OverdraftUsed.Equal(decimal.Zero))
}
