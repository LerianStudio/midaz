// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOperationByID tests getting an operation by ID successfully with metadata
func TestGetOperationByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		TransactionID:  transactionID.String(),
	}

	expectedMetadata := &mongodb.Metadata{
		EntityID:   operationID.String(),
		EntityName: reflect.TypeOf(operation.Operation{}).Name(),
		Data:       mongodb.JSON{"key": "value"},
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, transactionID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), operationID.String()).
		Return(expectedMetadata, nil).
		Times(1)

	result, err := uc.GetOperationByID(context.Background(), organizationID, ledgerID, transactionID, operationID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedOperation.ID, result.ID)
	assert.Equal(t, expectedOperation.OrganizationID, result.OrganizationID)
	assert.Equal(t, map[string]any{"key": "value"}, result.Metadata)
}

// TestGetOperationByID_WithoutMetadata tests getting an operation by ID successfully without metadata
func TestGetOperationByID_WithoutMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		TransactionID:  transactionID.String(),
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, transactionID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), operationID.String()).
		Return(nil, nil).
		Times(1)

	result, err := uc.GetOperationByID(context.Background(), organizationID, ledgerID, transactionID, operationID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedOperation.ID, result.ID)
	assert.Nil(t, result.Metadata)
}

// TestGetOperationByID_ErrorOperationRepo tests getting an operation by ID with operation repository error
func TestGetOperationByID_ErrorOperationRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()
	expectedError := errors.New("database error")

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, transactionID, operationID).
		Return(nil, expectedError).
		Times(1)

	result, err := uc.GetOperationByID(context.Background(), organizationID, ledgerID, transactionID, operationID)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
	assert.Nil(t, result)
}

// TestGetOperationByID_ErrorMetadataRepo tests getting an operation by ID with metadata repository error
func TestGetOperationByID_ErrorMetadataRepo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()
	metadataError := errors.New("metadata database error")

	expectedOperation := &operation.Operation{
		ID:             operationID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		TransactionID:  transactionID.String(),
	}

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, transactionID, operationID).
		Return(expectedOperation, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), reflect.TypeOf(operation.Operation{}).Name(), operationID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.GetOperationByID(context.Background(), organizationID, ledgerID, transactionID, operationID)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestGetOperationByID_NilOperation tests getting an operation by ID when operation is nil
func TestGetOperationByID_NilOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationID := libCommons.GenerateUUIDv7()
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	mockOperationRepo.EXPECT().
		Find(gomock.Any(), organizationID, ledgerID, transactionID, operationID).
		Return(nil, nil).
		Times(1)

	// Metadata should not be called when operation is nil

	result, err := uc.GetOperationByID(context.Background(), organizationID, ledgerID, transactionID, operationID)

	assert.NoError(t, err)
	assert.Nil(t, result)
}
