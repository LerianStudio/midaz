package command

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateAccountTypeSuccess tests creating account type successfully
func TestCreateAccountTypeSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateAccountTypeInput{
		Name:        "Current Assets",
		Description: "Assets that are expected to be converted to cash within one year",
		KeyValue:    "current_assets",
	}

	now := time.Now()

	expectedAccountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
			assert.NotZero(t, accountType.CreatedAt)
			assert.NotZero(t, accountType.UpdatedAt)
			assert.Equal(t, accountType.CreatedAt, accountType.UpdatedAt)

			return expectedAccountType, nil
		}).
		Times(1)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.Equal(t, expectedAccountType, result)
}

// TestCreateAccountTypeError tests creating account type with database error
func TestCreateAccountTypeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	errMsg := "failed to create account type in database"
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateAccountTypeInput{
		Name:        "Fixed Assets",
		Description: "Long-term tangible assets",
		KeyValue:    "fixed_assets",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errors.New(errMsg)).
		Times(1)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Equal(t, errMsg, err.Error())
	assert.Nil(t, result)
}

// TestCreateAccountTypeValidatesInput tests that input fields are properly validated and set
func TestCreateAccountTypeValidatesInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	testCases := []struct {
		name    string
		payload *mmodel.CreateAccountTypeInput
	}{
		{
			name: "all fields set",
			payload: &mmodel.CreateAccountTypeInput{
				Name:        "Revenue Accounts",
				Description: "Accounts that track revenue streams",
				KeyValue:    "revenue_accounts",
			},
		},
		{
			name: "minimal required fields",
			payload: &mmodel.CreateAccountTypeInput{
				Name:     "Expense Accounts",
				KeyValue: "expense_accounts",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

			uc := UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
			}

			mockAccountTypeRepo.EXPECT().
				Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				DoAndReturn(func(ctx context.Context, orgID, ledID interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
					assert.Equal(t, tc.payload.Name, accountType.Name)
					assert.Equal(t, tc.payload.Description, accountType.Description)
					assert.Equal(t, tc.payload.KeyValue, accountType.KeyValue)
					assert.Equal(t, organizationID, accountType.OrganizationID)
					assert.Equal(t, ledgerID, accountType.LedgerID)
					assert.NotZero(t, accountType.ID)
					assert.NotZero(t, accountType.CreatedAt)
					assert.NotZero(t, accountType.UpdatedAt)

					return accountType, nil
				}).
				Times(1)

			result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, tc.payload)

			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

// TestCreateAccountTypeDuplicateKeyValue tests creating account type with a duplicate key value
func TestCreateAccountTypeDuplicateKeyValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateAccountTypeInput{
		Name:        "Existing Account Type",
		Description: "This key value already exists",
		KeyValue:    "existing_key_value",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	expectedErr := pkg.ValidateBusinessError(constant.ErrDuplicateAccountTypeKeyValue, reflect.TypeOf(mmodel.AccountType{}).Name())

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, expectedErr).
		Times(1)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Nil(t, result)
}

// TestCreateAccountTypeWithMetadata tests creating account type with metadata successfully
func TestCreateAccountTypeWithMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateAccountTypeInput{
		Name:        "Liability Accounts",
		Description: "Accounts that track liabilities",
		KeyValue:    "liability_accounts",
		Metadata: map[string]any{
			"category": "balance_sheet",
			"order":    2,
		},
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	expectedMetadata := map[string]any{
		"category": "balance_sheet",
		"order":    2,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedAccountType, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, entityType string, meta *mongodb.Metadata) error {
			assert.Equal(t, reflect.TypeOf(mmodel.AccountType{}).Name(), entityType)
			assert.Equal(t, expectedAccountType.ID.String(), meta.EntityID)
			assert.Equal(t, expectedMetadata["category"], meta.Data["category"])
			assert.Equal(t, expectedMetadata["order"], meta.Data["order"])
			return nil
		}).
		Times(1)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedMetadata, result.Metadata)
}

// TestCreateAccountTypeMetadataError tests creating account type with metadata creation error
func TestCreateAccountTypeMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()

	payload := &mmodel.CreateAccountTypeInput{
		Name:        "Equity Accounts",
		Description: "Accounts that track equity",
		KeyValue:    "equity_accounts",
		Metadata: map[string]any{
			"category": "equity",
		},
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       payload.KeyValue,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	metadataErr := errors.New("failed to create metadata")

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedAccountType, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(metadataErr).
		Times(1)

	result, err := uc.CreateAccountType(context.Background(), organizationID, ledgerID, payload)

	assert.Error(t, err)
	assert.Equal(t, metadataErr, err)
	assert.Nil(t, result)
}
