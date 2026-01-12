package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAccountTypeByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	ctx := context.Background()

	t.Run("account type found with metadata", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		expectedAccountType := &mmodel.AccountType{
			ID:             accountTypeID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Test Account Type",
			KeyValue:       "test_account_type",
		}

		expectedMetadata := &mongodb.Metadata{
			Data: map[string]any{"key": "value"},
		}

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(expectedAccountType, nil)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AccountType", accountTypeID.String()).
			Return(expectedMetadata, nil)

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, accountTypeID, result.ID)
		assert.Equal(t, "Test Account Type", result.Name)
		assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
	})

	t.Run("account type found without metadata", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		expectedAccountType := &mmodel.AccountType{
			ID:             accountTypeID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Test Account Type",
			KeyValue:       "test_account_type",
		}

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(expectedAccountType, nil)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AccountType", accountTypeID.String()).
			Return(nil, nil)

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, accountTypeID, result.ID)
		assert.Equal(t, "Test Account Type", result.Name)
		assert.Nil(t, result.Metadata)
	})

	t.Run("error - account type not found", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(nil, services.ErrDatabaseItemNotFound)

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("error - account type repo failure", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(nil, errors.New("database connection error"))

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("error - metadata repo failure", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		expectedAccountType := &mmodel.AccountType{
			ID:             accountTypeID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Name:           "Test Account Type",
			KeyValue:       "test_account_type",
		}

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(expectedAccountType, nil)

		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), "AccountType", accountTypeID.String()).
			Return(nil, errors.New("metadata retrieval error"))

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}
