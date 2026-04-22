// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestUpdateBalance_EnableOverdraft_AutoCreatesOverdraftBalance verifies that
// flipping AllowOverdraft from false to true triggers the auto-creation of
// a system-managed "overdraft" balance with direction=debit and scope=internal.
func TestUpdateBalance_EnableOverdraft_AutoCreatesOverdraftBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.New()

	current := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@fresh",
		Key:            "default",
		AssetCode:      "USD",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
		Settings:       mmodel.NewDefaultBalanceSettings(),
	}

	update := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("500.00"),
		},
	}

	updated := *current
	updated.Settings = update.Settings

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		AnyTimes()

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, balanceID, update).
		Return(&updated, nil).
		Times(1)

	// No existing overdraft balance yet. The repository signals "not
	// found" by returning an EntityNotFoundError (mirroring the real
	// PostgreSQL adapter which maps sql.ErrNoRows to this error), and
	// the use case MUST treat it as the trigger for auto-creation.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "overdraft").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	// Auto-creation MUST happen with direction=debit, scope=internal.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, "overdraft", b.Key,
				"auto-created balance MUST use the overdraft key")
			assert.Equal(t, constant.DirectionDebit, b.Direction,
				"overdraft balance MUST have debit direction")
			assert.Equal(t, accountID.String(), b.AccountID,
				"overdraft balance MUST belong to the same account")
			assert.Equal(t, current.AssetCode, b.AssetCode,
				"overdraft balance MUST match the parent asset code")
			require.NotNil(t, b.Settings,
				"overdraft balance MUST carry settings")
			assert.Equal(t, mmodel.BalanceScopeInternal, b.Settings.BalanceScope,
				"overdraft balance MUST be scoped as internal")
			return b, nil
		}).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		AnyTimes()

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.NoError(t, err)
	require.NotNil(t, result)
}

// TestUpdateBalance_EnableOverdraft_Idempotent verifies that re-enabling
// overdraft on a balance that already has a system overdraft balance does
// NOT create a duplicate — the operation MUST be idempotent.
func TestUpdateBalance_EnableOverdraft_Idempotent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.New()

	current := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@fresh",
		Key:            "default",
		AssetCode:      "USD",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
		Settings:       mmodel.NewDefaultBalanceSettings(),
	}

	existingOverdraft := &mmodel.Balance{
		ID:             uuid.New().String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@fresh",
		Key:            "overdraft",
		AssetCode:      "USD",
		Direction:      constant.DirectionDebit,
		OverdraftUsed:  decimal.Zero,
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	update := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("500.00"),
		},
	}

	updated := *current
	updated.Settings = update.Settings

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		AnyTimes()

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, balanceID, update).
		Return(&updated, nil).
		Times(1)

	// Overdraft balance already exists — the use case MUST detect it.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "overdraft").
		Return(existingOverdraft, nil).
		Times(1)

	// Create MUST NOT be called when the overdraft balance already exists.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Times(0)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		AnyTimes()

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.NoError(t, err, "idempotent enable MUST succeed without errors")
	require.NotNil(t, result)
}
