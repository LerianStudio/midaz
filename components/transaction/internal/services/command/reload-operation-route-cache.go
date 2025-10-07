// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// ReloadOperationRouteCache refreshes caches for all transaction routes using an operation route.
//
// This method implements cache refresh when an operation route is modified:
// 1. Finds all transaction routes that reference the operation route
// 2. Fetches each transaction route with updated operation route data
// 3. Recreates cache for each transaction route
// 4. Continues on errors (best-effort refresh)
//
// Use Cases:
//   - Operation route is updated (account rules changed)
//   - Operation route relationships change
//   - Ensures transaction route caches reflect latest operation route data
//
// Error Handling:
//   - Logs warnings for individual failures
//   - Continues processing remaining routes
//   - Returns nil even if some routes fail (best-effort)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - id: UUID of the operation route that was modified
//
// Returns:
//   - error: nil on success (even with partial failures), error if finding route IDs fails
//
// OpenTelemetry: Creates span "command.reload_operation_route_cache"
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
