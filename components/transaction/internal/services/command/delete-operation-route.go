// Package command implements write operations (commands) for the transaction service.
// This file contains the command for deleting an operation route.
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

// DeleteOperationRouteByID soft-deletes an operation route, ensuring it is not in use.
//
// This use case performs a safe soft-delete by first verifying that the operation
// route is not linked to any transaction routes. If it is, the deletion is
// prevented to maintain data integrity.
//
// Business Rules:
//   - An operation route cannot be deleted if it is referenced by any
//     transaction routes. This prevents breaking existing transaction logic.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - id: The UUID of the operation route to delete.
//
// Returns:
//   - error: An error if the operation route is in use or if the deletion fails.
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
