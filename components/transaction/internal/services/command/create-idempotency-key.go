package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) error {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create an idempotency key operation telemetry entity
	op := uc.Telemetry.NewTransactionOperation("idempotency_key", key)

	// Add important attributes
	op.WithAttributes(
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
		attribute.String("hash", hash),
	)

	// Start tracing for this operation
	ctx = op.StartTrace(ctx)

	// Record systemic metric to track operation count
	op.RecordSystemicMetric(ctx)

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := pkg.InternalKey(organizationID, ledgerID, key)

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		// Record error
		op.RecordError(ctx, "idempotency_key_redis_error", err)
		op.End(ctx, "failed")

		logger.Error("Error to lock idempotency key on redis failed:", err.Error())
	}

	if !success {
		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		// Record duplicate key as error
		op.RecordError(ctx, "idempotency_key_duplicate", err)
		op.WithAttributes(
			attribute.String("organization_id", organizationID.String()),
			attribute.String("ledger_id", ledgerID.String()),
		)
		op.End(ctx, "duplicate")

		return err
	}

	// Record business metrics - TTL
	op.RecordBusinessMetric(ctx, "ttl_seconds", float64(ttl.Seconds()))

	// Mark operation as successful
	op.End(ctx, "success")

	return nil
}
