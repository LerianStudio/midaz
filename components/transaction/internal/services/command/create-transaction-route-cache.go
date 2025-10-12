package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateAccountingRouteCache caches a transaction route in Redis for fast validation lookups.
//
// Transaction routes combine multiple operation routes into validated transaction flows.
// To avoid database queries during high-throughput transaction processing, the route
// configuration is pre-cached in Redis using msgpack for efficient serialization.
//
// Cache Structure (msgpack serialized):
//
//	{
//	  "source": {
//	    "operation_route_id_1": { "account": { "ruleType": "alias", "validIf": "@cash" } },
//	    "operation_route_id_2": { "account": { "ruleType": "account_type", "validIf": ["deposit"] } }
//	  },
//	  "destination": {
//	    "operation_route_id_3": { "account": { "ruleType": "alias", "validIf": "@revenue" } }
//	  }
//	}
//
// Benefits:
// - Fast validation during transaction processing (no DB round-trip)
// - Atomic updates with cache invalidation on route changes
// - Redis Cluster friendly with composite keys
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - route: The transaction route with all operation routes to cache
//
// Returns:
//   - error: Cache serialization or Redis storage errors
func (uc *UseCase) CreateAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route_cache")
	defer span.End()

	logger.Infof("Creating transaction route cache for transaction route with id: %s", route.ID)

	// Generate Redis key for this transaction route cache
	internalKey := libCommons.AccountingRoutesInternalKey(route.OrganizationID, route.LedgerID, route.ID)

	// Convert route to cache structure (categorized by source/destination)
	cacheData := route.ToCache()

	// Serialize to msgpack for efficient binary storage
	cacheBytes, err := cacheData.ToMsgpack()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to convert route to cache data", err)

		logger.Errorf("Failed to convert route to cache data: %v", err)

		return err
	}

	// Store in Redis with no TTL (persists until explicitly deleted)
	err = uc.RedisRepo.SetBytes(ctx, internalKey, cacheBytes, 0)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)

		return err
	}

	logger.Infof("Successfully created transaction route cache for transaction route with id: %s", route.ID)

	return nil
}
