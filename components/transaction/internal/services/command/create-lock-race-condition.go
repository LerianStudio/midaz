package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
)

const TimeSetLock = 30

func (uc *UseCase) CreateLockAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) (bool, error) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "query.create_lock_race_condition")
	defer span.End()

	internalKey := pkg.LockInternalKey(organizationID, ledgerID, key)

	isLocked, err := uc.RedisRepo.SetNX(ctx, internalKey, "processing account...", TimeSetLock)
	if err != nil {
		logger.Infof("Error to acquire lock: %v", err)

		return false, err
	}

	logger.Infof("Account lock on redis: %v", internalKey)
	return isLocked, nil
}

func (uc *UseCase) ReleaseLockAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, key string) {
	logger := pkg.NewLoggerFromContext(context.Background())
	tracer := pkg.NewTracerFromContext(context.Background())

	ctx, span := tracer.Start(ctx, "query.release_lock_account")
	defer span.End()

	internalKey := pkg.LockInternalKey(organizationID, ledgerID, key)

	logger.Infof("Account release lock on redis: %v", internalKey)

	err := uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to release Accounts lock", err)

		logger.Errorf("Failed to release Accounts lock: %v", err)
	}
}
