package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// DeleteTransactionRouteCache removes cached transaction route data from Redis.
//
// Transaction routes are cached in Redis for fast lookup during transaction processing.
// This function invalidates the cache when a transaction route is modified or deleted,
// ensuring subsequent requests fetch fresh data from the database.
//
// Cache Invalidation Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Cache Key
//	  - Construct internal key using org/ledger/route IDs
//	  - Key format: "accounting_routes:{orgID}:{ledgerID}:{routeID}"
//
//	Step 3: Delete Cache Entry
//	  - Call RedisRepo.Del to remove the cached data
//	  - Handle Redis connection errors
//
// Why Cache Invalidation Matters:
//
// Transaction routes define how money flows between accounts. Stale cache data
// could cause transactions to use outdated routing rules, leading to:
//   - Incorrect account debits/credits
//   - Failed transaction validations
//   - Data inconsistency between cache and database
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for cache key construction
//   - ledgerID: Ledger scope for cache key construction
//   - transactionRouteID: UUID of the transaction route to invalidate
//
// Returns:
//   - error: Redis connection or operation error
//
// Error Scenarios:
//   - Redis connection timeout: Redis server unavailable
//   - Redis operation error: Key deletion failed
//
// Related Functions:
//   - CreateAccountingRouteCache: Creates the cache entry
//   - UpdateTransactionRoute: Should call this after route updates
//   - DeleteTransactionRouteByID: Should call this after route deletion
func (uc *UseCase) DeleteTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_transaction_route_cache")
	defer span.End()

	logger.Infof("Deleting transaction route cache for transaction route with id: %s", transactionRouteID)

	internalKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	err := uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete transaction route cache", err)

		logger.Errorf("Failed to delete transaction route cache: %v", err)

		return err
	}

	logger.Infof("Successfully deleted transaction route cache for transaction route with id: %s", transactionRouteID)

	return nil
}
