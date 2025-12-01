// Package command provides CQRS command handlers for the transaction component.
package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/google/uuid"
)

// ReloadOperationRouteCache refreshes the Redis cache for all transaction routes
// associated with a given operation route.
//
// Operation routes define how specific operation types are processed. Each operation
// route can be linked to multiple transaction routes. When an operation route is
// modified, all associated transaction route caches must be invalidated and rebuilt
// to ensure routing decisions use the latest configuration.
//
// # Cache Architecture
//
// Transaction routing uses a hierarchical cache structure:
//
//	Operation Route (defines operation processing rules)
//	    └── Transaction Route 1 (cached in Redis)
//	    └── Transaction Route 2 (cached in Redis)
//	    └── Transaction Route N (cached in Redis)
//
// When an operation route changes, all linked transaction routes need cache refresh.
//
// # Process
//
//  1. Extract logger and tracer from context for observability
//  2. Start tracing span "command.reload_operation_route_cache"
//  3. Query all transaction route IDs linked to this operation route
//  4. If no transaction routes found, return early (no cache to refresh)
//  5. For each transaction route:
//     a. Fetch the full transaction route from PostgreSQL
//     b. Recreate the cache entry in Redis via CreateAccountingRouteCache
//     c. Log success or warning for each route (continues on individual failures)
//  6. Return success (partial failures are logged but don't fail the operation)
//
// # Parameters
//
//   - ctx: Request context containing tenant info, tracing, and cancellation
//   - organizationID: The organization that owns this ledger (tenant isolation)
//   - ledgerID: The ledger containing the routes
//   - id: The operation route ID whose linked caches should be refreshed
//
// # Returns
//
//   - error: nil on success, or error if:
//   - Failed to find transaction route IDs (initial query failure)
//   - Context cancellation/timeout
//
// Note: Individual transaction route cache failures are logged but do not
// cause the entire operation to fail. This ensures partial progress is made
// even if some routes have issues.
//
// # Error Handling Strategy
//
// The function uses a "best effort" approach for individual routes:
//   - Query failures for transaction route IDs fail the entire operation
//   - Individual route fetch failures log warning and continue
//   - Individual cache creation failures log warning and continue
//
// This design ensures:
//   - Critical failures (can't query routes) stop immediately
//   - Partial failures don't block other routes from being cached
//   - All issues are logged for debugging
//
// # Performance Considerations
//
// This operation is typically triggered by:
//   - Operation route updates (configuration changes)
//   - Cache invalidation events
//   - Manual cache refresh requests
//
// For large numbers of transaction routes, consider async processing.
//
// # Observability
//
// Creates tracing span "command.reload_operation_route_cache" with error events.
// Logs each transaction route processing result (success or warning).
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
