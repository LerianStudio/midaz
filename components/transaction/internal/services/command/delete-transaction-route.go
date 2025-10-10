// Package command implements write operations (commands) for the transaction service.
// This file contains the command for deleting a transaction route.
package command

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

// DeleteTransactionRouteByID soft-deletes a transaction route and its associations.
//
// This use case performs a soft-delete of a transaction route and removes the links
// to its associated operation routes from the junction table. The operation routes
// themselves are not deleted.
//
// Business Rules:
//   - The transaction route must exist to be deleted.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionRouteID: The UUID of the transaction route to delete.
//
// Returns:
//   - error: An error if the transaction route is not found or if the deletion fails.
func (uc *UseCase) DeleteTransactionRouteByID(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_by_id")
	defer span.End()

	logger.Infof("Deleting transaction route with ID: %s", transactionRouteID.String())

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Transaction Route ID not found: %s", transactionRouteID.String())

			// FIXME: This error seems incorrect. It should be constant.ErrTransactionRouteNotFound
			// instead of constant.ErrOperationRouteNotFound.
			return pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
		}

		logger.Errorf("Error finding transaction route: %v", err)

		libOpentelemetry.HandleSpanError(&span, "Failed to find transaction route", err)

		return err
	}

	operationRoutesToRemove := make([]uuid.UUID, len(transactionRoute.OperationRoutes))
	for _, operationRoute := range transactionRoute.OperationRoutes {
		operationRoutesToRemove = append(operationRoutesToRemove, operationRoute.ID)
	}

	err = uc.TransactionRouteRepo.Delete(ctx, organizationID, ledgerID, transactionRouteID, operationRoutesToRemove)
	if err != nil {
		logger.Errorf("Error deleting transaction route: %v", err)

		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction route", err)

		return err
	}

	return nil
}
