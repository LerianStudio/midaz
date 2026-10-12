// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/LerianStudio/lib-commons/v7/commons/backoff"
	libOutbox "github.com/LerianStudio/lib-commons/v7/commons/outbox"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/LerianStudio/lib-system-facts/manifest"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/LerianStudio/lib-system-facts/registry"
)

// systemFactsContractState is the gate in front of the forwarder: the ledger
// records facts from the first commit, but delivers them only once the
// contract they validate against is ACTIVE at System Facts. Until then every
// record stays PENDING (a record turns INVALID only through dispatch attempts,
// and dispatch happens only inside the forwarder's Run).
type systemFactsContractState int32

const (
	// systemFactsContractPending: the reconcile has not released the forwarder
	// yet (first attempt in flight, or retrying an unavailable registry). The
	// zero value.
	systemFactsContractPending systemFactsContractState = iota
	// systemFactsContractActive: the contract is registered and verified; the
	// forwarder was released.
	systemFactsContractActive
	// systemFactsContractRejected: a permanent refusal. The inputs (manifest,
	// credential, URL) are static under a running process, so only a restart
	// retries it.
	systemFactsContractRejected
)

// The /readyz reasons of the gate, reported while the kit forwarder is still
// idle. Fixed strings: /readyz is unauthenticated.
const (
	systemFactsReasonContractPending  = "contract not active"
	systemFactsReasonContractRejected = "contract rejected"
)

// The call that decided an attempt, logged as `source` on the release line.
const (
	systemFactsSourcePreflight = "preflight"
	systemFactsSourceReconcile = "reconcile"
)

const (
	// systemFactsReconcileFirstDelay and systemFactsReconcileDelayCap bound the
	// wait between two reconcile attempts against an unavailable registry.
	// Each attempt already retries inside the kit, so the loop backs off on
	// top of that and never gives up while the process lives.
	systemFactsReconcileFirstDelay = 5 * time.Second
	systemFactsReconcileDelayCap   = 5 * time.Minute
	// defaultSystemFactsReconcileAttemptTimeout bounds one registry call
	// (Preflight or Run) when SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS is
	// unset, so a hung call cannot pin the loop.
	defaultSystemFactsReconcileAttemptTimeout = 60 * time.Second
)

// systemFactsContractReconcileTotal counts reconcile attempts by outcome
// (ok | unavailable | rejected | invalid), one sample per attempt.
var systemFactsContractReconcileTotal = metrics.Metric{
	Name:        "system_facts_contract_reconcile_total",
	Unit:        "1",
	Description: "System Facts contract reconcile attempts at boot, by outcome (ok, unavailable, rejected, invalid).",
}

// systemFactsReconciler is the seam the gate drives: the kit's read-only
// Preflight and its registering Run over the ledger's embedded manifest. A
// test injects a fake with no HTTP.
type systemFactsReconciler interface {
	Preflight(ctx context.Context) (reconciler.Report, error)
	Run(ctx context.Context) (reconciler.Report, error)
}

// kitSystemFactsReconciler is the production seam: the kit reconciler over
// the embedded manifest and the registry client. options is the zero value
// (the kit defaults).
type kitSystemFactsReconciler struct {
	manifest *manifest.Manifest
	registry reconciler.Registry
	options  reconciler.Options
}

// Preflight only reads, so a restart with the contract registered costs no PUT.
func (r *kitSystemFactsReconciler) Preflight(ctx context.Context) (reconciler.Report, error) {
	report, err := reconciler.Preflight(ctx, r.manifest, r.registry, r.options)
	if err != nil {
		return report, fmt.Errorf("system facts contract preflight: %w", err)
	}

	return report, nil
}

// Run registers and reads back. Replicas racing it are safe: a rerun on an
// unchanged manifest answers Replayed, so no lock is taken.
func (r *kitSystemFactsReconciler) Run(ctx context.Context) (reconciler.Report, error) {
	report, err := reconciler.Run(ctx, r.manifest, r.registry, r.options)
	if err != nil {
		return report, fmt.Errorf("system facts contract reconcile: %w", err)
	}

	return report, nil
}

