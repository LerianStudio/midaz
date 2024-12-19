package command

import (
	"context"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"time"
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, key, hash string, ttl time.Duration) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create-idempotency-key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	value, err := uc.RedisRepo.Get(ctx, key)
	if err != nil {
		logger.Error("Error to get idempotency key on redis failed:", err.Error())
	}
	if value == "" {
		err = uc.RedisRepo.Set(ctx, key, hash, ttl)
		if err != nil {
			logger.Error("Error to set idempotency key on redis failed:", err.Error())
		}
	} else {
		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "createTransaction", key)

		mopentelemetry.HandleSpanError(&span, "Failed exists value on redis with this key", err)

		return err
	}

	return nil
}
