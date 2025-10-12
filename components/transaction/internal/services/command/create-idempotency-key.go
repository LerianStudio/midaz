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

// CreateOrCheckIdempotencyKey implements idempotency guarantees for transaction creation.
//
// Idempotency ensures that duplicate requests (with the same idempotency key) don't
// create duplicate transactions. This is critical for financial accuracy when dealing
// with network retries, client retries, or distributed system failures.
//
// Mechanism:
// 1. Client provides an idempotency key in the request header (or uses request hash)
// 2. First request: Sets key in Redis with SetNX (SET if Not eXists), proceeds with transaction
// 3. Duplicate request: Key already exists, returns previously stored result
// 4. After TTL expires, key is auto-deleted and request can be processed again
//
// Returns:
// - nil, nil: Key successfully locked, proceed with transaction processing
// - &value, nil: Duplicate request, return cached result instead
// - nil, error: Key exists but empty (race condition or processing), return conflict error
//
// Parameters:
//   - ctx: Request context for tracing and cancellation
//   - organizationID: Organization UUID for scoping
//   - ledgerID: Ledger UUID for scoping
//   - key: Client-provided idempotency key (or empty to use hash)
//   - hash: Request hash used as fallback key
//   - ttl: Time-to-live for the idempotency key
//
// Returns:
//   - *string: Cached result if duplicate request, nil if new request
//   - error: ErrIdempotencyKey if conflict, or Redis errors
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	// Use request hash as key if client didn't provide one
	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	// Attempt to atomically set the key (lock)
	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

		logger.Error("Error to lock idempotency key on redis failed:", err.Error())

		return nil, err
	}

	// If key already exists, check for cached result
	if !success {
		value, err := uc.RedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error to get idempotency key on redis failed", err)

			logger.Error("Error to get idempotency key on redis failed:", err.Error())

			return nil, err
		}

		// Return cached result if available
		if !libCommons.IsNilOrEmpty(&value) {
			logger.Infof("Found value on redis with this key: %v", internalKey)

			return &value, nil
		} else {
			// Key exists but has no value yet (transaction still processing)
			err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

			logger.Warnf("Failed, exists value on redis with this key: %v", err)

			return nil, err
		}
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey stores the transaction result in the idempotency key for duplicate requests.
//
// After successful transaction creation, this function stores the result in Redis
// so that duplicate requests (with the same idempotency key) can retrieve the
// original response instead of creating duplicate transactions.
//
// Parameters:
//   - ctx: Request context for tracing
//   - organizationID: Organization UUID for scoping
//   - ledgerID: Ledger UUID for scoping
//   - key: Client-provided idempotency key (or empty to use hash)
//   - hash: Request hash used as fallback key
//   - t: The created transaction to cache
//   - ttl: Time-to-live for the cached result
func (uc *UseCase) SetValueOnExistingIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	logger.Infof("Trying to set value on idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := libCommons.IdempotencyInternalKey(organizationID, ledgerID, key)

	// Serialize transaction to JSON for caching
	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Error("Err to serialize transaction struct %v\n", err)
	}

	// Store result in Redis with TTL
	err = uc.RedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Error("Error to set value on lock idempotency key on redis:", err.Error())
	}
}
