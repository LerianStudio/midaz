// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/scopefilter"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// holderScopeColumns are the columns the holders of an organization are
// confined on: a holder is in scope when it owns a live account in scope.
var holderScopeColumns = map[string]string{
	"ledgerId":  "ledger_id",
	"accountId": "id",
}

// ListHolderIDs implements Repository.
func (r *AccountPostgreSQLRepository) ListHolderIDs(ctx context.Context, organizationID uuid.UUID, scope http.ScopeConfinement) ([]uuid.UUID, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_holder_ids_in_scope")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.organization_id", organizationID.String()))

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	query, args, err := scopefilter.Where(squirrel.Select("DISTINCT "+holderIDColumn).
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.NotEq{holderIDColumn: nil}), scope, holderScopeColumns).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(span, "Failed to execute query", mapped)

		return nil, mapped
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)

	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			err = fmt.Errorf("scan holder id: %w", err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	span.SetAttributes(attribute.Int("db.rows_returned", len(ids)))

	return ids, nil
}

// ListLedgerIDsOfHolders implements Repository.
func (r *AccountPostgreSQLRepository) ListLedgerIDsOfHolders(ctx context.Context, organizationID uuid.UUID, holderIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_ledger_ids_of_holders")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.Int("app.request.holders", len(holderIDs)),
	)

	ledgers := make(map[uuid.UUID][]uuid.UUID, len(holderIDs))

	if len(holderIDs) == 0 {
		return ledgers, nil
	}

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	query, args, err := squirrel.Select("DISTINCT "+holderIDColumn, "ledger_id").
		From(r.tableName).
		Where(squirrel.Eq{"organization_id": organizationID}).
		Where(squirrel.Eq{"deleted_at": nil}).
		Where(squirrel.Expr(holderIDColumn+" = ANY(?)", pq.Array(holderIDs))).
		OrderBy(holderIDColumn, "ledger_id").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to build query", err)

		return nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		mapped := mapReadError(err)

		libOpentelemetry.HandleSpanError(span, "Failed to execute query", mapped)

		return nil, mapped
	}
	defer rows.Close()

	count := 0

	for rows.Next() {
		var holderID, ledgerID uuid.UUID
		if err := rows.Scan(&holderID, &ledgerID); err != nil {
			err = fmt.Errorf("scan holder ledger: %w", err)

			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)

			return nil, err
		}

		ledgers[holderID] = append(ledgers[holderID], ledgerID)
		count++
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)

		return nil, err
	}

	span.SetAttributes(attribute.Int("db.rows_returned", count))

	return ledgers, nil
}
