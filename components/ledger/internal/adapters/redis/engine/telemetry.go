// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

var executionDuration = metrics.Metric{
	Name: "engine_duration_ms", Unit: "ms",
	Description: "Duration of one accounting adapter invocation, including preflight and limit repair.",
	Buckets:     []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000},
}

var executionSize = metrics.Metric{
	Name: "engine_request_size_bytes", Unit: "By",
	Description: "Validated accounting Lua JSON payload size, excluding Redis keys and RESP framing.",
	Buckets:     []float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216},
}

var executionPoolSize = metrics.Metric{
	Name: "engine_pool_balance_count", Unit: "1",
	Description: "Number of balance snapshots carried by a validated accounting request.",
	Buckets:     []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512},
}

var executionTouchedSize = metrics.Metric{
	Name: "engine_touched_balance_count", Unit: "1",
	Description: "Number of distinct balance references targeted by validated postings.",
	Buckets:     []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512},
}

func recordPreparedExecution(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, request accounting.Execution, payloadBytes int) {
	if factory == nil {
		return
	}

	counts := make(map[accounting.PostingType]int64, 6)

	for _, transaction := range request.Transactions {
		for _, posting := range transaction.Postings {
			counts[posting.Type]++
		}
	}

	// Iterate only the supported vocabulary even if validation changes later.
	for _, kind := range []accounting.PostingType{accounting.PostingDebit, accounting.PostingCredit, accounting.PostingReserve, accounting.PostingUnreserve, accounting.PostingHold, accounting.PostingRelease} {
		if count := counts[kind]; count > 0 {
			emitCounter(ctx, factory, logger, "engine_postings_total", "Requested postings in validated invocations, including replay; not applied movements.", map[string]string{"type": string(kind)}, count)
		}
	}

	histogram, err := factory.Histogram(executionSize)
	if err == nil {
		err = histogram.Record(ctx, int64(payloadBytes))
	}

	logMetricError(ctx, logger, err)

	pool, poolErr := factory.Histogram(executionPoolSize)
	if poolErr == nil {
		poolErr = pool.Record(ctx, int64(len(request.Balances)))
	}

	logMetricError(ctx, logger, poolErr)

	touched := make(map[string]struct{})

	for _, transaction := range request.Transactions {
		for _, posting := range transaction.Postings {
			touched[posting.BalanceRef] = struct{}{}
		}
	}

	touchedHistogram, touchedErr := factory.Histogram(executionTouchedSize)
	if touchedErr == nil {
		touchedErr = touchedHistogram.Record(ctx, int64(len(touched)))
	}

	logMetricError(ctx, logger, touchedErr)
}

func recordExecutionOutcome(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, duration time.Duration, err error) {
	if factory == nil {
		return
	}

	outcome, code := executionOutcome(err)
	emitCounter(ctx, factory, logger, "engine_requests_total", "Accounting adapter invocations by outcome, including replay and preflight rejection.", map[string]string{"outcome": outcome}, 1)

	if code != "" {
		emitCounter(ctx, factory, logger, "engine_failures_total", "Accounting adapter failures by closed protocol classification.", map[string]string{"code": code}, 1)
	}

	if outcome == "indeterminate" {
		emitCounter(ctx, factory, logger, "engine_indeterminate_total", "Accounting adapter invocations whose accounting outcome is unknown.", nil, 1)
	}

	histogram, emitErr := factory.Histogram(executionDuration)
	if emitErr == nil {
		emitErr = histogram.Record(ctx, duration.Milliseconds())
	}

	logMetricError(ctx, logger, emitErr)
}

func emitCounter(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, name, description string, labels map[string]string, value int64) {
	err := factory.AddCounter(ctx, name, description, "1", labels, value)
	logMetricError(ctx, logger, err)
}

func logMetricError(ctx context.Context, logger libLog.Logger, err error) {
	if err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit accounting metric", libLog.Err(err))
	}
}

func executionOutcome(err error) (string, string) {
	if err == nil {
		return "success", ""
	}

	var failure *TechnicalError
	if errors.As(err, &failure) {
		if failure == nil {
			return "technical_error", "unknown"
		}

		outcome := "technical_error"
		if failure.Indeterminate {
			outcome = "indeterminate"
		}

		return outcome, metricFailureCode(failure.Code)
	}

	var refusal *accounting.Failure
	if errors.As(err, &refusal) && refusal != nil {
		switch refusal.Code {
		case accounting.FailureInsufficientFunds, accounting.FailureOverdraftLimitExceeded,
			accounting.FailureOverdraftNotEligible, accounting.FailureOverdraftCompanionMissing,
			accounting.FailureBalanceDeleted, accounting.FailureOnHoldUnderflow,
			accounting.FailureBalanceMissing, accounting.FailureAssetMismatch,
			accounting.FailureSendingNotAllowed, accounting.FailureReceivingNotAllowed,
			accounting.FailureExternalHoldNotAllowed:
			return "refused", refusal.Code
		}
	}

	return "technical_error", "unknown"
}

func metricFailureCode(code string) string {
	switch code {
	case "context_canceled", "invalid_scope", "invalid_request", "invalid_recovery",
		"connection_unavailable", "unsupported_transport", "invalid_response", "transport",
		"invalid_failure", "invalid_technical_failure", "invalid_json", "invalid_protocol",
		"invalid_balance", "balance_identity_mismatch", "wrong_key_type",
		"execution_fingerprint_conflict", "execution_guard_conflict", "version_overflow",
		"invalid_companion", "prepared_bytes_exceeded", "request_bytes_exceeded",
		"serialization_failed", "script_runtime_failed", "indeterminate",
		"execution_outcome_unknown", "invalid_receipt", "unknown_technical_failure",
		"invalid_normalization_failure", "normalization_required", "script_runtime",
		"normalization_read_failed", "normalization_balance_missing",
		"normalization_invalid_balance", "normalization_repair_failed":
		return code
	default:
		return "unknown"
	}
}
