// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package shardrouting

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	internalsharding "github.com/LerianStudio/midaz/v3/components/transaction/internal/sharding"
	"github.com/LerianStudio/midaz/v3/pkg/shard"
)

// FallbackSentinel is the reserved sentinel value returned by the manager when
// resolution fails catastrophically (no router available, out of range on a
// path we explicitly forbid). Callers MUST compare against this sentinel, never
// against literal -1, so future changes to the sentinel value are safe.
const FallbackSentinel = -1

// fallbackCounter tracks the total number of times ResolveBalanceShard fell
// back to the static router because the dynamic manager errored but returned a
// valid shardID. Exposed as a counter via FallbackTotal() so callers (e.g. a
// telemetry exporter) can publish it as authorizer_shardrouting_fallback_total.
//
// The counter is intentionally package-level (not per-UseCase) because the
// shardrouting package has no logger/metrics dependency and we want a simple
// aggregate observable. Cardinality is bounded (one counter) so atomic.Uint64
// is the right primitive.
var fallbackCounter atomic.Uint64

// FallbackTotal returns the cumulative number of times ResolveBalanceShard
// swallowed a manager error and fell back to the static router shardID.
// Intended for /metrics scraping and regression tests.
func FallbackTotal() uint64 {
	return fallbackCounter.Load()
}

// ResetFallbackTotalForTest resets the package-level counter. Only for use in
// tests that assert on increment counts — never call from production code.
func ResetFallbackTotalForTest() {
	fallbackCounter.Store(0)
}

// Options gates optional behaviours of ResolveBalanceShard without exploding
// the positional argument list. Zero-value Options is the safe default:
// fail-closed when the manager returns an error, even if a fallback shardID
// would be technically valid.
type Options struct {
	// AllowFallback, when true, preserves the legacy behaviour: if the manager
	// returns an error AND a shardID within the router's valid range, log a
	// WARN, bump authorizer_shardrouting_fallback_total, and return the
	// fallback shardID with err=nil. When false (default), the manager error
	// is propagated so callers cannot silently read stale/fnv-hashed routes.
	AllowFallback bool
}

// ResolveBalanceShard determines which shard a balance belongs to using the manager (if available)
// or falling back to the static router. See Options.AllowFallback for the
// fail-closed vs fallback semantics when the manager errors.
func ResolveBalanceShard(
	ctx context.Context,
	router *shard.Router,
	manager *internalsharding.Manager,
	organizationID, ledgerID uuid.UUID,
	alias, balanceKey string,
) (int, error) {
	return ResolveBalanceShardWithOptions(ctx, router, manager, organizationID, ledgerID, alias, balanceKey, Options{})
}

// ResolveBalanceShardWithOptions is the full-fidelity entrypoint that accepts an
// Options struct. Callers that need AllowFallback MUST use this variant; the
// shorthand ResolveBalanceShard delegates here with zero-value Options for the
// safe, fail-closed default.
func ResolveBalanceShardWithOptions(
	ctx context.Context,
	router *shard.Router,
	manager *internalsharding.Manager,
	organizationID, ledgerID uuid.UUID,
	alias, balanceKey string,
	opts Options,
) (int, error) {
	if router != nil && router.ShardCount() <= 0 {
		return 0, internalsharding.ErrInvalidShardCount
	}

	if manager != nil {
		shardID, err := manager.ResolveBalanceShard(ctx, organizationID, ledgerID, alias, balanceKey)
		if err != nil {
			return handleManagerError(ctx, router, shardID, err, opts)
		}

		return shardID, nil
	}

	if router != nil {
		if router.ShardCount() <= 0 {
			return 0, internalsharding.ErrInvalidShardCount
		}

		return router.ResolveBalance(alias, balanceKey), nil
	}

	return 0, nil
}

// handleManagerError separates the two cases that were previously conflated:
//
//  1. Sentinel shardID (out of range, no-router) → error is propagated, never
//     fallback. Returning 0 with a masked error would route writes to shard 0,
//     which is a correctness hazard on a live cluster.
//  2. Valid shardID alongside an error → the manager has a usable FNV fallback;
//     return it only when the caller opted in via AllowFallback, bump the
//     fallback counter, and log the reason. Otherwise propagate the error so
//     the caller can decide (e.g. surface a 5xx rather than route blindly).
func handleManagerError(
	ctx context.Context,
	router *shard.Router,
	shardID int,
	err error,
	opts Options,
) (int, error) {
	shardInRange := router != nil && shardID >= 0 && shardID < router.ShardCount()

	if !shardInRange {
		return 0, fmt.Errorf("resolve balance shard: %w", err)
	}

	if !opts.AllowFallback {
		return 0, fmt.Errorf("resolve balance shard (fallback disabled, shard_id=%d): %w", shardID, err)
	}

	fallbackCounter.Add(1)

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // tracking tuple discards are idiomatic
	if logger != nil {
		reason := classifyFallbackReason(err)
		logger.Warnf(
			"shard resolution fell back to router default (shard %d, reason=%s) due to manager error: %v",
			shardID, reason, err,
		)
	}

	return shardID, nil
}

// classifyFallbackReason maps the manager error into a stable short label used
// by the authorizer_shardrouting_fallback_total counter. Keep labels bounded —
// they become metric dimensions and unbounded strings blow up cardinality.
func classifyFallbackReason(err error) string {
	if err == nil {
		return "unknown"
	}

	switch {
	case errors.Is(err, internalsharding.ErrInvalidShardCount):
		return "invalid_shard_count"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline"
	default:
		return "manager_error"
	}
}
