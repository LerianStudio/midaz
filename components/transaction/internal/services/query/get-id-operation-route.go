package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// GetOperationRouteByID retrieves an operation route by its ID.
// It returns the operation route if found, otherwise it returns an error.
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	logger.Infof("Retrieving operation route for id: %s", id)

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libCommons.NewLoggerFromContext(ctx).Errorf("Error getting operation route on repo by id: %v", err)

		logger.Errorf("Error getting operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	if operationRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRoute.ID.String())
		if err != nil {
			logger.Errorf("Error get metadata on mongodb operation route: %v", err)

			return nil, err
		}

		if metadata != nil {
			operationRoute.Metadata = metadata.Data
		}
	}

	logger.Infof("Successfully retrieved operation route for id: %s", id)

	return operationRoute, nil
}
