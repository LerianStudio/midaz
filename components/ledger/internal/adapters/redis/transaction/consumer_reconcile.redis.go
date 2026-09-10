// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// applyMarkerReconcileTimeout bounds the marker lookup that runs after a lost
// balance script response. It is deliberately short and independent of the
// caller's deadline: the lookup is the last chance to avoid answering 500 for a
// transaction the server already applied, and it runs precisely when the
// original deadline has typically expired.
const applyMarkerReconcileTimeout = 2 * time.Second

// Outcomes of the idempotency path, a closed set so the metric label stays
// bounded: the script answered from its own marker (replayed), a lost response
// was recovered from the marker (reconciled), or the marker could not prove the
// application and the original error stands (reconcile_miss).
const (
	balanceScriptIdempotencyReplayed      = "replayed"
	balanceScriptIdempotencyReconciled    = "reconciled"
	balanceScriptIdempotencyReconcileMiss = "reconcile_miss"
)

// balanceScriptIdempotencyTotal counts every execution the apply marker
// answered for, by outcome.
var balanceScriptIdempotencyTotal = metrics.Metric{
	Name:        "balance_script_idempotency_total",
	Unit:        "1",
	Description: "Count of balance atomic script executions served by the idempotency marker, by outcome.",
}

// isResponseLostError answers ONE question: could the server have applied this
// execution and only the response have been lost? It is not a retriability
// classifier — a retriable error whose command provably never ran must answer
// false here, because the caller acts on a true by treating the transaction as
// applied.
//
// True for the classes where the command may already sit in the server's log
// with its reply stranded: an expired deadline, a socket timeout, and a
// connection that died mid-flight.
//
// False for everything else, which is the fail-safe side: a business error (the
// script rolled back), a missing key, a cancelled caller (nobody is left to
// consume a converted success) and any error this function does not recognize.
func isResponseLostError(err error) bool {
	if err == nil || pkg.IsBusinessError(err) {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, redis.Nil) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}

// recordBalanceScriptIdempotency emits one idempotency outcome. A nil factory is
// a no-op so a binary with metrics disabled runs unchanged, and an emission
// failure logs at Debug per T11 and never affects the operation.
func recordBalanceScriptIdempotency(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, outcome string) {
	if factory == nil {
		return
	}

	counter, err := factory.Counter(balanceScriptIdempotencyTotal)
	if err == nil {
		err = counter.WithLabels(map[string]string{"outcome": outcome}).Add(ctx, 1)
	}

	if err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit balance script idempotency counter", libLog.Err(err))
	}
}

// reconcileFromApplyMarker recovers the response of an execution whose reply was
// lost. It returns a result and true ONLY when the marker proves the script
// applied; every other outcome returns false so the caller propagates its
// ORIGINAL error, never one raised here.
//
// The lookup runs on a context detached from the caller's: the deadline that
// expired is usually the very reason this function was reached, so inheriting it
// would guarantee the lookup fails.
//
// applyMarkerKey is already tenant-namespaced by the caller, so the raw client
// is used deliberately — the repository's own accessors would namespace it a
// second time.
func (rr *RedisConsumerRepository) reconcileFromApplyMarker(
	ctx context.Context,
	span trace.Span,
	rds redis.UniversalClient,
	applyMarkerKey, transactionID string,
	mapBalances map[string]*mmodel.Balance,
) (*mmodel.BalanceAtomicResult, bool) {
	logger, _, _, metricsFactory := libObservability.NewTrackingFromContext(ctx)

	reconCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), applyMarkerReconcileTimeout)
	defer cancel()

	stored, err := rds.Get(reconCtx, applyMarkerKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			logger.Log(
				ctx, libLog.LevelWarn, "Balance script response lost and no apply marker exists; the original error stands",
				libLog.String("transaction_id", transactionID),
			)
		} else {
			logger.Log(
				ctx, libLog.LevelWarn, "Failed to read the apply marker after a lost balance script response",
				libLog.String("transaction_id", transactionID), libLog.Err(err),
			)
		}

		recordBalanceScriptIdempotency(ctx, metricsFactory, logger, balanceScriptIdempotencyReconcileMiss)

		return nil, false
	}

	result, _, err := decodeBalanceAtomicResult(ctx, stored, mapBalances)
	if err != nil {
		logger.Log(
			ctx, libLog.LevelWarn, "Failed to decode the apply marker after a lost balance script response",
			libLog.String("transaction_id", transactionID), libLog.Err(err),
		)

		recordBalanceScriptIdempotency(ctx, metricsFactory, logger, balanceScriptIdempotencyReconcileMiss)

		return nil, false
	}

	logger.Log(
		ctx, libLog.LevelWarn, "Lost balance script response converted to success from the apply marker",
		libLog.String("transaction_id", transactionID),
	)

	span.SetAttributes(attribute.Bool("app.balance_script_reconciled", true))

	recordBalanceScriptIdempotency(ctx, metricsFactory, logger, balanceScriptIdempotencyReconciled)

	return result, true
}

// recordBalanceScriptReplay reports an execution the script itself answered from
// its marker: the client library resent a command the server had already run, an
// event invisible to this code except through this flag.
func recordBalanceScriptReplay(ctx context.Context, span trace.Span, logger libLog.Logger, factory *metrics.MetricsFactory, transactionID string) {
	logger.Log(
		ctx, libLog.LevelWarn, "Balance script response replayed from the apply marker after a client resend",
		libLog.String("transaction_id", transactionID),
	)

	span.SetAttributes(attribute.Bool("app.balance_script_replayed", true))

	recordBalanceScriptIdempotency(ctx, factory, logger, balanceScriptIdempotencyReplayed)
}
