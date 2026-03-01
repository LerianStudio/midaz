// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
)

// GetAllBalancesByAccountID methods responsible to get all balances by account id from a database.
// This method is used to get all balances by account id from a database and return them in a cursor pagination format.
// It also validates if the balance is currently in the redis cache and if so, it uses the cached values instead of the database values.
func (uc *UseCase) GetAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_account_id")
	defer span.End()

	balance, cur, err := uc.BalanceRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

		logger.Errorf("Error getting balances on repo: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balance) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found")

		return balance, cur, nil
	}

	balanceCacheKeys := make([]string, len(balance))

	for i, b := range balance {
		cacheKey, cacheKeyErr := uc.resolveBalanceCacheKey(ctx, organizationID, ledgerID, b.Alias, b.Key)
		if cacheKeyErr != nil {
			// Fail-open: if shard routing fails, fall back to the non-sharded key
			// so the database data (which already succeeded) is never discarded.
			libOpentelemetry.HandleSpanEvent(&span, "Failed to resolve balance cache key, falling back to non-sharded key")
			logger.Warnf("Failed to resolve balance cache key, falling back to non-sharded key: %v", cacheKeyErr)

			cacheKey = utils.BalanceInternalKey(organizationID, ledgerID, b.Alias+"#"+b.Key)
		}

		balanceCacheKeys[i] = cacheKey
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis", err)

		logger.Warnf("Failed to get balance cache values on redis: %v", err)
	}

	for i := range balance {
		data, ok := balanceCacheValues[balanceCacheKeys[i]]
		if !ok {
			continue
		}

		cachedBalance := mmodel.BalanceRedis{}

		if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
			logger.Warnf("Error unmarshalling balance cache value: %v", err)

			continue
		}

		balance[i].Available = cachedBalance.Available
		balance[i].OnHold = cachedBalance.OnHold
		balance[i].Version = cachedBalance.Version
	}

	return balance, cur, nil
}
