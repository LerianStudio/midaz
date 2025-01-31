package query

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mlog"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

// GetAccountsCache retrieves cached account data from Redis and fetches missing accounts from the ledger if necessary.
func (uc *UseCase) GetAccountsCache(ctx context.Context, logger mlog.Logger, token string, organizationID, ledgerID uuid.UUID, input []string) ([]*account.Account, error) {
	span := trace.SpanFromContext(ctx)

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

		uc.SetAccountsInCache(ctx, logger, organizationID, ledgerID, accounts)
		allAccounts = append(allAccounts, accounts...)
	}

	return allAccounts, nil
}

// filterMissingAccounts identifies accounts missing in the provided accountsMap and returns a slice of these missing account IDs.
func filterMissingAccounts(accounts []string, accountsMap map[string]string) []string {
	var missing []string

	for _, acc := range accounts {
		if _, exists := accountsMap[acc]; !exists {
			missing = append(missing, acc)
		}
	}

	return missing
}

// SetAccountsInCache caches account data for a given organization and ledger to improve retrieval performance.
func (uc *UseCase) SetAccountsInCache(ctx context.Context, logger mlog.Logger, organizationID, ledgerID uuid.UUID, accounts []*account.Account) {
	span := trace.SpanFromContext(ctx)

	for _, acc := range accounts {
		am, err := json.Marshal(acc)
		if err != nil {
			logger.Warnf("Error marshaling account: %v", err)
		}

		jsonAccount := string(am)

		aliasLockAccount := pkg.LockAccount(organizationID, ledgerID, acc.GetAlias())

		err = uc.RedisRepo.Set(ctx, aliasLockAccount, jsonAccount, constant.TimeToSetAccountsInRedis)
		if err != nil && !errors.Is(err, redis.Nil) {
			mopentelemetry.HandleSpanError(&span, "Error setting account in cache", err)

			logger.Errorf("Error setting account by alias in cache: %v", err)
		}

		idLockAccount := pkg.LockAccount(organizationID, ledgerID, acc.GetId())

		err = uc.RedisRepo.Set(ctx, idLockAccount, jsonAccount, constant.TimeToSetAccountsInRedis)
		if err != nil && !errors.Is(err, redis.Nil) {
			mopentelemetry.HandleSpanError(&span, "Error setting account by id in cache", err)

			logger.Errorf("Error setting account in cache: %v", err)
		}
	}
}
