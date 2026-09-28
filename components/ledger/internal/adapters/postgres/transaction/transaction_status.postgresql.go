// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// applyTransactionMutableSets adds the conditional SET clauses a transaction
// write shares — the deliberate body NULL, the description and the status pair —
// to a squirrel UPDATE builder.
//
// The body is nulled only on a status-carrying update: nulling it is the
// terminal transition's doing, so a write that names no status is a field patch
// and leaves the body alone. A caller that always carries a status therefore
// gets the null whenever the entity's body is empty.
func applyTransactionMutableSets(qb squirrel.UpdateBuilder, entity *Transaction, record *TransactionPostgreSQLModel) squirrel.UpdateBuilder {
	carriesStatus := !entity.Status.IsEmpty()

	if carriesStatus && entity.Body.IsEmpty() {
		qb = qb.Set("body", nil)
	}

	if entity.Description != "" {
		qb = qb.Set("description", record.Description)
	}

	if carriesStatus {
		qb = qb.Set("status", record.Status).Set("status_description", record.StatusDescription)
	}

	return qb
}

func (r *TransactionPostgreSQLRepository) UpdateStatusFromPending(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction_status_from_pending")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_id", id.String()),
	)

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, false, err
	}

	record := &TransactionPostgreSQLModel{}
	record.FromEntity(transaction)
	record.UpdatedAt = time.Now()

	qb := squirrel.Update(r.tableName).
		Set("updated_at", record.UpdatedAt).
		Where(squirrel.Eq{
			"organization_id": organizationID,
			"ledger_id":       ledgerID,
			"id":              id,
			"deleted_at":      nil,
			"status":          constant.PENDING,
		}).
		PlaceholderFormat(squirrel.Dollar)

	qb = applyTransactionMutableSets(qb, transaction, record)

	query, args, err := qb.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, false, err
	}

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute query", err)

		return nil, false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get rows affected", err)

		return nil, false, err
	}

	span.SetAttributes(attribute.Int64("db.rows_affected", rowsAffected))

	if rowsAffected == 0 {
		return nil, false, nil
	}

	return record.ToEntity(), true, nil
}
