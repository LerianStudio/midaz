// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// CreateAccountingRouteCache creates and stores a transaction route cache in Redis.
//
// This method implements transaction route caching for performance optimization:
// 1. Converts transaction route to cache structure (ToCache method)
// 2. Serializes cache data to msgpack format
// 3. Stores in Redis with no expiration (TTL = 0)
// 4. Cache key format: "accounting_routes:{org_id}:{ledger_id}:{route_id}"
//
// Cache Structure:
//   - Source map: operation_route_id → {rule_type, valid_if}
//   - Destination map: operation_route_id → {rule_type, valid_if}
//   - Pre-categorized by operation type for fast lookups
//
// Use Case:
//   - Transaction processing needs to quickly determine which accounts match routes
//   - Cache avoids database queries during high-frequency transaction processing
//   - Msgpack provides efficient binary serialization
//
// Cache Invalidation:
//   - Cache is invalidated when transaction route is updated or deleted
//   - No TTL means cache persists until explicitly deleted
//   - Must be manually invalidated on route changes
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - route: Transaction route to cache
//
// Returns:
//   - error: nil on success, error if serialization or Redis operation fails
//
// OpenTelemetry: Creates span "command.create_transaction_route_cache"
func (uc *UseCase) CreateAccountingRouteCache(ctx context.Context, route *mmodel.TransactionRoute) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_route_cache")
	defer span.End()

	logger.Infof("Creating transaction route cache for transaction route with id: %s", route.ID)

	internalKey := libCommons.AccountingRoutesInternalKey(route.OrganizationID, route.LedgerID, route.ID)

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
