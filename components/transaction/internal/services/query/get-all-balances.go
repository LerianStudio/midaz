// Package query implements read operations (queries) for the transaction service.
// This file contains queries for retrieving balances.
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

// GetAllBalances retrieves a paginated list of all balances.
//
// This use case fetches balances from PostgreSQL and then attempts to retrieve the
// most up-to-date balance information from the Redis cache. If cached data is
// available, it is used to override the database values.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - filter: Query parameters for pagination, sorting, and date range filtering.
//
// Returns:
//   - []*mmodel.Balance: A slice of balances.
//   - libHTTP.CursorPagination: Pagination information for the result set.
//   - error: An error if the retrieval fails.
func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	balances, cur, err := uc.BalanceRepo.ListAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting balances on repo: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found")

		return nil, libHTTP.CursorPagination{}, nil
	}

	balanceCacheKeys := make([]string, len(balances))

	for i, b := range balances {
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis", err)

		logger.Warnf("Failed to get balance cache values on redis: %v", err)
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value: %v", err)

				continue
			}

			balances[i].Available = cachedBalance.Available
			balances[i].OnHold = cachedBalance.OnHold
			balances[i].Version = cachedBalance.Version
		}
	}

	return balances, cur, nil
}

// GetAllBalancesByAlias retrieves all balances for an account, identified by its alias.
//
// This use case fetches balance entries from PostgreSQL and then attempts to retrieve
// the most up-to-date balance information from the Redis cache. This version of the
// query does not enrich the balances with metadata, optimizing for performance.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - alias: The account alias to look up.
//
// Returns:
//   - []*mmodel.Balance: A slice of balances for the account.
//   - error: An error if the retrieval fails.
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_alias")
	defer span.End()

	logger.Infof("Retrieving all balances by alias")

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to list balances by alias on balance database", err)

		logger.Error("Failed to list balances by alias on balance database", err.Error())

		return nil, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(&span, "No balances found for alias")

		return nil, nil
	}

	balanceCacheKeys := make([]string, len(balances))
	for i, b := range balances {
		balanceCacheKeys[i] = libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
	}

	balanceCacheValues, err := uc.RedisRepo.MGet(ctx, balanceCacheKeys)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balance cache values on redis (alias)", err)

		logger.Warnf("Failed to get balance cache values on redis (alias): %v", err)
	}

	for i := range balances {
		if data, ok := balanceCacheValues[balanceCacheKeys[i]]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value (alias): %v", err)

				continue
			}

			balances[i].Available = cachedBalance.Available
			balances[i].OnHold = cachedBalance.OnHold
			balances[i].Version = cachedBalance.Version
		}
	}

	return balances, nil
}
