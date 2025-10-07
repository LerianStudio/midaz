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

// DeleteOperationRouteByID soft-deletes an operation route with relationship validation.
//
// This method implements the delete operation route use case with safety checks:
// 1. Checks if operation route is linked to any transaction routes
// 2. Returns error if links exist (cannot delete referenced operation routes)
// 3. Performs soft delete if no links exist
// 4. Operation route remains in database for audit purposes
//
// Business Rules:
//   - Operation route cannot be deleted if referenced by transaction routes
//   - This prevents breaking transaction routing logic
//   - Must remove from transaction routes first
//   - Soft delete sets deleted_at timestamp
//
// Safety Check:
//   - Queries junction table for transaction route relationships
//   - Returns ErrOperationRouteLinkedToTransactionRoutes if links found
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the operation route to delete
//
// Returns:
//   - error: nil on success, business error if linked or deletion fails
//
// OpenTelemetry: Creates span "command.delete_operation_route_by_id"
func (uc *UseCase) DeleteOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_operation_route_by_id")
	defer span.End()

	logger.Infof("Remove operation route for id: %s", id.String())

	hasLinks, err := uc.OperationRouteRepo.HasTransactionRouteLinks(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check transaction route links", err)

		logger.Errorf("Error checking transaction route links for operation route %s: %v", id.String(), err)

		return err
	}

	if hasLinks {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation Route cannot be deleted because it is linked to transaction routes", nil)

		logger.Warnf("Operation Route ID %s cannot be deleted because it is linked to transaction routes", id.String())

		return pkg.ValidateBusinessError(constant.ErrOperationRouteLinkedToTransactionRoutes, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	if err := uc.OperationRouteRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrOperationRouteNotFound, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation Route ID not found", err)

			logger.Warnf("Operation Route ID not found: %s", id.String())

			return err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete operation route on repo by id", err)

		logger.Errorf("Error deleting operation route: %v", err)

		return err
	}

	return nil
}
