// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
)

// Sentinel errors for test assertions.
var (
	errTestDBConnectionDOR            = errors.New("database connection error")
	errTestCheckTransactionRouteLinks = errors.New("failed to check transaction route links")
)

// TestDeleteOperationRouteByIDSuccess tests successful deletion of an operation route.
func TestDeleteOperationRouteByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(nil).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	require.NoError(t, err)
}

// TestDeleteOperationRouteByIDNotFound tests deletion when operation route is not found.
func TestDeleteOperationRouteByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	require.Error(t, err)

	// Check if it's the proper business error
	var entityNotFoundError pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundError)
	assert.Equal(t, "0101", entityNotFoundError.Code)
}

// TestDeleteOperationRouteByIDError tests deletion with database error.
func TestDeleteOperationRouteByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	databaseError := errTestDBConnectionDOR

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(databaseError).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	require.Error(t, err)
	assert.Equal(t, databaseError, err)
}

// TestDeleteOperationRouteByIDLinkedToTransactionRoutes tests deletion when operation route is linked to transaction routes.
func TestDeleteOperationRouteByIDLinkedToTransactionRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(true, nil).
		Times(1)

	// Delete should not be called since operation route is linked
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	require.Error(t, err)

	// Check if it's the proper business error for linked operation routes
	var unprocessableOperationError pkg.UnprocessableOperationError
	require.ErrorAs(t, err, &unprocessableOperationError)
	assert.Equal(t, "0107", unprocessableOperationError.Code)
}

// TestDeleteOperationRouteByIDHasLinksCheckError tests deletion when checking for links fails.
func TestDeleteOperationRouteByIDHasLinksCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	linkCheckError := errTestCheckTransactionRouteLinks

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), operationRouteID).
		Return(false, linkCheckError).
		Times(1)

	// Delete should not be called since link check failed
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, ledgerID, operationRouteID)

	require.Error(t, err)
	assert.Equal(t, linkCheckError, err)
}
