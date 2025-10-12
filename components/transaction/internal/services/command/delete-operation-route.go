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

// DeleteOperationRouteByID deletes an operation route with referential integrity checks.
//
// Operation routes cannot be deleted if they are referenced by transaction routes,
// as this would orphan the transaction route configuration and break transaction validation.
//
// Deletion Process:
// 1. Check if operation route is linked to any transaction routes
// 2. If linked, reject deletion with ErrOperationRouteLinkedToTransactionRoutes
// 3. If not linked, perform soft delete
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the operation route
//   - ledgerID: Ledger UUID containing the operation route
//   - id: UUID of the operation route to delete
//
// Returns:
//   - error: ErrOperationRouteLinkedToTransactionRoutes if referenced, ErrOperationRouteNotFound if not found
func (uc *UseCase) DeleteOperationRouteByID(ctx context.Context, organizationID, ledgerID uuid.UUID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_operation_route_by_id")
	defer span.End()

	logger.Infof("Remove operation route for id: %s", id.String())

	// Step 1: Check referential integrity - operation route may be used in transaction routes
	hasLinks, err := uc.OperationRouteRepo.HasTransactionRouteLinks(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to check transaction route links", err)

		logger.Errorf("Error checking transaction route links for operation route %s: %v", id.String(), err)

		return err
	}

	// Step 2: Prevent deletion if linked to transaction routes (referential integrity)
	if hasLinks {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation Route cannot be deleted because it is linked to transaction routes", nil)

		logger.Warnf("Operation Route ID %s cannot be deleted because it is linked to transaction routes", id.String())

		return pkg.ValidateBusinessError(constant.ErrOperationRouteLinkedToTransactionRoutes, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	// Step 3: Perform soft delete
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
