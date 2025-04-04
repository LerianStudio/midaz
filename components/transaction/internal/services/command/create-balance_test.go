package command

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestCreateBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		BalanceRepo: mockBalanceRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New().String()
	alias := "test-alias"

	t.Run("success", func(t *testing.T) {
		// Create test account
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

		// Mock BalanceRepo.Create
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, b *mmodel.Balance) error {
				// Verify balance properties
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

		// Call the method
		err := uc.CreateBalance(ctx, queue)

		// Assertions
		assert.NoError(t, err)
	})

	t.Run("balance already exists", func(t *testing.T) {
		// Create test account
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

		// Mock BalanceRepo.Create with duplicate key error
		pgErr := &pgconn.PgError{Code: "23505"}
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(pgErr).
			Times(1)

		// Call the method
		err := uc.CreateBalance(ctx, queue)

		// Assertions
		assert.NoError(t, err) // Should not return error for duplicate balance
	})

	t.Run("error creating balance", func(t *testing.T) {
		// Create test account
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

		// Mock BalanceRepo.Create with error
		mockBalanceRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(errors.New("database error")).
			Times(1)

		// Call the method
		err := uc.CreateBalance(ctx, queue)

		// Assertions
		assert.Error(t, err)
		assert.Equal(t, "database error", err.Error())
	})

	t.Run("unmarshal error", func(t *testing.T) {
		// Invalid JSON data
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

		// Call the method
		err := uc.CreateBalance(ctx, queue)

		// Assertions
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}
