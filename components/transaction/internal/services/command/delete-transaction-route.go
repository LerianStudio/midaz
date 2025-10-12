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
// This function removes both the transaction route and its associations with operation routes.
// The cache is NOT automatically deleted here - caller must call DeleteTransactionRouteCache separately.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID owning the route
//   - ledgerID: Ledger UUID containing the route
//   - transactionRouteID: UUID of the transaction route to delete
//
// Returns:
//   - error: ErrOperationRouteNotFound if not found, or repository errors
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
