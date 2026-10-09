// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package systemfacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/LerianStudio/lib-system-facts/emitter"
	"github.com/LerianStudio/lib-system-facts/outbox"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

// ErrTransactionFactAlreadyRecorded reports a transaction whose fact the kit
// already holds: an earlier attempt recorded it. The fact id is derived from
// the transaction id, so the answer is the same on every retry; the caller
// rolls back and treats the completion as already done.
var ErrTransactionFactAlreadyRecorded = errors.New("systemfacts: transaction fact already recorded")

// Recorder records the midaz.transaction_committed fact of a completion write
// set through the System Facts kit.
type Recorder struct {
	facts  emitter.Emitter
	logger libLog.Logger
}

// NewRecorder returns a Recorder emitting through facts.
func NewRecorder(facts emitter.Emitter, logger libLog.Logger) (*Recorder, error) {
	if facts == nil {
		return nil, errors.New("systemfacts: nil emitter")
	}

	if logger == nil {
		return nil, errors.New("systemfacts: nil logger")
	}

	return &Recorder{facts: facts, logger: logger}, nil
}

// RecordTransactionCommitted derives the fact of record and records it in tx,
// the completion's SQL transaction, so the fact commits or rolls back with the
// rows. A write set that is not a fact records nothing. A fact the kit refuses
// is lost: the business write commits without it, logged at Warn with the
// transaction id so it can be rebuilt. ErrTransactionFactAlreadyRecorded means
// an earlier attempt recorded it; any other error is the kit's or the driver's
// and the caller rolls back.
func (r *Recorder) RecordTransactionCommitted(ctx context.Context, tx *sql.Tx, record command.TransactionWriteSet) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "systemfacts.record_transaction_committed")
	defer span.End()

	if record.Transaction != nil {
		span.SetAttributes(attribute.String("app.request.transaction_id", record.Transaction.ID))
	}

	payload, isFact := NewTransactionCommitted(record)
	if !isFact {
		return nil
	}

	span.SetAttributes(attribute.String("app.systemfacts.fact_type", transactionCommittedFactType))

	transactionID, err := uuid.Parse(record.Transaction.ID)
	if err != nil {
		err = fmt.Errorf("systemfacts: transaction id is not a UUID: %w", err)
		libOpentelemetry.HandleSpanError(span, "Transaction id is not a UUID", err)

		return err
	}

	if _, err := r.facts.Emit(ctx, tx, newTransactionCommittedFact(transactionID, record, payload)); err != nil {
		return r.handleEmitError(ctx, span, transactionID, err)
	}

	return nil
}

// newTransactionCommittedFact assembles the fact: the transaction is the
// subject, occurredAt is the updated_at the completion writes, and the one
// reference is the transaction id scoped by organization and ledger.
func newTransactionCommittedFact(transactionID uuid.UUID, record command.TransactionWriteSet, payload TransactionCommittedPayload) emitter.Fact {
	return emitter.Fact{
		Type:          transactionCommittedFactType,
		SchemaVersion: transactionCommittedSchemaVersion,
		FactID:        TransactionCommittedFactID(transactionID),
		OccurredAt:    record.Transaction.UpdatedAt.UTC(),
		Subject:       emitter.Subject{Type: transactionSubjectType, ID: transactionID.String()},
		References: []emitter.Reference{{
			Type:  transactionIDReferenceType,
			Value: transactionID.String(),
			Scope: map[string]string{
				scopeKeyOrganizationID: record.Transaction.OrganizationID,
				scopeKeyLedgerID:       record.Transaction.LedgerID,
			},
		}},
		Data: payload,
	}
}

// handleEmitError classifies an Emit error the way the kit documents: a fact
// already recorded is the caller's idempotent replay; a refusal is a
// producer-side defect, technical on the span but never a failed write; any
// other error is the kit's or the driver's and propagates.
func (r *Recorder) handleEmitError(ctx context.Context, span trace.Span, transactionID uuid.UUID, err error) error {
	if errors.Is(err, outbox.ErrFactAlreadyRecorded) {
		wrapped := fmt.Errorf("%w: transaction %s: %w", ErrTransactionFactAlreadyRecorded, transactionID, err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "System fact already recorded", wrapped)

		return wrapped
	}

	if reason, refused := emitter.IsValidationRefusal(err); refused {
		libOpentelemetry.HandleSpanError(span, "System fact refused", err)
		r.logger.Log(ctx, libLog.LevelWarn, "System fact refused",
			libLog.String("reason", string(reason)),
			libLog.String("transaction_id", transactionID.String()),
			libLog.Err(err))

		return nil
	}

	wrapped := fmt.Errorf("systemfacts: record transaction fact: %w", err)
	libOpentelemetry.HandleSpanError(span, "Failed to record system fact", wrapped)

	return wrapped
}
