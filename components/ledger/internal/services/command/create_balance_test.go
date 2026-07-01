// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	midazpkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func setupCreateBalanceUseCase(t *testing.T) (*UseCase, *balance.MockRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	return &UseCase{
		BalanceRepo: mockBalanceRepo,
	}, mockBalanceRepo
}

func TestCreateDefaultBalance(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	t.Run("creates default balance", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		// Defensive duplicate guard: no default balance should exist yet.
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, "test-alias", b.Alias)
				assert.Equal(t, constant.DefaultBalanceKey, b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "USD", b.AssetCode)
				assert.Equal(t, "deposit", b.AccountType)
				assert.True(t, b.AllowSending)
				assert.True(t, b.AllowReceiving)
				assert.Equal(t, constant.DirectionCredit, b.Direction)
				return b, nil
			}).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constant.DefaultBalanceKey, result.Key)
		assert.Equal(t, "test-alias", result.Alias)
	})

	t.Run("ignores caller-supplied non-default key and always writes default", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "  UPPER-CASE-KEY  ", // ignored — function hardcodes default
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				assert.Equal(t, constant.DefaultBalanceKey, b.Key)
				return b, nil
			}).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, constant.DefaultBalanceKey, result.Key)
	})

	t.Run("error checking duplicate", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		expectedErr := errors.New("database connection error")
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, expectedErr).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("default balance already exists", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)

		var conflictErr midazpkg.EntityConflictError
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrDuplicatedAliasKeyValue.Error(), conflictErr.Code)
	})

	t.Run("error creating balance", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, nil).
			Times(1)

		expectedErr := errors.New("database error")
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, expectedErr).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("verifies balance properties on creation", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			RequestID:      "req-123",
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "my-alias",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			AccountType:    "savings",
			AllowSending:   false,
			AllowReceiving: false,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) (*mmodel.Balance, error) {
				// Verify all input fields are mapped correctly
				assert.NotEmpty(t, b.ID)
				assert.Equal(t, "my-alias", b.Alias)
				assert.Equal(t, constant.DefaultBalanceKey, b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "BRL", b.AssetCode)
				assert.Equal(t, "savings", b.AccountType)
				assert.False(t, b.AllowSending)
				assert.False(t, b.AllowReceiving)
				assert.Equal(t, constant.DirectionCredit, b.Direction)
				assert.False(t, b.CreatedAt.IsZero())
				assert.False(t, b.UpdatedAt.IsZero())
				return b, nil
			}).
			Times(1)

		result, err := uc.CreateDefaultBalance(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "my-alias", result.Alias)
		assert.Equal(t, "BRL", result.AssetCode)
		assert.Equal(t, "savings", result.AccountType)
	})
}
