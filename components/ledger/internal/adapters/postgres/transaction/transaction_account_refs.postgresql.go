// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// AccountRefs is everything the persisted record says about which accounts a
// transaction touches: the account of every live operation row, and the body
// the transaction was submitted with.
//
// Both halves are needed. A hold writes rows for its source side only, so the
// destination of a pending transaction is named nowhere but in the body; and a
// transaction recorded without a body is named only by its rows.
type AccountRefs struct {
	AccountIDs []uuid.UUID
	Body       mtransaction.Transaction
}

// accountRefsQuery reads, in one statement, the transaction row and the distinct
// accounts of its live operation rows. The LEFT JOIN keeps the transaction when
// it has no rows yet, so "found without rows" stays distinct from "not found".
func accountRefsQuery(transactionTable string, organizationID, ledgerID, transactionID uuid.UUID) squirrel.SelectBuilder {
	return squirrel.Select(
		"t.body",
		"COALESCE(array_agg(DISTINCT o.account_id::text) FILTER (WHERE o.account_id IS NOT NULL), '{}')",
	).
		From(transactionTable + " t").
		LeftJoin("operation o ON o.transaction_id = t.id AND o.organization_id = t.organization_id AND o.ledger_id = t.ledger_id AND o.deleted_at IS NULL").
		Where(squirrel.Expr("t.organization_id = ?", organizationID)).
		Where(squirrel.Expr("t.ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("t.id = ?", transactionID)).
		Where(squirrel.Eq{"t.deleted_at": nil}).
		GroupBy("t.id").
		PlaceholderFormat(squirrel.Dollar)
}

// ListAccountRefsByTransaction returns the accounts a live transaction of the
// scope touches. An absent or soft-deleted transaction answers the
// entity-not-found business error; a transaction whose operation rows are not
// projected yet answers an empty AccountIDs.
func (r *TransactionPostgreSQLRepository) ListAccountRefsByTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*AccountRefs, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_account_refs_by_transaction")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.transaction_id", transactionID.String()),
	)

	db, release, err := r.acquireRead(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}
	defer releaseRead(span, release)

	query, args, err := accountRefsQuery(r.tableName, organizationID, ledgerID, transactionID).ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	var (
		body       sql.NullString
		accountIDs pq.StringArray
	)

	err = db.QueryRowContext(ctx, query, args...).Scan(&body, &accountIDs)
	if errors.Is(err, sql.ErrNoRows) {
		notFound := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction not found", notFound)

		return nil, notFound
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute account refs query", err)

		return nil, err
	}

	refs := &AccountRefs{AccountIDs: make([]uuid.UUID, 0, len(accountIDs))}

	for _, raw := range accountIDs {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			err := fmt.Errorf("operation account id %q is not a uuid: %w", raw, parseErr)

			libOpentelemetry.HandleSpanError(span, "Failed to parse operation account id", err)

			return nil, err
		}

		refs.AccountIDs = append(refs.AccountIDs, id)
	}

	if body.Valid && body.String != "" {
		if err := json.Unmarshal([]byte(body.String), &refs.Body); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to unmarshal body", err)

			return nil, err
		}
	}

	span.SetAttributes(attribute.Int("app.transaction.account_refs", len(refs.AccountIDs)))

	return refs, nil
}