// systemFactsContractGate holds the forwarder until the contract is ACTIVE.
// now, delay and wait are the test seams; nil means the real clock,
// systemFactsReconcileDelay and backoff.WaitContext.
type systemFactsContractGate struct {
	reconciler     systemFactsReconciler
	attemptTimeout time.Duration
	logger         libLog.Logger
	metricsFactory *metrics.MetricsFactory
	state          atomic.Int32
	now            func() time.Time
	delay          func(attempt int) time.Duration
	wait           func(ctx context.Context, d time.Duration) error
}

// newSystemFactsContractGate builds the gate over r. A nil factory records no
// metric; a nil logger becomes a no-op logger.
func newSystemFactsContractGate(r systemFactsReconciler, attemptTimeout time.Duration, logger libLog.Logger, factory *metrics.MetricsFactory) *systemFactsContractGate {
	if logger == nil {
		logger = libLog.NewNop()
	}

	return &systemFactsContractGate{
		reconciler:     r,
		attemptTimeout: attemptTimeout,
		logger:         logger,
		metricsFactory: factory,
	}
}

// contractState reads the gate; a nil gate has released nothing.
func (g *systemFactsContractGate) contractState() systemFactsContractState {
	if g == nil {
		return systemFactsContractPending
	}

	return systemFactsContractState(g.state.Load())
}

// reconcile holds the forwarder until the contract is ACTIVE and answers
// whether it was released. One attempt is a Preflight; a gap (Rejected) is
// registered by Run. Unavailable from either waits and retries, forever,
// while ctx lives, and so does a Run refused with a 404 (a registry route
// being rolled out). A permanent refusal (Run Rejected or Invalid, Preflight
// Invalid) logs ONE Error and leaves the gate rejected: the inputs are
// static, so only a restart retries. The end of ctx (shutdown) ends the loop
// quietly with the gate still pending.
func (g *systemFactsContractGate) reconcile(ctx context.Context) bool {
	if g == nil {
		return false
	}

	logger := g.log()

	if g.reconciler == nil {
		logger.Log(ctx, libLog.LevelError, "System Facts contract reconciler is not wired; the forwarder stays stopped")

		return false
	}

	delay, wait := g.pacing()

	for attempt := 1; ; attempt++ {
		report, source, err := g.attempt(ctx)
		if ctx.Err() != nil {
			return false
		}

		outcome := systemFactsAttemptOutcome(report, source, err)
		g.count(ctx, logger, outcome)

		switch outcome {
		case reconciler.OutcomeOK:
			g.state.Store(int32(systemFactsContractActive))
			logSystemFactsContractActive(ctx, logger, source, report)

			return true
		case reconciler.OutcomeUnavailable:
			next := delay(attempt)
			logSystemFactsReconcileDeferred(ctx, logger, attempt, report, next)

			if wait(ctx, next) != nil {
				return false
			}
		case reconciler.OutcomeRejected, reconciler.OutcomeInvalid:
			g.state.Store(int32(systemFactsContractRejected))
			logSystemFactsContractRejected(ctx, logger, outcome, report)

			return false
		}
	}
}

// attempt is one gate attempt: Preflight, and Run only on a gap. It answers
// the report that decides the attempt and which call produced it. Each call
// runs under its own attempt timeout.
func (g *systemFactsContractGate) attempt(ctx context.Context) (reconciler.Report, string, error) {
	report, err := g.call(ctx, g.reconciler.Preflight)
	if systemFactsReconcileOutcome(report, err) != reconciler.OutcomeRejected {
		return report, systemFactsSourcePreflight, err
	}

	report, err = g.call(ctx, g.reconciler.Run)

	return report, systemFactsSourceReconcile, err
}

