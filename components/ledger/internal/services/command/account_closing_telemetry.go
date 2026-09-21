// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Bounded outcome vocabulary of one closing attempt. Four values, and every
// attempt lands on exactly one of them.
const (
	accountClosingOutcomeClosed        = "closed"
	accountClosingOutcomeRefused       = "refused"
	accountClosingOutcomeIndeterminate = "indeterminate"
	accountClosingOutcomeTechnical     = "technical_error"
)

// Bounded refusal vocabulary. Each value names WHY a closing did not happen, and
// the set is closed: an error the mapping does not recognize collapses onto
// accountClosingReasonBusinessOther or accountClosingReasonTechnical rather than
// widening the label. Nothing here carries an account, an organization, a ledger
// or an amount, so the series count is the size of this list.
const (
	accountClosingReasonAlreadyClosed           = "already_closed"
	accountClosingReasonClosingInProgress       = "closing_in_progress"
	accountClosingReasonBalanceNotZero          = "balance_not_zero"
	accountClosingReasonPendingTransactions     = "pending_transactions"
	accountClosingReasonPersistencePending      = "persistence_pending"
	accountClosingReasonAccountClosed           = "account_closed"
	accountClosingReasonProtectionIndeterminate = "protection_indeterminate"
	accountClosingReasonExternalAccount         = "external_account"
	accountClosingReasonAccountNotFound         = "account_not_found"
	accountClosingReasonBusinessOther           = "business_other"
	accountClosingReasonTechnical               = "technical"
)

// Bounded stage vocabulary of a reconciliation failure: the step of the pass that
// could not complete, never the key or the account it was working on.
const (
	accountClosingStageScanMarkers          = "scan_markers"
	accountClosingStageScanOwnerships       = "scan_ownerships"
	accountClosingStageReadMarker           = "read_marker"
	accountClosingStageReadAccount          = "read_account"
	accountClosingStageListBalances         = "list_balances"
	accountClosingStageEvictBalance         = "evict_balance"
	accountClosingStageInstallClosedMarker  = "install_closed_marker"
	accountClosingStageReleaseClosedMarker  = "release_closed_marker"
	accountClosingStageReleaseAbortedMarker = "release_aborted_marker"
	accountClosingStageReleaseOwnership     = "release_ownership"
)

// Bounded outcome vocabulary of one reconciliation pass. `scanned` is the
// denominator of the pass and is deliberately NOT disjoint from the others.
const (
	accountClosingReconcileScanned    = "scanned"
	accountClosingReconcileCompleted  = "completed"
	accountClosingReconcileReleased   = "released"
	accountClosingReconcileRetained   = "retained"
	accountClosingReconcileUnreadable = "unreadable"
)

// Bounded backlog vocabulary: what the pass left behind and why it may not be
// given back.
const (
	accountClosingBacklogOwnership = "ownership"
	accountClosingBacklogRetained  = "retained"
)

// accountClosingReconcileDurationBuckets is a millisecond ladder wide enough for a
// paginated scan of the protection namespace, which is bounded by pages rather
// than by a request deadline.
var accountClosingReconcileDurationBuckets = []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

