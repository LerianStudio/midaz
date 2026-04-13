// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// TransactionIdempotencyResult holds the outcome of an idempotency check.
//
//   - Replay is non-nil when the key already existed and contained a cached
//     transaction response (the caller should return this directly).
//   - InternalKey is always set and used for cleanup (Del) on error paths.
type TransactionIdempotencyResult struct {
	Replay      *transaction.Transaction
	InternalKey *string
}

// CreateOrCheckTransactionIdempotency atomically claims an idempotency slot in Redis.
//
// If the key is new (SetNX succeeds), the result contains no Replay and the
// caller should proceed with the transaction. If the key already holds a
// serialized transaction, the result contains the deserialized Replay and the
// caller should return it directly as a cached response.
//
// InternalKey is always populated so the caller can clean up on error.
func (uc *UseCase) CreateOrCheckTransactionIdempotency(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*TransactionIdempotencyResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
	result := &TransactionIdempotencyResult{InternalKey: &internalKey}

	success, err := uc.TransactionRedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to lock idempotency key in redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to lock idempotency key in redis", libLog.Err(err))

		return result, fmt.Errorf("failed to lock idempotency key: %w", err)
	}

	if !success {
		value, err := uc.TransactionRedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(span, "Failed to get idempotency key from redis", err)
			logger.Log(ctx, libLog.LevelError, "Failed to get idempotency key from redis", libLog.Err(err))

			return result, fmt.Errorf("failed to get idempotency value: %w", err)
		}

		if !libCommons.IsNilOrEmpty(&value) {
			logger.Log(ctx, libLog.LevelInfo, "Found cached value for idempotency key lookup")

			replay := &transaction.Transaction{}
			if err := json.Unmarshal([]byte(value), replay); err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to deserialize idempotency transaction from redis", err)
				logger.Log(ctx, libLog.LevelError, "Failed to deserialize idempotency transaction from redis", libLog.Err(err))

				return result, err
			}

			result.Replay = replay

			return result, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckTransactionIdempotency", key)
		logger.Log(ctx, libLog.LevelWarn, "Idempotency key already in use", libLog.Err(err))

		return result, err
	}

	return result, nil
}

// SetTransactionIdempotencyValue func that set value on idempotency key to return to user.
func (uc *UseCase) SetTransactionIdempotencyValue(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Trying to set value on idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to serialize transaction for idempotency", libLog.Err(err))
		return // Do not store invalid data
	}

	err = uc.TransactionRedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to store idempotency value in redis", libLog.Err(err))
	}
}
