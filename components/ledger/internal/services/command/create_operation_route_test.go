// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestCreateOperationRouteSuccess is responsible to test CreateOperationRoute with success
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
		Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID uuid.UUID, ledID *uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, payload.Title, operationRoute.Title)
			assert.Equal(t, payload.Description, operationRoute.Description)
			assert.Equal(t, payload.OperationType, operationRoute.OperationType)
			assert.Equal(t, payload.Account, operationRoute.Account)
			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, payload.Title, operationRoute.Title)
	assert.Equal(t, payload.Description, operationRoute.Description)
	assert.Equal(t, payload.OperationType, operationRoute.OperationType)
	assert.Equal(t, payload.Account, operationRoute.Account)
}

// TestCreateOperationRouteSuccessWithAccountAlias tests creating an operation route with account alias only
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
		Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
		DoAndReturn(func(ctx context.Context, orgID uuid.UUID, ledID *uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
			assert.Equal(t, payload.Title, operationRoute.Title)
			assert.Equal(t, payload.Description, operationRoute.Description)
			assert.Equal(t, payload.OperationType, operationRoute.OperationType)
			assert.Equal(t, payload.Account, operationRoute.Account)
			return operationRoute, nil
		})

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, operationRoute)
	assert.Equal(t, payload.Title, operationRoute.Title)
	assert.Equal(t, payload.Description, operationRoute.Description)
	assert.Equal(t, payload.OperationType, operationRoute.OperationType)
	assert.Equal(t, payload.Account, operationRoute.Account)
}

// TestCreateOperationRouteWithEmptyAccount tests creating an operation route with empty account
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
		LedgerID:       &ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		OperationType:  payload.OperationType,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
		Return(expectedOperationRoute, nil).
		Times(1)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.Equal(t, expectedOperationRoute, result)
	assert.Nil(t, err)
}

// TestCreateOperationRouteWithEmptyMetadata asserts that an empty metadata map,
// as produced by the decode path for an absent key, persists no metadata document.
func TestCreateOperationRouteWithEmptyMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		Description:   "Test Description",
		OperationType: "source",
		Metadata:      map[string]any{},
	}

	expectedOperationRoute := &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		LedgerID:       &ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		OperationType:  payload.OperationType,
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
	}

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
		Return(expectedOperationRoute, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Metadata)
}

// TestCreateOperationRouteError is responsible to test CreateOperationRoute with error
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
		Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
		Return(nil, errors.New("failed to create operation route"))

	useCase := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	operationRoute, err := useCase.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.Error(t, err)
	assert.Nil(t, operationRoute)
}

// TestCreateOperationRouteWithMetadataStagesBeforeInsert asserts that the metadata document is
// written before the row is inserted, both under the ID the use case assigned.
func TestCreateOperationRouteWithMetadataStagesBeforeInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		OperationType: "source",
		Metadata:      map[string]any{"key": "value"},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		Streaming:               emitter,
	}

	var stagedEntityID string

	gomock.InOrder(
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedEntityID = meta.EntityID
				assert.Empty(t, emitter.Events(), "no event may be published before the insert")
				assert.Equal(t, constant.EntityOperationRoute, meta.EntityName)
				assert.Equal(t, "value", meta.Data["key"])

				return nil
			}).
			Times(1),
		mockOperationRouteRepo.EXPECT().
			Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
				assert.Equal(t, stagedEntityID, operationRoute.ID.String())

				return operationRoute, nil
			}).
			Times(1),
	)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, stagedEntityID)
	assert.Equal(t, stagedEntityID, result.ID.String())
	assert.Equal(t, payload.Metadata, result.Metadata)

	emitted := emitter.Events()
	require.Len(t, emitted, 1, "the created event is published once after the insert")
	assert.Equal(t, events.OperationRouteCreatedDefinition.Key(), emitted[0].DefinitionKey)
}

// TestCreateOperationRouteMetadataFailureSkipsInsert asserts that a metadata store failure
// aborts the create before anything is inserted.
func TestCreateOperationRouteMetadataFailureSkipsInsert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		OperationType: "source",
		Metadata:      map[string]any{"key": "value"},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		Streaming:               emitter,
	}

	expectedError := errors.New("mongodb connection error")

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).
		Return(expectedError).
		Times(1)

	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Times(0)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
	assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
}

// TestCreateOperationRouteInsertFailureDiscardsStagedMetadata asserts that a failed insert
// purges the metadata document staged under the same ID and returns the insert error.
func TestCreateOperationRouteInsertFailureDiscardsStagedMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Operation Route",
		OperationType: "source",
		Metadata:      map[string]any{"key": "value"},
	}

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		OperationRouteRepo:      mockOperationRouteRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		Streaming:               emitter,
	}

	var stagedEntityID, insertedID string

	expectedError := errors.New("failed to create operation route")

	gomock.InOrder(
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
				stagedEntityID = meta.EntityID
				assert.Empty(t, emitter.Events(), "no event may be published before the insert")

				return nil
			}).
			Times(1),
		mockOperationRouteRepo.EXPECT().
			Create(gomock.Any(), organizationID, &ledgerID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, operationRoute *mmodel.OperationRoute) (*mmodel.OperationRoute, error) {
				insertedID = operationRoute.ID.String()

				return nil, expectedError
			}).
			Times(1),
		mockMetadataRepo.EXPECT().
			Purge(gomock.Any(), constant.EntityOperationRoute, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, id string) error {
				assert.Equal(t, stagedEntityID, id)

				return nil
			}).
			Times(1),
	)

	result, err := uc.CreateOperationRoute(context.Background(), organizationID, &ledgerID, payload)

	assert.Nil(t, result)
	assert.Equal(t, expectedError, err)
	assert.NotEmpty(t, stagedEntityID)
	assert.Equal(t, stagedEntityID, insertedID)
	assert.Empty(t, emitter.Events(), "no event may be published when the create fails")
}
