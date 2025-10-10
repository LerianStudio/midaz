// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving a transaction route by its ID.
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

// GetTransactionRouteByID retrieves a transaction route by its ID, enriched with metadata.
//
// This use case fetches a transaction route from PostgreSQL and its corresponding
// metadata from MongoDB. Transaction routes define the flow of funds by specifying
// rules for automated transaction processing.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the transaction route to retrieve.
//
// Returns:
//   - *mmodel.TransactionRoute: The transaction route with its metadata.
//   - error: An error if the transaction route is not found or if the retrieval fails.
func (uc *UseCase) GetTransactionRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_route_by_id")
	defer span.End()

	logger.Infof("Retrieving transaction route for id: %s", id)

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Error getting transaction route on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrTransactionRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get transaction route", err)

			logger.Warnf("Error getting transaction route on repo by id: %v", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction route", err)

		return nil, err
	}

	if transactionRoute != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.TransactionRoute{}).Name(), transactionRoute.ID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb transaction route", err)

			logger.Errorf("Error get metadata on mongodb transaction route: %v", err)

			return nil, err
		}

		if metadata != nil {
			transactionRoute.Metadata = metadata.Data
		}
	}

	return transactionRoute, nil
}
