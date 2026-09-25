// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

type ContextTracerInput struct {
	Key         tracerreservation.Key
	ExecutionID uuid.UUID
	Settings    mmodel.TracerSettings
	Timestamp   time.Time
	Amount      decimal.Decimal
	AssetCode   string
	Entries     []traceradapter.PreparedEntry
	HonoredSkip bool
	LongLived   bool
	// DispatchDeadline shares a preparation window across an atomic batch.
	// It is internal producer input, never copied from an HTTP request field.
	DispatchDeadline time.Time
}

// ContextTracerAttempt distinguishes a failed preflight from an uncertain
// durable write. An uncertain write must never authorize accounting dispatch.
type ContextTracerAttempt struct {
	Key             tracerreservation.Key
	Skipped         bool
	IntentAttempted bool
	Frozen          bool
	Result          *tracercontract.ReserveResult
}

// Admit freezes official facts before calling Tracer. The caller interprets the
// returned decision according to Ledger mode; Tracer alone evaluates policies.
func (c *ContextTracerCoordinator) Admit(ctx context.Context, input ContextTracerInput) (attempt ContextTracerAttempt, retErr error) {
	attempt.Key = input.Key
	if input.HonoredSkip || input.Settings.Mode == "" || input.Settings.Mode == "off" {
		attempt.Skipped = true
		return attempt, nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	// Nest official fact loading, durable admission and Reserve in one span.
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

	created := c.recovery.now().UTC()
	request.TransactionTimestamp = created

	scope := tracercontract.ReserveScope{TenantID: tmcore.GetTenantIDContext(ctx), IntegrationID: c.recovery.config.IntegrationID, AssetNamespace: c.recovery.config.Namespace, SingleTenant: c.recovery.config.SingleTenant}
	// The dispatch grace is bounded independently of Reserve. This permits the
	// configured fail-open path to acquire its fence after a Reserve timeout.
	deadline := input.DispatchDeadline
	if deadline.IsZero() {
		deadline = created.Add(budget).Add(c.recovery.config.AttemptTimeout)
	}

	intent, err := tracerreservation.NewIntent(ctx, input.Key, input.ExecutionID, scope, request, created, deadline, c.config.Facts)
	if err != nil {
		return attempt, err
	}

	attempt.IntentAttempted = true

	record, err := c.recovery.store.Prepare(ctx, intent)
	if err != nil {
		return attempt, fmt.Errorf("persist tracer intent: %w", err)
	}

	if !matchesPreparedTracerIntent(record, intent) {
		return attempt, constant.ErrTracerContractUnavailable
	}

	request, err = record.Intent.Request(ctx, c.config.Facts)
	if err != nil {
		return attempt, err
	}

	attempt.Frozen = true

	result, err := c.recovery.client.Reserve(ctx, request)
	if err != nil {
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
	request, err := c.request(factsCtx, input, c.recovery.now().UTC())

	cancel()

	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return tracercontract.ReserveRequest{}, constant.ErrTracerFactsUnavailable
	}

	return request, err
}

func (c *ContextTracerCoordinator) request(ctx context.Context, input ContextTracerInput, admittedAt time.Time) (tracercontract.ReserveRequest, error) {
	amount, err := tracercontract.AmountFromDecimal(ctx, input.Amount, c.config.Facts.Bounds)
	if err != nil {
		return tracercontract.ReserveRequest{}, err
	}

	facts, err := c.facts.EvaluationContext(ctx, input.Key.OrganizationID, input.Key.LedgerID, input.Entries)
	if err != nil {
		return tracercontract.ReserveRequest{}, fmt.Errorf("load tracer facts: %w", err)
	}

	var asset tracercontract.AssetRef

	for _, entry := range facts.Entries {
		if entry.Asset.Code != input.AssetCode {
			continue
		}

		if asset.ID != "" && asset != entry.Asset {
			return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
		}

		asset = entry.Asset
	}

	if asset.ID == "" {
		return tracercontract.ReserveRequest{}, constant.ErrInvalidRequestBody
	}

	// Freshness describes this admission attempt. The caller's business date
	// remains on the Ledger transaction and must not bypass current controls.
	return tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: input.Key.TransactionID, RequestID: reservationRequestID(input.Key.TransactionID), ContextID: input.Key.LedgerID.String(), ValidationMode: tracercontract.ValidationMode(input.Settings.ValidationMode), TransactionTimestamp: admittedAt.UTC(), LongLived: &input.LongLived, Amount: amount, Asset: asset, Context: facts}, nil
}

// A durable acknowledgement must name the same execution and frozen facts.
func matchesPreparedTracerIntent(record *tracerreservation.Record, intent tracerreservation.Intent) bool {
	return record != nil && record.State == tracerreservation.Prepared && record.Intent.ExecutionID == intent.ExecutionID && record.Intent.Key == intent.Key && record.Intent.Scope == intent.Scope && record.Intent.Fingerprint == intent.Fingerprint
}
