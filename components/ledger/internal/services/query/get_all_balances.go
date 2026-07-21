// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// GetAllBalances methods responsible to get all balances from a database.
	// This method is used to get all balances from a database and return them in a cursor pagination format.
	// It also validates if the balance is currently in the redis cache and if so, it uses the cached values instead of the database values.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	balances, cur, err := uc.BalanceRepo.ListAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting balances on repo", libLog.Err(err))

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found")

		return []*mmodel.Balance{}, libHTTP.CursorPagination{}, nil
	}

	balanceCacheKeys := make([]string, len(balances))

	for i, b := range balances {
		balanceCacheKeys[i] = utils.BalanceInternalKey(organizationID, ledgerID, b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.TransactionRedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache values on redis", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to get balance cache values on redis", libLog.Err(err))
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling balance cache value", libLog.Err(err))

				continue
			}

			applyBalanceCacheOverlay(balances[i], &cachedBalance)
		}
	}

	return balances, cur, nil
}

// GetAllBalancesByAlias methods responsible to get all balances from a database by alias.
// This method is used to get all balances from a database by alias and return them in a slice.
// It also validates if the balance is currently in the redis cache and if so, it uses the cached values instead of the database values.
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_alias")
	defer span.End()

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to list balances by alias on balance database", err)

		logger.Log(ctx, libLog.LevelError, "Failed to list balances by alias on balance database", libLog.Err(err))

		return nil, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found for alias")

		return nil, nil
	}

	balanceCacheKeys := make([]string, len(balances))
	for i, b := range balances {
		balanceCacheKeys[i] = utils.BalanceInternalKey(organizationID, ledgerID, b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.TransactionRedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache values on redis (alias)", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to get balance cache values on redis (alias)", libLog.Err(err))
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling balance cache value (alias)", libLog.Err(err))

				continue
			}

			applyBalanceCacheOverlay(balances[i], &cachedBalance)
		}
	}

	return balances, nil
}