var (
	// accountClosingRequests counts closing attempts by bounded outcome. The
	// DURATION of a closing is not declared here on purpose: the use case already
	// records domain_operation_duration_ms{component="ledger",operation="close_account"}
	// through utils.RecordDomainOperation, which carries no high-cardinality label,
	// and a second declaration of one measurement is what makes dashboards disagree.
	accountClosingRequests = metrics.Metric{
		Name:        "account_closing_total",
		Unit:        "1",
		Description: "Account closing attempts by bounded outcome.",
	}

	// accountClosingRefusals counts attempts that did not close the account, by the
	// bounded reason that stopped them.
	accountClosingRefusals = metrics.Metric{
		Name:        "account_closing_refusals_total",
		Unit:        "1",
		Description: "Account closing attempts that did not close the account, by bounded reason.",
	}

	// accountClosingReconcileMarkers counts the closing markers one reconciliation
	// pass resolved, by bounded outcome.
	accountClosingReconcileMarkers = metrics.Metric{
		Name:        "account_closing_reconciliation_markers_total",
		Unit:        "1",
		Description: "Closing markers seen by a reconciliation pass, by bounded outcome.",
	}

	// accountClosingReconcileFailures counts the steps of a reconciliation pass that
	// could not complete, by bounded stage.
	accountClosingReconcileFailures = metrics.Metric{
		Name:        "account_closing_reconciliation_failures_total",
		Unit:        "1",
		Description: "Reconciliation steps that could not complete, by bounded stage.",
	}

	// accountClosingReconcileBacklog is the protection a pass deliberately left in
	// place. It is the measure to alert on: a backlog that stops draining means
	// closings, balance creation and cache-miss admission stay blocked on those
	// accounts while nothing is failing per request.
	accountClosingReconcileBacklog = metrics.Metric{
		Name:        "account_closing_reconciliation_backlog",
		Unit:        "1",
		Description: "Account protection left installed after a reconciliation pass, by bounded kind.",
	}

	// accountClosingReconcileLastSuccess is the unix instant of the last pass that
	// walked both namespaces without a scan failure. The AGE of the reconciliation
	// is `time() - metric`: a failure counter cannot see a pass that stopped running
	// altogether, which is the silent form of the same outage.
	//
	// The declared name carries NO unit suffix: the OTLP-to-Prometheus translation
	// appends one from Unit, so this reaches Mimir as
	// `account_closing_reconciliation_last_success_timestamp_seconds`.
	accountClosingReconcileLastSuccess = metrics.Metric{
		Name:        "account_closing_reconciliation_last_success_timestamp",
		Unit:        "s",
		Description: "Unix timestamp of the last account closing reconciliation pass that completed both scans.",
	}

	// accountClosingReconcileDuration measures one reconciliation pass end to end.
	accountClosingReconcileDuration = metrics.Metric{
		Name:        "account_closing_reconciliation_duration_ms",
		Unit:        "ms",
		Description: "Duration of one account closing reconciliation pass in milliseconds.",
		Buckets:     accountClosingReconcileDurationBuckets,
	}
)

// recordAccountClosingOutcome emits the closing metrics for one attempt.
//
// Call it at the single exit boundary of the use case, beside the domain metric
// it complements: that one answers "did the operation succeed", this one answers
// "why did it not". A nil factory is a no-op so a binary with metrics disabled —
// and every unit test — runs the same path; an emit failure logs at Debug and
// never reaches the caller.
func recordAccountClosingOutcome(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, err error) {
	if factory == nil {
		return
	}

	outcome, reason := accountClosingOutcome(err)

	addAccountClosingCounter(ctx, factory, logger, accountClosingRequests, map[string]string{"outcome": outcome}, 1)

	if reason != "" {
		addAccountClosingCounter(ctx, factory, logger, accountClosingRefusals, map[string]string{"reason": reason}, 1)
	}
}

// recordAccountClosingReconciliation emits the metrics of one reconciliation pass:
// what it resolved, what failed, what it left behind, and when it last completed.
//
// complete says both namespace walks finished without a scan failure. Only then is
// the last-success instant advanced, so an age alert measures reconciliation that
// actually ran rather than a pass that aborted on its first page.
func recordAccountClosingReconciliation(
	ctx context.Context,
	factory *metrics.MetricsFactory,
	logger libLog.Logger,
	stats AccountClosingReconciliationStats,
	duration time.Duration,
	complete bool,
	now time.Time,
) {
	if factory == nil {
		return
	}

	outcomes := map[string]int{
		accountClosingReconcileScanned:    stats.Scanned,
		accountClosingReconcileCompleted:  stats.Completed,
		accountClosingReconcileReleased:   stats.Released,
		accountClosingReconcileRetained:   stats.Retained,
		accountClosingReconcileUnreadable: stats.Unreadable,
	}

	for _, outcome := range []string{
		accountClosingReconcileScanned,
		accountClosingReconcileCompleted,
		accountClosingReconcileReleased,
		accountClosingReconcileRetained,
		accountClosingReconcileUnreadable,
	} {
		if count := outcomes[outcome]; count > 0 {
			addAccountClosingCounter(ctx, factory, logger, accountClosingReconcileMarkers, map[string]string{"outcome": outcome}, int64(count))
		}
	}

	for _, stage := range accountClosingFailureStages {
		if count := stats.failures[stage]; count > 0 {
			addAccountClosingCounter(ctx, factory, logger, accountClosingReconcileFailures, map[string]string{"stage": stage}, int64(count))
		}
	}

	setAccountClosingGauge(ctx, factory, logger, accountClosingReconcileBacklog, map[string]string{"kind": accountClosingBacklogOwnership}, int64(stats.Ownerships))
	setAccountClosingGauge(ctx, factory, logger, accountClosingReconcileBacklog, map[string]string{"kind": accountClosingBacklogRetained}, int64(stats.Retained))

	histogram, err := factory.Histogram(accountClosingReconcileDuration)
	if err == nil {
		err = histogram.Record(ctx, duration.Milliseconds())
	}

	logAccountClosingMetricError(ctx, logger, err)

	if complete {
		setAccountClosingGauge(ctx, factory, logger, accountClosingReconcileLastSuccess, nil, now.Unix())
	}
}

