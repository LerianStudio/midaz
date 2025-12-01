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

// DeleteTransactionRouteByID deletes a transaction route and its operation route relationships.
//
// Transaction routes define complete transaction patterns. When deleted, all
// relationships to operation routes are also removed (but operation routes
// themselves are preserved for potential use in other transaction routes).
//
// Deletion Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Fetch Transaction Route
//	  - Retrieve route with associated operation routes
//	  - Validate route exists (return ErrOperationRouteNotFound if not)
//
//	Step 3: Collect Relationships
//	  - Build list of operation route IDs to unlink
//	  - These relationships will be removed from join table
//
//	Step 4: Delete Route and Relationships
//	  - Remove transaction route record
//	  - Remove all join table entries
//	  - Operation routes remain intact
//
// Cascade Behavior:
//
// Deletion cascades to the transaction_route_operation_routes join table
// but does NOT cascade to operation routes themselves. This allows operation
// routes to be reused across multiple transaction routes.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for multi-tenant isolation
//   - ledgerID: Ledger scope within the organization
//   - transactionRouteID: UUID of the transaction route to delete
//
// Returns:
//   - error: Business or infrastructure error
//
// Error Scenarios:
//   - ErrOperationRouteNotFound: Transaction route with given ID doesn't exist
//   - Database errors: PostgreSQL unavailable
//
// Related Functions:
//   - DeleteTransactionRouteCache: Should be called after successful deletion
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
