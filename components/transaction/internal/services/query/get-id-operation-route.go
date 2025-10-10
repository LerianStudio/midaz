// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving an operation route by its ID.
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
)

// GetOperationRouteByID retrieves an operation route by its ID, enriched with metadata.
//
// This use case fetches an operation route from PostgreSQL and its corresponding
// metadata from MongoDB. Operation routes define the rules for selecting source or
// destination accounts in a transaction.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - portfolioID: The UUID of the portfolio (currently unused).
//   - id: The UUID of the operation route to retrieve.
//
// Returns:
//   - *mmodel.OperationRoute: The operation route with its metadata.
//   - error: An error if the operation route is not found or if the retrieval fails.
func (uc *UseCase) GetOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.OperationRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_operation_route_by_id")
	defer span.End()

	logger.Infof("Retrieving operation route for id: %s", id)

	operationRoute, err := uc.OperationRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting operation route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

			logger.Warnf("Error getting operation route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operation route on repo by id", err)

		return nil, err
	}

	if operationRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.OperationRoute{}).Name(), operationRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation route", err)

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
