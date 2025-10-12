package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// ReloadOperationRouteCache refreshes Redis cache for all transaction routes using an operation route.
//
// When an operation route is updated, all transaction routes referencing it need their
// caches refreshed to reflect the changes. This function finds all affected transaction
// routes and regenerates their Redis cache entries.
//
// Use Case: After updating an operation route's account validation rules, all transaction
// routes using that rule must have their caches updated to enforce the new rules.
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID for scoping
//   - ledgerID: Ledger UUID for scoping
//   - id: UUID of the operation route that was updated
//
// Returns:
//   - error: Repository or cache errors (logs and continues on individual failures)
func (uc *UseCase) ReloadOperationRouteCache(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.reload_operation_route_cache")
	defer span.End()

	logger.Infof("Reloading operation route cache for operation route with id: %s", id)

	transactionRouteIDs, err := uc.OperationRouteRepo.FindTransactionRouteIDs(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find transaction route IDs", err)

		logger.Errorf("Failed to find transaction route IDs for operation route %s: %v", id, err)

		return err
	}

	if len(transactionRouteIDs) == 0 {
		logger.Infof("No transaction routes found for operation route %s, no cache reload needed", id)

		return nil
	}

	logger.Infof("Found %d transaction routes associated with operation route %s", len(transactionRouteIDs), id)

	for _, transactionRouteID := range transactionRouteIDs {
		transactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve transaction route", err)

			logger.Warnf("Failed to retrieve transaction route %s: %v", transactionRouteID, err)

			continue
		}

		if err := uc.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create cache for transaction route", err)

			logger.Warnf("Failed to create cache for transaction route %s: %v", transactionRouteID, err)

			continue
		}

		logger.Infof("Successfully reloaded cache for transaction route %s", transactionRouteID)
	}

	logger.Infof("Successfully completed cache reload for operation route %s", id)

	return nil
}
