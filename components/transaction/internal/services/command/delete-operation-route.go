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

// DeleteOperationRouteByID deletes an operation route if it has no transaction route links.
//
// Operation routes define how money moves in transactions. Before deletion,
// this function validates that the route is not linked to any transaction routes.
// Linked operation routes cannot be deleted to maintain referential integrity.
//
// Deletion Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Check Transaction Route Links
//	  - Query join table for any relationships
//	  - If links exist, return ErrOperationRouteLinkedToTransactionRoutes
//
//	Step 3: Delete Operation Route
//	  - Remove record from PostgreSQL
//	  - Handle not-found scenarios
//
// Referential Integrity:
//
// Operation routes are referenced by transaction routes through a join table.
// Deleting an operation route that's in use would break transaction route
// functionality. This check ensures data consistency.
//
// To delete a linked operation route:
//  1. Update all transaction routes to remove the operation route
//  2. Then delete the operation route
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - id: UUID of the operation route to delete
//
// Returns:
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrOperationRouteLinkedToTransactionRoutes: Route is in use by transaction routes
//   - ErrOperationRouteNotFound: Operation route with given ID doesn't exist
//   - Database errors: PostgreSQL unavailable
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
