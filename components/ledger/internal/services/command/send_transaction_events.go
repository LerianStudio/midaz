// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	libObs "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"go.opentelemetry.io/otel/trace"
)

const (
	Source    string = "midaz"
	EventType string = "transaction"

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
	// transition). SendTransactionEvents emits NEITHER the lib-streaming
	// lifecycle event nor the legacy rabbit publish in this phase.
	TransactionLifecyclePhaseNoop = "noop"
)

// SendTransactionEvents publishes the post-commit notifications for a
// persisted transaction state change.
//
// During the lib-streaming cutover window this function emits BOTH the
// legacy transaction.transaction_events rabbit publish AND the new
// lib-streaming transaction.{posted,committed,canceled,reverted}
// CloudEvent. The two transports are independent: a rabbit failure does
// not block the lib-streaming emit and vice versa. The disabled flag
// RABBITMQ_TRANSACTION_EVENTS_ENABLED=false short-circuits BOTH
// transports together (per cutover discipline).
//
// phase is the lifecycle phase returned by CreateOrUpdateTransaction
// (TransactionLifecyclePhaseCreated / TransactionLifecyclePhaseUpdated /
// TransactionLifecyclePhaseNoop). The lib-streaming emission picks
// posted vs reverted vs committed vs canceled from phase + status +
// parent. Callers that don't have a phase tracked (e.g. the bulk path
// at create_bulk_transaction_operations_async.go:555 which only does
// fresh inserts) pass TransactionLifecyclePhaseCreated explicitly.
func (uc *UseCase) SendTransactionEvents(ctx context.Context, tran *transaction.Transaction, phase string) {
	logger, tracer, _, _ := libObs.NewTrackingFromContext(ctx)

	if !isTransactionEventEnabled() {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction event not enabled. RABBITMQ_TRANSACTION_EVENTS_ENABLED='%s'", os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
		return
	}

	ctxSendTransactionEvents, spanTransactionEvents := tracer.Start(ctx, "command.send_transaction_events_async")
	defer spanTransactionEvents.End()

	payload, err := json.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanTransactionEvents, "Failed to marshal transaction to JSON string", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to marshal transaction to JSON string: %s", err.Error()))
	}

	event := mmodel.Event{
		Source:         Source,
		EventType:      EventType,
		Action:         tran.Status.Code,
		TimeStamp:      time.Now(),
		Version:        os.Getenv("VERSION"),
		OrganizationID: tran.OrganizationID,
		LedgerID:       tran.LedgerID,
		Payload:        payload,
	}

	var key strings.Builder

	key.WriteString(Source)
	key.WriteString(".")
	key.WriteString(EventType)
	key.WriteString(".")
	key.WriteString(tran.Status.Code)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Sending transaction events to key: %s", key.String()))

	message, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanTransactionEvents, "Failed to marshal exchange message struct", err)

		logger.Log(ctx, libLog.LevelError, "Failed to marshal exchange message struct")
	}

	if _, err := uc.RabbitMQRepo.ProducerDefault(
		ctxSendTransactionEvents,
		os.Getenv("RABBITMQ_TRANSACTION_EVENTS_EXCHANGE"),
		key.String(),
		message,
	); err != nil {
		libOpentelemetry.HandleSpanError(spanTransactionEvents, "Failed to send transaction events to exchange", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to send message: %s", err.Error()))
	}

	// lib-streaming emission runs alongside the rabbit publish during
	// the cutover window. The phase parameter discriminates which of
	// the four lifecycle events to fire — see emitTransactionLifecycleEvent.
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
			logger.Log(ctx, libLog.LevelInfo, "Skipping transaction lifecycle emit; updated phase with non-terminal status",
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
}

// buildTransactionEventSource maps a persisted Transaction into the
// wire-decoupled TransactionSource consumed by the events package
// constructors. The mapping does the one heavy lift the events package
// cannot do for itself: marshaling each *operation.Operation into
// json.RawMessage so the events package stays decoupled from the
// internal/ domain operation type. The on-the-wire bytes match what the
// legacy transaction.transaction_events rabbit publish produces for the
// `operations` array — consumers migrating off the rabbit topic see no
// payload shape drift.
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
		Route:                    tran.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		RouteID:                  tran.RouteID,
		Operations:               operationsRaw,
		Metadata:                 tran.Metadata,
		CreatedAt:                tran.CreatedAt,
		UpdatedAt:                tran.UpdatedAt,
	}, nil
}

func isTransactionEventEnabled() bool {
	envValue := strings.ToLower(strings.TrimSpace(os.Getenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED")))
	return envValue != "false"
}
