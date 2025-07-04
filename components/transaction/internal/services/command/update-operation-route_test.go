package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountTypes: []string{"asset", "liability", "equity"},
		// AccountAlias not provided to avoid mutual exclusion
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{"asset"},
		AccountAlias:   "", // Empty to avoid conflict
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, opID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, input.Title, operationRoute.Title)
			assert.Equal(t, input.Description, operationRoute.Description)
			assert.Equal(t, input.AccountTypes, operationRoute.AccountTypes)
			assert.Equal(t, input.AccountAlias, operationRoute.AccountAlias)
			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, input.Title, operationRoute.Title)
	assert.Equal(t, input.Description, operationRoute.Description)
	assert.Equal(t, input.AccountTypes, operationRoute.AccountTypes)
	assert.Equal(t, input.AccountAlias, operationRoute.AccountAlias)
}

// TestUpdateOperationRouteSuccessWithAccountAlias tests updating an operation route with account alias only
func TestUpdateOperationRouteSuccessWithAccountAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountAlias: "@updated_cash_account",
		// AccountTypes not provided to avoid mutual exclusion
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{}, // Empty to avoid conflict
		AccountAlias:   "@old_account",
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	mockOperationRouteRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID, ledID, opID uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, input.Title, operationRoute.Title)
			assert.Equal(t, input.Description, operationRoute.Description)
			assert.Equal(t, input.AccountTypes, operationRoute.AccountTypes)
			assert.Equal(t, input.AccountAlias, operationRoute.AccountAlias)
			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, input.Title, operationRoute.Title)
	assert.Equal(t, input.Description, operationRoute.Description)
	assert.Equal(t, input.AccountTypes, operationRoute.AccountTypes)
	assert.Equal(t, input.AccountAlias, operationRoute.AccountAlias)
}

// TestUpdateOperationRouteAccountTypesOnly tests updating only account types
func TestUpdateOperationRouteAccountTypesOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		AccountTypes: []string{"asset", "liability"},
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Type:           "debit",
		AccountTypes:   []string{"asset"},
		AccountAlias:   "", // Empty to avoid conflict
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Type:           "debit",
		AccountTypes:   input.AccountTypes,
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}

// TestUpdateOperationRouteMutuallyExclusiveFieldsInInput tests providing both AccountTypes and AccountAlias in input
func TestUpdateOperationRouteMutuallyExclusiveFieldsInInput(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountTypes: []string{"asset", "liability"},
		AccountAlias: "@updated_cash_account", // Both fields provided - should trigger validation error
	}

	useCase := &UseCase{
		OperationRouteRepo: nil, // Repository shouldn't be called
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
	// Verify it's the expected validation error
	expectedError := pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveFields, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "accountTypes", "accountAlias")
	assert.Equal(t, expectedError, err)
}

// TestUpdateOperationRouteConflictExistingAccountTypesWithNewAccountAlias tests conflict when existing has AccountTypes and input has AccountAlias
func TestUpdateOperationRouteConflictExistingAccountTypesWithNewAccountAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountAlias: "@updated_cash_account", // Trying to set AccountAlias
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{"asset", "liability"}, // Existing has AccountTypes
		AccountAlias:   "",
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
	// Verify it's the expected validation error
	expectedError := pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveFields, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "accountTypes", "accountAlias")
	assert.Equal(t, expectedError, err)
}

// TestUpdateOperationRouteConflictExistingAccountAliasWithNewAccountTypes tests conflict when existing has AccountAlias and input has AccountTypes
func TestUpdateOperationRouteConflictExistingAccountAliasWithNewAccountTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountTypes: []string{"asset", "liability"}, // Trying to set AccountTypes
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{},
		AccountAlias:   "@existing_account", // Existing has AccountAlias
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
	// Verify it's the expected validation error
	expectedError := pkg.ValidateBusinessError(constant.ErrMutuallyExclusiveFields, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "accountTypes", "accountAlias")
	assert.Equal(t, expectedError, err)
}

// TestUpdateOperationRouteNotFound tests updating a non-existent operation route
func TestUpdateOperationRouteNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	operationRouteID := uuid.New()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateOperationRouteInput{
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountTypes: []string{"asset", "liability"},
		// AccountAlias not provided to avoid validation error
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
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
		Title:        "Updated Operation Route",
		Description:  "Updated Description",
		AccountTypes: []string{"asset", "liability"},
		// AccountAlias not provided to avoid validation error
	}

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{"asset"},
		AccountAlias:   "", // Empty to avoid conflict
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockOperationRouteRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

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

	existingOperationRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "Original Title",
		Description:    "Original Description",
		Type:           "debit",
		AccountTypes:   []string{"asset"},
		AccountAlias:   "", // Empty to avoid conflict
	}

	updatedRoute := &mmodel.OperationRoute{
		ID:             operationRouteID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          "", // Title not provided in input
		Description:    input.Description,
		Type:           "debit",
	}

	mockRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockRepo,
	}

	mockRepo.EXPECT().
		FindByID(gomock.Any(), organizationID, ledgerID, operationRouteID).
		Return(existingOperationRoute, nil).
		Times(1)

	mockRepo.EXPECT().
		Update(gomock.Any(), organizationID, ledgerID, operationRouteID, gomock.Any()).
		Return(updatedRoute, nil).
		Times(1)

	result, err := uc.UpdateOperationRoute(context.Background(), organizationID, ledgerID, operationRouteID, input)

	assert.NoError(t, err)
	assert.Equal(t, updatedRoute, result)
}
