//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestIntegration_BalanceRepository_AccountBlocked_PersistsAndReadsBack proves
// the column is written by Create and projected by every read path that shares
// balanceColumnList. Without the column in the INSERT the flag would silently
// fall back to the schema default on every newly created balance.
func TestIntegration_BalanceRepository_AccountBlocked_PersistsAndReadsBack(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	ctx := context.Background()

	blocked := &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@blocked-persist",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		AccountBlocked: true,
	}

	created, err := repo.Create(ctx, blocked)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.True(t, created.AccountBlocked, "Create must return the persisted block state")

	found, err := repo.Find(ctx, orgID, ledgerID, uuid.MustParse(created.ID))
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.True(t, found.AccountBlocked, "Find must project account_blocked from the row")

	listed, err := repo.ListByAliasesWithKeys(ctx, orgID, ledgerID, []string{"@blocked-persist#default"})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.True(t, listed[0].AccountBlocked,
		"the transaction-flow read path must carry the block state to the validator")
}

// TestIntegration_BalanceRepository_AccountBlocked_AllListPathsScanTheColumn
// sweeps the read paths that share balanceColumnList but had no coverage of
// their own. Every one of them scans positionally, so a scan site that missed
// the new column fails here with a scan-arity error rather than silently in
// production.
func TestIntegration_BalanceRepository_AccountBlocked_AllListPathsScanTheColumn(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	ctx := context.Background()

	created, err := repo.Create(ctx, &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@blocked-scan-sweep",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(42),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		AccountBlocked: true,
	})
	require.NoError(t, err)

	balanceID := uuid.MustParse(created.ID)

	t.Run("ListByAccountIDs", func(t *testing.T) {
		got, err := repo.ListByAccountIDs(ctx, orgID, ledgerID, []uuid.UUID{accountID})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].AccountBlocked)
	})

	t.Run("ListByIDs", func(t *testing.T) {
		got, err := repo.ListByIDs(ctx, orgID, ledgerID, []uuid.UUID{balanceID})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].AccountBlocked)
	})

	t.Run("ListByAccountID", func(t *testing.T) {
		got, err := repo.ListByAccountID(ctx, orgID, ledgerID, accountID)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].AccountBlocked)
	})

	t.Run("ListByAliases", func(t *testing.T) {
		got, err := repo.ListByAliases(ctx, orgID, ledgerID, []string{"@blocked-scan-sweep"})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].AccountBlocked)
	})

	t.Run("FindByAccountIDAndKey", func(t *testing.T) {
		got, err := repo.FindByAccountIDAndKey(ctx, orgID, ledgerID, accountID, "default")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.AccountBlocked)
	})
}

// TestIntegration_BalanceRepository_AccountBlocked_DefaultsToUnblocked locks
// the backward-compatible default: a row inserted without the column (every row
// that existed before migration 000036) reads as not blocked.
func TestIntegration_BalanceRepository_AccountBlocked_DefaultsToUnblocked(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	ctx := context.Background()

	// The fixture INSERT does not name account_blocked at all.
	balanceID := pgtestutil.CreateTestBalance(t, container.DB, orgID, ledgerID, accountID, pgtestutil.BalanceParams{
		Alias:          "@legacy-row",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(10),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})

	found, err := repo.Find(ctx, orgID, ledgerID, balanceID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.False(t, found.AccountBlocked,
		"a row predating the column must read as not blocked")
}

// TestIntegration_BalanceRepository_UpdateAccountBlockedByAccountID_PropagatesToEveryBalance
// proves consistency guarantee #1: one atomic UPDATE flips the projection on
// every live balance of the account, in both directions, and never touches a
// balance belonging to a different account.
func TestIntegration_BalanceRepository_UpdateAccountBlockedByAccountID_PropagatesToEveryBalance(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	targetAccountID := createTestAccountID()
	otherAccountID := createTestAccountID()
	ctx := context.Background()

	newBalance := func(accountID uuid.UUID, alias, key string) *mmodel.Balance {
		return &mmodel.Balance{
			ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          alias,
			Key:            key,
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}
	}

	for _, b := range []*mmodel.Balance{
		newBalance(targetAccountID, "@propagate", "default"),
		newBalance(targetAccountID, "@propagate", "savings"),
		newBalance(otherAccountID, "@bystander", "default"),
	} {
		_, err := repo.Create(ctx, b)
		require.NoError(t, err)
	}

	require.NoError(t, repo.UpdateAccountBlockedByAccountID(ctx, orgID, ledgerID, targetAccountID, true))

	blocked, err := repo.ListByAccountID(ctx, orgID, ledgerID, targetAccountID)
	require.NoError(t, err)
	require.Len(t, blocked, 2)

	for _, b := range blocked {
		assert.True(t, b.AccountBlocked, "every balance of the account must carry the block state")
	}

	bystanders, err := repo.ListByAccountID(ctx, orgID, ledgerID, otherAccountID)
	require.NoError(t, err)
	require.Len(t, bystanders, 1)
	assert.False(t, bystanders[0].AccountBlocked, "a sibling account must never be touched")

	// The reverse direction has to converge just as completely.
	require.NoError(t, repo.UpdateAccountBlockedByAccountID(ctx, orgID, ledgerID, targetAccountID, false))

	unblocked, err := repo.ListByAccountID(ctx, orgID, ledgerID, targetAccountID)
	require.NoError(t, err)
	require.Len(t, unblocked, 2)

	for _, b := range unblocked {
		assert.False(t, b.AccountBlocked, "unblock must clear the projection on every balance")
	}
}

// TestIntegration_BalanceRepository_UpdateAccountBlockedByAccountID_IsIdempotent
// pins the retry contract: repeating the same desired state is a success, and
// an account with no balances is a success too. Either one erroring would make
// the command's convergence retry impossible.
func TestIntegration_BalanceRepository_UpdateAccountBlockedByAccountID_IsIdempotent(t *testing.T) {
	container := pgtestutil.SetupMigratedContainer(t, "transaction")
	repo := createRepository(t, container)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := createTestAccountID()
	ctx := context.Background()

	_, err := repo.Create(ctx, &mmodel.Balance{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@idempotent",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1),
		OnHold:         decimal.Zero,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateAccountBlockedByAccountID(ctx, orgID, ledgerID, accountID, true))
	require.NoError(t, repo.UpdateAccountBlockedByAccountID(ctx, orgID, ledgerID, accountID, true),
		"re-applying the same desired state must stay a success")

	// An account with zero balances is a legitimate convergence target: the
	// propagation has nothing to do and MUST NOT report a missing entity.
	require.NoError(t, repo.UpdateAccountBlockedByAccountID(ctx, orgID, ledgerID, createTestAccountID(), true),
		"an account with no balances must not fail the propagation")
}
