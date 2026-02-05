// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateTransaction tests successful update of transaction with description and metadata
func TestUpdateTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	input := &transaction.UpdateTransactionInput{
		Description: "Updated description",
		Metadata: map[string]any{
			"key1": "value1",
		},
	}

	expectedTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Description:    "Updated description",
		UpdatedAt:      time.Now(),
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, &transaction.Transaction{
			Description: input.Description,
		}).
		Return(expectedTransaction, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "Transaction", transactionID.String(), input.Metadata).
		Return(nil).
		Times(1)

	result, err := uc.UpdateTransaction(context.Background(), organizationID, ledgerID, transactionID, input)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedTransaction.ID, result.ID)
	assert.Equal(t, input.Metadata, result.Metadata)
}

// TestUpdateTransaction_NotFound tests update when transaction is not found
func TestUpdateTransaction_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	input := &transaction.UpdateTransactionInput{
		Description: "Updated description",
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, &transaction.Transaction{
			Description: input.Description,
		}).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateTransaction(context.Background(), organizationID, ledgerID, transactionID, input)

	assert.Error(t, err)
	assert.Nil(t, result)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
}

// TestUpdateTransaction_RepositoryError tests update when repository returns generic error
func TestUpdateTransaction_RepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	databaseError := errors.New("database connection error")

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	input := &transaction.UpdateTransactionInput{
		Description: "Updated description",
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, &transaction.Transaction{
			Description: input.Description,
		}).
		Return(nil, databaseError).
		Times(1)

	result, err := uc.UpdateTransaction(context.Background(), organizationID, ledgerID, transactionID, input)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
	assert.Nil(t, result)
}

// TestUpdateTransaction_MetadataFindError tests update when metadata find fails
func TestUpdateTransaction_MetadataFindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	metadataError := errors.New("metadata update failed")

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	input := &transaction.UpdateTransactionInput{
		Description: "Updated description",
		Metadata: map[string]any{
			"key1": "value1",
		},
	}

	expectedTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Description:    "Updated description",
		UpdatedAt:      time.Now(),
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, &transaction.Transaction{
			Description: input.Description,
		}).
		Return(expectedTransaction, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, metadataError).
		Times(1)

	result, err := uc.UpdateTransaction(context.Background(), organizationID, ledgerID, transactionID, input)

	assert.Error(t, err)
	assert.Equal(t, metadataError, err)
	assert.Nil(t, result)
}

// TestUpdateTransaction_MetadataUpdateError tests update when metadata update fails
func TestUpdateTransaction_MetadataUpdateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	metadataUpdateError := errors.New("metadata update failed")

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
	}

	input := &transaction.UpdateTransactionInput{
		Description: "Updated description",
		Metadata: map[string]any{
			"key1": "value1",
		},
	}

	expectedTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Description:    "Updated description",
		UpdatedAt:      time.Now(),
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, &transaction.Transaction{
			Description: input.Description,
		}).
		Return(expectedTransaction, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), "Transaction", transactionID.String()).
		Return(nil, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "Transaction", transactionID.String(), input.Metadata).
		Return(metadataUpdateError).
		Times(1)

	result, err := uc.UpdateTransaction(context.Background(), organizationID, ledgerID, transactionID, input)

	assert.Error(t, err)
	assert.Equal(t, metadataUpdateError, err)
	assert.Nil(t, result)
}

// TestUpdateTransactionStatus tests successful update of transaction status
func TestUpdateTransactionStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	inputTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Status: transaction.Status{
			Code:        "COMPLETED",
			Description: ptr("Transaction completed"),
		},
	}

	expectedTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Status: transaction.Status{
			Code:        "COMPLETED",
			Description: ptr("Transaction completed"),
		},
		UpdatedAt: time.Now(),
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, inputTransaction).
		Return(expectedTransaction, nil).
		Times(1)

	result, err := uc.UpdateTransactionStatus(context.Background(), inputTransaction)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "COMPLETED", result.Status.Code)
}

// TestUpdateTransactionStatus_NotFound tests status update when transaction is not found
func TestUpdateTransactionStatus_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	inputTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Status: transaction.Status{
			Code:        "COMPLETED",
			Description: ptr("Transaction completed"),
		},
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, inputTransaction).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	result, err := uc.UpdateTransactionStatus(context.Background(), inputTransaction)

	assert.Error(t, err)
	assert.Nil(t, result)

	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
}

// TestUpdateTransactionStatus_RepositoryError tests status update when repository returns generic error
func TestUpdateTransactionStatus_RepositoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	transactionID := libCommons.GenerateUUIDv7()
	databaseError := errors.New("database connection error")

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	inputTransaction := &transaction.Transaction{
		ID:             transactionID.String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Status: transaction.Status{
			Code:        "COMPLETED",
			Description: ptr("Transaction completed"),
		},
	}

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, transactionID, inputTransaction).
		Return(nil, databaseError).
		Times(1)

	result, err := uc.UpdateTransactionStatus(context.Background(), inputTransaction)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
	assert.Nil(t, result)
}

// ptr is a helper function to create a pointer to a string
func ptr(s string) *string {
	return &s
}
