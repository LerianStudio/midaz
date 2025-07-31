package command

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateAccountTypeSuccess tests updating account type successfully
func TestUpdateAccountTypeSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()
	now := time.Now()

	payload := &mmodel.UpdateAccountTypeInput{
		Name:        "Updated Asset Type",
		Description: "Updated description for asset accounts",
		Metadata: map[string]any{
			"category": "updated_category",
			"priority": 1,
		},
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             accountTypeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       "asset_accounts", // This would come from the existing record
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Metadata will be merged: existing + new
	expectedMergedMetadata := map[string]any{
		"existing_key": "existing_value",
		"category":     "updated_category",
		"priority":     1,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, id interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
			assert.Equal(t, payload.Name, accountType.Name)
			assert.Equal(t, payload.Description, accountType.Description)
			return expectedAccountType, nil
		}).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), accountTypeID.String()).
		Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), expectedMergedMetadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedAccountType.Name, result.Name)
	assert.Equal(t, expectedAccountType.Description, result.Description)
	assert.Equal(t, expectedMergedMetadata, result.Metadata)
}

// TestUpdateAccountTypeSuccessWithoutMetadata tests updating account type successfully without metadata
func TestUpdateAccountTypeSuccessWithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()
	now := time.Now()

	payload := &mmodel.UpdateAccountTypeInput{
		Name:        "Updated Asset Type",
		Description: "Updated description for asset accounts",
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             accountTypeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		Description:    payload.Description,
		KeyValue:       "asset_accounts",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, id interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
			assert.Equal(t, payload.Name, accountType.Name)
			assert.Equal(t, payload.Description, accountType.Description)
			return expectedAccountType, nil
		}).
		Times(1)

	// UpdateMetadata is always called, even with empty metadata
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), accountTypeID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedAccountType.Name, result.Name)
	assert.Equal(t, expectedAccountType.Description, result.Description)
	assert.Equal(t, map[string]any{}, result.Metadata)
}

// TestUpdateAccountTypeError tests database error handling
func TestUpdateAccountTypeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()

	payload := &mmodel.UpdateAccountTypeInput{
		Name:        "Updated Asset Type",
		Description: "Updated description",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	expectedError := errors.New("database error")

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
}

// TestUpdateAccountTypeNotFound tests handling of account type not found
func TestUpdateAccountTypeNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()

	payload := &mmodel.UpdateAccountTypeInput{
		Name:        "Updated Asset Type",
		Description: "Updated description",
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	expectedErr := pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

// TestUpdateAccountTypeMetadataError tests handling metadata update error
func TestUpdateAccountTypeMetadataError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()
	now := time.Now()

	payload := &mmodel.UpdateAccountTypeInput{
		Name:     "Updated Asset Type",
		Metadata: map[string]any{"key": "value"},
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             accountTypeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           payload.Name,
		KeyValue:       "asset_accounts",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	metadataError := errors.New("metadata update failed")

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		Return(expectedAccountType, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), accountTypeID.String()).
		Return(&mongodb.Metadata{Data: map[string]any{"existing": "data"}}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(metadataError).
		Times(1)

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, metadataError, err)
}

// TestUpdateAccountTypePartialUpdate tests updating account type with partial input
func TestUpdateAccountTypePartialUpdate(t *testing.T) {
	tests := []struct {
		name    string
		payload *mmodel.UpdateAccountTypeInput
	}{
		{
			name: "only name provided",
			payload: &mmodel.UpdateAccountTypeInput{
				Name: "Updated Name Only",
			},
		},
		{
			name: "only description provided",
			payload: &mmodel.UpdateAccountTypeInput{
				Description: "Updated description only",
			},
		},
		{
			name: "both name and description provided",
			payload: &mmodel.UpdateAccountTypeInput{
				Name:        "Updated Name",
				Description: "Updated description",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			organizationID := libCommons.GenerateUUIDv7()
			ledgerID := libCommons.GenerateUUIDv7()
			accountTypeID := libCommons.GenerateUUIDv7()
			now := time.Now()

			expectedAccountType := &mmodel.AccountType{
				ID:             accountTypeID,
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Name:           tc.payload.Name,
				Description:    tc.payload.Description,
				KeyValue:       "test_key",
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)

			uc := UseCase{
				AccountTypeRepo: mockAccountTypeRepo,
				MetadataRepo:    mockMetadataRepo,
			}

			mockAccountTypeRepo.EXPECT().
				Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
				DoAndReturn(func(ctx context.Context, orgID, ledID, id interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
					assert.Equal(t, tc.payload.Name, accountType.Name)
					assert.Equal(t, tc.payload.Description, accountType.Description)
					return expectedAccountType, nil
				}).
				Times(1)

			mockMetadataRepo.EXPECT().
				Update(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), accountTypeID.String(), map[string]any{}).
				Return(nil).
				Times(1)

			result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, tc.payload)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, expectedAccountType, result)
		})
	}
}

// TestUpdateAccountTypeEmptyInput tests updating account type with empty input
func TestUpdateAccountTypeEmptyInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountTypeID := libCommons.GenerateUUIDv7()
	now := time.Now()

	payload := &mmodel.UpdateAccountTypeInput{
		// Empty input - no fields to update
	}

	expectedAccountType := &mmodel.AccountType{
		ID:             accountTypeID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           "",
		Description:    "",
		KeyValue:       "test_key",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	mockAccountTypeRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, accountTypeID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, id interface{}, accountType *mmodel.AccountType) (*mmodel.AccountType, error) {
			assert.Equal(t, "", accountType.Name)
			assert.Equal(t, "", accountType.Description)
			return expectedAccountType, nil
		}).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), reflect.TypeOf(mmodel.AccountType{}).Name(), accountTypeID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	result, err := uc.UpdateAccountType(context.Background(), organizationID, ledgerID, accountTypeID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedAccountType, result)
}
