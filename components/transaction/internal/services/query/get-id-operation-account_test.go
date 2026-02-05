// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOperationByAccount_WithMetadata tests getting an operation by account successfully with metadata
func TestGetOperationByAccount_WithMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
	}

	expectedMetadata := &mongodb.Metadata{
		EntityID:   operationID.String(),
		EntityName: reflect.TypeFor[operation.Operation]().Name(),
		Data:       mongodb.JSON{"key": "value"},
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeFor[operation.Operation]().Name(), operationID.String()).
		Return(expectedMetadata, nil).
		Times(1)

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedOperation.ID, result.ID)
	assert.Equal(t, expectedOperation.OrganizationID, result.OrganizationID)
	assert.Equal(t, expectedOperation.AccountID, result.AccountID)
	assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
}

// TestGetOperationByAccount_WithoutMetadata tests getting an operation by account successfully without metadata
func TestGetOperationByAccount_WithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeFor[operation.Operation]().Name(), operationID.String()).
		Return(nil, nil).
		Times(1)

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedOperation.ID, result.ID)
	assert.Nil(t, result.Metadata)
}

// TestGetOperationByAccount_ErrorOperationRepo tests getting an operation by account with operation repository error
func TestGetOperationByAccount_ErrorOperationRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	expectedError := errors.New("database error")

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetOperationByAccount_NotFound tests getting an operation by account when not found (ErrDatabaseItemNotFound)
func TestGetOperationByAccount_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "No operations were found in the search")
}

// TestGetOperationByAccount_ErrorMetadataRepo tests getting an operation by account with metadata repository error
func TestGetOperationByAccount_ErrorMetadataRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()
	metadataError := errors.New("metadata database error")

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		AccountID:      accountID.String(),
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeFor[operation.Operation]().Name(), operationID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestGetOperationByAccount_NilOperation tests getting an operation by account when operation is nil
func TestGetOperationByAccount_NilOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	accountID := libCommons.GenerateUUIDv7()

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		FindByAccount(gomock.Any(), organizationID, ledgerID, accountID, operationID).
		Return(nil, nil).
		Times(1)

	// Metadata should not be called when operation is nil

	result, err := uc.GetOperationByAccount(context.Background(), organizationID, ledgerID, accountID, operationID)

	assert.NoError(t, err)
	assert.Nil(t, result)
}
