// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type ContextTracerConfig struct {
	Bounds          tracercontract.Limits
	MaxReservations int
	// AdmissionTimeout caps the entire facts/Reserve phase.
	AdmissionTimeout time.Duration
}

// ContextTracerCoordinator admits prepared transactions and delivers their
// known accounting outcome by transaction. It keeps no state between the two:
// a completion that is never delivered is released by the Tracer TTL.
type ContextTracerCoordinator struct {
	client ContextTracerReserver
	facts  TracerFactsLoader
	config ContextTracerConfig
	now    Clock
}

func NewContextTracerCoordinator(client ContextTracerReserver, facts TracerFactsLoader, cfg ContextTracerConfig, now Clock) (*ContextTracerCoordinator, error) {
	if client == nil || facts == nil || now == nil || cfg.MaxReservations <= 0 || cfg.AdmissionTimeout <= 0 {
		return nil, constant.ErrTracerContractUnavailable
	}

	if err := cfg.Bounds.Validate(); err != nil {
		return nil, err
	}

	return &ContextTracerCoordinator{client: client, facts: facts, config: cfg, now: now}, nil
}

// ValidateActivation is local and safe under the settings merge lock. Deployed
// artifact compatibility is verified by rollout, with strict response checks on
// every call; this method never creates a probe reservation or dials the peer.
func (c *ContextTracerCoordinator) ValidateActivation(ctx context.Context) error { return ctx.Err() }

// Complete delivers a known accounting outcome for one transaction within
// min(timeout, AdmissionTimeout) of the caller's context; a non-positive
// timeout uses AdmissionTimeout. Only confirm and release are accepted. The
// error is returned to the caller, which owns logging and redelivery.
func (c *ContextTracerCoordinator) Complete(ctx context.Context, transactionID uuid.UUID, action string, timeout time.Duration) error {
	var (
		complete func(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error)
		status   string
	)

	switch action {
	case reservationActionConfirm:
		complete, status = c.client.ConfirmByTransaction, "CONFIRMED"
	case reservationActionRelease:
		complete, status = c.client.ReleaseByTransaction, "RELEASED"
	default:
		return fmt.Errorf("%w: unsupported completion action %q", constant.ErrTracerContractUnavailable, action)
	}

	bound := c.config.AdmissionTimeout
	if timeout > 0 {
		bound = min(timeout, bound)
	}

	ctx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()

	result, err := complete(ctx, transactionID)
	if err != nil {
		return err
	}

	if result == nil || result.Validate() != nil || result.TransactionID != transactionID || result.Status != status {
		return constant.ErrTracerContractUnavailable
	}

	return nil
}

// contextCompletionTerminal reports failures a redelivery cannot change: an
// operation conflict (for example a confirm after the Tracer TTL expired the
// reservation) or a response that contradicts the request.
func contextCompletionTerminal(err error) bool {
	return errors.Is(err, constant.ErrReserveOperationConflict) || errors.Is(err, constant.ErrInvalidRequestBody) || errors.Is(err, constant.ErrTracerContractUnavailable)
}

// contextReleaseSettled reports a release answered with an operation conflict:
// the operation already reached a terminal state, such as expiry, so its
// capacity is no longer held and nothing is lost.
func contextReleaseSettled(transition reservationTransition, err error) bool {
	return transition.Action == reservationActionRelease && errors.Is(err, constant.ErrReserveOperationConflict)
}

// concludeContextCompletion reports a completion failure no redelivery can
// change and says whether it did. A settled release is logged without
// flipping the span; any other terminal failure is the last word on a loss.
func concludeContextCompletion(ctx context.Context, span trace.Span, logger libLog.Logger, transition reservationTransition, err error) bool {
	switch {
	case contextReleaseSettled(transition, err):
		logger.Log(ctx, libLog.LevelInfo, "Tracer reservation already settled; release not needed",
			append(transition.logFields(), libLog.Err(err)))

		return true
	case contextCompletionTerminal(err):
		libOpentelemetry.HandleSpanError(span, "Tracer reservation "+transition.Action+" rejected permanently", err)

		logger.Log(ctx, libLog.LevelError,
			"Tracer reservation transition rejected permanently; "+transition.lossConsequence(),
			append(transition.logFields(), libLog.Err(err)))

		return true
	default:
		return false
	}
}

// contextTracerRetryTransport lets the in-memory reservation retrier redeliver
// a context completion. It satisfies the retrier's full TracerReserver
// interface, but the context contract addresses completion only by
// transaction, so the reservation-id and reserve operations are unsupported.
// A failure no redelivery can change is reported here and answered with
// errReservationRetryStop, which ends the sequence without a further log.
type contextTracerRetryTransport struct {
	coordinator *ContextTracerCoordinator
	logger      libLog.Logger
	timeout     time.Duration
	transition  reservationTransition
}

func (t contextTracerRetryTransport) Reserve(context.Context, traceradapter.ReserveRequest) (*traceradapter.ReserveResult, error) {
	return nil, constant.ErrTracerContractUnavailable
}

func (t contextTracerRetryTransport) Confirm(context.Context, uuid.UUID) error {
	return constant.ErrTracerContractUnavailable
}

func (t contextTracerRetryTransport) Release(context.Context, uuid.UUID) error {
	return constant.ErrTracerContractUnavailable
}

func (t contextTracerRetryTransport) ConfirmByTransaction(ctx context.Context, _ uuid.UUID) error {
	return t.deliver(ctx)
}

func (t contextTracerRetryTransport) ReleaseByTransaction(ctx context.Context, _ uuid.UUID) error {
	return t.deliver(ctx)
}

func (t contextTracerRetryTransport) deliver(ctx context.Context) error {
	err := t.coordinator.Complete(ctx, t.transition.TransactionID, t.transition.Action, t.timeout)
	if err != nil && concludeContextCompletion(ctx, trace.SpanFromContext(ctx), t.logger, t.transition, err) {
		return errReservationRetryStop
	}

	return err
}
