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
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Sentinel errors for test assertions.
var (
	errTestCreateOperationRoute = errors.New("failed to create operation route")
)

// TestCreateOperationRouteSuccess is responsible to test CreateOperationRoute with success.
func TestCreateOperationRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		Description:   "Test Description",
		OperationType: "source",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability"},
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, payload.Title, operationRoute.Title)
			assert.Equal(t, payload.Description, operationRoute.Description)
			assert.Equal(t, payload.OperationType, operationRoute.OperationType)
			assert.Equal(t, payload.Account, operationRoute.Account)

			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	require.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, payload.Title, operationRoute.Title)
	assert.Equal(t, payload.Description, operationRoute.Description)
	assert.Equal(t, payload.OperationType, operationRoute.OperationType)
	assert.Equal(t, payload.Account, operationRoute.Account)
}

// TestCreateOperationRouteSuccessWithAccountAlias tests creating an operation route with account alias only.
func TestCreateOperationRouteSuccessWithAccountAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		Description:   "Test Description",
		OperationType: "source",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAlias,
			ValidIf:  "@cash_account",
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, payload.Title, operationRoute.Title)
			assert.Equal(t, payload.Description, operationRoute.Description)
			assert.Equal(t, payload.OperationType, operationRoute.OperationType)
			assert.Equal(t, payload.Account, operationRoute.Account)

			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	require.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, payload.Title, operationRoute.Title)
	assert.Equal(t, payload.Description, operationRoute.Description)
	assert.Equal(t, payload.OperationType, operationRoute.OperationType)
	assert.Equal(t, payload.Account, operationRoute.Account)
}

// TestCreateOperationRouteWithEmptyAccount tests creating an operation route with empty account.
func TestCreateOperationRouteWithEmptyAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		Description:   "Test Description",
		OperationType: "source",
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		OperationType:  payload.OperationType,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	assert.Equal(t, expectedOperationRoute, result)
	require.NoError(t, err)
}

// TestCreateOperationRouteError is responsible to test CreateOperationRoute with error.
func TestCreateOperationRouteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		Description:   "Test Description",
		OperationType: "source",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability"},
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, errTestCreateOperationRoute)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, ledgerID, payload)

	require.Error(t, err)
	assert.Nil(t, operationRoute)
}
