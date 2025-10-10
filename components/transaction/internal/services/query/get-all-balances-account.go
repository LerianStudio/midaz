// Package query implements read operations (queries) for the transaction service.
// This file contains the query for retrieving all balances for a specific account.
package query

import (
	"context"
	"encoding/json"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllBalancesByAccountID retrieves all balances for a specific account, enriched with metadata.
//
// This use case fetches all balance entries for an account from PostgreSQL and then
// attempts to retrieve the most up-to-date balance information from the Redis cache.
// If cached data is available, it is used to override the database values.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - accountID: The UUID of the account.
//   - filter: Query parameters for pagination and sorting.
//
// Returns:
//   - []*mmodel.Balance: A slice of balances for the account.
//   - libHTTP.CursorPagination: Pagination information for the result set.
//   - error: An error if the retrieval fails.
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
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis", err)

		logger.Warnf("Failed to get balance cache values on redis: %v", err)
	}

	for i := range balance {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value: %v", err)

				continue
			}

			balance[i].Available = cachedBalance.Available
			balance[i].OnHold = cachedBalance.OnHold
			balance[i].Version = cachedBalance.Version
		}
	}

	return balance, cur, nil
}
