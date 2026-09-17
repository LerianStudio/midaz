// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const (
	atomicTransactionBatchOutcomeReceived   = "received"
	atomicTransactionBatchOutcomeApplied    = "applied"
	atomicTransactionBatchOutcomeReplayed   = "replayed"
	atomicTransactionBatchOutcomeRecovering = "recovering"
	atomicTransactionBatchOutcomeRejected   = "rejected"

	atomicTransactionBatchMetricCodeNone          = "none"
	atomicTransactionBatchMetricCodeTechnical     = "technical"
	atomicTransactionBatchMetricCodeOtherBusiness = "other_business"
	atomicTransactionBatchMetricDimensionNone     = "none"
)

var (
	atomicTransactionBatchSizeMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_size",
		Unit:        "1",
		Description: "Number of ordered transactions received by one atomic batch command.",
		Buckets:     []float64{1, 2, 5, 10, 25, 50},
	}
	atomicTransactionBatchInputLegsMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_input_legs",
		Unit:        "1",
		Description: "Number of debit and credit legs received by one atomic batch command.",
		Buckets:     []float64{2, 10, 25, 50, 100, 200, 500, 1000},
	}
	atomicTransactionBatchExpandedPostingsMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_expanded_postings",
		Unit:        "1",
		Description: "Number of fee-inclusive postings prepared by one atomic batch command.",
		Buckets:     []float64{2, 10, 25, 50, 100, 150, 200},
	}
	atomicTransactionBatchExecutionBalancesMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_execution_balances",
		Unit:        "1",
		Description: "Number of balance snapshots admitted to one atomic batch execution.",
		Buckets:     []float64{2, 10, 25, 50, 100, 200, 300, 400},
	}
	atomicTransactionBatchInternalBytesMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_internal_bytes",
		Unit:        "By",
		Description: "Serialized internal byte dimensions for one prepared atomic batch.",
		Buckets:     []float64{1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864},
	}
	atomicTransactionBatchDurationMetric = metrics.Metric{
		Name:        "atomic_transaction_batch_duration_ms",
		Unit:        "ms",
		Description: "Atomic batch command duration by closed execution phase.",
		Buckets:     []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
	}
)

