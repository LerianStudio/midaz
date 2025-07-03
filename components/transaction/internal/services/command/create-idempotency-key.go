package command

import (
	"context"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"time"
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		logger.Error("Error to lock idempotency key on redis failed:", err.Error())
	}

	if !success {
		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		libOpentelemetry.HandleSpanError(&span, "Failed exists value on redis with this key", err)
		logger.Errorf("Failed exists value on redis with this key: %v", err)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
			return &value, err
		}

		return nil, err
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey func that set value on idempotency key to return to user.
func (uc *UseCase) SetValueOnExistingIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "command.set_value_idempotency_key")
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

// RemoveIdempotencyKey func that is responsible to remove idempotency key on redis
func (uc *UseCase) RemoveIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	
	_, span := tracer.Start(ctx, "command.remove_idempotency_key")
	defer span.End()

	logger.Infof("Trying to remove idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	err := uc.RedisRepo.Del(ctx, internalKey)
	if err != nil {
		logger.Errorf("Error to remove idempotency key on redis with this key: %s and error: %s ", internalKey, err.Error())
	}

	logger.Infof("Removed idempotency key on redis with this key: %s", internalKey)
}
