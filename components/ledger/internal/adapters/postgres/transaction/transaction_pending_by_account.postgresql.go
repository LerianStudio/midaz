// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// pendingByAccountQuery builds the existence query for a PENDING transaction the
// account participates in, on either side.
//
// Participation is read from the operation rows, which is where an account meets a
// transaction: one row per leg, carrying the account it moved and the direction it
// moved in, so source and destination are both covered without the query having to
// name a side. The scope is complete on both tables — organization, ledger and the
// account identifier — and both soft-delete filters are kept, so the operation
// predicate matches idx_operation_account_id
// (organization_id, ledger_id, account_id, id) WHERE deleted_at IS NULL and the
// transaction side is reached by its primary key.
//
// LIMIT 1 is what makes it an existence check: the closing needs to know that a
// pending transaction is there, never how many.
func pendingByAccountQuery(transactionTable string, organizationID, ledgerID, accountID uuid.UUID) squirrel.SelectBuilder {
	return squirrel.Select("1").
		From("operation o").
		InnerJoin(transactionTable + " t ON t.id = o.transaction_id AND t.organization_id = o.organization_id AND t.ledger_id = o.ledger_id").
		Where(squirrel.Expr("o.organization_id = ?", organizationID)).
		Where(squirrel.Expr("o.ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("o.account_id = ?", accountID)).
		Where(squirrel.Eq{"o.deleted_at": nil}).
		Where(squirrel.Eq{"t.status": constant.PENDING}).
		Where(squirrel.Eq{"t.deleted_at": nil}).
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)
}

// HasPendingByAccount reports whether a PENDING transaction involves the account.
//
// A false answer says only that no pending transaction is VISIBLE in PostgreSQL:
// a pending execution whose rows are still being projected is invisible here and
// is answered by the completion evidence instead, not by this query.
func (r *TransactionPostgreSQLRepository) HasPendingByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.has_pending_transaction_by_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.account_id", accountID.String()),
	)

	db, release, err := r.acquireRead(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return false, err
	}
	defer releaseRead(span, release)

	query, args, err := pendingByAccountQuery(r.tableName, organizationID, ledgerID, accountID).ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return false, err
	}

	var exists int

	err = db.QueryRowContext(ctx, query, args...).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute pending transaction query", err)

		return false, err
	}

	span.SetAttributes(attribute.Bool("app.account_closing.pending_transaction_found", true))

	return true, nil
}
