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

func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	logger.Infof("Retrieving all balances")

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

	for i, b := range balances {
		cachedBalanceKey := libCommons.BalanceInternalKey(organizationID.String(), ledgerID.String(), b.Alias+"#"+b.Key)
		if data, ok := balanceCacheValues[cachedBalanceKey]; ok {
			cachedBalance := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(data), &cachedBalance); err != nil {
				logger.Warnf("Error unmarshalling balance cache value: %v", err)
				continue
			}

			balances[i].Available = cachedBalance.Available
			balances[i].OnHold = cachedBalance.OnHold
		}
	}

	return balances, cur, nil
}

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

	return balances, nil
}
