package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetOperationRouteByID retrieves an operation route by its ID.
// It returns the operation route if found, otherwise it returns an error.
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.operation_route_id", id.String()),
	)

	if portfolioID != nil {
		span.SetAttributes(attribute.String("app.request.portfolio_id", portfolioID.String()))
	}

	logger.Infof("Retrieving operation route for id: %s", id)

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get operation route by ID on repository", err)

		logger.Errorf("Error getting operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		}

		return nil, err
	}

	if operationRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb operation route", err)

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
