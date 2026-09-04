// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
)

const (
	// balanceDeleteMarkerTTLSeconds is the lifetime, in whole seconds, of a balance
	// delete marker key. It is deliberately short: it must outlive the worst-case
	// duration of a delete so the honored-lock pre-pass keeps rejecting mutations for the
	// whole operation, yet expire quickly so a process that crashes mid-delete self-heals
	// instead of leaving a permanently poisoned balance key. It is intentionally NOT the
	// 3600s balance cache TTL. SetNX multiplies its ttl argument by time.Second, so it is
	// passed as a whole-second count via time.Duration(balanceDeleteMarkerTTLSeconds).
	balanceDeleteMarkerTTLSeconds = 30

	// deleteMarkerKeySuffix is appended to a balance's internal cache key to form its delete marker
	// key. It MUST match the suffix the Redis Lua pre-pass derives (ARGV[i] .. ":deleted").
	deleteMarkerKeySuffix = ":deleted"

	// deleteMarkerValue is the placeholder stored under a delete marker key. The value is
	// never read: only the key's existence is meaningful to the honored-lock pre-pass.
	deleteMarkerValue = "1"
)

// balanceCacheKeyFor returns the un-prefixed internal cache key for a balance. The
// TransactionRedisRepo wrappers apply tenant namespacing to the whole key, so callers pass
// this output directly.
func balanceCacheKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)
}

// deleteMarkerKeyFor returns the delete marker key for a balance: its internal cache key plus the
// ":deleted" suffix. The delete marker is a SEPARATE key from the balance cache key.
func deleteMarkerKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return balanceCacheKeyFor(organizationID, ledgerID, balance) + deleteMarkerKeySuffix
}

// plantBalanceDeleteMarkers best-effort writes a short-lived delete marker key for each balance so
// the Redis honored-lock pre-pass rejects concurrent mutations while a delete is in flight.
// A failed SetNX is logged and skipped (it weakens the guard but must not crash the delete).
// The returned release closure Dels exactly the delete marker keys that were planted, for
// defer-on-error rollback by the caller.
func (uc *UseCase) plantBalanceDeleteMarkers(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) func() {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanCtx, span := tracer.Start(ctx, "exec.plant_balance_delete_markers")
	defer span.End()

	planted := make([]string, 0, len(balances))

	for _, balance := range balances {
		deleteMarkerKey := deleteMarkerKeyFor(organizationID, ledgerID, balance)

		set, err := uc.TransactionRedisRepo.SetNX(spanCtx, deleteMarkerKey, deleteMarkerValue, time.Duration(balanceDeleteMarkerTTLSeconds))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to plant balance delete marker", err)

			logger.Log(spanCtx, libLog.LevelWarn, "Failed to plant balance delete marker", libLog.Err(err))

			continue
		}

		// Track only a delete marker THIS operation acquired (SetNX true). A (false, nil)
		// means a concurrent delete already owns the key: the guard is armed either
		// way, so the delete proceeds, but releasing here would Del a sibling's
		// delete marker and reopen the honored-lock hole. Only acquired keys are releasable.
		if set {
			planted = append(planted, deleteMarkerKey)
		}
	}

	return func() {
		uc.releaseBalanceDeleteMarkers(ctx, planted)
	}
}

// releaseBalanceDeleteMarkers Dels each previously planted delete marker key, logging a Warn on
// error and continuing. A no-op Del on a missing key is safe.
func (uc *UseCase) releaseBalanceDeleteMarkers(ctx context.Context, deleteMarkerKeys []string) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.release_balance_delete_markers")
	defer span.End()

	for _, deleteMarkerKey := range deleteMarkerKeys {
		if err := uc.TransactionRedisRepo.Del(ctx, deleteMarkerKey); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to release balance delete marker", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to release balance delete marker", libLog.Err(err))
		}
	}
}

// evictBalanceCaches Dels each balance's internal cache key after a committed delete. A
// lingering cache key after a persisted delete is the bug this guards against, so a failed
// Del is Warn-worthy but must not fail the already-committed delete.
func (uc *UseCase) evictBalanceCaches(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.evict_balance_caches")
	defer span.End()

	for _, balance := range balances {
		cacheKey := balanceCacheKeyFor(organizationID, ledgerID, balance)

		if err := uc.TransactionRedisRepo.Del(ctx, cacheKey); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to evict balance cache key", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to evict balance cache key", libLog.Err(err))
		}
	}
}
