// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	midazpkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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

func TestCreateBalance(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New().String()
	alias := "test-alias"

	t.Run("creates balance from valid queue data", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		account := mmodel.Account{
			ID:             accountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			Type:           "deposit",
			AssetCode:      "USD",
			Alias:          &alias,
		}

		accountBytes, _ := json.Marshal(account)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(accountID),
				Value: accountBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(accountID),
			QueueData:      queueData,
		}

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, alias, b.Alias)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID, b.AccountID)
				assert.Equal(t, "USD", b.AssetCode)
				assert.Equal(t, "deposit", b.AccountType)
				assert.True(t, b.AllowSending)
				assert.True(t, b.AllowReceiving)
				return nil
			}).
			Times(1)

		err := uc.CreateBalance(ctx, queue)

		assert.NoError(t, err)
	})

	t.Run("balance already exists", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		account := mmodel.Account{
			ID:             accountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			Type:           "deposit",
			AssetCode:      "USD",
			Alias:          &alias,
		}

		accountBytes, _ := json.Marshal(account)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(accountID),
				Value: accountBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(accountID),
			QueueData:      queueData,
		}

		pgErr := &pgconn.PgError{Code: "23505"}
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(pgErr).
			Times(1)

		err := uc.CreateBalance(ctx, queue)

		assert.NoError(t, err)
	})

	t.Run("error creating balance", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		account := mmodel.Account{
			ID:             accountID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			Type:           "deposit",
			AssetCode:      "USD",
			Alias:          &alias,
		}

		accountBytes, _ := json.Marshal(account)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(accountID),
				Value: accountBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(accountID),
			QueueData:      queueData,
		}

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(errors.New("database error")).
			Times(1)

		err := uc.CreateBalance(ctx, queue)

		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())
	})

	t.Run("unmarshal error", func(t *testing.T) {
		uc, _ := setupCreateBalanceUseCase(t)

		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(accountID),
				Value: []byte("invalid json"),
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(accountID),
			QueueData:      queueData,
		}

		err := uc.CreateBalance(ctx, queue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("empty queue data returns nil", func(t *testing.T) {
		uc, _ := setupCreateBalanceUseCase(t)

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      uuid.MustParse(accountID),
			QueueData:      []mmodel.QueueData{},
		}

		err := uc.CreateBalance(ctx, queue)

		assert.NoError(t, err)
	})
}

func TestCreateBalanceSync(t *testing.T) {
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
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		// For default key, skip validation of default balance existence
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, "test-alias", b.Alias)
				assert.Equal(t, "default", b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "USD", b.AssetCode)
				assert.Equal(t, "deposit", b.AccountType)
				assert.True(t, b.AllowSending)
				assert.True(t, b.AllowReceiving)
				return nil
			}).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "default", result.Key)
		assert.Equal(t, "test-alias", result.Alias)
	})

	t.Run("normalizes key to lowercase and trims whitespace", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "  UPPER-CASE-KEY  ",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		// First check for default balance existence (non-default key)
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		// Then check for normalized key existence
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "upper-case-key").
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, "upper-case-key", b.Key)
				return nil
			}).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "upper-case-key", result.Key)
	})

	t.Run("error checking default balance existence", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
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

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("default balance not found", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(false, nil).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)

		var notFoundErr midazpkg.EntityNotFoundError
		assert.True(t, errors.As(err, &notFoundErr))
		assert.Equal(t, constant.ErrDefaultBalanceNotFound.Error(), notFoundErr.Code)
	})

	t.Run("additional balance not allowed for external account", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
			AssetCode:      "USD",
			AccountType:    constant.ExternalAccountType,
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)

		var validationErr midazpkg.ValidationError
		assert.True(t, errors.As(err, &validationErr))
		assert.Equal(t, constant.ErrAdditionalBalanceNotAllowed.Error(), validationErr.Code)
	})

	t.Run("error checking key existence", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		expectedErr := errors.New("database error")
		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "custom-key").
			Return(false, expectedErr).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("balance key already exists", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "custom-key").
			Return(true, nil).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.Error(t, err)
		assert.Nil(t, result)

		var conflictErr midazpkg.EntityConflictError
		assert.True(t, errors.As(err, &conflictErr))
		assert.Equal(t, constant.ErrDuplicatedAliasKeyValue.Error(), conflictErr.Code)
	})

	t.Run("creates non-default balance", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "custom-key",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   false,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, constant.DefaultBalanceKey).
			Return(true, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "custom-key").
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				assert.Equal(t, "test-alias", b.Alias)
				assert.Equal(t, "custom-key", b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "USD", b.AssetCode)
				assert.Equal(t, "deposit", b.AccountType)
				assert.False(t, b.AllowSending)
				assert.True(t, b.AllowReceiving)
				return nil
			}).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "custom-key", result.Key)
		assert.False(t, result.AllowSending)
		assert.True(t, result.AllowReceiving)
	})

	t.Run("error creating balance", func(t *testing.T) {
		uc, mockBalanceRepo := setupCreateBalanceUseCase(t)

		input := mmodel.CreateBalanceInput{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			Alias:          "test-alias",
			Key:            "default",
			AssetCode:      "USD",
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(false, nil).
			Times(1)

		expectedErr := errors.New("database error")
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(expectedErr).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

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
			Key:            "default",
			AssetCode:      "BRL",
			AccountType:    "savings",
			AllowSending:   false,
			AllowReceiving: false,
		}

		mockBalanceRepo.EXPECT().
			ExistsByAccountIDAndKey(gomock.Any(), organizationID, ledgerID, accountID, "default").
			Return(false, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				// Verify all input fields are mapped correctly
				assert.NotEmpty(t, b.ID)
				assert.Equal(t, "my-alias", b.Alias)
				assert.Equal(t, "default", b.Key)
				assert.Equal(t, organizationID.String(), b.OrganizationID)
				assert.Equal(t, ledgerID.String(), b.LedgerID)
				assert.Equal(t, accountID.String(), b.AccountID)
				assert.Equal(t, "BRL", b.AssetCode)
				assert.Equal(t, "savings", b.AccountType)
				assert.False(t, b.AllowSending)
				assert.False(t, b.AllowReceiving)
				assert.False(t, b.CreatedAt.IsZero())
				assert.False(t, b.UpdatedAt.IsZero())
				return nil
			}).
			Times(1)

		result, err := uc.CreateBalanceSync(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "my-alias", result.Alias)
		assert.Equal(t, "BRL", result.AssetCode)
		assert.Equal(t, "savings", result.AccountType)
	})
}
