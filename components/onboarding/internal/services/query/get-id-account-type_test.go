// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

//nolint:funlen
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

		require.NoError(t, err)
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

		require.NoError(t, err)
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

		require.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("error - account type repo failure", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		accountTypeID := uuid.New()

		mockAccountTypeRepo.EXPECT().
			FindByID(gomock.Any(), organizationID, ledgerID, accountTypeID).
			Return(nil, errDatabaseConnectionError)

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		require.Error(t, err)
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
			Return(nil, errMetadataRetrievalError)

		result, err := uc.GetAccountTypeByID(ctx, organizationID, ledgerID, accountTypeID)

		require.Error(t, err)
		assert.Nil(t, result)
	})
}
