package query

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func (uc *UseCase) GetAccountsCache(ctx context.Context, token string, organizationID, ledgerID uuid.UUID, input []string) ([]*account.Account, error) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "query.get_accounts_cache")
	defer span.End()

	allAccounts := make([]*account.Account, 0)

	accountsMap := make(map[string]string)
	for _, acc := range input {
		lockAccount := pkg.LockAccount(organizationID, ledgerID, acc)

		val, err := uc.RedisRepo.Get(ctx, lockAccount)
		if !errors.Is(err, redis.Nil) {
			logger.Errorf("Error getting account from redis: %v", err)
		}

		if !pkg.IsNilOrEmpty(&val) {
			accountsMap[acc] = val
			var redisAccount account.Account
			if err := json.Unmarshal([]byte(val), &redisAccount); err != nil {
				logger.Errorf("Error unmarshaling account: %v", err)
			}

			allAccounts = append(allAccounts, &redisAccount)
		}
	}

	missingAccounts := filterMissingAccounts(input, accountsMap)
	if len(missingAccounts) != 0 {
		logger.Infof("Missing accounts in cache: %v", missingAccounts)
		accounts, err := uc.GetAccountsLedger(ctx, logger, token, organizationID, ledgerID, missingAccounts)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get grpc accounts on ledger", err)

			return nil, err
		}

		uc.SetAccountsInCache(ctx, organizationID, ledgerID, accounts)
		allAccounts = append(allAccounts, accounts...)
	}

	return allAccounts, nil
}

func filterMissingAccounts(accounts []string, accountsMap map[string]string) []string {
	var missing []string

	for _, acc := range accounts {
		if _, exists := accountsMap[acc]; !exists {
			missing = append(missing, acc)
		}
	}

	return missing
}

func (uc *UseCase) SetAccountsInCache(ctx context.Context, organizationID, ledgerID uuid.UUID, accounts []*account.Account) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "query.set_accounts_in_cache")
	defer span.End()

	for _, acc := range accounts {
		lockAccount := pkg.LockAccount(organizationID, ledgerID, acc.GetAlias())

		err := uc.RedisRepo.Set(ctx, lockAccount, acc, constant.TimeToSetAccountsInRedis)
		if !errors.Is(err, redis.Nil) {
			mopentelemetry.HandleSpanError(&span, "Error setting account in cache", err)

			logger.Errorf("Error setting account in cache: %v", err)
		}
	}
}
