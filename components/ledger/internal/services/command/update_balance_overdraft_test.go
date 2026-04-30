// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// strPtr returns a pointer to the given string, used for decimal string
// literals carried by BalanceSettings.OverdraftLimit.
func strPtr(s string) *string { return &s }

// TestUpdateBalance_WithSettings verifies that a valid Settings payload is
// forwarded to the repository and reflected on the returned balance.
func TestUpdateBalance_WithSettings(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	newSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        strPtr("1000.00"),
	}

	update := mmodel.UpdateBalance{Settings: newSettings}

	baseBalance := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@alice",
		Key:            "default",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.Zero,
	}

	existing := *baseBalance
	existing.Settings = mmodel.NewDefaultBalanceSettings()

	expected := *baseBalance
	expected.Settings = newSettings

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// The use case MUST read the current balance BEFORE applying the
	// update so it can enforce transition invariants (usage vs. limit).
	// Exactly one Find call is expected.
	mockBalanceRepo.EXPECT().Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(&existing, nil).Times(1)

	// Idempotency check for the auto-managed overdraft balance.
	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, gomock.Any(), "overdraft").
		Return(nil, nil).AnyTimes()

	mockBalanceRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) { return b, nil }).
		AnyTimes()

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, balanceID, update).
		Return(&expected, nil).Times(1)

	mockRedisRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
	// Cache settings are rewritten in-place after a settings update to make
	// the new overdraft configuration visible without discarding live
	// transactional state (Available, OnHold, Version, OverdraftUsed).
	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Settings,
		"Settings MUST be present on the updated balance")
	assert.True(t, result.Settings.AllowOverdraft,
		"AllowOverdraft MUST be persisted from the update payload")
	assert.True(t, result.Settings.OverdraftLimitEnabled)
	require.NotNil(t, result.Settings.OverdraftLimit)
	assert.Equal(t, "1000.00", *result.Settings.OverdraftLimit)
}

// TestUpdateBalance_SettingsValidation verifies that invalid settings are
// rejected before the repository is hit (HARD GATE — no partial writes).
func TestUpdateBalance_SettingsValidation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	cases := []struct {
		name     string
		settings *mmodel.BalanceSettings
	}{
		{
			name: "limit enabled without limit value",
			settings: &mmodel.BalanceSettings{
				AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: nil,
			},
		},
		{
			name: "limit disabled but limit value present",
			settings: &mmodel.BalanceSettings{
				AllowOverdraft: true, OverdraftLimitEnabled: false, OverdraftLimit: strPtr("100.00"),
			},
		},
		{
			name:     "invalid balance scope",
			settings: &mmodel.BalanceSettings{BalanceScope: "galactic"},
		},
		{
			name: "overdraft limit zero with flag enabled",
			settings: &mmodel.BalanceSettings{
				AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: strPtr("0"),
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockBalanceRepo := balance.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			// Repository MUST NOT receive a corrupt settings payload.
			mockBalanceRepo.EXPECT().
				Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Times(0)

			mockBalanceRepo.EXPECT().
				Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				AnyTimes().
				Return(&mmodel.Balance{
					ID:            balanceID.String(),
					Direction:     constant.DirectionCredit,
					OverdraftUsed: decimal.Zero,
					Settings:      mmodel.NewDefaultBalanceSettings(),
				}, nil)

			uc := UseCase{
				BalanceRepo:          mockBalanceRepo,
				TransactionRedisRepo: mockRedisRepo,
			}

			update := mmodel.UpdateBalance{Settings: tc.settings}

			result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

			require.Error(t, err, "invalid settings MUST produce a validation error")
			assert.Nil(t, result, "no balance should be returned when settings are invalid")
		})
	}
}

// TestUpdateBalance_CanDisableOverdraftWithUsage verifies that disabling
// overdraft is allowed while the balance is still carrying debt. The setting
// only blocks future overdraft usage; repayment credits must still be accepted.
func TestUpdateBalance_CanDisableOverdraftWithUsage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	current := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@indebted",
		Key:            "default",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.NewFromInt(250),
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("500.00"),
		},
	}

	update := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        false,
			OverdraftLimitEnabled: false,
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		Times(1)

	expected := *current
	expected.Settings = update.Settings

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, balanceID, update).
		Return(&expected, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return("", nil).
		Times(1)

	mockRedisRepo.EXPECT().
		UpdateBalanceCacheSettings(gomock.Any(), orgID, ledgerID, "@indebted#default", update.Settings).
		Return(nil).
		Times(1)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.NoError(t, err, "disabling overdraft with outstanding usage must be allowed")
	require.NotNil(t, result)
	require.NotNil(t, result.Settings)
	assert.False(t, result.Settings.AllowOverdraft)
	assert.Equal(t, decimal.NewFromInt(250), result.OverdraftUsed)
}

// TestUpdateBalance_CannotReduceLimitBelowUsage verifies that the limit
// cannot be reduced below the currently used overdraft amount.
func TestUpdateBalance_CannotReduceLimitBelowUsage(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	current := &mmodel.Balance{
		ID:             balanceID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "@indebted",
		Key:            "default",
		Direction:      constant.DirectionCredit,
		OverdraftUsed:  decimal.NewFromInt(400),
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("1000.00"),
		},
	}

	update := mmodel.UpdateBalance{
		Settings: &mmodel.BalanceSettings{
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        strPtr("100.00"),
		},
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	mockBalanceRepo.EXPECT().
		Find(gomock.Any(), orgID, ledgerID, balanceID).
		Return(current, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	uc := UseCase{
		BalanceRepo:          mockBalanceRepo,
		TransactionRedisRepo: mockRedisRepo,
	}

	result, err := uc.Update(context.TODO(), orgID, ledgerID, balanceID, update)

	require.Error(t, err, "reducing limit below current usage MUST fail")
	assert.Nil(t, result)
}
