package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateOperationRouteSuccess tests updating an operation route successfully
func TestUpdateOperationRouteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Operation Route",
		Description: "Updated Description",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability", "equity"},
		},
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
		OperationType:  "source",
		Account:        input.Account,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, opID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, input.Title, operationRoute.Title)
			assert.Equal(t, input.Description, operationRoute.Description)
			assert.Equal(t, input.Account, operationRoute.Account)
			return expectedOperationRoute, nil
		})

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "OperationRoute", operationRouteID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, expectedOperationRoute, operationRoute)
}

// TestUpdateOperationRouteSuccessWithAccountAlias tests updating an operation route with account alias only
func TestUpdateOperationRouteSuccessWithAccountAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Operation Route",
		Description: "Updated Description",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAlias,
			ValidIf:  "@updated_cash_account",
		},
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          input.Title,
		Description:    input.Description,
		OperationType:  "source",
		Account:        input.Account,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, opID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, input.Title, operationRoute.Title)
			assert.Equal(t, input.Description, operationRoute.Description)
			assert.Equal(t, input.Account, operationRoute.Account)
			return expectedOperationRoute, nil
		})

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "OperationRoute", operationRouteID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, expectedOperationRoute, operationRoute)
}

// TestUpdateOperationRouteAccountTypesOnly tests updating only account types
func TestUpdateOperationRouteAccountTypesOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability"},
		},
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		OperationType:  "source",
		Account:        input.Account,
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "OperationRoute", operationRouteID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}

// TestUpdateOperationRouteNotFound tests updating a non-existent operation route
func TestUpdateOperationRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Operation Route",
		Description: "Updated Description",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability"},
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(nil, services.ErrDatabaseItemNotFound).
		Times(1)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
	assert.Equal(t, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name()), err)
}

// TestUpdateOperationRouteError tests updating an operation route with an error
func TestUpdateOperationRouteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:       "Updated Operation Route",
		Description: "Updated Description",
		Account: &mmodel.AccountRule{
			RuleType: constant.AccountRuleTypeAccountType,
			ValidIf:  []string{"asset", "liability"},
		},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(nil, errors.New("failed to update operation route"))

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
}

// TestUpdateOperationRoutePartialUpdate tests partial update with only description
func TestUpdateOperationRoutePartialUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Description: "Updated Description Only",
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "", // Title not provided in input
		Description:    input.Description,
		OperationType:  "source",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), "OperationRoute", operationRouteID.String(), map[string]any{}).
		Return(nil).
		Times(1)

	uc := &UseCase{
		OperationRouteRepo: mockRepo,
		MetadataRepo:       mockMetadataRepo,
	}

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}
