// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextTracerKey names the transaction admitted within its ledger scope.
type ContextTracerKey struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

type ContextTracerInput struct {
	Key         ContextTracerKey
	Settings    mmodel.TracerSettings
	Timestamp   time.Time
	Amount      decimal.Decimal
	AssetCode   string
	Entries     []traceradapter.PreparedEntry
	HonoredSkip bool
	LongLived   bool
}

// ContextTracerAttempt carries the admission decision and the settings its
// completion is bounded by. A skipped attempt has nothing to complete.
// Dispatched means Reserve was sent, so Tracer may hold capacity even when
// no valid result came back. Unavailable means the dispatched Reserve failed
// for availability, so its completion is not attempted on the request path.
type ContextTracerAttempt struct {
	Skipped     bool
	Dispatched  bool
	Unavailable bool
	Settings    mmodel.TracerSettings
	Result      *tracercontract.ReserveResult
}

// Admit loads official facts and calls Tracer. The caller interprets the
// returned decision according to Ledger mode; Tracer alone evaluates policies.
func (c *ContextTracerCoordinator) Admit(ctx context.Context, input ContextTracerInput) (attempt ContextTracerAttempt, retErr error) {
	attempt.Settings = input.Settings
	if input.HonoredSkip || input.Settings.Mode == "" || input.Settings.Mode == "off" {
		attempt.Skipped = true
		return attempt, nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	// Nest official fact loading and Reserve in one span.
	ctx, span := tracer.Start(ctx, "command.admit_tracer_context")
	defer span.End()
	defer func() { recordTracerCoordinationError(span, retErr) }()

	if err := ctx.Err(); err != nil {
		return attempt, err
	}

	if (input.Settings.Mode != "enforce" && input.Settings.Mode != "advisory") || input.Settings.TimeoutMs < mmodel.TracerTimeoutMsMin || input.Settings.TimeoutMs > mmodel.TracerTimeoutMsMax {
		return attempt, constant.ErrInvalidRequestBody
	}

	budget := min(time.Duration(input.Settings.TimeoutMs)*time.Millisecond, c.config.AdmissionTimeout)

	request, err := c.requestWithLocalDeadline(ctx, input, budget)
	if err != nil {
		return attempt, err
	}

	ctx, cancelAdmission := context.WithTimeout(ctx, budget)
	defer cancelAdmission()

	attempt.Dispatched = true

	result, err := c.client.Reserve(ctx, request)
	if err != nil {
		attempt.Unavailable = tracerAdmissionUnavailable(err)

		return attempt, fmt.Errorf("reserve tracer context: %w", err)
	}

	if result == nil {
		return attempt, constant.ErrTracerContractUnavailable
	}

	if err := result.ValidateFor(request, c.config.MaxReservations); err != nil {
		return attempt, err
	}

	attempt.Result = result

	return attempt, nil
}

func (c *ContextTracerCoordinator) requestWithLocalDeadline(ctx context.Context, input ContextTracerInput, budget time.Duration) (tracercontract.ReserveRequest, error) {
	factsCtx, cancel := context.WithTimeout(ctx, budget)
	request, err := c.request(factsCtx, input, c.now().UTC())

	cancel()

	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return tracercontract.ReserveRequest{}, constant.ErrTracerFactsUnavailable
	}

	return request, err
}

func (c *ContextTracerCoordinator) request(ctx context.Context, input ContextTracerInput, admittedAt time.Time) (tracercontract.ReserveRequest, error) {
	amount, err := tracercontract.AmountFromDecimal(ctx, input.Amount, c.config.Bounds)
	if err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	facts, err := c.facts.EvaluationContext(ctx, input.Key.OrganizationID, input.Key.LedgerID, input.Entries)
	if err != nil {
		return tracercontract.ReserveRequest{}, fmt.Errorf("load tracer facts: %w", err)
	}

	// The reserved asset must be one the fee-inclusive postings actually move.
	if !slices.ContainsFunc(facts.Entries, func(entry tracercontract.Entry) bool { return entry.Asset == input.AssetCode }) {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	// Freshness describes this admission attempt. The caller's business date
	// remains on the Ledger transaction and must not bypass current controls.
	return tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: input.Key.TransactionID, RequestID: reservationRequestID(input.Key.TransactionID), ContextID: input.Key.LedgerID.String(), ValidationMode: tracercontract.ValidationMode(input.Settings.ValidationMode), TransactionTimestamp: admittedAt.UTC(), LongLived: &input.LongLived, Amount: amount, Asset: input.AssetCode, Context: facts}, nil
}
