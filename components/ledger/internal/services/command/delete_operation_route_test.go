// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestDeleteOperationRouteByIDSuccess tests successful deletion of an operation route
func TestDeleteOperationRouteByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(&mmodel.OperationRoute{ID: operationRouteID, OrganizationID: organizationID}, nil).
		Times(1)

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), organizationID, operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, operationRouteID).
		Return(nil).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	assert.NoError(t, err)
}

// TestDeleteOperationRouteByIDContextCanceled tests deletion with canceled context.
func TestDeleteOperationRouteByIDContextCanceled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := uc.DeleteOperationRouteByID(ctx, organizationID, operationRouteID)

	assert.ErrorIs(t, err, context.Canceled)
}

// TestDeleteOperationRouteByIDNotFound tests deletion when operation route is not found
func TestDeleteOperationRouteByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(&mmodel.OperationRoute{ID: operationRouteID, OrganizationID: organizationID}, nil).
		Times(1)

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), organizationID, operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, operationRouteID).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	assert.Error(t, err)

	// Check if it's the proper business error
	var entityNotFoundError pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFoundError))
	assert.Equal(t, "0101", entityNotFoundError.Code)
}

// TestDeleteOperationRouteByIDError tests deletion with database error
func TestDeleteOperationRouteByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	databaseError := errors.New("database connection error")

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(&mmodel.OperationRoute{ID: operationRouteID, OrganizationID: organizationID}, nil).
		Times(1)

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), organizationID, operationRouteID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, operationRouteID).
		Return(databaseError).
		Times(1)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, databaseError, err)
}

// TestDeleteOperationRouteByIDLinkedToTransactionRoutes tests deletion when operation route is linked to transaction routes
func TestDeleteOperationRouteByIDLinkedToTransactionRoutes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(&mmodel.OperationRoute{ID: operationRouteID, OrganizationID: organizationID}, nil).
		Times(1)

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), organizationID, operationRouteID).
		Return(true, nil).
		Times(1)

	// Delete should not be called since operation route is linked
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	assert.Error(t, err)

	// Check if it's the proper business error for linked operation routes
	var unprocessableOperationError pkg.UnprocessableOperationError
	assert.True(t, errors.As(err, &unprocessableOperationError))
	assert.Equal(t, "0107", unprocessableOperationError.Code)
}

// TestDeleteOperationRouteByIDHasLinksCheckError tests deletion when checking for links fails
func TestDeleteOperationRouteByIDHasLinksCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	linkCheckError := errors.New("failed to check transaction route links")

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(&mmodel.OperationRoute{ID: operationRouteID, OrganizationID: organizationID}, nil).
		Times(1)

	mockRepo.EXPECT().
		HasTransactionRouteLinks(gomock.Any(), organizationID, operationRouteID).
		Return(false, linkCheckError).
		Times(1)

	// Delete should not be called since link check failed
	mockRepo.EXPECT().
		Delete(gomock.Any(), organizationID, operationRouteID).
		Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	assert.Error(t, err)
	assert.Equal(t, linkCheckError, err)
}

func TestDeleteOperationRouteByID_UnknownRouteIsNotFoundBeforeLinkCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, operationRouteID).
		Return(nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, constant.EntityOperationRoute)).
		Times(1)

	mockRepo.EXPECT().HasTransactionRouteLinks(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := uc.DeleteOperationRouteByID(context.Background(), organizationID, operationRouteID)

	var entityNotFoundError pkg.EntityNotFoundError
	require.ErrorAs(t, err, &entityNotFoundError)
	assert.Equal(t, constant.ErrOperationRouteNotFound.Error(), entityNotFoundError.Code)
}
