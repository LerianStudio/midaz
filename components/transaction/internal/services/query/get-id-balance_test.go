package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/mock/gomock"
	"reflect"
	"testing"
	"time"
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

	// Create an ObjectID for the metadata
	objectID, _ := primitive.ObjectIDFromHex("507f1f77bcf86cd799439011")

	metadata := &mongodb.Metadata{
		ID:         objectID,
		EntityID:   id.String(),
		EntityName: reflect.TypeOf(mmodel.Balance{}).Name(),
		Data: mongodb.JSON{
			"custom_field": "custom_value",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Create mocks
	balanceRepo := balance.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)

	// Create use case with mocks
	uc := UseCase{
		BalanceRepo:  balanceRepo,
		MetadataRepo: metadataRepo,
	}

	// Test cases
	tests := []struct {
		name           string
		setupMocks     func()
		expectedResult *mmodel.Balance
		expectedError  error
	}{
		{
			name: "success_with_metadata",
			setupMocks: func() {
				// Setup BalanceRepo mock
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(balanceData, nil)

				// Setup MetadataRepo mock
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.Balance{}).Name(), id.String()).
					Return(metadata, nil)
			},
			expectedResult: &mmodel.Balance{
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
				Metadata: map[string]interface{}{
					"custom_field": "custom_value",
				},
			},
			expectedError: nil,
		},
		{
			name: "success_without_metadata",
			setupMocks: func() {
				// Setup BalanceRepo mock
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(balanceData, nil)

				// Setup MetadataRepo mock
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.Balance{}).Name(), id.String()).
					Return(nil, nil)
			},
			expectedResult: balanceData,
			expectedError:  nil,
		},
		{
			name: "error_finding_balance",
			setupMocks: func() {
				// Setup BalanceRepo mock with error
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(nil, errors.New("database error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("database error"),
		},
		{
			name: "error_finding_metadata",
			setupMocks: func() {
				// Setup BalanceRepo mock
				balanceRepo.EXPECT().
					Find(gomock.Any(), orgID, ledgerID, id).
					Return(balanceData, nil)

				// Setup MetadataRepo mock with error
				metadataRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.Balance{}).Name(), id.String()).
					Return(nil, errors.New("metadata error"))
			},
			expectedResult: nil,
			expectedError:  errors.New("metadata error"),
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