// call runs one registry call under the attempt timeout.
func (g *systemFactsContractGate) call(ctx context.Context, fn func(context.Context) (reconciler.Report, error)) (reconciler.Report, error) {
	timeout := g.attemptTimeout
	if timeout <= 0 {
		timeout = defaultSystemFactsReconcileAttemptTimeout
	}

	callCtx, cancel := context.WithDeadline(ctx, g.clock().Add(timeout))
	defer cancel()

	return fn(callCtx)
}

// pacing is the delay and the wait between attempts: the injected ones, or
// systemFactsReconcileDelay and backoff.WaitContext.
func (g *systemFactsContractGate) pacing() (func(int) time.Duration, func(context.Context, time.Duration) error) {
	delay, wait := g.delay, g.wait

	if delay == nil {
		delay = systemFactsReconcileDelay
	}

	if wait == nil {
		wait = systemFactsReconcileWait
	}

	return delay, wait
}

func (g *systemFactsContractGate) clock() time.Time {
	if g.now == nil {
		return time.Now()
	}

	return g.now()
}

func (g *systemFactsContractGate) log() libLog.Logger {
	if g.logger == nil {
		return libLog.NewNop()
	}

	return g.logger
}

// count records one attempt; a metric failure never alters the gate.
func (g *systemFactsContractGate) count(ctx context.Context, logger libLog.Logger, outcome reconciler.Outcome) {
	incrementSystemFactsCounter(ctx, g.metricsFactory, logger, systemFactsContractReconcileTotal,
		map[string]string{"outcome": systemFactsReconcileOutcomeLabel(outcome)})
}

// systemFactsReconcileBaseDelay is the un-jittered wait before the attempt
// after attempt: exponential from systemFactsReconcileFirstDelay, capped at
// systemFactsReconcileDelayCap BEFORE the jitter, so replicas that reach the
// cap stay spread instead of retrying in lockstep.
func systemFactsReconcileBaseDelay(attempt int) time.Duration {
	return min(backoff.Exponential(systemFactsReconcileFirstDelay, attempt-1), systemFactsReconcileDelayCap)
}

// systemFactsReconcileDelay is lib-commons' full jitter over the capped base.
func systemFactsReconcileDelay(attempt int) time.Duration {
	return backoff.FullJitter(systemFactsReconcileBaseDelay(attempt))
}

// systemFactsReconcileWait is lib-commons' context-aware wait: the run
// context ending at shutdown ends it.
func systemFactsReconcileWait(ctx context.Context, d time.Duration) error {
	if err := backoff.WaitContext(ctx, d); err != nil {
		return fmt.Errorf("system facts reconcile wait: %w", err)
	}

	return nil
}

// systemFactsReconcileOutcome classifies one answer. The kit returns a nil
// error exactly on OK; anything it did not classify is a refusal, never a
// release.
func systemFactsReconcileOutcome(report reconciler.Report, err error) reconciler.Outcome {
	switch {
	case err == nil && report.Outcome == reconciler.OutcomeOK:
		return reconciler.OutcomeOK
	case report.Outcome == reconciler.OutcomeUnavailable || errors.Is(err, reconciler.ErrUnavailable):
		return reconciler.OutcomeUnavailable
	case report.Outcome == reconciler.OutcomeInvalid || errors.Is(err, reconciler.ErrInvalid):
		return reconciler.OutcomeInvalid
	default:
		return reconciler.OutcomeRejected
	}
}

// systemFactsAttemptOutcome classifies one attempt. It is
// systemFactsReconcileOutcome with one narrowing: a Run refused with a 404 is
// Unavailable, not a permanent refusal. System Facts always serves the
// registry route, so a 404 on the registration is an ingress default backend
// mid-rollout or a SYSTEM_FACTS_URL naming something else; both are retried.
// A 404 at Preflight is the ordinary gap that leads to Run, and stays so.
func systemFactsAttemptOutcome(report reconciler.Report, source string, err error) reconciler.Outcome {
	outcome := systemFactsReconcileOutcome(report, err)
	if outcome != reconciler.OutcomeRejected || source != systemFactsSourceReconcile {
		return outcome
	}

	if errors.Is(err, registry.ErrNotFound) {
		return reconciler.OutcomeUnavailable
	}

	if step, ok := firstFailingStep(report); ok && step.Problem != nil && step.Problem.Status == http.StatusNotFound {
		return reconciler.OutcomeUnavailable
	}

	return outcome
}

