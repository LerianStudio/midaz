// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

const (
	Source string = "midaz"

	// TransactionLifecyclePhaseCreated marks a freshly persisted
	// transaction (TransactionRepo.Create returned success). Emits
	// transaction.posted when ParentTransactionID is nil, otherwise
	// transaction.reverted.
	TransactionLifecyclePhaseCreated = "created"

	// TransactionLifecyclePhaseUpdated marks a status transition via
	// the unique-violation idempotency branch
	// (UpdateTransactionStatus). Emits transaction.committed when
	// Status.Code is APPROVED, transaction.canceled when CANCELED.
	TransactionLifecyclePhaseUpdated = "updated"

	// TransactionLifecyclePhaseNoop marks a code path that observed no
	// state change (e.g. a unique violation with no eligible status
	// transition). SendTransactionEvents emits no lifecycle event in
	// this phase.
	TransactionLifecyclePhaseNoop = "noop"
)

// SendTransactionEvents emits the post-commit lib-streaming lifecycle
// event for a persisted transaction state change.
//
// phase is the lifecycle phase returned by CreateOrUpdateTransaction
// (TransactionLifecyclePhaseCreated / TransactionLifecyclePhaseUpdated /
// TransactionLifecyclePhaseNoop). The emission picks posted vs reverted
// vs committed vs canceled from phase + status + parent. Callers that
// don't have a phase tracked (e.g. the bulk path at
// create_bulk_transaction_operations_async.go:555 which only does fresh
// inserts) pass TransactionLifecyclePhaseCreated explicitly.
func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction, phase string) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctxSendTransactionEvents, spanTransactionEvents := tracer.Start(ctx, "command.send_transaction_events_async")
	defer spanTransactionEvents.End()

	uc.emitTransactionLifecycleEvent(ctxSendTransactionEvents, spanTransactionEvents, logger, tran, phase)
}

// emitTransactionLifecycleEvent publishes one of the four
// transaction.{posted,committed,canceled,reverted} lib-streaming events
// based on the (phase, status, parent) discriminator triple.
//
// IMPORTANT posture (catalog says CRITICAL with outbox: always, but the
// outbox subsystem is not yet wired in midaz — see handoff). Build and
// emit failures are span-recorded and logged at Warn, never returned to
// the caller; durability of these events is owned by PG + (follow-up
// task) the outbox subsystem, not by this synchronous Emit call.
//
// Discriminator table:
//
//	┌────────────────┬─────────────────┬──────────────┬───────────────────────┐
//	│ phase          │ ParentTxID      │ Status.Code  │ Definition            │
//	├────────────────┼─────────────────┼──────────────┼───────────────────────┤
//	│ created        │ nil             │ APPROVED     │ transaction.posted    │
//	│ created        │ non-nil         │ APPROVED     │ transaction.reverted  │
//	│ created        │ ignored         │ PENDING      │ skipped (pre-commit)  │
//	│ created        │ ignored         │ NOTED        │ skipped (annotation)  │
//	│ created        │ ignored         │ other        │ skipped (defensive)   │
//	│ updated        │ ignored         │ APPROVED     │ transaction.committed │
//	│ updated        │ ignored         │ CANCELED     │ transaction.canceled  │
//	│ updated        │ ignored         │ other        │ skipped (defensive)   │
//	│ noop / unknown │ ignored         │ ignored      │ skipped               │
//	└────────────────┴─────────────────┴──────────────┴───────────────────────┘
//
// Status-gate rationale (created phase):
//   - APPROVED is the only status broadcast on fresh insert. The
//     CREATED-input branch promotes to APPROVED at L181-188 of
//     CreateOrUpdateTransaction — that's the canonical posted path.
//     The revert flow also creates a child transaction in APPROVED.
//   - PENDING is a pre-commit state. No business fact has occurred yet
//     (no balance movement, no settlement) — the broadcast happens later
//     via transaction.committed or transaction.canceled.
//   - NOTED is annotation-only (no balance impact, no operations); not
//     a broadcastable business fact.
//   - Other statuses (CANCELED on a fresh insert, etc.) are defensive
//     skips — they shouldn't occur on the fresh-insert path but if they
//     do, we don't fabricate a posted/reverted event for them.
//
// Wire-format mapping lives in pkg/streaming/events/transaction_lifecycle.go;
// changes to the payload contract belong there, not here.
func (uc *UseCase) emitTransactionLifecycleEvent(ctx context.Context, span trace.Span, logger libLog.Logger, tran *transaction.Transaction, phase string) {
	if tran == nil {
		return
	}

	src, err := buildTransactionEventSource(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build transaction event source", err)
		logger.Log(ctx, libLog.LevelWarn, "Skipping transaction lifecycle emit; source build failed",
			libLog.String("phase", phase),
			libLog.Err(err))

		return
	}

	var (
		definitionKey string
		buildFn       func(string) (libStreaming.EmitRequest, error)
		posted        bool
	)

	switch phase {
	case TransactionLifecyclePhaseCreated:
		// Gate on status=APPROVED. PENDING transactions await /commit
		// or /cancel before broadcasting; NOTED is excluded by scope
		// fence (see docstring above).
		if tran.Status.Code != constant.APPROVED {
			return
		}

		if tran.ParentTransactionID != nil && *tran.ParentTransactionID != "" {
			definitionKey = events.TransactionRevertedDefinition.Key()
			buildFn = func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewTransactionReverted(src).ToEmitRequestReverted(tenantID, time.Now())
			}
		} else {
			definitionKey = events.TransactionPostedDefinition.Key()
			buildFn = func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewTransactionPosted(src).ToEmitRequestPosted(tenantID, time.Now())
			}
			posted = true
		}
	case TransactionLifecyclePhaseUpdated:
		switch tran.Status.Code {
		case constant.APPROVED:
			definitionKey = events.TransactionCommittedDefinition.Key()
			buildFn = func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewTransactionCommitted(src).ToEmitRequestCommitted(tenantID, time.Now())
			}
		case constant.CANCELED:
			definitionKey = events.TransactionCanceledDefinition.Key()
			buildFn = func(tenantID string) (libStreaming.EmitRequest, error) {
				return events.NewTransactionCanceled(src).ToEmitRequestCanceled(tenantID, time.Now())
			}
		default:
			logger.Log(ctx, libLog.LevelDebug, "Skipping transaction lifecycle emit; updated phase with non-terminal status",
				libLog.String("status", tran.Status.Code),
				libLog.String("phase", phase))

			return
		}
	default:
		// TransactionLifecyclePhaseNoop or unrecognised phase. Nothing
		// to emit — the caller observed no eligible state change.
		return
	}

	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, definitionKey, buildFn)

	// fee-charge.applied rides alongside transaction.posted only. Commit/cancel/
	// revert do NOT re-emit it (the fee charge happened once, at post).
	if posted {
		uc.emitFeesAppliedEvent(ctx, span, logger, tran)
	}
}

