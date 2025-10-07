// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GetOrCreateTransactionRouteCache retrieves transaction route cache from Redis with database fallback.
//
// This method implements cache-aside pattern for transaction route caching:
// 1. Checks Redis cache for transaction route (msgpack format)
// 2. If found: Deserializes and returns cached data
// 3. If not found: Fetches from PostgreSQL, creates cache, returns data
//
// Cache Strategy:
//   - Persistent cache (no TTL) - routes rarely change
//   - Msgpack binary format for efficiency
//   - Cache-aside pattern (lazy loading)
//   - Automatic cache creation on miss
//
// The cache structure pre-categorizes operation routes by type (source/destination)
// for fast lookup during transaction processing.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionRouteID: UUID of the transaction route
//
// Returns:
//   - mmodel.TransactionRouteCache: Cached route data with operation routes by type
//   - error: Error if not found or cache operation fails
//
// OpenTelemetry: Creates span "command.get_or_create_transaction_route_cache"
func (uc *UseCase) GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.get_or_create_transaction_route_cache")
	defer span.End()

	internalKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	cachedValue, err := uc.RedisRepo.GetBytes(ctx, internalKey)
	if err != nil && err != redis.Nil {
		logger.Warnf("Error retrieving binary transaction route from cache: %v", err.Error())
	}

	if err == nil && len(cachedValue) > 0 {
		var cacheData mmodel.TransactionRouteCache

		if err := cacheData.FromMsgpack(cachedValue); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode msgpack cache data", err)

			logger.Errorf("Failed to decode msgpack cache data: %v", err)

			return mmodel.TransactionRouteCache{}, err
		}

		return cacheData, nil
	}

	foundTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		if err == services.ErrDatabaseItemNotFound {
			msg := "Transaction route not found in database"

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, msg, err)

			logger.Warn(msg)

			return mmodel.TransactionRouteCache{}, err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to fetch transaction route from database", err)

		logger.Errorf("Error fetching transaction route from database: %v", err.Error())

		return mmodel.TransactionRouteCache{}, err
	}

	cacheData := foundTransactionRoute.ToCache()

	cacheBytes, err := cacheData.ToMsgpack()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert route to msgpack cache data", err)

		logger.Errorf("Failed to convert route to msgpack cache data: %v", err)

		return mmodel.TransactionRouteCache{}, err
	}

	err = uc.RedisRepo.SetBytes(ctx, internalKey, cacheBytes, 0)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create transaction route cache", err)

		logger.Errorf("Failed to create transaction route cache: %v", err)

		return mmodel.TransactionRouteCache{}, err
	}

	return cacheData, nil
}
