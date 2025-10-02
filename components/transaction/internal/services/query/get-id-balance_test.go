package query

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetBalanceByID(t *testing.T) {
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	balanceRes := &mmodel.Balance{
		ID:             ID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
	}

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(balanceRes, nil).
		Times(1)
	res, err := uc.BalanceRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.Equal(t, balanceRes, res)
	assert.Nil(t, err)
}

func TestGetBalanceIDError(t *testing.T) {
	errMSG := "err to get balance on database"
	ID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	uc := UseCase{
		BalanceRepo: balance.NewMockRepository(gomock.NewController(t)),
	}

	uc.BalanceRepo.(*balance.MockRepository).
		EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, ID).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.BalanceRepo.Find(context.TODO(), organizationID, ledgerID, ID)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}

func TestGetBalanceByIDUseCase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	id := libCommons.GenerateUUIDv7()
	orgID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	now := time.Now()

	balanceData := &mmodel.Balance{
		ID:             id.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
		Alias:          "@user1",
		AssetCode:      "USD",
		Available:      decimal.NewFromFloat(1000),
		OnHold:         decimal.NewFromFloat(200),
		Version:        1,
		AccountType:    "checking",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Create mocks
	balanceRepo := balance.NewMockRepository(ctrl)

	// Create use case with mocks
	uc := UseCase{
		BalanceRepo: balanceRepo,
	}

	// Test cases
	tests := []struct {
		name           string
		setupMocks     func()
		expectedResult *mmodel.Balance
		expectedError  error
	}{
		{
			name: "success",
			setupMocks: func() {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(balanceData, nil)
			},
			expectedResult: balanceData,
			expectedError:  nil,
		},
		{
			name: "error_finding_balance",
			setupMocks: func() {
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(nil, errors.New("database error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks for this test case
			tc.setupMocks()

			// Call the method being tested
			result, err := uc.GetBalanceByID(context.Background(), orgID, ledgerID, id)

			// Assert results
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
