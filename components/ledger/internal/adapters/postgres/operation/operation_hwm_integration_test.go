//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

import (
	"context"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// hwmOperation builds one balance-affecting operation row for the high-water-mark
// tests. Callers vary only what the scenario is about: which balance it belongs to,
// which version it left behind, and when it happened.
func hwmOperation(ids testIDs, balanceID uuid.UUID, versionAfter int64, availableAfter, onHoldAfter decimal.Decimal, overdraftAfter string, createdAt time.Time) *Operation {
	amount := decimal.NewFromInt(10)
	availableBefore := availableAfter
	onHoldBefore := onHoldAfter
	versionBefore := versionAfter - 1

	return &Operation{
		ID:              uuid.Must(libCommons.GenerateUUIDv7()).String(),
		TransactionID:   ids.TransactionID.String(),
		Description:     "High-water mark fixture",
		Type:            "CREDIT",
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
		BalanceID:       balanceID.String(),
		OrganizationID:  ids.OrgID.String(),
		LedgerID:        ids.LedgerID.String(),
		BalanceAffected: true,
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
		Snapshot: mmodel.OperationSnapshot{
			OverdraftUsedBefore: "0",
			OverdraftUsedAfter:  overdraftAfter,
		},
	}
}

// hwmBaseTime is a fixed instant the scenarios offset from, so ordering is
// deterministic and no assertion depends on the wall clock.
var hwmBaseTime = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// TestIntegration_ListLatestByBalances_MixedBatch covers a batch where one balance has
// a trail, one has none, and one has only soft-deleted operations.
func TestIntegration_ListLatestByBalances_MixedBatch(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	// One balance per key: the account may hold only one balance per asset and key.
	balanceParams := pgtestutil.DefaultBalanceParams()
	balanceParams.Alias = "@test-balance"
	balanceParams.Key = "hwm-untouched"
	untouchedBalanceID := pgtestutil.CreateTestBalance(t, container.DB, ids.OrgID, ids.LedgerID, ids.AccountID, balanceParams)

	balanceParams.Key = "hwm-deleted"
	deletedBalanceID := pgtestutil.CreateTestBalance(t, container.DB, ids.OrgID, ids.LedgerID, ids.AccountID, balanceParams)

	// Three versions of the same balance; the newest one is the high-water mark.
	for _, version := range []int64{2, 3, 4} {
		available := decimal.NewFromInt(100 * version)
		onHold := decimal.NewFromInt(version)

		_, err := repo.Create(ctx, hwmOperation(ids, ids.BalanceID, version,
			available, onHold, "0", hwmBaseTime.Add(time.Duration(version)*time.Minute)))
		require.NoError(t, err)
	}

	deletedOp, err := repo.Create(ctx, hwmOperation(ids, deletedBalanceID, 7,
		decimal.NewFromInt(700), decimal.Zero, "0", hwmBaseTime))
	require.NoError(t, err)
	require.NoError(t, repo.Delete(ctx, ids.OrgID, ids.LedgerID, uuid.MustParse(deletedOp.ID)))

	result, err := repo.ListLatestByBalances(ctx, ids.OrgID, ids.LedgerID, []BalanceHWMRef{
		{AccountID: ids.AccountID, BalanceID: ids.BalanceID},
		{AccountID: ids.AccountID, BalanceID: untouchedBalanceID},
		{AccountID: ids.AccountID, BalanceID: deletedBalanceID},
	})

	require.NoError(t, err)
	require.Len(t, result, 1, "only the balance with a live trail may appear")

	hwm := result[ids.BalanceID.String()]
	require.NotNil(t, hwm, "the balance with a trail must carry its high-water mark")
	require.NotNil(t, hwm.BalanceAfter.Version)
	assert.Equal(t, int64(4), *hwm.BalanceAfter.Version, "the newest operation must win")
	assert.True(t, decimal.NewFromInt(400).Equal(*hwm.BalanceAfter.Available),
		"the after-values must be the ones the winning operation left behind")
	assert.True(t, decimal.NewFromInt(4).Equal(*hwm.BalanceAfter.OnHold))

	assert.NotContains(t, result, untouchedBalanceID.String(), "a balance with no operation must be absent, not zero")
	assert.NotContains(t, result, deletedBalanceID.String(), "a soft-deleted trail must not resurface as a high-water mark")
}

// TestIntegration_ListLatestByBalances_SnapshotIsCarried proves the overdraft snapshot
// survives the lookup, which is what the seed guard rebuilds OverdraftUsed from.
func TestIntegration_ListLatestByBalances_SnapshotIsCarried(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	_, err := repo.Create(ctx, hwmOperation(ids, ids.BalanceID, 2,
		decimal.NewFromInt(150), decimal.NewFromInt(30), "20", hwmBaseTime))
	require.NoError(t, err)

	result, err := repo.ListLatestByBalances(ctx, ids.OrgID, ids.LedgerID, []BalanceHWMRef{
		{AccountID: ids.AccountID, BalanceID: ids.BalanceID},
	})

	require.NoError(t, err)

	hwm := result[ids.BalanceID.String()]
	require.NotNil(t, hwm)
	assert.Equal(t, "20", hwm.Snapshot.OverdraftUsedAfter)
	assert.True(t, decimal.NewFromInt(20).Equal(hwm.BalanceAfter.OverdraftUsed))
}

// TestIntegration_ListLatestByBalances_EmptyRefs proves an empty batch never reaches
// the database.
func TestIntegration_ListLatestByBalances_EmptyRefs(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	result, err := repo.ListLatestByBalances(context.Background(), ids.OrgID, ids.LedgerID, nil)

	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestIntegration_ListLatestByBalances_IgnoresNonBalanceAffecting proves an annotation
// row never becomes a high-water mark: it moved no money, so it left no balance state.
func TestIntegration_ListLatestByBalances_IgnoresNonBalanceAffecting(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	_, err := repo.Create(ctx, hwmOperation(ids, ids.BalanceID, 2,
		decimal.NewFromInt(150), decimal.Zero, "0", hwmBaseTime))
	require.NoError(t, err)

	annotation := hwmOperation(ids, ids.BalanceID, 9,
		decimal.NewFromInt(999), decimal.Zero, "0", hwmBaseTime.Add(time.Hour))
	annotation.BalanceAffected = false

	_, err = repo.Create(ctx, annotation)
	require.NoError(t, err)

	result, err := repo.ListLatestByBalances(ctx, ids.OrgID, ids.LedgerID, []BalanceHWMRef{
		{AccountID: ids.AccountID, BalanceID: ids.BalanceID},
	})

	require.NoError(t, err)

	hwm := result[ids.BalanceID.String()]
	require.NotNil(t, hwm)
	require.NotNil(t, hwm.BalanceAfter.Version)
	assert.Equal(t, int64(2), *hwm.BalanceAfter.Version,
		"the newer annotation row must not override the last balance-affecting operation")
}

// TestIntegration_ListLatestByBalances_ScopeIsolation proves the scope filters hold: an
// identical balance reference under another organization or ledger returns nothing.
func TestIntegration_ListLatestByBalances_ScopeIsolation(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	_, err := repo.Create(ctx, hwmOperation(ids, ids.BalanceID, 2,
		decimal.NewFromInt(150), decimal.Zero, "0", hwmBaseTime))
	require.NoError(t, err)

	refs := []BalanceHWMRef{{AccountID: ids.AccountID, BalanceID: ids.BalanceID}}

	otherOrg, err := repo.ListLatestByBalances(ctx, uuid.Must(libCommons.GenerateUUIDv7()), ids.LedgerID, refs)
	require.NoError(t, err)
	assert.Empty(t, otherOrg, "another organization must not read this trail")

	otherLedger, err := repo.ListLatestByBalances(ctx, ids.OrgID, uuid.Must(libCommons.GenerateUUIDv7()), refs)
	require.NoError(t, err)
	assert.Empty(t, otherLedger, "another ledger must not read this trail")

	otherAccount, err := repo.ListLatestByBalances(ctx, ids.OrgID, ids.LedgerID, []BalanceHWMRef{
		{AccountID: uuid.Must(libCommons.GenerateUUIDv7()), BalanceID: ids.BalanceID},
	})
	require.NoError(t, err)
	assert.Empty(t, otherAccount, "the account/balance pair must match as a pair")
}

// TestIntegration_ListLatestByBalances_UsesPointInTimeIndex proves the query shape can
// be served by idx_operation_account_balance_pit rather than scanning the table.
func TestIntegration_ListLatestByBalances_UsesPointInTimeIndex(t *testing.T) {
	container := pgtestutil.SetupContainer(t)
	repo := createRepository(t, container)
	ids := createTestDependencies(t, container)

	ctx := context.Background()

	// The trail of the balance under test, plus the traffic of many other accounts:
	// without the neighbours the predicate matches the whole table and any index that
	// satisfies the sort looks equally good, which says nothing about production.
	rows := make([]*Operation, 0, 4000)

	for version := int64(2); version <= 200; version++ {
		rows = append(rows, hwmOperation(ids, ids.BalanceID, version,
			decimal.NewFromInt(version), decimal.Zero, "0", hwmBaseTime.Add(time.Duration(version)*time.Second)))
	}

	for account := 0; account < 40; account++ {
		neighbour := ids
		neighbour.AccountID = uuid.Must(libCommons.GenerateUUIDv7())
		neighbourBalanceID := uuid.Must(libCommons.GenerateUUIDv7())

		for version := int64(2); version <= 96; version++ {
			rows = append(rows, hwmOperation(neighbour, neighbourBalanceID, version,
				decimal.NewFromInt(version), decimal.Zero, "0", hwmBaseTime.Add(time.Duration(version)*time.Second)))
		}
	}

	_, err := repo.CreateBulk(ctx, rows)
	require.NoError(t, err)

	_, err = container.DB.ExecContext(ctx, "ANALYZE operation")
	require.NoError(t, err)

	query, args, err := buildBalanceHWMQuery("operation", ids.OrgID, ids.LedgerID, []BalanceHWMRef{
		{AccountID: ids.AccountID, BalanceID: ids.BalanceID},
	})
	require.NoError(t, err)

	planRows, err := container.DB.QueryContext(ctx, "EXPLAIN "+query, args...)
	require.NoError(t, err)

	defer planRows.Close()

	var plan strings.Builder

	for planRows.Next() {
		var line string

		require.NoError(t, planRows.Scan(&line))
		plan.WriteString(line)
		plan.WriteString("\n")
	}

	require.NoError(t, planRows.Err())

	assert.Contains(t, plan.String(), "idx_operation_account_balance_pit",
		"the lookup must ride the point-in-time index:\n%s", plan.String())
	assert.NotContains(t, plan.String(), "Seq Scan",
		"an indexed lookup must not scan the operation table:\n%s", plan.String())
}
