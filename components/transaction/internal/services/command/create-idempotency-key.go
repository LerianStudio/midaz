// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
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
	idempotencyHashKeySuffix     = ":hash"
)

// CreateOrCheckIdempotencyKey checks whether an idempotency key already exists
// in Redis. If it does, the cached response is returned; otherwise, a new key
// is claimed so the caller can proceed with the original request.
//
//nolint:gocognit,gocyclo,cyclop,nestif,funlen
func (uc *UseCase) CreateOrCheckIdempotencyKey(ctx context.Context, organizationID, ledgerID uuid.UUID, key, hash string, ttl time.Duration) (*string, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_idempotency_key")
	defer span.End()

	logger.Infof("Trying to create or check idempotency key in redis")

	if key == "" {
		key = hash
	}

	internalKey := utils.IdempotencyInternalKey(organizationID, ledgerID, key)
	hashKey := internalKey + idempotencyHashKeySuffix

	if checker, ok := uc.RedisRepo.(idempotencyLockChecker); ok {
		existingValue, acquired, err := checker.CheckOrAcquireIdempotencyKey(ctx, internalKey, ttl)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

			logger.Error("Error to lock idempotency key on redis failed:", err)

			return nil, err
		}

		if acquired {
			if claimErr := uc.claimIdempotencyHash(ctx, hashKey, hash, key, ttl); claimErr != nil {
				return nil, claimErr
			}

			return nil, nil
		}

		if hashErr := uc.ensureIdempotencyHashMatches(ctx, hashKey, hash, key); hashErr != nil {
			return nil, hashErr
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

		err = fmt.Errorf("create idempotency key: %w", pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key))

		logger.Warn("Failed to create idempotency key because another request is still in flight")

		return nil, err
	}

	success, err := uc.RedisRepo.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error to lock idempotency key on redis failed", err)

		logger.Error("Error to lock idempotency key on redis failed:", err)

		return nil, err
	}

	if !success {
		if hashErr := uc.ensureIdempotencyHashMatches(ctx, hashKey, hash, key); hashErr != nil {
			return nil, hashErr
		}

		value, err := uc.RedisRepo.Get(ctx, internalKey)
		if err != nil && !errors.Is(err, redis.Nil) {
			libOpentelemetry.HandleSpanError(&span, "Error to get idempotency key on redis failed", err)

			logger.Error("Error to get idempotency key on redis failed:", err)

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

		err = fmt.Errorf("create idempotency key: %w", pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key))

		logger.Warn("Failed to create idempotency key because another request is still in flight")

		return nil, err
	}

	if claimErr := uc.claimIdempotencyHash(ctx, hashKey, hash, key, ttl); claimErr != nil {
		return nil, claimErr
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
	hashKey := internalKey + idempotencyHashKeySuffix
	reverseKey := utils.IdempotencyReverseKey(organizationID, ledgerID, t.ID)

	value, err := libCommons.StructToJSONString(t)
	if err != nil {
		logger.Errorf("Err to serialize transaction struct %v", err)
		return fmt.Errorf("failed to serialize transaction: %w", err)
	}

	if pipeline, ok := uc.RedisRepo.(redisPipelineSetter); ok {
		if err := pipeline.SetPipeline(
			ctx,
			[]string{internalKey, reverseKey, hashKey},
			[]string{value, key, hash},
			[]time.Duration{resultTTL, mappingTTL, resultTTL},
		); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Error setting idempotency values via redis pipeline", err)
			logger.Errorf("Error setting idempotency values via redis pipeline: %s", err)

			return err
		}

		return nil
	}

	var setErr error

	if err := uc.RedisRepo.Set(ctx, internalKey, value, resultTTL); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting idempotency value in redis", err)
		logger.Errorf("Error setting idempotency value in redis: %s", err)
		setErr = errors.Join(setErr, err)
	}

	if err := uc.RedisRepo.Set(ctx, reverseKey, key, mappingTTL); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting transaction idempotency mapping in redis", err)
		logger.Errorf("Error setting transaction idempotency mapping in redis for transactionID %s: %s", t.ID, err)
		setErr = errors.Join(setErr, err)
	}

	if err := uc.RedisRepo.Set(ctx, hashKey, hash, resultTTL); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error setting idempotency hash in redis", err)
		logger.Errorf("Error setting idempotency hash in redis: %s", err)
		setErr = errors.Join(setErr, err)
	}

	return setErr
}

func (uc *UseCase) claimIdempotencyHash(ctx context.Context, hashKey, hash, key string, ttl time.Duration) error {
	if uc == nil || uc.RedisRepo == nil || hash == "" {
		return nil
	}

	claimed, err := uc.RedisRepo.SetNX(ctx, hashKey, hash, ttl)
	if err != nil {
		return err
	}

	if claimed {
		return nil
	}

	return uc.ensureIdempotencyHashMatches(ctx, hashKey, hash, key)
}

func (uc *UseCase) ensureIdempotencyHashMatches(ctx context.Context, hashKey, hash, key string) error {
	if uc == nil || uc.RedisRepo == nil || hash == "" {
		return nil
	}

	existingHash, err := uc.RedisRepo.Get(ctx, hashKey)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}

		return err
	}

	if !libCommons.IsNilOrEmpty(&existingHash) && existingHash != hash {
		return pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckIdempotencyKey", key) //nolint:wrapcheck
	}

	return nil
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
