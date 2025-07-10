package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GetOrCreateTransactionRouteCache retrieves a transaction route cache from Redis or database with fallback.
// If the transaction route cache exists in Redis, it returns the cached data as a string.
// If not found in cache, it fetches the transaction route from database and creates the cache for future use.
// The cache is persistent (no TTL) and stores the JSON representation of the transaction route cache structure.
func (uc *UseCase) GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) (string, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.get_or_create_transaction_route_cache")
	defer span.End()

	logger.Infof("Trying to retrieve transaction route cache for ID: %s", transactionRouteID)

	internalKey := libCommons.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID.String())

	cachedValue, err := uc.RedisRepo.Get(ctx, internalKey)
	if err != nil && err != redis.Nil {
		logger.Warnf("Error retrieving transaction route from cache: %v", err.Error())
	}

	if err == nil && cachedValue != "" {
		logger.Infof("Transaction route cache hit for ID: %s", transactionRouteID)
		return cachedValue, nil
	}

	logger.Infof("Transaction route cache miss for ID: %s, fetching from database", transactionRouteID)

	foundTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		if err == services.ErrDatabaseItemNotFound {
			logger.Infof("Transaction route not found for ID: %s", transactionRouteID)
			return "", err
		}

		libOpentelemetry.HandleSpanError(&span, "Failed to fetch transaction route from database", err)

		logger.Errorf("Error fetching transaction route from database: %v", err.Error())

		return "", err
	}

	if err := uc.CreateAccountingRouteCache(ctx, foundTransactionRoute); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to cache transaction route", err)

		logger.Warnf("Failed to cache transaction route with ID %s: %v", transactionRouteID, err)
	}

	cacheData, err := foundTransactionRoute.ToCacheData()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert transaction route to cache data", err)

		logger.Errorf("Failed to convert transaction route to cache data: %v", err.Error())

		return "", err
	}

	return cacheData, nil
}
