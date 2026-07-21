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

	// GetAllBalancesByAccountID methods responsible to get all balances by account id from a database.
	// This method is used to get all balances by account id from a database and return them in a cursor pagination format.
	// It also validates if the balance is currently in the redis cache and if so, it uses the cached values instead of the database values.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_account_id")
	defer span.End()

	balance, cur, err := uc.BalanceRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID, filter.ToCursorPagination())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balances on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error getting balances on repo", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balance) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found")

		return balance, cur, nil
	}

	balanceCacheKeys := make([]string, len(balance))

	for i, b := range balance {
		balanceCacheKeys[i] = utils.BalanceInternalKey(organizationID, ledgerID, b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.TransactionRedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get balance cache values on redis", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to get balance cache values on redis", libLog.Err(err))
	}

	for i := range balance {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Log(ctx, libLog.LevelWarn, "Error unmarshalling balance cache value", libLog.Err(err))

				continue
			}

			applyBalanceCacheOverlay(balance[i], &cachedBalance)
		}
	}

	return balance, cur, nil
}
