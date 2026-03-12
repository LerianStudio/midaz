// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"bytes"
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// cacheNotFoundSentinel is the sentinel value stored in Redis when a transaction route is not found in the database.
// This prevents repeated DB lookups for non-existent routes (negative caching).
var cacheNotFoundSentinel = []byte("NOT_FOUND")

// sentinelTTL is the expiration time for not-found sentinel entries in Redis.
// NOTE: SetBytes multiplies this value by time.Second internally, so 60 means 60 seconds.
const sentinelTTL = time.Duration(60)

// GetOrCreateTransactionRouteCache retrieves a transaction route cache from Redis or database with fallback.
// If the transaction route cache exists in Redis, it returns the cached data as TransactionRouteCache.
// If not found in cache, it fetches the transaction route from database and creates the cache for future use.
// Valid cache entries are persistent (no TTL). Not-found sentinel entries expire after 60 seconds.
func (uc *UseCase) GetOrCreateTransactionRouteCache(ctx context.Context, organizationID, ledgerID, transactionRouteID uuid.UUID) (mmodel.TransactionRouteCache, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.get_or_create_transaction_route_cache")
	defer span.End()

	internalKey := utils.AccountingRoutesInternalKey(organizationID, ledgerID, transactionRouteID)

	cachedValue, err := uc.RedisRepo.GetBytes(ctx, internalKey)
	if err != nil && !errors.Is(err, redis.Nil) {
		logger.Warnf("Error retrieving binary transaction route from cache: %v", err.Error())
	}

	if err == nil && len(cachedValue) > 0 {
		if bytes.Equal(cachedValue, cacheNotFoundSentinel) {
			logger.Infof("Cache hit: not-found sentinel for transaction route %s", transactionRouteID)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction route not found (sentinel cache hit)", services.ErrDatabaseItemNotFound)

			return mmodel.TransactionRouteCache{}, services.ErrDatabaseItemNotFound
		}

		var cacheData mmodel.TransactionRouteCache

		if err := cacheData.FromMsgpack(cachedValue); err != nil {
			logger.Warnf("Corrupted cache data for transaction route %s, falling back to database: %v", transactionRouteID, err)

			libOpentelemetry.HandleSpanError(&span, "Corrupted cache data, falling back to database", err)
		} else {
			return cacheData, nil
		}
	}

	foundTransactionRoute, err := uc.TransactionRouteRepo.FindByID(ctx, organizationID, ledgerID, transactionRouteID)
	if err != nil {
		var entityNotFound pkg.EntityNotFoundError

		isNotFound := errors.Is(err, services.ErrDatabaseItemNotFound) || errors.As(err, &entityNotFound)
		if isNotFound {
			msg := "Transaction route not found in database"

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, msg, err)

			logger.Warn(msg)

			if setErr := uc.RedisRepo.SetBytes(ctx, internalKey, cacheNotFoundSentinel, sentinelTTL); setErr != nil {
				logger.Warnf("Failed to store not-found sentinel in cache: %v", setErr)
			} else {
				logger.Infof("Stored not-found sentinel for transaction route %s with TTL %ds", transactionRouteID, sentinelTTL)
			}

			return mmodel.TransactionRouteCache{}, services.ErrDatabaseItemNotFound
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
