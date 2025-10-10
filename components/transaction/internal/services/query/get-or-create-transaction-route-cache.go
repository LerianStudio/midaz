// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving a transaction route from the cache.
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

// GetOrCreateTransactionRouteCache retrieves a transaction route from the cache,
// with a fallback to the database.
//
// This use case implements a cache-aside pattern. It first attempts to fetch the
// transaction route from the Redis cache. If the route is not found in the cache,
// it retrieves it from PostgreSQL, stores it in the cache for future requests,
// and then returns the data.
//
// The cached data is pre-categorized by operation route type (source/destination)
// for efficient lookups during transaction processing.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - transactionRouteID: The UUID of the transaction route.
//
// Returns:
//   - mmodel.TransactionRouteCache: The cached transaction route data.
//   - error: An error if the route is not found or if a cache operation fails.
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