// systemFactsReconcileOutcomeLabel is the metric label of an outcome.
func systemFactsReconcileOutcomeLabel(outcome reconciler.Outcome) string {
	switch outcome {
	case reconciler.OutcomeOK:
		return "ok"
	case reconciler.OutcomeUnavailable:
		return "unavailable"
	case reconciler.OutcomeInvalid:
		return "invalid"
	case reconciler.OutcomeRejected:
		return "rejected"
	default:
		return "rejected"
	}
}

// firstFailingStep is the first step that did not succeed, if any.
func firstFailingStep(report reconciler.Report) (reconciler.Step, bool) {
	for _, step := range report.Steps {
		switch step.Result {
		case reconciler.ResultCreated, reconciler.ResultReplayed, reconciler.ResultVerified:
			continue
		case reconciler.ResultRejected, reconciler.ResultUnavailable, reconciler.ResultInvalid:
			return step, true
		default:
			return step, true
		}
	}

	return reconciler.Step{}, false
}

func logSystemFactsContractActive(ctx context.Context, logger libLog.Logger, source string, report reconciler.Report) {
	var created, replayed, verified int

	for _, step := range report.Steps {
		switch step.Result {
		case reconciler.ResultCreated:
			created++
		case reconciler.ResultReplayed:
			replayed++
		case reconciler.ResultVerified:
			verified++
		case reconciler.ResultRejected, reconciler.ResultUnavailable, reconciler.ResultInvalid:
		}
	}

	logger.Log(ctx, libLog.LevelInfo, "System Facts contract active",
		libLog.String("source", source),
		libLog.Int("created", created),
		libLog.Int("replayed", replayed),
		libLog.Int("verified", verified))
}

func logSystemFactsReconcileDeferred(ctx context.Context, logger libLog.Logger, attempt int, report reconciler.Report, next time.Duration) {
	fields := []any{
		libLog.Int("attempt", attempt),
		libLog.String("outcome", systemFactsReconcileOutcomeLabel(reconciler.OutcomeUnavailable)),
		libLog.String("next_retry_in", next.String()),
	}

	if step, ok := firstFailingStep(report); ok {
		fields = append(fields, libLog.String("step_kind", string(step.Kind)), libLog.String("step_subject", step.Subject))
	}

	logger.Log(ctx, libLog.LevelWarn, "System Facts contract reconcile deferred", fields...)
}

// logSystemFactsContractRejected is the ONE Error of a permanent refusal. It
// names the failing step and the registry's problem code and detail (server
// text, sanitized), never the step detail: a registry client error embeds the
// target URL, and the bearer is never rendered by the client.
func logSystemFactsContractRejected(ctx context.Context, logger libLog.Logger, outcome reconciler.Outcome, report reconciler.Report) {
	fields := []any{libLog.String("outcome", systemFactsReconcileOutcomeLabel(outcome))}

	if step, ok := firstFailingStep(report); ok {
		fields = append(fields,
			libLog.String("step_kind", string(step.Kind)),
			libLog.String("step_subject", step.Subject))

		if step.Problem != nil {
			fields = append(fields,
				libLog.String("problem_code", step.Problem.Code),
				libLog.String("problem_detail", libOutbox.SanitizeErrorMessageForStorage(step.Problem.Detail)))
		}
	}

	logger.Log(ctx, libLog.LevelError, "System Facts contract rejected; the forwarder stays stopped", fields...)
}
