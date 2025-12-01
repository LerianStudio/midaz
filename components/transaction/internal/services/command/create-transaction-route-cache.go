package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// CreateAccountingRouteCache creates a Redis cache entry for a transaction route.
//
// Transaction routes define how money flows in transactions. Caching these routes
// in Redis significantly improves transaction processing performance by avoiding
// repeated database lookups during high-throughput transaction processing.
//
// Cache Structure:
//
// The cache stores a map of operation route IDs to their routing rules:
//
//	{
//	  "operation_route_id_1": {"type": "source", "account_rule": "..."},
//	  "operation_route_id_2": {"type": "destination", "account_rule": "..."}
//	}
//
// This structure allows O(1) lookup of operation routing rules during transaction
// execution, eliminating the need for database joins.
//
// Caching Process:
//
//	Step 1: Context Setup
//	  - Extract logger and tracer from context
//	  - Start OpenTelemetry span for observability
//
//	Step 2: Build Cache Key
//	  - Construct internal key using org/ledger/route IDs
//	  - Key format: "accounting_routes:{orgID}:{ledgerID}:{routeID}"
//
//	Step 3: Serialize Cache Data
//	  - Convert TransactionRoute to cache-optimized structure
//	  - Serialize using MessagePack for compact binary format
//
//	Step 4: Store in Redis
//	  - Store serialized bytes with no expiration (TTL=0)
//	  - Cache is explicitly invalidated on route updates/deletes
//
// Why MessagePack:
//
// MessagePack provides ~50% smaller serialization compared to JSON while
// maintaining fast encode/decode performance. For high-frequency cache
// operations, this reduces network bandwidth and Redis memory usage.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - route: TransactionRoute to cache (must include OperationRoutes)
//
// Returns:
//   - error: Serialization or Redis operation error
//
// Error Scenarios:
//   - Serialization error: Failed to convert route to msgpack bytes
//   - Redis connection error: Redis server unavailable
//   - Redis write error: Failed to store cache entry
//
// Related Functions:
//   - DeleteTransactionRouteCache: Invalidates this cache entry
//   - CreateTransactionRoute: Should call this after creating a route
//   - UpdateTransactionRoute: Should invalidate and recreate cache
func (uc *UseCase) CreateAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route_cache")
	defer span.End()

	logger.Infof("Creating transaction route cache for transaction route with id: %s", route.ID)

	internalKey := utils.AccountingRoutesInternalKey(route.OrganizationID, route.LedgerID, route.ID)

	cacheData := route.ToCache()

	cacheBytes, err := cacheData.ToMsgpack()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to convert route to cache data", err)

		logger.Errorf("Failed to convert route to cache data: %v", err)

		return err
	}

	err = uc.RedisRepo.SetBytes(ctx, internalKey, cacheBytes, 0)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)

		return err
	}

	logger.Infof("Successfully created transaction route cache for transaction route with id: %s", route.ID)

	return nil
}