func (uc *UseCase) recordAtomicTransactionBatchReceived(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
) {
	if uc.MetricsFactory == nil {
		return
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	uc.emitAtomicTransactionBatchOutcome(
		ctx,
		logger,
		atomicTransactionBatchOutcomeReceived,
		atomicTransactionBatchMetricCodeNone,
		atomicTransactionBatchMetricDimensionNone,
	)
	uc.recordAtomicTransactionBatchHistogram(ctx, logger, atomicTransactionBatchSizeMetric, nil, len(in.Transactions))

	inputLegs := 0

	for index := range in.Transactions {
		transaction := in.Transactions[index].Transaction
		inputLegs += len(transaction.Send.Source.From) + len(transaction.Send.Distribute.To)
	}

	uc.recordAtomicTransactionBatchHistogram(ctx, logger, atomicTransactionBatchInputLegsMetric, nil, inputLegs)
}

func (uc *UseCase) recordAtomicTransactionBatchCompleted(
	ctx context.Context,
	result *CreateAtomicTransactionBatchV2Result,
	run *atomicTransactionBatchRun,
	err error,
	duration time.Duration,
) {
	if uc.MetricsFactory == nil {
		return
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	outcome := atomicTransactionBatchOutcomeRejected
	code := atomicTransactionBatchMetricCode(err)

	dimension := atomicTransactionBatchMetricDimension(run)
	switch {
	case err == nil && result != nil && result.Replayed:
		outcome = atomicTransactionBatchOutcomeReplayed
		code = atomicTransactionBatchMetricCodeNone
		dimension = atomicTransactionBatchMetricDimensionNone
	case err == nil:
		outcome = atomicTransactionBatchOutcomeApplied
		code = atomicTransactionBatchMetricCodeNone
		dimension = atomicTransactionBatchMetricDimensionNone
	case run != nil && run.idempotencyHandedOff:
		outcome = atomicTransactionBatchOutcomeRecovering
	}

	uc.emitAtomicTransactionBatchOutcome(ctx, logger, outcome, code, dimension)
	uc.recordAtomicTransactionBatchPhaseDuration(ctx, "total", duration)
	uc.recordAtomicTransactionBatchWork(ctx, logger, run)
}

func (uc *UseCase) recordAtomicTransactionBatchRecovering(ctx context.Context) {
	if uc.MetricsFactory == nil {
		return
	}

	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	uc.emitAtomicTransactionBatchOutcome(
		ctx,
		logger,
		atomicTransactionBatchOutcomeRecovering,
		atomicTransactionBatchMetricCodeNone,
		atomicTransactionBatchMetricDimensionNone,
	)
}

func (uc *UseCase) recordAtomicTransactionBatchPhaseDuration(
	ctx context.Context,
	phase string,
	duration time.Duration,
) {
	if uc.MetricsFactory == nil {
		return
	}

	phase = atomicTransactionBatchMetricPhase(phase)
	logger, _, _, _ := libObservability.NewTrackingFromContext(ctx)
	uc.recordAtomicTransactionBatchHistogram(
		ctx,
		logger,
		atomicTransactionBatchDurationMetric,
		map[string]string{"phase": phase},
		int(duration.Milliseconds()),
	)
}

func (uc *UseCase) emitAtomicTransactionBatchOutcome(
	ctx context.Context,
	logger libLog.Logger,
	outcome, code, dimension string,
) {
	labels := map[string]string{
		"outcome":   atomicTransactionBatchMetricOutcome(outcome),
		"code":      atomicTransactionBatchMetricCodeLabel(code),
		"dimension": atomicTransactionBatchMetricDimensionLabel(dimension),
	}
	if err := uc.MetricsFactory.AddCounter(
		ctx,
		"atomic_transaction_batches_total",
		"Atomic transaction batch command events by bounded outcome and rejection classification.",
		"1",
		labels,
		1,
	); err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit atomic transaction batch outcome metric", libLog.Err(err))
	}
}

func (uc *UseCase) recordAtomicTransactionBatchWork(
	ctx context.Context,
	logger libLog.Logger,
	run *atomicTransactionBatchRun,
) {
	if run == nil || len(run.items) == 0 {
		return
	}

	expandedPostings := 0
	for index := range run.items {
		expandedPostings += len(run.items[index].input.Send.Source.From) +
			len(run.items[index].input.Send.Distribute.To)
	}

	uc.recordAtomicTransactionBatchHistogram(
		ctx,
		logger,
		atomicTransactionBatchExpandedPostingsMetric,
		nil,
		expandedPostings,
	)

	measurements := run.budgetMeasurements
	if measurements == nil || len(measurements.executionBalances) == 0 {
		return
	}

	last := len(measurements.executionBalances) - 1
	uc.recordAtomicTransactionBatchHistogram(
		ctx,
		logger,
		atomicTransactionBatchExecutionBalancesMetric,
		nil,
		measurements.executionBalances[last],
	)

	byteDimensions := []struct {
		kind  string
		value int
	}{
		{"completion_plan", measurements.completionPlanBytes[last]},
		{"accounting_request", measurements.accountingRequestBytes[last]},
		{"prepared_response", measurements.preparedResponseBytes[last]},
		{"recovery", measurements.recoveryBytes[last]},
		{"cached_response", measurements.cachedResponseBytes[last]},
	}
	for _, dimension := range byteDimensions {
		uc.recordAtomicTransactionBatchHistogram(
			ctx,
			logger,
			atomicTransactionBatchInternalBytesMetric,
			map[string]string{"kind": dimension.kind},
			dimension.value,
		)
	}
}

func (uc *UseCase) recordAtomicTransactionBatchHistogram(
	ctx context.Context,
	logger libLog.Logger,
	metric metrics.Metric,
	labels map[string]string,
	value int,
) {
	histogram, err := uc.MetricsFactory.Histogram(metric)
	if err == nil {
		if labels != nil {
			histogram = histogram.WithLabels(labels)
		}

		err = histogram.Record(ctx, int64(value))
	}

	if err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit atomic transaction batch histogram", libLog.Err(err))
	}
}

