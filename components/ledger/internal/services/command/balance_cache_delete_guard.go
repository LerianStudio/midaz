// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

const (
	// balanceCacheSnapshotTTLSeconds must stay in lock-step with the 86400-second TTL
	// used by balance_atomic_operation.lua and update_balance_settings.lua. A cache
	// snapshot can be stale for this entire period when post-commit eviction fails.
	balanceCacheSnapshotTTLSeconds = 24 * 60 * 60

	// balanceDeleteMarkerTTLSeconds is the lifetime, in whole seconds, of a balance
	// delete marker key. It covers one complete balance snapshot lifetime plus an
	// equally sized operation grace period. The grace period matters because markers
	// are planted before the funds guard and soft delete, while successful deletes
	// intentionally retain the marker when best-effort cache eviction fails. Thus a
	// marker cannot expire while a stale snapshot that existed at acquisition can still
	// be used, and a slow single or cascade delete has a full snapshot lifetime to
	// finish. SetNX multiplies its ttl argument by time.Second, so this is passed as a
	// whole-second count via time.Duration(balanceDeleteMarkerTTLSeconds).
	balanceDeleteMarkerTTLSeconds = 2 * balanceCacheSnapshotTTLSeconds

	// balanceDeleteMarkerShortTTLSeconds is the 30-second post-eviction in-flight window. A successful
	// eviction no longer needs snapshot-lifetime protection, but the marker remains owned briefly
	// so a concurrent operation cannot race a just-committed delete. ExpireIfValue changes it
	// atomically without releasing and reacquiring the marker.
	balanceDeleteMarkerShortTTLSeconds = 30

	// deleteMarkerNamespacePrefix is a top-level key namespace reserved for delete markers. It
	// intentionally differs from the balance cache namespace: balance keys allow arbitrary ':'
	// characters, so appending a suffix to a balance key could collide with a valid sibling balance
	// key. The {transactions} hash tag keeps markers colocated with their balance keys in Redis
	// Cluster. The same prefix is used by the transaction Lua scripts.
	deleteMarkerNamespacePrefix = "balance_delete_marker:{transactions}:"
	balanceCacheNamespacePrefix = "balance:{transactions}:"
	deleteMarkerLegacySuffix    = ":deleted"

	// balanceDeleteMarkerCleanupTimeout bounds every best-effort Redis cleanup that runs after
	// the delete decision is made: marker rollback, cache eviction and marker shortening.
	// WithoutCancel preserves the opportunity to finish that cleanup after the request context
	// is canceled, while this timeout prevents a stuck Redis call from holding the caller
	// indefinitely.
	balanceDeleteMarkerCleanupTimeout = 5 * time.Second
)

type balanceDeleteMarker struct {
	key       string
	legacyKey string
	token     string
}

// balanceCacheKeyFor returns the un-prefixed internal cache key for a balance. The
// TransactionRedisRepo wrappers apply tenant namespacing to the whole key, so callers pass
// this output directly.
func balanceCacheKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return utils.BalanceInternalKey(organizationID, ledgerID, balance.Alias+"#"+balance.Key)
}

// deleteMarkerKeyFor returns a delete marker in the dedicated marker namespace. Replacing the
// balance namespace keeps the tenant prefix (added by the Redis adapter) in front of the marker
// namespace and preserves the balance key's Redis hash slot.
func deleteMarkerKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	cacheKey := balanceCacheKeyFor(organizationID, ledgerID, balance)

	return deleteMarkerNamespacePrefix + strings.TrimPrefix(cacheKey, balanceCacheNamespacePrefix)
}

// legacyDeleteMarkerKeyFor returns the marker key used by the pre-namespace release. It remains
// dual-written for one rolling-deploy release so old Lua consumers cannot mutate a balance while
// a new command owns only the namespaced marker. The suffix has a known collision tradeoff: a
// legitimate sibling balance whose key ends in ":deleted" occupies this key, so deletion fails
// closed rather than risk mutating through an old consumer. Legacy rollback still uses an
// unconditional DEL, so draining old pods is required before new traffic can acquire this key.
// Remove this helper and the legacy Lua checks after every old deployment is retired.
func legacyDeleteMarkerKeyFor(organizationID, ledgerID uuid.UUID, balance *mmodel.Balance) string {
	return balanceCacheKeyFor(organizationID, ledgerID, balance) + deleteMarkerLegacySuffix
}

