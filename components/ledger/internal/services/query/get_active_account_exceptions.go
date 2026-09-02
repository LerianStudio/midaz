// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// accountExceptionsCacheTTL bounds how long a cached exception set can outlive a
// missed invalidation. 5 minutes mirrors SettingsCacheTTL: exceptions are a
// configuration rule invalidated explicitly by the CRUD write path, so the TTL is
// only a floor on the blast radius of an invalidation that was never delivered,
// never the primary consistency mechanism.
const accountExceptionsCacheTTL = 5 * time.Minute

// GetActiveAccountExceptions returns every live (non-deleted) account exception for
// one account, oldest first, cache-first.
//
// "Active" here means live, NOT temporally valid: the validity window is evaluated by
// the enrichment at the instant of the decision, not by this query. The query delivers
// the account's living rules; the cache stores no temporal state.
//
// Cache-aside semantics:
//   - a cache hit (including the literal "[]" empty set) is served without touching Postgres;
//   - a miss ("" from Get) reads Postgres and repopulates the cache with a TTL;
//   - a Redis read error is a graceful-degradation path — Postgres is queried directly;
//   - when Postgres also errors, the error is returned. The fail-closed decision on a
//     read failure belongs to the caller (the transaction-time enrichment, Task 2.3.4).
func (uc *UseCase) GetActiveAccountExceptions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) ([]*mmodel.AccountException, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_active_account_exceptions")
	defer span.End()

	cacheKey := utils.AccountExceptionsInternalKey(organizationID, ledgerID, accountID)

	// Cache read (best-effort). A hit — empty set included — is authoritative.
	if exceptions, ok := uc.readAccountExceptionsFromCache(ctx, tracer, logger, cacheKey); ok {
		return exceptions, nil
	}

	exceptions, err := uc.AccountExceptionRepo.ListByAccountID(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list account exceptions", err)
		logger.Log(ctx, libLog.LevelError, "Failed to list account exceptions from database", libLog.Err(err))

		return nil, err
	}

	// Cache write (best-effort). An empty result is cached as "[]" so a would-be-deny
	// of an account with no rules does not hit Postgres every time.
	uc.writeAccountExceptionsToCache(ctx, tracer, logger, cacheKey, exceptions)

	return exceptions, nil
}

// readAccountExceptionsFromCache attempts to read the exception set from Redis.
// Returns (exceptions, true) on a hit — a cached "[]" is a hit for an empty slice —
// or (nil, false) on miss, error or unmarshal failure. Errors are logged but never
// propagated: a cache failure simply falls the caller through to Postgres.
func (uc *UseCase) readAccountExceptionsFromCache(ctx context.Context, tracer trace.Tracer, logger libLog.Logger, cacheKey string) ([]*mmodel.AccountException, bool) {
	if uc.OnboardingRedisRepo == nil {
		return nil, false
	}

	cacheCtx, cacheSpan := tracer.Start(ctx, "query.get_active_account_exceptions.cache_read")
	defer cacheSpan.End()

	cached, err := uc.OnboardingRedisRepo.Get(cacheCtx, cacheKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(cacheSpan, "Cache read error", err)
		logger.Log(cacheCtx, libLog.LevelWarn, "Account exceptions cache read error, falling back to database", libLog.Err(err))

		return nil, false
	}

	// "" is the adapter's cache-miss sentinel — distinct from the cached-empty "[]".
	if cached == "" {
		return nil, false
	}

	var exceptions []*mmodel.AccountException
	if err := json.Unmarshal([]byte(cached), &exceptions); err != nil {
		libOpentelemetry.HandleSpanError(cacheSpan, "Failed to unmarshal cached account exceptions", err)
		logger.Log(cacheCtx, libLog.LevelWarn, "Failed to unmarshal cached account exceptions, falling back to database", libLog.Err(err))

		return nil, false
	}

	// Normalize a JSON null (or absent array) to an empty slice so callers never
	// have to distinguish nil from empty on a hit.
	if exceptions == nil {
		exceptions = []*mmodel.AccountException{}
	}

	logger.Log(cacheCtx, libLog.LevelDebug, "Cache hit for account exceptions", libLog.String("cache_entry", cacheKey))

	return exceptions, true
}

// writeAccountExceptionsToCache stores the exception set in Redis for future reads.
// An empty result is stored as the literal "[]" — never the empty string, which the
// adapter contract forbids and which the read path reads as a miss. Errors are logged
// but never propagated: a cache write failure does not affect the read result.
func (uc *UseCase) writeAccountExceptionsToCache(ctx context.Context, tracer trace.Tracer, logger libLog.Logger, cacheKey string, exceptions []*mmodel.AccountException) {
	if uc.OnboardingRedisRepo == nil {
		return
	}

	cacheCtx, cacheSpan := tracer.Start(ctx, "query.get_active_account_exceptions.cache_write")
	defer cacheSpan.End()

	payload := "[]"

	if len(exceptions) > 0 {
		data, err := json.Marshal(exceptions)
		if err != nil {
			libOpentelemetry.HandleSpanError(cacheSpan, "Failed to marshal account exceptions for cache", err)
			logger.Log(cacheCtx, libLog.LevelWarn, "Failed to marshal account exceptions for cache", libLog.Err(err))

			return
		}

		payload = string(data)
	}

	if err := uc.OnboardingRedisRepo.Set(cacheCtx, cacheKey, payload, accountExceptionsCacheTTL); err != nil {
		libOpentelemetry.HandleSpanError(cacheSpan, "Failed to cache account exceptions", err)
		logger.Log(cacheCtx, libLog.LevelWarn, "Failed to cache account exceptions", libLog.Err(err))

		return
	}

	logger.Log(cacheCtx, libLog.LevelDebug, "Cached account exceptions", libLog.String("cache_entry", cacheKey))
}
