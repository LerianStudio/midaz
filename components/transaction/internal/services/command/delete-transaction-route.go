// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// DeleteTransactionRouteByID soft-deletes a transaction route and its operation route relationships.
//
// This method implements the delete transaction route use case, which:
// 1. Fetches the transaction route to validate it exists
// 2. Extracts all associated operation route IDs
// 3. Deletes the transaction route (soft delete)
// 4. Removes operation route relationships from junction table
// 5. Cache invalidation should be handled separately
//
// Business Rules:
//   - Transaction route must exist
//   - All operation route relationships are removed
//   - Soft delete sets deleted_at timestamp
//   - Operation routes themselves are not deleted (only relationships)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionRouteID: UUID of the transaction route to delete
//
// Returns:
//   - error: nil on success, business error if not found or deletion fails
//
// OpenTelemetry: Creates span "command.delete_transaction_route_by_id"
func (uc *UseCase) DeleteTransactionRouteByID(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_by_id")
	defer span.End()

	logger.Infof("Deleting transaction route with ID: %s", transactionRouteID.String())

	transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Transaction Route ID not found: %s", transactionRouteID.String())

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