func atomicTransactionBatchMetricOutcome(value string) string {
	switch value {
	case atomicTransactionBatchOutcomeReceived,
		atomicTransactionBatchOutcomeApplied,
		atomicTransactionBatchOutcomeReplayed,
		atomicTransactionBatchOutcomeRecovering,
		atomicTransactionBatchOutcomeRejected:
		return value
	default:
		return atomicTransactionBatchOutcomeRejected
	}
}

func atomicTransactionBatchMetricPhase(value string) string {
	switch value {
	case "identity", "idempotency", "preparation", "reservation", "accounting", "completion", "total":
		return value
	default:
		return "total"
	}
}

func atomicTransactionBatchMetricCode(err error) string {
	if err == nil {
		return atomicTransactionBatchMetricCodeNone
	}

	known := []error{
		constant.ErrInsufficientFunds,
		constant.ErrInsufficientAccountBalance,
		constant.ErrSkipNotPermitted,
		constant.ErrTransactionScopeMismatch,
		constant.ErrAccountBlocked,
		constant.ErrAccountBlockExceptionInvalid,
		constant.ErrTransactionBatchCardinality,
		constant.ErrTransactionBatchInputLegsLimitExceeded,
		constant.ErrTransactionBatchBudgetExceeded,
	}
	for _, candidate := range known {
		if errors.Is(err, candidate) {
			return candidate.Error()
		}
	}

	if pkg.IsBusinessError(err) {
		if code := atomicTransactionBatchBusinessErrorCode(err); code != "" {
			bounded := atomicTransactionBatchMetricCodeLabel(code)
			if bounded != atomicTransactionBatchMetricCodeTechnical {
				return bounded
			}
		}

		return atomicTransactionBatchMetricCodeOtherBusiness
	}

	return atomicTransactionBatchMetricCodeTechnical
}

func atomicTransactionBatchBusinessErrorCode(err error) string {
	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		return validation.Code
	}

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		return unprocessable.Code
	}

	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		return notFound.Code
	}

	var unauthorized pkg.UnauthorizedError
	if errors.As(err, &unauthorized) {
		return unauthorized.Code
	}

	var forbidden pkg.ForbiddenError
	if errors.As(err, &forbidden) {
		return forbidden.Code
	}

	var failedPrecondition pkg.FailedPreconditionError
	if errors.As(err, &failedPrecondition) {
		return failedPrecondition.Code
	}

	return ""
}

func atomicTransactionBatchMetricCodeLabel(value string) string {
	switch value {
	case atomicTransactionBatchMetricCodeNone,
		atomicTransactionBatchMetricCodeTechnical,
		atomicTransactionBatchMetricCodeOtherBusiness,
		constant.ErrInsufficientFunds.Error(),
		constant.ErrInsufficientAccountBalance.Error(),
		constant.ErrSkipNotPermitted.Error(),
		constant.ErrTransactionScopeMismatch.Error(),
		constant.ErrAccountBlocked.Error(),
		constant.ErrAccountBlockExceptionInvalid.Error(),
		constant.ErrTransactionBatchCardinality.Error(),
		constant.ErrTransactionBatchInputLegsLimitExceeded.Error(),
		constant.ErrTransactionBatchBudgetExceeded.Error():
		return value
	default:
		return atomicTransactionBatchMetricCodeTechnical
	}
}

func atomicTransactionBatchMetricDimension(run *atomicTransactionBatchRun) string {
	if run == nil {
		return atomicTransactionBatchMetricDimensionNone
	}

	return atomicTransactionBatchMetricDimensionLabel(run.rejectionDimension)
}

func atomicTransactionBatchMetricDimensionLabel(value string) string {
	switch value {
	case atomicTransactionBatchBudgetExpandedPostings,
		atomicTransactionBatchBudgetExecutionBalances,
		atomicTransactionBatchBudgetCompletionPlanBytes,
		atomicTransactionBatchBudgetAccountingRequestBytes,
		atomicTransactionBatchBudgetPreparedResponseBytes,
		atomicTransactionBatchBudgetRecoveryBytes,
		atomicTransactionBatchBudgetCachedResponseBytes:
		return value
	default:
		return atomicTransactionBatchMetricDimensionNone
	}
}
