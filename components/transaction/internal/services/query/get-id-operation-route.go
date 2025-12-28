package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// GetOperationRouteByID retrieves an operation route by its ID.
// It returns the operation route if found, otherwise it returns an error.
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	logger.Infof("Retrieving operation route for id: %s", id)

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

			logger.Warnf("Error getting operation route on repo by id: %v", err)

			return nil, err //nolint:wrapcheck // EntityNotFoundError is already a typed business error
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

		logger.Errorf("Error getting operation route on repo by id: %v", err)

		return nil, pkg.ValidateInternalError(err, "OperationRoute")
	}

	if operationRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation route", err)

			logger.Errorf("Error get metadata on mongodb operation route: %v", err)

			return nil, pkg.ValidateInternalError(err, "OperationRoute")
		}

		if metadata != nil {
			operationRoute.Metadata = metadata.Data
		}
	}

	logger.Infof("Successfully retrieved operation route for id: %s", id)

	return operationRoute, nil
}
