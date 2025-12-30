package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateOperationRoute creates a new operation route.
func (uc *UseCase) CreateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation_route")
	defer span.End()

	now := time.Now()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		Code:           payload.Code,
		OperationType:  payload.OperationType,
		Account:        payload.Account,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route", err)

		logger.Errorf("Failed to create operation route: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	assert.NotNil(createdOperationRoute, "repository Create must return non-nil operation route on success",
		"organization_id", organizationID,
		"ledger_id", ledgerID,
		"code", operationRoute.Code)
	assert.That(createdOperationRoute.OrganizationID == organizationID, "operation route organization id mismatch after create",
		"expected_organization_id", organizationID,
		"actual_organization_id", createdOperationRoute.OrganizationID)
	assert.That(createdOperationRoute.LedgerID == ledgerID, "operation route ledger id mismatch after create",
		"expected_ledger_id", ledgerID,
		"actual_ledger_id", createdOperationRoute.LedgerID)
	assert.That(createdOperationRoute.ID != uuid.Nil, "operation route id must be generated",
		"operation_route_id", createdOperationRoute.ID)
	assert.That(createdOperationRoute.OperationType == payload.OperationType, "operation route type mismatch after create",
		"expected_operation_type", payload.OperationType,
		"actual_operation_type", createdOperationRoute.OperationType)
	assert.That(createdOperationRoute.Code == payload.Code, "operation route code mismatch after create",
		"expected_code", payload.Code,
		"actual_code", createdOperationRoute.Code)

	if payload.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   createdOperationRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.OperationRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route metadata", err)

			logger.Errorf("Failed to create operation route metadata: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		createdOperationRoute.Metadata = payload.Metadata
	}

	return createdOperationRoute, nil
}
