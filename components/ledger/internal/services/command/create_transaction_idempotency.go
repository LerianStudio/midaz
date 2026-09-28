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

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
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

// transactionIdempotencySlot is the value stored in a transaction idempotency
// slot: the transaction's JSON object plus the fingerprint of the request that
// created it. The fingerprint sits beside the transaction's own fields rather than
// in an envelope, because pods that predate it decode the slot straight into a
// transaction.Transaction and must keep replaying it. Embedding flattens the
// transaction's fields only because transaction.Transaction defines no MarshalJSON.
type transactionIdempotencySlot struct {
	transaction.Transaction
	RequestFingerprint string `json:"idempotencyFingerprint,omitempty"`
}

// CreateOrCheckTransactionIdempotency atomically claims an idempotency slot in Redis.
//
// If the key is new (SetNX succeeds), the result contains no Replay and the
// caller should proceed with the transaction. If the key already holds a
// serialized transaction created by the same request (same fingerprint), the
// result contains the deserialized Replay and the caller should return it
// directly as a cached response. A slot created by a different request, or one
// still being created, answers ErrIdempotencyKey. A slot written without a
// fingerprint replays regardless of the request.
//
// InternalKey is always populated so the caller can clean up on error.
func (uc *UseCase) CreateOrCheckTransactionIdempotency(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash, fingerprint string, ttl time.Duration) (*TransactionIdempotencyResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	// Recorded before the key falls back to the hash, so a conflict on a key the
	// client chose can be told apart from two key-less requests that share a body hash.
	explicitKey := key != ""

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
			logger.Log(ctx, libLog.LevelDebug, "Found cached value for idempotency key lookup")

			slot := &transactionIdempotencySlot{}
			if err := json.Unmarshal([]byte(value), slot); err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to deserialize idempotency transaction from redis", err)
				logger.Log(ctx, libLog.LevelError, "Failed to deserialize idempotency transaction from redis", libLog.Err(err))

				return result, err
			}

			if slot.RequestFingerprint != "" && slot.RequestFingerprint != fingerprint {
				err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckTransactionIdempotency", key)
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency key reused with a different request", err)
				logger.Log(ctx, libLog.LevelWarn, "Idempotency key reused with a different request",
					libLog.Bool("idempotency_key_explicit", explicitKey), libLog.Err(err))

				return result, err
			}

			result.Replay = &slot.Transaction

			return result, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckTransactionIdempotency", key)
		logger.Log(ctx, libLog.LevelWarn, "Idempotency key already in use", libLog.Err(err))

		return result, err
	}

	return result, nil
}

// SetTransactionIdempotencyValue func that set value on idempotency key to return to user.
// The fingerprint is stored with the transaction so a later request reusing the
// key can be told apart from a retry of this one.
func (uc *UseCase) SetTransactionIdempotencyValue(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash, fingerprint string, t transaction.Transaction, ttl time.Duration) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_value_idempotency_key")
	defer span.End()

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	value, err := libCommons.StructToJSONString(transactionIdempotencySlot{Transaction: t, RequestFingerprint: fingerprint})
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to serialize transaction for idempotency", libLog.Err(err))
		return // Do not store invalid data
	}

	err = uc.TransactionRedisRepo.Set(ctx, internalKey, value, ttl)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Failed to store idempotency value in redis", libLog.Err(err))
	}
}
