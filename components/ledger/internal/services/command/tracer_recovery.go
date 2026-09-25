// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LerianStudio/lib-commons/v7/commons/backoff"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type TracerRecoveryConfig struct {
	IntegrationID    string
	Namespace        string
	SingleTenant     bool
	MaxBatch         int
	RetryInterval    time.Duration
	AttemptTimeout   time.Duration
	MaxRetryInterval time.Duration
}

func (c TracerRecoveryConfig) Validate() error {
	if c.MaxRetryInterval < 0 || (c.MaxRetryInterval > 0 && c.MaxRetryInterval < c.RetryInterval) {
		return constant.ErrInvalidRequestBody
	}

	if !validTracerIdentity(c.IntegrationID) || !validTracerIdentity(c.Namespace) || c.MaxBatch <= 0 || c.RetryInterval <= 0 || c.AttemptTimeout <= 0 {
		return constant.ErrInvalidRequestBody
	}

	return nil
}

func validTracerIdentity(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && strings.TrimSpace(value) == value && strings.IndexFunc(value, unicode.IsControl) < 0
}

type TracerRecoverySummary struct {
	Claimed    int
	Delivered  int
	Unresolved int
	Failed     int
}

// TracerRecoveryProcessor finishes known outcomes. Its dependency list contains
// neither an accounting executor nor a facts/settings loader. Worker lifetime is
// independent of per-ledger participation and current validation settings.
type TracerRecoveryProcessor struct {
	// MetricsFactory is configured at composition time; nil disables recording.
	MetricsFactory *metrics.MetricsFactory
	store          TracerObligationStore
	client         ContextTracerReserver
	evidence       TracerAccountingEvidence
	config         TracerRecoveryConfig
	now            Clock
}

func NewTracerRecoveryProcessor(store TracerObligationStore, client ContextTracerReserver, evidence TracerAccountingEvidence, cfg TracerRecoveryConfig, now Clock) (*TracerRecoveryProcessor, error) {
	if store == nil || client == nil || evidence == nil || now == nil {
		return nil, constant.ErrTracerContractUnavailable
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.MaxRetryInterval == 0 {
		cfg.MaxRetryInterval = max(5*time.Minute, cfg.RetryInterval)
	}

	return &TracerRecoveryProcessor{store: store, client: client, evidence: evidence, config: cfg, now: now}, nil
}

func (p *TracerRecoveryProcessor) RunOnce(ctx context.Context) (summary TracerRecoverySummary, retErr error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	// Nest each reconciliation attempt under the cycle's semantic span.
	ctx, span := tracer.Start(ctx, "command.recover_tracer_reservations")
	defer span.End()
	defer func() {
		span.SetAttributes(attribute.Int("app.response.tracer.recovery.claimed", summary.Claimed), attribute.Int("app.response.tracer.recovery.delivered", summary.Delivered), attribute.Int("app.response.tracer.recovery.unresolved", summary.Unresolved), attribute.Int("app.response.tracer.recovery.failed", summary.Failed))

		recordTracerCoordinationError(span, retErr)
	}()

	if err := ctx.Err(); err != nil {
		return summary, err
	}

	if !p.config.SingleTenant && tmcore.GetTenantIDContext(ctx) == "" {
		return summary, constant.ErrTracerContractUnavailable
	}

	now := p.now().UTC()

	records, err := p.store.ClaimDue(ctx, now, now.Add(p.config.RetryInterval), p.config.MaxBatch)
	if err != nil {
		return summary, fmt.Errorf("claim tracer obligations: %w", err)
	}

	summary.Claimed = len(records)
	if len(records) > p.config.MaxBatch {
		return summary, constant.ErrTracerContractUnavailable
	}

	var failures []error

	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return summary, errors.Join(append(failures, err)...)
		}

		emitTracerObligationAge(ctx, p.MetricsFactory, record, now)

		attempt, cancel := context.WithTimeout(ctx, p.config.AttemptTimeout)
		delivered, err := p.process(attempt, record)

		cancel()

		if !delivered {
			quarantine := p.validatePending(ctx, record) != nil

			next := p.now().UTC().Add(p.retryDelay(record.RecoveryAttempts))
			if scheduleErr := p.store.ScheduleRetry(ctx, record, next, quarantine); scheduleErr != nil {
				err = errors.Join(err, fmt.Errorf("schedule tracer retry: %w", scheduleErr))
			}
		}

		switch {
		case err != nil:
			summary.Failed++

			failures = append(failures, err)
		case delivered:
			summary.Delivered++
		default:
			summary.Unresolved++
		}
	}

	return summary, errors.Join(failures...)
}

