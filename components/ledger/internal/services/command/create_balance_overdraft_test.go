// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// defaultBalanceForOverdraft builds a default balance fixture used as the
// reference account for additional-balance creation in these tests.
func defaultBalanceForOverdraft(orgID, ledgerID, accountID uuid.UUID, alias string) *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.New().String(),
		Alias:          alias,
		Key:            constant.DefaultBalanceKey,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		AssetCode:      "USD",
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// TestCreateAdditionalBalance_WithDirection verifies that when the caller
// provides Direction="debit", the created balance carries that direction
// instead of the default "credit".
func TestCreateAdditionalBalance_WithDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@debit-holder"

	direction := constant.DirectionDebit
	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "savings",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		Direction:      &direction,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "savings").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.DefaultBalanceKey).
		Return(defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias), nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, constant.DirectionDebit, b.Direction,
				"created balance MUST carry the requested direction")
			return b, nil
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, constant.DirectionDebit, result.Direction,
		"returned balance MUST expose the requested direction")
}

// TestCreateAdditionalBalance_DefaultDirection verifies that when Direction
// is omitted from the request, the created balance defaults to "credit".
func TestCreateAdditionalBalance_DefaultDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "@default-direction"

	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "reserve",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		// Direction intentionally nil
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, "reserve").
		Return(nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityBalance)).
		Times(1)

	mockBalanceRepo.EXPECT().
		FindByAccountIDAndKey(gomock.Any(), orgID, ledgerID, accountID, constant.DefaultBalanceKey).
		Return(defaultBalanceForOverdraft(orgID, ledgerID, accountID, alias), nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
			assert.Equal(t, constant.DirectionCredit, b.Direction,
				"missing direction MUST default to credit")
			return b, nil
		}).
		Times(1)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, constant.DirectionCredit, result.Direction,
		"returned balance MUST default to credit")
}

// TestCreateAdditionalBalance_InvalidDirection verifies that supplying an
// unsupported direction returns a validation error before any persistence
// is attempted.
func TestCreateAdditionalBalance_InvalidDirection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	invalid := "sideways"
	allow := true

	cbi := &mmodel.CreateAdditionalBalance{
		Key:            "weird",
		AllowSending:   &allow,
		AllowReceiving: &allow,
		Direction:      &invalid,
	}

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	// Create MUST NOT be called when direction is invalid.
	mockBalanceRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Times(0)

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "invalid direction MUST return an error")
	assert.Nil(t, result, "no balance should be returned on validation failure")
}

// TestCreateAdditionalBalance_ReservedKey verifies that the reserved
// "overdraft" key cannot be created through the public API — it is
// exclusively managed by the system when overdraft is enabled.
func TestCreateAdditionalBalance_ReservedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	allow := true

	cases := []struct {
		name string
		key  string
	}{
		{name: "lowercase", key: "overdraft"},
		{name: "uppercase", key: "OVERDRAFT"},
		{name: "mixed case", key: "Overdraft"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cbi := &mmodel.CreateAdditionalBalance{
				Key:            tc.key,
				AllowSending:   &allow,
				AllowReceiving: &allow,
			}

			mockBalanceRepo := balance.NewMockRepository(ctrl)

			// Create MUST NOT be invoked for the reserved key.
			mockBalanceRepo.EXPECT().
				Create(gomock.Any(), gomock.Any()).
				Times(0)

			uc := &UseCase{BalanceRepo: mockBalanceRepo}

			result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

			require.Error(t, err, "reserved key MUST be rejected")
			assert.Nil(t, result, "no balance should be returned when key is reserved")
		})
	}
}

// TestCreateAdditionalBalance_RejectsInternalScope verifies that clients
// cannot create balances with balanceScope="internal" through the public API.
func TestCreateAdditionalBalance_RejectsInternalScope(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	cbi := &mmodel.CreateAdditionalBalance{
		Key: "savings",
		Settings: &mmodel.BalanceSettings{
			BalanceScope: mmodel.BalanceScopeInternal,
		},
	}

	uc := &UseCase{BalanceRepo: mockBalanceRepo}

	result, err := uc.CreateAdditionalBalance(ctx, orgID, ledgerID, accountID, cbi)

	require.Error(t, err, "internal scope MUST be rejected on client creation")
	assert.Nil(t, result, "no balance should be returned when scope is internal")
	assert.Contains(t, err.Error(), constant.ErrInvalidBalanceSettings.Error(),
		"error must reference the invalid-settings code")
}
