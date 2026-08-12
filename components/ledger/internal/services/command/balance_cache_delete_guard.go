// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

const (
	// balanceDeleteTombstoneTTLSeconds is the lifetime, in whole seconds, of a balance
	// delete tombstone key. It is deliberately short: it must outlive the worst-case
	// duration of a delete so the honored-lock pre-pass keeps rejecting mutations for the
	// whole operation, yet expire quickly so a process that crashes mid-delete self-heals
	// instead of leaving a permanently poisoned balance key. It is intentionally NOT the
	// 3600s balance cache TTL. SetNX multiplies its ttl argument by time.Second, so it is
	// passed as a whole-second count via time.Duration(balanceDeleteTombstoneTTLSeconds).
	balanceDeleteTombstoneTTLSeconds = 30

	// tombstoneKeySuffix is appended to a balance's internal cache key to form its tombstone
	// key. It MUST match the suffix the Redis Lua pre-pass derives (ARGV[i] .. ":deleted").
	tombstoneKeySuffix = ":deleted"

	// tombstoneMarkerValue is the placeholder stored under a tombstone key. The value is
	// never read: only the key's existence is meaningful to the honored-lock pre-pass.
	tombstoneMarkerValue = "1"
)

// balanceCacheKeyFor returns the un-prefixed internal cache key for a balance. The
// TransactionRedisRepo wrappers apply tenant namespacing to the whole key, so callers pass
// this output directly.
func balanceCacheKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)
}

// tombstoneKeyFor returns the tombstone key for a balance: its internal cache key plus the
// ":deleted" suffix. The tombstone is a SEPARATE key from the balance cache key.
func tombstoneKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return balanceCacheKeyFor(organizationID, ledgerID, balance) + tombstoneKeySuffix
}

// plantBalanceTombstones best-effort writes a short-lived tombstone key for each balance so
// the Redis honored-lock pre-pass rejects concurrent mutations while a delete is in flight.
// A failed SetNX is logged and skipped (it weakens the guard but must not crash the delete).
// The returned release closure Dels exactly the tombstone keys that were planted, for
// defer-on-error rollback by the caller.
func (uc *UseCase) plantBalanceTombstones(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) func() {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	spanCtx, span := tracer.Start(ctx, "exec.plant_balance_tombstones")
	defer span.End()

	planted := make([]string, 0, len(balances))

	for _, balance := range balances {
		tombstoneKey := tombstoneKeyFor(organizationID, ledgerID, balance)

		set, err := uc.TransactionRedisRepo.SetNX(spanCtx, tombstoneKey, tombstoneMarkerValue, time.Duration(balanceDeleteTombstoneTTLSeconds))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to plant balance delete tombstone", err)

			logger.Log(spanCtx, libLog.LevelWarn, "Failed to plant balance delete tombstone", libLog.Err(err))

			continue
		}

		// Track only a tombstone THIS operation acquired (SetNX true). A (false, nil)
		// means a concurrent delete already owns the key: the guard is armed either
		// way, so the delete proceeds, but releasing here would Del a sibling's
		// tombstone and reopen the honored-lock hole. Only acquired keys are releasable.
		if set {
			planted = append(planted, tombstoneKey)
		}
	}

	return func() {
		uc.releaseBalanceTombstones(ctx, planted)
	}
}

// releaseBalanceTombstones Dels each previously planted tombstone key, logging a Warn on
// error and continuing. A no-op Del on a missing key is safe.
func (uc *UseCase) releaseBalanceTombstones(ctx context.Context, tombstoneKeys []string) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.release_balance_tombstones")
	defer span.End()

	for _, tombstoneKey := range tombstoneKeys {
		if err := uc.TransactionRedisRepo.Del(ctx, tombstoneKey); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to release balance delete tombstone", err)

			logger.Log(ctx, libLog.LevelWarn, "Failed to release balance delete tombstone", libLog.Err(err))
		}
	}
}

// evictBalanceCaches Dels each balance's internal cache key after a committed delete. A
// lingering cache key after a persisted delete is the bug this guards against, so a failed
// Del is Warn-worthy but must not fail the already-committed delete.
func (uc *UseCase) evictBalanceCaches(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

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
