// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

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

type idempotencyLockChecker interface {
	CheckOrAcquireIdempotencyKey(ctx context.Context, key string, ttl time.Duration) (existingValue string, acquired bool, err error)
}

type redisPipelineSetter interface {
	SetPipeline(ctx context.Context, keys, values []string, ttls []time.Duration) error
}

const (
	idempotencyReplayWaitTimeout = 200 * time.Millisecond
	idempotencyReplayPollStep    = 15 * time.Millisecond
)

func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)

	if checker, ok := uc.RedisRepo.(idempotencyLockChecker); ok {
		existingValue, acquired, err := checker.CheckOrAcquireIdempotencyKey(ctx, internalKey, ttl)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

			logger.Error("Error to lock idempotency key on redis failed:", err.Error())

			return nil, err
		}

		if acquired {
			return nil, nil
		}

		if !libCommons.IsNilOrEmpty(&existingValue) {
			logger.Info("Found existing idempotency response in redis")

			return &existingValue, nil
		}

		resolvedValue, waitErr := uc.waitForInFlightIdempotencyValue(ctx, internalKey)
		if waitErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Error waiting in-flight idempotency value", waitErr)

			logger.Errorf("Error waiting in-flight idempotency value on redis: %v", waitErr)

			return nil, waitErr
		}

		if resolvedValue != nil {
			logger.Info("Resolved in-flight idempotency response from redis")

			return resolvedValue, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		logger.Warn("Failed to create idempotency key because another request is still in flight")

		return nil, err
	}

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
			logger.Info("Found existing idempotency response in redis")

			return &value, nil
		}

		resolvedValue, waitErr := uc.waitForInFlightIdempotencyValue(ctx, internalKey)
		if waitErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Error waiting in-flight idempotency value", waitErr)

			logger.Errorf("Error waiting in-flight idempotency value on redis: %v", waitErr)

			return nil, waitErr
		}

		if resolvedValue != nil {
			logger.Info("Resolved in-flight idempotency response from redis")

			return resolvedValue, nil
		}

		err = pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key)

		logger.Warn("Failed to create idempotency key because another request is still in flight")

		return nil, err
	}

	return nil, nil
}

// SetIdempotencyValueAndMapping stores the serialized idempotency response and
// reverse transaction mapping in Redis, using a single pipeline round-trip when
// the underlying Redis adapter supports it.
func (uc *UseCase) SetIdempotencyValueAndMapping(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	key, hash string,
	t transaction.Transaction,
	resultTTL, mappingTTL time.Duration,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.set_idempotency_value_and_mapping")
	defer span.End()

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, t.ID)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Errorf("Err to serialize transaction struct %v", err)
		return err
	}

	if pipeline, ok := uc.RedisRepo.(redisPipelineSetter); ok {
		if err := pipeline.SetPipeline(
			ctx,
			[]string{internalKey, reverseKey},
			[]string{value, key},
			[]time.Duration{resultTTL, mappingTTL},
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error setting idempotency values via redis pipeline", err)
			logger.Errorf("Error setting idempotency values via redis pipeline: %s", err.Error())

			return err
		}

		return nil
	}

	var setErr error

	if err := uc.RedisRepo.Set(ctx, internalKey, value, resultTTL); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting idempotency value in redis", err)
		logger.Errorf("Error setting idempotency value in redis: %s", err.Error())
		setErr = errors.Join(setErr, err)
	}

	if err := uc.RedisRepo.Set(ctx, reverseKey, key, mappingTTL); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting transaction idempotency mapping in redis", err)
		logger.Errorf("Error setting transaction idempotency mapping in redis for transactionID %s: %s", t.ID, err.Error())
		setErr = errors.Join(setErr, err)
	}

	return setErr
}

func (uc *UseCase) idempotencyReplayTimeout() time.Duration {
	if uc.IdempotencyReplayTimeout > 0 {
		return uc.IdempotencyReplayTimeout
	}

	return idempotencyReplayWaitTimeout
}

func (uc *UseCase) waitForInFlightIdempotencyValue(ctx context.Context, internalKey string) (*string, error) {
	if uc == nil || uc.RedisRepo == nil {
		return nil, nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, uc.idempotencyReplayTimeout())
	defer cancel()

	ticker := time.NewTicker(idempotencyReplayPollStep)
	defer ticker.Stop()

	for {
		if waitErr := waitCtx.Err(); waitErr != nil {
			if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
				return nil, nil
			}

			return nil, waitErr
		}

		value, err := uc.RedisRepo.Get(waitCtx, internalKey)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, nil
			}

			return nil, err
		}

		if !libCommons.IsNilOrEmpty(&value) {
			return &value, nil
		}

		select {
		case <-waitCtx.Done():
			continue
		case <-ticker.C:
		}
	}
}
