package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// OrphanChecker finds transactions without operations
type OrphanChecker struct {
	db *sql.DB
}

// NewOrphanChecker creates a new orphan checker
func NewOrphanChecker(db *sql.DB) *OrphanChecker {
	return &OrphanChecker{db: db}
}

// Name returns the unique name of this checker.
func (c *OrphanChecker) Name() string {
	return CheckerNameOrphans
}

// Check finds transactions without operations
func (c *OrphanChecker) Check(ctx context.Context, config CheckerConfig) (CheckResult, error) {
	result := &domain.OrphanCheckResult{}

	// Summary query
	summaryQuery := `
		SELECT
			COUNT(*) FILTER (WHERE operation_count = 0) as orphan_transactions,
			COUNT(*) FILTER (WHERE operation_count = 1) as partial_transactions
		FROM (
			SELECT t.id, COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			  AND t.status NOT IN ('NOTED', 'PENDING')
			  AND COALESCE((t.body->>'pending')::boolean, false) = false
			GROUP BY t.id
		) sub
	`

	err := c.db.QueryRowContext(ctx, summaryQuery).Scan(
		&result.OrphanTransactions,
		&result.PartialTransactions,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOrphanSummaryQuery, err)
	}

	result.Status = DetermineStatusWithPartial(result.OrphanTransactions, result.PartialTransactions)

	// Get detailed orphans
	if (result.OrphanTransactions > 0 || result.PartialTransactions > 0) && config.MaxResults > 0 {
		orphans, err := c.fetchOrphanTransactions(ctx, config.MaxResults)
		if err != nil {
			return nil, err
		}

		result.Orphans = orphans
	}

	return result, nil
}

// fetchOrphanTransactions retrieves detailed orphan transaction records.
func (c *OrphanChecker) fetchOrphanTransactions(ctx context.Context, limit int) ([]domain.OrphanTransaction, error) {
	detailQuery := `
		SELECT
			t.id as transaction_id,
			t.organization_id::text,
			t.ledger_id::text,
			t.status,
			t.amount,
			t.asset_code,
			t.created_at,
			COUNT(o.id)::int as operation_count
		FROM transaction t
		LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
		WHERE t.deleted_at IS NULL
		  AND t.status NOT IN ('NOTED', 'PENDING')
		  AND COALESCE((t.body->>'pending')::boolean, false) = false
		GROUP BY t.id, t.organization_id, t.ledger_id, t.status, t.amount, t.asset_code, t.created_at
		HAVING COUNT(o.id) < 2
		ORDER BY t.created_at DESC
		LIMIT $1
	`

	rows, err := c.db.QueryContext(ctx, detailQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOrphanDetailQuery, err)
	}
	defer rows.Close()

	var orphans []domain.OrphanTransaction

	for rows.Next() {
		var o domain.OrphanTransaction

		err := rows.Scan(
			&o.TransactionID, &o.OrganizationID, &o.LedgerID,
			&o.Status, &o.Amount, &o.AssetCode, &o.CreatedAt, &o.OperationCount,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrOrphanRowScan, err)
		}

		orphans = append(orphans, o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOrphanRowIteration, err)
	}

	return orphans, nil
}
