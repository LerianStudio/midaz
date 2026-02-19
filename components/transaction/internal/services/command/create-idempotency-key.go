package command

import (
	"context"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, *string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

		logger.Error("Error to lock idempotency key on redis failed:", err.Error())

		return nil, &internalKey, err
	}

	if !success {
		value, err := uc.RedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error to get idempotency key on redis failed", err)

			logger.Error("Error to get idempotency key on redis failed:", err.Error())

			return nil, &internalKey, err
		}

		if !libCommons.IsNilOrEmpty(&value) {
			logger.Infof("Found cached value for idempotency key lookup")

			return &value, &internalKey, nil
		} else {
			err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

			logger.Warnf("Failed, exists value on redis with this key: %v", err)

			return nil, &internalKey, err
		}
	}

	return nil, &internalKey, nil
}

// SetValueOnExistingIdempotencyKey func that set value on idempotency key to return to user.
func (uc *UseCase) SetValueOnExistingIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	logger.Infof("Trying to set value on idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Error("Err to serialize transaction struct %v\n", err)
	}

	err = uc.RedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Error("Error to set value on lock idempotency key on redis:", err.Error())
	}
}
