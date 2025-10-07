// Package command implements write operations (commands) for the transaction service.
// This file contains command implementation.

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

// CreateOrCheckIdempotencyKey creates or checks an idempotency key in Redis.
//
// This method implements idempotency for transaction creation, preventing duplicate
// transactions from being processed. It:
// 1. Generates internal key from organization, ledger, and idempotency key
// 2. Attempts to set the key in Redis (SetNX - set if not exists)
// 3. If key already exists, retrieves the stored transaction ID
// 4. Returns nil if key was successfully created (first request)
// 5. Returns transaction ID if key already exists (duplicate request)
//
// Idempotency Flow:
//   - First request: SetNX succeeds, key is created with empty value, returns nil
//   - Duplicate request: SetNX fails, retrieves stored value, returns transaction ID
//   - After processing: SetValueOnExistingIdempotencyKey stores transaction ID
//
// Business Rules:
//   - Idempotency key is optional (defaults to request hash if not provided)
//   - TTL determines how long the key is valid (typically 24 hours)
//   - Key format: "idempotency:{org_id}:{ledger_id}:{key}"
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - key: Idempotency key from header (optional)
//   - hash: Request hash (used if key is empty)
//   - ttl: Time-to-live for the idempotency key
//
// Returns:
//   - *string: nil if first request, transaction ID if duplicate
//   - error: Redis error or ErrIdempotencyKey if key exists but has no value
//
// OpenTelemetry: Creates span "command.create_idempotency_key"
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
		} else {
			err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

			logger.Warnf("Failed, exists value on redis with this key: %v", err)

			return nil, err
		}
	}

	return nil, nil
}

// SetValueOnExistingIdempotencyKey stores the transaction result in the idempotency key.
//
// This method is called after successful transaction creation to store the transaction
// data in Redis. Subsequent requests with the same idempotency key will receive this
// stored transaction instead of creating a duplicate.
//
// Idempotency Flow:
//  1. CreateOrCheckIdempotencyKey creates empty key (first request)
//  2. Transaction is processed and created
//  3. SetValueOnExistingIdempotencyKey stores transaction data
//  4. Duplicate requests retrieve stored transaction data
//
// The method does not return errors - failures are logged but don't affect the
// transaction creation (the transaction was already created successfully).
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - key: Idempotency key from header (optional)
//   - hash: Request hash (used if key is empty)
//   - t: Created transaction to store
//   - ttl: Time-to-live for the idempotency key
//
// OpenTelemetry: Creates span "command.set_value_idempotency_key"
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
