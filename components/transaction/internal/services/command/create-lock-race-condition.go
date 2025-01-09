package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"sync"
	"time"
)

const TimeSetLock = 2

func (uc *UseCase) AllKeysUnlocked(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []string) {
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
		go uc.checkAndReleaseLock(ctx, &wg, internalKey, resultChan)
	}

	wg.Wait()
	close(resultChan)
}

func (uc *UseCase) checkAndReleaseLock(ctx context.Context, wg *sync.WaitGroup, internalKey string, resultChan chan bool) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.check_and_release_lock")
	defer span.End()

	defer wg.Done()

	for {
		success, err := uc.RedisRepo.SetNX(context.Background(), internalKey, "processing account...", TimeSetLock)
		if err != nil {
			resultChan <- false
			return
		}

		logger.Infof("Account locked on redis: %v", internalKey)

		if success {
			resultChan <- true
			return
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func (uc *UseCase) DeleteLocks(ctx context.Context, organizationID, ledgerID uuid.UUID, keys []string) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "redis.delete_locks")
	defer span.End()

	for _, key := range keys {
		internalKey := pkg.LockInternalKey(organizationID, ledgerID, key)

		logger.Infof("Account releasing lock on redis: %v", internalKey)

		err := uc.RedisRepo.Del(ctx, internalKey)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to release Accounts lock", err)

			logger.Errorf("Failed to release Accounts lock: %v", err)
		}
	}
}
