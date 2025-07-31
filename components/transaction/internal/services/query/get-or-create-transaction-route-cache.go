package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GetOrCreateTransactionRouteCache retrieves a transaction route cache from Redis or database with fallback.
// If the transaction route cache exists in Redis, it returns the cached data as TransactionRouteCache.
// If not found in cache, it fetches the transaction route from database and creates the cache for future use.
// The cache is persistent (no TTL) and stores the msgpack-encoded binary representation of the transaction route cache structure.
func (uc *UseCase) GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.get_or_create_transaction_route_cache")
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
			logger.Info("Transaction route not found in database")

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