// emitFeesAppliedEvent emits fee-charge.applied for a posted transaction that
// actually charged a fee. It fires only when feeApplied=true and a
// packageAppliedID are present in metadata (charged-only, set by the fee
// engine on the real-charge branch); pure exemptions still carry
// packageAppliedID but omit feeApplied=true, so the feeApplied guard suppresses
// the emit. IMPORTANT posture: EmitImportant swallows build/emit failures.
func (uc *UseCase) emitFeesAppliedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, tran *transaction.Transaction) {
	if applied, _ := tran.Metadata["feeApplied"].(string); applied != "true" {
		return
	}

	packageID, _ := tran.Metadata["packageAppliedID"].(string)
	if packageID == "" {
		return
	}

	appliedAt := tran.CreatedAt

	pkgStreaming.EmitImportant(ctx, span, logger, uc.Streaming, events.FeesAppliedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewFeesApplied(tran.ID, tran.OrganizationID, tran.LedgerID, packageID, appliedAt).
				ToEmitRequest(tenantID, appliedAt)
		})
}

// buildTransactionEventSource maps a persisted Transaction into the
// wire-decoupled TransactionSource consumed by the events package
// constructors. The mapping does the one heavy lift the events package
// cannot do for itself: marshaling each *operation.Operation into
// json.RawMessage so the events package stays decoupled from the
// internal/ domain operation type.
//
// Returns the assembled source plus a build error if any operation
// fails to marshal. The caller (emitTransactionLifecycleEvent) treats
// a non-nil error as a skip — the lifecycle event is not emitted, but
// the calling request continues.
func buildTransactionEventSource(tran *transaction.Transaction) (events.TransactionSource, error) {
	operationsRaw := make([]json.RawMessage, 0, len(tran.Operations))

	for i, op := range tran.Operations {
		if op == nil {
			continue
		}

		raw, err := json.Marshal(op)
		if err != nil {
			return events.TransactionSource{}, fmt.Errorf("marshal operation[%d]: %w", i, err)
		}

		operationsRaw = append(operationsRaw, raw)
	}

	// Status from the postgres adapter shares the same JSON tags as the
	// public mmodel.Status (code + description). The conversion is a
	// field-by-field copy rather than a struct cast because the two
	// types live in different packages and Go's structural typing does
	// not allow direct conversion across package boundaries.
	status := mmodel.Status{
		Code:        tran.Status.Code,
		Description: tran.Status.Description,
	}

	return events.TransactionSource{
		ID:                       tran.ID,
		ParentTransactionID:      tran.ParentTransactionID,
		OrganizationID:           tran.OrganizationID,
		LedgerID:                 tran.LedgerID,
		Status:                   status,
		Amount:                   tran.Amount,
		AssetCode:                tran.AssetCode,
		ChartOfAccountsGroupName: tran.ChartOfAccountsGroupName,
		Description:              tran.Description,
		Source:                   tran.Source,
		Destination:              tran.Destination,
		Route:                    tran.Route, //nolint:staticcheck // deprecated field kept for backward compatibility; RouteID is canonical
		RouteID:                  tran.RouteID,
		Operations:               operationsRaw,
		Metadata:                 tran.Metadata,
		FeesSkipped:              tran.FeesSkipped,
		TracerSkipped:            tran.TracerSkipped,
		CreatedAt:                tran.CreatedAt,
		UpdatedAt:                tran.UpdatedAt,
	}, nil
}
