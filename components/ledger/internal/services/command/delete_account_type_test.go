// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	onbMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAccountTypeByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Delete(gomock.Any(), constant.EntityAccountType, id.String()).
		Return(nil).
		Times(1)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	assert.NoError(t, err)
}

func TestDeleteAccountTypeByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Delete(gomock.Any(), constant.EntityAccountType, id.String()).
		Times(0)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	expectedErr := pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, constant.EntityAccountType)

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	assert.Error(t, err)
	assert.Equal(t, expectedErr.Error(), err.Error())
}

func TestDeleteAccountTypeByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Delete(gomock.Any(), constant.EntityAccountType, id.String()).
		Times(0)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	expectedErr := errors.New("repository error")

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(expectedErr).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// The account type is already deleted when its metadata soft delete keeps failing, so the request still succeeds.
func TestDeleteAccountTypeByID_MetadataSoftDeleteFailureDoesNotFailTheDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	mockMetadataRepo := onbMongo.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Delete(gomock.Any(), constant.EntityAccountType, id.String()).
		Return(errors.New("mongo unavailable")).
		Times(3)

	uc := &UseCase{
		AccountTypeRepo:        mockAccountTypeRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
		metadataDeleteRetry:    fastMetadataDeleteRetryPolicy(),
	}

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	assert.NoError(t, err)
}
