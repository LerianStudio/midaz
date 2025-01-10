package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mgrpc/account"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"strconv"
	"sync"
	"time"
)

func (uc *UseCase) AllKeysUnlocked(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []string, hash string) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.all_keys_unlocked")
	defer span.End()

	var wg sync.WaitGroup
	resultChan := make(chan bool, len(keys))

	for _, key := range keys {
		internalKey := pkg.LockInternalKey(organizationID, ledgerID, key)

		logger.Infof("Account try to lock on redis: %v", internalKey)

		wg.Add(1)
		go uc.checkAndReleaseLock(ctx, &wg, internalKey, hash, resultChan)
	}

	wg.Wait()
	close(resultChan)
}

func (uc *UseCase) checkAndReleaseLock(ctx context.Context, wg *sync.WaitGroup, internalKey, hash string, resultChan chan bool) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.check_and_release_lock")
	defer span.End()

	defer wg.Done()

	for {
		success, err := uc.RedisRepo.SetNX(context.Background(), internalKey, hash, constant.TimeSetLock)
		if err != nil {
			resultChan <- false
			return
		}

		logger.Infof("Account locked on redis: %v", internalKey)

		if success {
			resultChan <- true
			return
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func (uc *UseCase) DeleteLocks(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []string, hash string) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.delete_locks")
	defer span.End()

	for _, key := range keys {
		internalKey := pkg.LockInternalKey(organizationID, ledgerID, key)

		logger.Infof("Account releasing lock on redis: %v", internalKey)

		val, err := uc.RedisRepo.Get(ctx, key)
		if !errors.Is(err, redis.Nil) && err != nil && val == hash {
			err = uc.RedisRepo.Del(ctx, internalKey)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to release Accounts lock", err)

				logger.Errorf("Failed to release Accounts lock: %v", err)
			}
		}
	}
}

func (uc *UseCase) LockBalanceVersion(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []string, accounts []*account.Account) (bool, error) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.lock_balance_version")
	defer span.End()

	accountsMap := make(map[string]*account.Account)
	for _, acc := range accounts {
		accountsMap[acc.Id] = acc
		accountsMap[acc.Alias] = acc
	}

	for _, key := range keys {
		if acc, exists := accountsMap[key]; exists {
			balanceAvailable := strconv.FormatFloat(acc.Balance.Available, 'f', -1, 64)

			internalKey := pkg.LockInternalKey(organizationID, ledgerID, key) + ":" + balanceAvailable

			logger.Infof("Account balance version releasing lock on redis: %v", internalKey)

			success, err := uc.RedisRepo.SetNX(context.Background(), internalKey, constant.ValueBalanceLock, constant.TimeSetLockBalance)
			if err != nil {
				mopentelemetry.HandleSpanError(&span, "Failed to lock Account balance version: ", err)

				logger.Errorf("Failed to lock Account balance version: %v", err)

				return false, err
			}

			if !success {
				logger.Infof("Lock already exists for key, get Accounts again: %v", internalKey)

				return true, nil
			}
		}
	}

	return false, nil
}
