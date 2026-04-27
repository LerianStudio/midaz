// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/components/crm/internal/adapters/mongodb/idempotency"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"go.opentelemetry.io/otel/attribute"
)

// tenantIDFromContext returns the current tenant ID from context, falling back
// to the empty string in single-tenant mode. The Mongo unique index keys off
// (tenant_id, idempotency_key), so an empty tenant ID is still safe — it just
// means single-tenant deployments share a single namespace for idempotency
// keys, which is the intended behaviour.
func tenantIDFromContext(ctx context.Context) string {
	return tmcore.GetTenantIDContext(ctx)
}

// ExecuteIdempotent wraps fn with the CRM idempotency state machine.
//
// Behavior matrix:
//
//	idempotencyKey == ""                               → fn runs directly, no store (bypass)
//	key+hash match existing record                     → cached response returned, fn NOT run
//	key match + hash mismatch                          → constant.ErrIdempotencyKey (409), fn NOT run
//	no existing record                                 → fn runs; on success, response is stored
//
// T is the response type of fn. On a cache hit the stored JSON is decoded into
// a fresh T and returned; on first-time success the JSON-serialized result is
// persisted keyed by (tenantID, idempotencyKey).
//
// The ttl argument controls the TTL index expiry. Zero falls back to
// idempotency.DefaultTTL (24h), matching the Ledger-side saga's retention.
func ExecuteIdempotent[T any](
	ctx context.Context,
	repo idempotency.Repository,
	idempotencyKey string,
	requestHash string,
	ttl time.Duration,
	fn func(ctx context.Context) (*T, error),
) (*T, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.idempotency_guard.execute")
	defer span.End()

	span.SetAttributes(
		attribute.Bool("app.request.has_idempotency_key", idempotencyKey != ""),
	)

	// Bypass: no key supplied means the caller opted out of idempotency
	// protection. We simply execute fn with no persistence. This matches the
	// Ledger pattern in create_transaction_idempotency.go: when the
	// Idempotency-Key header is absent, the guard is a no-op.
	if idempotencyKey == "" {
		return fn(ctx)
	}

	if ttl <= 0 {
		ttl = idempotency.DefaultTTL
	}

	tenantID := tenantIDFromContext(ctx)

	existing, err := repo.Find(ctx, tenantID, idempotencyKey)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to probe idempotency store", err)
		logger.Log(ctx, libLog.LevelError, "Failed to probe idempotency store", libLog.Err(err))

		return nil, fmt.Errorf("idempotency guard: probe failed: %w", err)
	}

	if existing != nil {
		if existing.RequestHash != requestHash {
			// Same key, different payload → the caller is misusing the key.
			// This is a business-level conflict (caller's problem → Warn).
			conflictErr := pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "idempotency_guard", idempotencyKey)
			libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Idempotency key hash mismatch", conflictErr)
			logger.Log(ctx, libLog.LevelWarn, "Idempotency key reused with different payload", libLog.Err(conflictErr))

			return nil, conflictErr
		}

		// Cache hit — decode the stored response and return it.
		var cached T
		if err := json.Unmarshal(existing.ResponseDocument, &cached); err != nil {
			libOpenTelemetry.HandleSpanError(span, "Failed to decode cached idempotency response", err)
			logger.Log(ctx, libLog.LevelError, "Failed to decode cached idempotency response", libLog.Err(err))

			return nil, fmt.Errorf("idempotency guard: decode cached response: %w", err)
		}

		return &cached, nil
	}

	// First time seeing this key → run fn, store on success.
	result, err := fn(ctx)
	if err != nil {
		// Deliberately NOT storing failures. Retrying with the same key after
		// a transient failure must be allowed; callers should use a fresh key
		// for semantically different attempts.
		return nil, err
	}

	payload, err := json.Marshal(result)
	if err != nil {
		// The operation succeeded; failing to cache the response is worth a
		// log line but should not fail the request to the client.
		libOpenTelemetry.HandleSpanError(span, "Failed to serialize response for idempotency cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to serialize idempotency response", libLog.Err(err))

		return result, nil
	}

	now := time.Now().UTC()
	rec := &idempotency.Record{
		TenantID:         tenantID,
		IdempotencyKey:   idempotencyKey,
		RequestHash:      requestHash,
		ResponseDocument: payload,
		CreatedAt:        now,
		ExpiresAt:        now.Add(ttl),
	}

	if err := repo.Store(ctx, rec); err != nil {
		// A duplicate-key error here means a concurrent request beat us to
		// the store between Find and Store. The work is already done; we do
		// not want to discard the valid response. Log and return the result.
		libOpenTelemetry.HandleSpanError(span, "Failed to persist idempotency record", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to persist idempotency record (operation already completed)", libLog.Err(err))
	}

	return result, nil
}
