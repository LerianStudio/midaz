//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var legacyDirectionTestTime = time.Date(2026, time.May, 21, 12, 0, 0, 0, time.UTC)

func legacyDirectionTestContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return ctx
}

func insertLegacyDirectionNullOperation(
	t *testing.T,
	container *pgtestutil.ContainerResult,
	ids testIDs,
	opType string,
) uuid.UUID {
	t.Helper()

	id := uuid.Must(libCommons.GenerateUUIDv7())
	amount := decimal.NewFromInt(100)
	available := decimal.NewFromInt(1000)
	zero := decimal.Zero

	_, err := container.DB.Exec(`
		INSERT INTO operation (
			id, transaction_id, description, type, account_id, account_alias,
			balance_id, balance_key, asset_code, chart_of_accounts, amount,
			available_balance, on_hold_balance, available_balance_after,
			on_hold_balance_after, balance_version_before, balance_version_after,
			status, balance_affected, organization_id, ledger_id, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
	`,
		id, ids.TransactionID, "legacy v3.5.3 fixture", opType,
		ids.AccountID, "@legacy-account", ids.BalanceID, "default",
		"USD", "default", amount, available, zero, available.Sub(amount), zero,
		int64(1), int64(2), "APPROVED", true,
		ids.OrgID, ids.LedgerID, legacyDirectionTestTime, legacyDirectionTestTime,
	)
	require.NoError(t, err)

	var direction sql.NullString
	require.NoError(t, container.DB.QueryRow(`SELECT direction FROM operation WHERE id = $1`, id).Scan(&direction))
	require.False(t, direction.Valid, "fixture must persist with direction = NULL")

	return id
}

func TestIntegration_OperationRepository_LegacyDirectionFallback(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)
	ctx := legacyDirectionTestContext(t)

	cases := []struct {
		opType  string
		wantDir string
	}{
		{constant.DEBIT, constant.DirectionDebit},
		{constant.CREDIT, constant.DirectionCredit},
		{constant.ONHOLD, constant.DirectionDebit},
		{constant.RELEASE, constant.DirectionCredit},
	}

	for _, tc := range cases {
		tc := tc
		t.Run("type_"+tc.opType, func(t *testing.T) {
			opID := insertLegacyDirectionNullOperation(t, container, ids, tc.opType)
			oppositeType, oppositeDirection := oppositeLegacyDirectionCase(tc.wantDir)
			oppositeID := insertLegacyDirectionNullOperation(t, container, ids, oppositeType)

			got, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, opID)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.wantDir, got.Direction)

			gotByAccount, err := repo.FindByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, opID)
			require.NoError(t, err)
			require.NotNil(t, gotByAccount)
			assert.Equal(t, tc.wantDir, gotByAccount.Direction)

			list, err := repo.ListByIDs(ctx, ids.OrgID, ids.LedgerID, []uuid.UUID{opID})
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, tc.wantDir, list[0].Direction)

			page := http.Pagination{
				Limit:     20,
				SortOrder: "ASC",
				StartDate: legacyDirectionTestTime.Add(-time.Hour),
				EndDate:   legacyDirectionTestTime.Add(time.Hour),
			}

			allByTransaction, _, err := repo.FindAll(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, page)
			require.NoError(t, err)
			assertOperationDirectionInList(t, allByTransaction, opID.String(), tc.wantDir)

			filter := OperationFilter{Direction: &tc.wantDir}
			allByAccount, _, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, filter, page)
			require.NoError(t, err)
			assertOperationDirectionInList(t, allByAccount, opID.String(), tc.wantDir)
			assertOperationNotInList(t, allByAccount, oppositeID.String(), oppositeDirection)
		})
	}
}

func oppositeLegacyDirectionCase(direction string) (string, string) {
	if direction == constant.DirectionDebit {
		return constant.CREDIT, constant.DirectionCredit
	}

	return constant.DEBIT, constant.DirectionDebit
}

func assertOperationDirectionInList(t *testing.T, operations []*Operation, operationID, wantDirection string) {
	t.Helper()

	for _, op := range operations {
		if op.ID == operationID {
			assert.Equal(t, wantDirection, op.Direction)
			return
		}
	}

	t.Fatalf("operation %s not returned", operationID)
}

func assertOperationNotInList(t *testing.T, operations []*Operation, operationID, direction string) {
	t.Helper()

	for _, op := range operations {
		if op.ID == operationID {
			t.Fatalf("operation %s with direction %s should not be returned", operationID, direction)
		}
	}
}

func TestIntegration_OperationRepository_DirectionPassthroughOnModernRow(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)
	ctx := legacyDirectionTestContext(t)

	amount := decimal.NewFromInt(75)
	available := decimal.NewFromInt(1000)
	zero := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	statusDesc := "modern row"

	op := &Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     "modern row passthrough",
		Type:            constant.DEBIT,
		AssetCode:       "USD",
		ChartOfAccounts: "1000",
		Amount:          Amount{Value: &amount},
		Balance:         Balance{Available: &available, OnHold: &zero, Version: &versionBefore},
		BalanceAfter:    Balance{Available: &available, OnHold: &zero, Version: &versionAfter},
		Status:          Status{Code: "APPROVED", Description: &statusDesc},
		AccountID:       ids.AccountID.String(),
		AccountAlias:    "@modern-account",
		BalanceKey:      "default",
		BalanceID:       ids.BalanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		Direction:       constant.DirectionCredit,
		CreatedAt:       legacyDirectionTestTime,
		UpdatedAt:       legacyDirectionTestTime,
	}

	created, err := repo.Create(ctx, op)
	require.NoError(t, err)
	require.NotNil(t, created)

	got, err := repo.Find(ctx, ids.OrgID, ids.LedgerID, ids.TransactionID, uuid.MustParse(created.ID))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, constant.DirectionCredit, got.Direction)

	page := http.Pagination{
		Limit:     10,
		SortOrder: "ASC",
		StartDate: legacyDirectionTestTime.Add(-time.Hour),
		EndDate:   legacyDirectionTestTime.Add(time.Hour),
	}
	list, _, err := repo.FindAllByAccount(ctx, ids.OrgID, ids.LedgerID, ids.AccountID, OperationFilter{}, page)
	require.NoError(t, err)
	require.NotEmpty(t, list)

	for _, item := range list {
		if item.ID == created.ID {
			assert.Equal(t, constant.DirectionCredit, item.Direction)
			return
		}
	}

	t.Fatal("created row not returned by FindAllByAccount")
}