// accountClosingFailureStages is the emission order of the failure vocabulary. It
// is iterated instead of the map so the counter only ever carries a stage this
// package declares, whatever a caller managed to put in the map.
var accountClosingFailureStages = []string{
	accountClosingStageScanMarkers,
	accountClosingStageScanOwnerships,
	accountClosingStageReadMarker,
	accountClosingStageReadAccount,
	accountClosingStageListBalances,
	accountClosingStageEvictBalance,
	accountClosingStageInstallClosedMarker,
	accountClosingStageReleaseClosedMarker,
	accountClosingStageReleaseAbortedMarker,
	accountClosingStageReleaseOwnership,
}

// accountClosingOutcome classifies one closing attempt into its bounded outcome
// and, when the account did not close, the bounded reason.
//
// The two are derived from the same sentinel so they cannot disagree. The
// indeterminate protection is its own outcome rather than a refusal: it reports
// that the closing state could not be established at all, which is an operator
// signal, while a refusal is the coordination answering as designed.
func accountClosingOutcome(err error) (outcome, reason string) {
	if err == nil {
		return accountClosingOutcomeClosed, ""
	}

	reason = accountClosingReason(err)

	switch reason {
	case accountClosingReasonProtectionIndeterminate:
		return accountClosingOutcomeIndeterminate, reason
	case accountClosingReasonTechnical:
		return accountClosingOutcomeTechnical, reason
	default:
		return accountClosingOutcomeRefused, reason
	}
}

// accountClosingReason maps one refusal onto the bounded vocabulary.
func accountClosingReason(err error) string {
	code := accountClosingErrorCode(err)

	switch code {
	case constant.ErrAccountAlreadyClosed.Error():
		return accountClosingReasonAlreadyClosed
	case constant.ErrAccountClosingInProgress.Error():
		return accountClosingReasonClosingInProgress
	case constant.ErrAccountBalanceNotZero.Error():
		return accountClosingReasonBalanceNotZero
	case constant.ErrAccountHasPendingTransactions.Error():
		return accountClosingReasonPendingTransactions
	case constant.ErrAccountClosingPersistencePending.Error():
		return accountClosingReasonPersistencePending
	case constant.ErrAccountClosed.Error():
		return accountClosingReasonAccountClosed
	case constant.ErrAccountClosingProtectionIndeterminate.Error():
		return accountClosingReasonProtectionIndeterminate
	case constant.ErrForbiddenExternalAccountManipulation.Error():
		return accountClosingReasonExternalAccount
	case constant.ErrAccountIDNotFound.Error():
		return accountClosingReasonAccountNotFound
	}

	if pkg.IsBusinessError(err) {
		return accountClosingReasonBusinessOther
	}

	return accountClosingReasonTechnical
}

// accountClosingErrorCode reads the registry code carried by a typed error of the
// error platform. It returns an empty string for anything else, including a raw
// driver error, which the caller classifies as technical.
//
// Only the classes the closing paths actually produce are read. A class not listed
// here is not silently mapped to a wrong reason: it has no code, so it falls
// through to the business/technical split.
func accountClosingErrorCode(err error) string {
	var conflict pkg.EntityConflictError
	if errors.As(err, &conflict) {
		return conflict.Code
	}

	var unprocessable pkg.UnprocessableOperationError
	if errors.As(err, &unprocessable) {
		return unprocessable.Code
	}

	var unavailable pkg.ServiceUnavailableError
	if errors.As(err, &unavailable) {
		return unavailable.Code
	}

	var notFound pkg.EntityNotFoundError
	if errors.As(err, &notFound) {
		return notFound.Code
	}

	var validation pkg.ValidationError
	if errors.As(err, &validation) {
		return validation.Code
	}

	return ""
}

func addAccountClosingCounter(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, metric metrics.Metric, labels map[string]string, value int64) {
	counter, err := factory.Counter(metric)
	if err == nil {
		err = counter.WithLabels(labels).Add(ctx, value)
	}

	logAccountClosingMetricError(ctx, logger, err)
}

func setAccountClosingGauge(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, metric metrics.Metric, labels map[string]string, value int64) {
	gauge, err := factory.Gauge(metric)
	if err == nil {
		err = gauge.WithLabels(labels).Set(ctx, value)
	}

	logAccountClosingMetricError(ctx, logger, err)
}

func logAccountClosingMetricError(ctx context.Context, logger libLog.Logger, err error) {
	if err != nil && logger != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit an account closing metric", libLog.Err(err))
	}
}