// plantBalanceDeleteMarkerLease acquires both the current namespaced marker and the legacy
// :deleted marker for each balance. Both are needed while old and new binaries can be live: new
// Lua honors legacy markers, while old Lua only honors legacy markers. Acquisition is fail-closed;
// if either key is unavailable, all keys acquired by this invocation are compare-and-deleted.
func (uc *UseCase) plantBalanceDeleteMarkerLease(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) ([]balanceDeleteMarker, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	spanCtx, span := tracer.Start(ctx, "exec.plant_balance_delete_markers")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.Int("app.balance_delete.marker_count", len(balances)),
	)

	planted := make([]balanceDeleteMarker, 0, len(balances))
	ownerToken := uuid.NewString()

	for _, balance := range balances {
		deleteMarkerKey := deleteMarkerKeyFor(organizationID, ledgerID, balance)
		legacyMarkerKey := legacyDeleteMarkerKeyFor(organizationID, ledgerID, balance)

		set, err := uc.TransactionRedisRepo.SetNX(spanCtx, deleteMarkerKey, ownerToken, time.Duration(balanceDeleteMarkerTTLSeconds))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to plant balance delete marker", err)

			// This call owns only the markers in planted. Do not attempt to remove the
			// marker whose SetNX failed: it may belong to another delete operation.
			uc.releaseBalanceDeleteMarkers(ctx, planted)

			return nil, fmt.Errorf("failed to plant balance delete marker: %w", err)
		}

		if !set {
			// A marker owned by a concurrent delete is not ours to release, and this
			// operation cannot safely proceed without an exclusive barrier of its own.
			releaseErr := pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Balance delete marker is already owned", releaseErr)

			uc.releaseBalanceDeleteMarkers(ctx, planted)

			return nil, releaseErr
		}

		legacySet, legacyErr := uc.TransactionRedisRepo.SetNX(spanCtx, legacyMarkerKey, ownerToken, time.Duration(balanceDeleteMarkerTTLSeconds))
		if legacyErr != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to plant legacy balance delete marker", legacyErr)

			// The current marker is ours; compare its token before deleting it. The failed legacy
			// SetNX may belong to another owner and is never released here.
			uc.releaseBalanceDeleteMarkers(ctx, append(planted, balanceDeleteMarker{key: deleteMarkerKey, legacyKey: legacyMarkerKey, token: ownerToken}))

			return nil, fmt.Errorf("failed to plant legacy balance delete marker: %w", legacyErr)
		}

		if !legacySet {
			conflictErr := pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, constant.EntityBalance)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Legacy balance delete marker is already owned", conflictErr)

			uc.releaseBalanceDeleteMarkers(ctx, append(planted, balanceDeleteMarker{key: deleteMarkerKey, legacyKey: legacyMarkerKey, token: ownerToken}))

			return nil, conflictErr
		}

		planted = append(planted, balanceDeleteMarker{key: deleteMarkerKey, legacyKey: legacyMarkerKey, token: ownerToken})
	}

	return planted, nil
}

// releaseBalanceDeleteMarkers atomically compare-and-deletes each previously planted marker,
// logging a Warn on Redis errors and continuing. A false result means the marker is already
// missing or belongs to a different owner and is intentionally not treated as an error.
func (uc *UseCase) releaseBalanceDeleteMarkers(ctx context.Context, markers []balanceDeleteMarker) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), balanceDeleteMarkerCleanupTimeout)
	defer cancel()

	releaseCtx, span := tracer.Start(releaseCtx, "exec.release_balance_delete_markers")
	defer span.End()

	for _, marker := range markers {
		for _, key := range []string{marker.key, marker.legacyKey} {
			deleted, err := uc.TransactionRedisRepo.DeleteIfValue(releaseCtx, key, marker.token)
			if err != nil {
				libOpentelemetry.HandleSpanError(span, "Failed to release balance delete marker", err)

				logger.Log(releaseCtx, libLog.LevelWarn, "Failed to release balance delete marker", libLog.Err(err))

				continue
			}

			if !deleted {
				logger.Log(releaseCtx, libLog.LevelDebug, "Balance delete marker was not owned at release")
			}
		}
	}
}

// evictBalanceCaches Dels each balance's internal cache key after a committed delete. A
// lingering cache key after a persisted delete is the bug this guards against, so a failed Del is
// Warn-worthy but must not fail the already-committed delete. The whole cleanup runs on a bounded
// context detached from the caller: the row is already gone, so a canceled request must not be
// able to leave a stale snapshot and a long-lived marker behind.
//
// A nil markers slice means no shortening. Otherwise markers[i] must be the lease planted for
// balances[i]; a length mismatch cannot be resolved safely, so shortening is skipped for every
// balance and all markers keep their long TTL. Each successful Del is followed by token-checked
// shortening of both marker namespaces, and a shortening failure likewise leaves the long marker
// in place for fail-closed protection.
func (uc *UseCase) evictBalanceCaches(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, markers []balanceDeleteMarker) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	evictCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), balanceDeleteMarkerCleanupTimeout)
	defer cancel()

	evictCtx, span := tracer.Start(evictCtx, "exec.evict_balance_caches")
	defer span.End()

	if len(markers) > 0 && len(markers) != len(balances) {
		mismatchErr := fmt.Errorf("balance delete marker count %d does not match balance count %d", len(markers), len(balances))

		libOpentelemetry.HandleSpanError(span, "Balance delete marker count does not match balance count", mismatchErr)

		logger.Log(evictCtx, libLog.LevelWarn, "Skipping balance delete marker shortening", libLog.Err(mismatchErr))

		markers = nil
	}

	for i, balance := range balances {
		cacheKey := balanceCacheKeyFor(organizationID, ledgerID, balance)

		if err := uc.TransactionRedisRepo.Del(evictCtx, cacheKey); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to evict balance cache key", err)

			logger.Log(evictCtx, libLog.LevelWarn, "Failed to evict balance cache key", libLog.Err(err))

			continue
		}

		if i < len(markers) {
			uc.shortenBalanceDeleteMarker(evictCtx, markers[i])
		}
	}
}

// shortenBalanceDeleteMarker keeps the marker barrier for a short in-flight window after cache
// eviction. Every expiry is token-checked; a missing/foreign marker is never modified.
func (uc *UseCase) shortenBalanceDeleteMarker(ctx context.Context, marker balanceDeleteMarker) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "exec.shorten_balance_delete_marker")
	defer span.End()

	for _, key := range []string{marker.key, marker.legacyKey} {
		ok, err := uc.TransactionRedisRepo.ExpireIfValue(ctx, key, marker.token, time.Duration(balanceDeleteMarkerShortTTLSeconds))
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to shorten balance delete marker", err)
			logger.Log(ctx, libLog.LevelWarn, "Failed to shorten balance delete marker", libLog.Err(err))

			continue
		}

		if !ok {
			logger.Log(ctx, libLog.LevelDebug, "Balance delete marker was not owned while shortening")
		}
	}
}
