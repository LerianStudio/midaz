package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	// Record operation metrics
	uc.recordBusinessMetrics(ctx, "idempotency_key_check_attempt",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("hash", hash))

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := pkg.InternalKey(organizationID, ledgerID, key)

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		// Record error metrics
		uc.recordTransactionError(ctx, "idempotency_key_redis_error",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("key", key),
			attribute.String("error_detail", err.Error()))

		// Record duration with error
		uc.recordTransactionDuration(ctx, startTime, "idempotency_key", "error",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("error", "redis_error"))

		logger.Error("Error to lock idempotency key on redis failed:", err.Error())
	}

	if !success {
		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		mopentelemetry.HandleSpanError(&span, "Failed exists value on redis with this key", err)

		// Record duplicate key metrics
		uc.recordBusinessMetrics(ctx, "idempotency_key_duplicate",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("key", key))

		// Record transaction error
		uc.recordTransactionError(ctx, "idempotency_key_duplicate",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
			attribute.String("key", key))

		// Record duration with duplicate status
		uc.recordTransactionDuration(ctx, startTime, "idempotency_key", "duplicate",
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()))

		return err
	}

	// Record success metrics
	uc.recordBusinessMetrics(ctx, "idempotency_key_success",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("key", key),
		attribute.Int64("ttl_seconds", int64(ttl.Seconds())))

	// Record duration with success
	uc.recordTransactionDuration(ctx, startTime, "idempotency_key", "success",
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()))

	return nil
}