func (p *TracerRecoveryProcessor) retryDelay(attempts int) time.Duration {
	ceiling := min(backoff.Exponential(p.config.RetryInterval, max(0, attempts-1)), p.config.MaxRetryInterval)
	// Keep at least one poll interval, with jitter over the remaining range.
	return p.config.RetryInterval + backoff.FullJitter(ceiling-p.config.RetryInterval)
}

func recordTracerCoordinationError(span trace.Span, err error) {
	if err == nil {
		return
	}

	classified := err
	for errors.Unwrap(classified) != nil {
		classified = errors.Unwrap(classified)
	}

	if pkg.IsBusinessError(pkg.ValidateBusinessError(classified, "TracerCoordination")) {
		libOtel.HandleSpanBusinessErrorEvent(span, "Tracer coordination rejected", err)
		return
	}

	libOtel.HandleSpanError(span, "Tracer coordination incomplete", err)
}

func (p *TracerRecoveryProcessor) process(ctx context.Context, record tracerreservation.Pending) (delivered bool, retErr error) {
	started := time.Now()
	operation := "recovery"

	defer func() {
		result := "unresolved"
		if retErr != nil {
			result = "failed"
		} else if delivered {
			result = "delivered"
		}

		emitTracerMetric(ctx, p.MetricsFactory, operation, result, time.Since(started))
	}()

	if err := p.validatePending(ctx, record); err != nil {
		return false, err
	}

	outcome, known, err := p.resolve(ctx, record)
	if err != nil || !known {
		return false, err
	}

	operation = "release"
	if outcome == tracerreservation.Confirmed {
		operation = "confirm"
	}

	var response *tracercontract.TransactionCompletionResult
	if outcome == tracerreservation.Confirmed {
		response, err = p.client.ConfirmByTransaction(ctx, record.Key.TransactionID)
	} else {
		response, err = p.client.ReleaseByTransaction(ctx, record.Key.TransactionID)
	}

	if err != nil {
		return false, fmt.Errorf("deliver tracer outcome: %w", err)
	}

	if response == nil || response.Validate() != nil || response.TransactionID != record.Key.TransactionID || response.Status != string(outcome) {
		return false, constant.ErrTracerContractUnavailable
	}

	if err := p.store.MarkDelivered(ctx, record.Key, outcome, p.now().UTC()); err != nil {
		return false, fmt.Errorf("acknowledge tracer outcome: %w", err)
	}

	return true, nil
}

func (p *TracerRecoveryProcessor) validatePending(ctx context.Context, record tracerreservation.Pending) error {
	if err := record.Key.Validate(); err != nil {
		return err
	}

	if record.ExecutionID == uuid.Nil || record.Scope.TenantID != tmcore.GetTenantIDContext(ctx) || record.Scope.IntegrationID != p.config.IntegrationID || record.Scope.AssetNamespace != p.config.Namespace || record.ContractRevision != tracercontract.ReserveContractRevision {
		return constant.ErrTracerContractUnavailable
	}

	switch record.State {
	case tracerreservation.Prepared, tracerreservation.Executing, tracerreservation.Confirmed, tracerreservation.Released:
		return nil
	default:
		return constant.ErrTracerContractUnavailable
	}
}

func (p *TracerRecoveryProcessor) resolve(ctx context.Context, record tracerreservation.Pending) (tracerreservation.State, bool, error) {
	if record.State.Terminal() {
		return record.State, true, nil
	}

	if record.State == tracerreservation.Prepared {
		expired, err := p.store.ExpirePrepared(ctx, record.Key, p.now().UTC())
		return tracerreservation.Released, expired, err
	}

	status, err := p.evidence.ReadAccountingStatus(ctx, record.Key)
	if err != nil {
		return "", false, fmt.Errorf("read accounting evidence: %w", err)
	}

	var outcome tracerreservation.State

	switch status {
	case constant.APPROVED:
		outcome = tracerreservation.Confirmed
	case constant.CANCELED:
		outcome = tracerreservation.Released
	default:
		return "", false, nil
	}

	if err := p.store.SetOutcome(ctx, record.Key, outcome, p.now().UTC()); err != nil {
		return "", false, fmt.Errorf("record proven accounting outcome: %w", err)
	}

	return outcome, true, nil
}
