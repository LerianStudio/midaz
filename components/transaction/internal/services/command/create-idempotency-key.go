package command

import (
	"context"
	"errors"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// CreateOrCheckIdempotencyKey creates or validates an idempotency key for transaction deduplication.
//
// Idempotency keys prevent duplicate transaction processing when clients retry requests.
// This function uses Redis SETNX for atomic lock acquisition, ensuring only the first
// request with a given key can proceed with transaction creation.
//
// Idempotency Flow:
//
//	Case 1: New Key (first request)
//	  - SETNX succeeds, lock acquired
//	  - Returns nil, nil - caller should proceed with transaction
//	  - After transaction completes, call SetValueOnExistingIdempotencyKey
//
//	Case 2: Existing Key with Value (completed transaction)
//	  - SETNX fails, key exists
//	  - GET returns serialized transaction
//	  - Returns transaction JSON - caller should return cached result
//
//	Case 3: Existing Key without Value (in-progress transaction)
//	  - SETNX fails, key exists
//	  - GET returns empty string
//	  - Returns ErrIdempotencyKey - caller should retry later
//
// Key Structure:
//
//	Format: "idempotency:{orgID}:{ledgerID}:{key}"
//	TTL: Configurable, typically 24-48 hours
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for key namespacing
//   - ledgerID: Ledger scope for key namespacing
//   - key: Client-provided idempotency key (or empty to use hash)
//   - hash: Request body hash as fallback key
//   - ttl: Time-to-live for the idempotency key
//
// Returns:
//   - *string: Cached transaction JSON if key exists with value, nil otherwise
//   - error: Redis error or ErrIdempotencyKey if key locked by another request
//
// Error Scenarios:
//   - ErrIdempotencyKey: Key exists but transaction not yet completed
//   - Redis connection error: Redis server unavailable
//   - Redis operation error: SETNX or GET failed
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

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
		} else {
			err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

			logger.Warnf("Failed, exists value on redis with this key: %v", err)

			return nil, err
		}
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey stores the transaction result for an idempotency key.
//
// After a transaction is successfully created, this function stores the serialized
// transaction in the existing idempotency key. Subsequent requests with the same
// key will receive this cached result instead of creating a duplicate transaction.
//
// Storage Process:
//
//	Step 1: Resolve Key
//	  - Use client-provided key, or fall back to request hash
//	  - Build internal key with org/ledger scope
//
//	Step 2: Serialize Transaction
//	  - Convert transaction to JSON string
//	  - Log error if serialization fails (non-blocking)
//
//	Step 3: Store in Redis
//	  - Overwrite existing empty value with transaction JSON
//	  - Maintain original TTL from SETNX
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - organizationID: Organization scope for key namespacing
//   - ledgerID: Ledger scope for key namespacing
//   - key: Client-provided idempotency key (or empty to use hash)
//   - hash: Request body hash as fallback key
//   - t: Completed transaction to cache
//   - ttl: Time-to-live for the value
//
// Note: This function does not return errors - failures are logged but don't
// affect the transaction result. The transaction is already committed at this point.
func (uc *UseCase) SetValueOnExistingIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	logger.Infof("Trying to set value on idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Error("Err to serialize transaction struct %v\n", err)
	}

	err = uc.RedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Error("Error to set value on lock idempotency key on redis:", err.Error())
	}
}
