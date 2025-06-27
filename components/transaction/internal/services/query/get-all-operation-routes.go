package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// GetAllOperationRoutes retrieves all operation routes from the database.
// It returns a list of operation routes and an error if the operation fails.
func (uc *UseCase) GetAllOperationRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, pagination libPostgres.Pagination) ([]*mmodel.OperationRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operation_routes")
	defer span.End()

	logger.Infof("Retrieving all operation routes")

	operationRoutes, err := uc.OperationRouteRepo.FindAll(ctx, organizationID, ledgerID, pagination)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation routes on repo", err)

		logger.Errorf("Error getting operation routes on repo: %v", err)

		return nil, err
	}

	return operationRoutes, nil
}
