package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateOperationRoute creates a new operation route.
func (uc *UseCase) CreateOperationRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateOperationRouteInput) (*mmodel.OperationRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_operation_route")
	defer span.End()

	now := time.Now()

	operationRoute := &mmodel.OperationRoute{
		ID:             libCommons.GenerateUUIDv7(),
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Title:          payload.Title,
		Description:    payload.Description,
		OperationType:  payload.OperationType,
		Account:        payload.Account,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create operation route", err)

		logger.Errorf("Failed to create operation route: %v", err)

		return nil, err
	}

	if payload.Metadata != nil {
		if err := libCommons.CheckMetadataKeyAndValueLength(100, payload.Metadata); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to check metadata key and value length", err)

			return nil, err
		}

		meta := mongodb.Metadata{
			EntityID:   createdOperationRoute.ID.String(),
			EntityName: reflect.TypeOf(mmodel.OperationRoute{}).Name(),
			Data:       payload.Metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), &meta); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to create operation route metadata", err)

			logger.Errorf("Failed to create operation route metadata: %v", err)

			return nil, err
		}

		createdOperationRoute.Metadata = payload.Metadata
	}

	return createdOperationRoute, nil
}
