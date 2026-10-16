// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	pgoperation "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// balanceTrailStatementTimeout bounds one page read so a slow page fails instead of
// holding a primary connection open.
const balanceTrailStatementTimeout = "SET LOCAL statement_timeout = '30s'"

// balanceTrailPairsQuery pairs each non-deleted balance after the cursor with the head of
// its trail: the balance-affecting operation with the highest balance_version_after.
// The version leads the ORDER BY because it is the balance's monotonic clock; created_at
// is stamped before the version is assigned and can invert between two operations, so it
// and id only break a tie. The lateral lookup is served by idx_operation_balance_version.
//
// The head reports when it was recorded on the PIT axis (recorded_at, falling back to
// created_at for legacy rows) and whether it is the CREDIT row of a cancelled pending
// transaction. operation.status is not a reliable discriminator for that row, so the
// transaction is joined after the lateral has already chosen the head: the join only
// annotates the chosen row and cannot change which row is chosen.
const balanceTrailPairsQuery = `
SELECT b.id, b.organization_id, b.ledger_id, b.account_id,
       b.version, b.available, b.on_hold, b.updated_at,
       o.balance_version_after, o.available_balance_after, o.on_hold_balance_after, o.recorded_at,
       COALESCE(o.type = '` + constant.CREDIT + `' AND t.status = '` + constant.CANCELED + `', false)
FROM balance b
LEFT JOIN LATERAL (
  SELECT balance_version_after, available_balance_after, on_hold_balance_after,
         ` + pgoperation.PointInTimeRecordedAtExpression + ` AS recorded_at, type, transaction_id
  FROM operation o
  WHERE o.organization_id = b.organization_id AND o.ledger_id = b.ledger_id
    AND o.account_id = b.account_id AND o.balance_id = b.id
    AND o.deleted_at IS NULL AND o.balance_affected = true
  ORDER BY balance_version_after DESC, created_at DESC, id DESC
  LIMIT 1
) o ON true
LEFT JOIN "transaction" t ON t.id = o.transaction_id
WHERE b.deleted_at IS NULL AND b.id > $1
ORDER BY b.id
LIMIT $2`

// ListBalanceTrailPairs returns up to pageSize non-deleted balances with id > cursor,
// ordered by id, each paired with the head of its trail. next is the last id of the
// page; a page shorter than pageSize means the table ended. The read runs in one
// read-only transaction on the primary.
//
// The primary is mandatory: a replica lags both tables independently, so a pair read
// there can show a divergence that never existed on the primary, or hide one that does.
func (r *BalancePostgreSQLRepository) ListBalanceTrailPairs(ctx context.Context, cursor uuid.UUID, pageSize int) (pairs []mmodel.BalanceTrailPair, next uuid.UUID, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.list_balance_trail_pairs")
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)
		return nil, cursor, err
	}

	// A read-only transaction is what makes dbresolver target the primary. There is no
	// replica fallback: a failure to open it fails the page.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		err = fmt.Errorf("open read-only primary transaction for balance trail pairs: %w", err)
		libOpentelemetry.HandleSpanError(span, "Failed to open read-only transaction", err)

		return nil, cursor, err
	}

	defer func() {
		// A failed read rolls the transaction back; a rollback that itself fails joins
		// the original error so the caller still sees it.
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				err = errors.Join(err, fmt.Errorf("rollback read-only primary transaction for balance trail pairs: %w", rollbackErr))
			}

			libOpentelemetry.HandleSpanError(span, "Failed to list balance trail pairs", err)

			return
		}

		if commitErr := tx.Commit(); commitErr != nil {
			err = fmt.Errorf("close read-only primary transaction for balance trail pairs: %w", commitErr)
			libOpentelemetry.HandleSpanError(span, "Failed to close read-only transaction", err)
		}
	}()

	if _, err = tx.ExecContext(ctx, balanceTrailStatementTimeout); err != nil {
		return nil, cursor, fmt.Errorf("set statement timeout for balance trail pairs: %w", err)
	}

	rows, err := tx.QueryContext(ctx, balanceTrailPairsQuery, cursor, pageSize)
	if err != nil {
		return nil, cursor, fmt.Errorf("query balance trail pairs: %w", err)
	}

	defer rows.Close()

	// The caller clamps pageSize; max only keeps a bad value from panicking here, the
	// query itself already rejects a negative LIMIT.
	pairs = make([]mmodel.BalanceTrailPair, 0, max(pageSize, 0))

	for rows.Next() {
		pair, scanErr := scanBalanceTrailPair(rows)
		if scanErr != nil {
			return nil, cursor, fmt.Errorf("scan balance trail pair: %w", scanErr)
		}

		pairs = append(pairs, pair)
	}

	if err = rows.Err(); err != nil {
		return nil, cursor, fmt.Errorf("iterate balance trail pairs: %w", err)
	}

	next = cursor
	if len(pairs) > 0 {
		next = pairs[len(pairs)-1].BalanceID
	}

	logger.Log(
		ctx, libLog.LevelDebug, "balance trail page read",
		libLog.String("cursor", cursor.String()),
		libLog.Int("page_size", pageSize),
		libLog.Int("pairs", len(pairs)),
	)

	return pairs, next, nil
}

// scanBalanceTrailPair reads one row of balanceTrailPairsQuery. The operation columns
// are all NULL when the balance has no balance-affecting operation; Trail is then nil.
func scanBalanceTrailPair(rows *sql.Rows) (mmodel.BalanceTrailPair, error) {
	var (
		pair           mmodel.BalanceTrailPair
		trailVersion   sql.NullInt64
		trailAvailable decimal.NullDecimal
		trailOnHold    decimal.NullDecimal
		trailRecorded  sql.NullTime
		cancelCredit   bool
	)

	if err := rows.Scan(
		&pair.BalanceID,
		&pair.OrganizationID,
		&pair.LedgerID,
		&pair.AccountID,
		&pair.Version,
		&pair.Available,
		&pair.OnHold,
		&pair.UpdatedAt,
		&trailVersion,
		&trailAvailable,
		&trailOnHold,
		&trailRecorded,
		&cancelCredit,
	); err != nil {
		return mmodel.BalanceTrailPair{}, err
	}

	if trailVersion.Valid {
		pair.Trail = &mmodel.BalanceTrailHead{
			Version:               trailVersion.Int64,
			Available:             trailAvailable.Decimal,
			OnHold:                trailOnHold.Decimal,
			RecordedAt:            trailRecorded.Time,
			ValidatedCancelCredit: cancelCredit,
		}
	}

	return pair, nil
}
