// Package command implements write operations (commands) for the transaction service.
// This file contains commands for handling idempotency keys.
package command

import (
	"context"
	"errors"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CreateOrCheckIdempotencyKey creates or checks an idempotency key in Redis to prevent duplicate transactions.
//
// This function implements the first step of an idempotency check. It attempts to
// create a lock in Redis using the provided key. If successful, it means this is
// the first request with this key. If the lock already exists, it retrieves the
// stored transaction data, indicating a duplicate request.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - key: The idempotency key from the request header.
//   - hash: A hash of the request payload, used as a fallback if the key is empty.
//   - ttl: The time-to-live for the idempotency key in Redis.
//
// Returns:
//   - *string: A pointer to the stored transaction data if it's a duplicate request, otherwise nil.
//   - error: An error if there's an issue with Redis or if the key exists but has no value.
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
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

		return nil, err
	}

	if !success {
		value, err := uc.RedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error to get idempotency key on redis failed", err)

			logger.Error("Error to get idempotency key on redis failed:", err.Error())

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(&value) {
			logger.Infof("Found value on redis with this key: %v", internalKey)

			return &value, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		logger.Warnf("Failed, exists value on redis with this key: %v", err)

		return nil, err
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey stores the transaction result in an existing idempotency key.
//
// This function is the second step of the idempotency flow. After a transaction
// has been successfully processed, this function is called to store the resulting
// transaction data in the Redis key that was created by CreateOrCheckIdempotencyKey.
// Any subsequent requests with the same idempotency key will then receive this stored data.
//
// Failures in this function are only logged and do not return an error, as the
// primary transaction has already been completed successfully.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization.
//   - ledgerID: The UUID of the ledger.
//   - key: The idempotency key from the request header.
//   - hash: A hash of the request payload, used as a fallback if the key is empty.
//   - t: The transaction data to be stored.
//   - ttl: The time-to-live for the idempotency key in Redis.
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
