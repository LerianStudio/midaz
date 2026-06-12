// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/redis/go-redis/v9"
)

// IdempotencyRepo is the narrow CRM-local port over the shared Redis
// infrastructure. It is satisfied structurally by the transaction
// RedisConsumerRepository; the CRM use case never depends on the transaction
// idempotency methods (those are typed to transaction.Transaction and live on
// the money path).
type IdempotencyRepo interface {
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// CRMIdempotencyResult holds the outcome of a CRM idempotency claim.
//
// Replay is non-nil when the key already held a serialized entity (the caller
// should deserialize it and return it as the cached response). A nil Replay on
// a successful call means the slot was freshly claimed and the caller should
// proceed with the create.
type CRMIdempotencyResult struct {
	Replay *string
}

// HolderIdempotencyKey builds the CRM-namespaced Redis key for a holder
// create. Holders are org-scoped (no ledger). It does NOT reuse
// utils.IdempotencyInternalKey, which emits the transaction key shape.
func HolderIdempotencyKey(organizationID, key string) string {
	return fmt.Sprintf("idempotency:crm:holder:%s:%s", organizationID, key)
}

// InstrumentIdempotencyKey builds the CRM-namespaced Redis key for an
// instrument create. Instruments are scoped by their parent holder.
func InstrumentIdempotencyKey(organizationID, holderID, key string) string {
	return fmt.Sprintf("idempotency:crm:instrument:%s:%s:%s", organizationID, holderID, key)
}

// CreateOrCheckCRMIdempotency atomically claims an idempotency slot in Redis
// under the already-namespaced internalKey.
//
// On a fresh claim (SetNX succeeds) the result has a nil Replay and the caller
// proceeds with the create. On a losing claim the stored value is fetched: a
// non-empty value is returned as Replay (the cached entity JSON); an empty
// value means a concurrent request holds the slot in-flight and the call
// returns ErrIdempotencyKey.
//
// A nil Idempotency repo means the feature is disabled: the call returns a
// zero result (no claim), mirroring the streaming nil-emitter guard.
func (uc *UseCase) CreateOrCheckCRMIdempotency(ctx context.Context, internalKey, hash string, ttl time.Duration) (*CRMIdempotencyResult, error) {
	if uc.Idempotency == nil {
		return &CRMIdempotencyResult{}, nil
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_or_check_crm_idempotency")
	defer span.End()

	success, err := uc.Idempotency.SetNX(ctx, internalKey, "", ttl)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to lock idempotency key in redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to lock idempotency key in redis", libLog.Err(err))

		return &CRMIdempotencyResult{}, fmt.Errorf("failed to lock idempotency key: %w", err)
	}

	if success {
		return &CRMIdempotencyResult{}, nil
	}

	value, err := uc.Idempotency.Get(ctx, internalKey)
	if err != nil && !errors.Is(err, redis.Nil) {
		libOpentelemetry.HandleSpanError(span, "Failed to get idempotency key from redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get idempotency key from redis", libLog.Err(err))

		return &CRMIdempotencyResult{}, fmt.Errorf("failed to get idempotency value: %w", err)
	}

	if !libCommons.IsNilOrEmpty(&value) {
		logger.Log(ctx, libLog.LevelDebug, "Found cached value for CRM idempotency key lookup")

		return &CRMIdempotencyResult{Replay: &value}, nil
	}

	businessErr := pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "CreateOrCheckCRMIdempotency", hash)
	recordSpanError(span, "Idempotency key already in use", businessErr)
	logger.Log(ctx, libLog.LevelWarn, "Idempotency key already in use", libLog.Err(businessErr))

	return &CRMIdempotencyResult{}, businessErr
}

// SetCRMIdempotencyValue stores the serialized entity under the claimed slot so
// a subsequent retry replays it. A nil Idempotency repo is a no-op. Store
// failures are logged and swallowed: the create already succeeded, so the
// request must not fail on a cache write.
func (uc *UseCase) SetCRMIdempotencyValue(ctx context.Context, internalKey, valueJSON string, ttl time.Duration) {
	if uc.Idempotency == nil {
		return
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.set_crm_idempotency_value")
	defer span.End()

	if err := uc.Idempotency.Set(ctx, internalKey, valueJSON, ttl); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to store CRM idempotency value in redis", err)
		logger.Log(ctx, libLog.LevelError, "Failed to store CRM idempotency value in redis", libLog.Err(err))
	}
}
