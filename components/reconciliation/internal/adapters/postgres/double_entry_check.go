package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// DoubleEntryChecker validates credits = debits for transactions
type DoubleEntryChecker struct {
	db *sql.DB
}

// NewDoubleEntryChecker creates a new double-entry checker
func NewDoubleEntryChecker(db *sql.DB) *DoubleEntryChecker {
	return &DoubleEntryChecker{db: db}
}

// Check verifies every transaction has balanced credits and debits
// NOTE: ALL transactions (including NOTED) must have balanced operations for audit integrity
func (c *DoubleEntryChecker) Check(ctx context.Context, limit int) (*domain.DoubleEntryCheckResult, error) {
	result := &domain.DoubleEntryCheckResult{}

	// Summary query - only exclude PENDING (no operations yet)
	// NOTED transactions must still balance for audit integrity
	summaryQuery := `
		WITH transaction_balance AS (
			SELECT
				t.id,
				t.status,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
				COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			  AND t.status != 'PENDING'
			GROUP BY t.id, t.status
		)
		SELECT
			COUNT(*) as total_transactions,
			COUNT(*) FILTER (WHERE total_credits != total_debits) as unbalanced,
			COUNT(*) FILTER (WHERE operation_count = 0) as no_operations
		FROM transaction_balance
	`

	err := c.db.QueryRowContext(ctx, summaryQuery).Scan(
		&result.TotalTransactions,
		&result.UnbalancedTransactions,
		&result.TransactionsNoOperations,
	)
	if err != nil {
		return nil, fmt.Errorf("double-entry summary query failed: %w", err)
	}

	if result.TotalTransactions > 0 {
		result.UnbalancedPercentage = float64(result.UnbalancedTransactions) / float64(result.TotalTransactions) * 100
	}

	// Status - unbalanced transactions are CRITICAL
	if result.UnbalancedTransactions == 0 {
		result.Status = domain.StatusHealthy
	} else {
		result.Status = domain.StatusCritical
	}

	// Get detailed imbalances
	if result.UnbalancedTransactions > 0 && limit > 0 {
		detailQuery := `
			WITH transaction_balance AS (
				SELECT
					t.id as transaction_id,
					t.status,
					t.asset_code,
					COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
					COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
					COUNT(o.id) as operation_count
				FROM transaction t
				LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
				WHERE t.deleted_at IS NULL
				  AND t.status != 'PENDING'
				GROUP BY t.id, t.status, t.asset_code
			)
			SELECT
				transaction_id, status, asset_code,
				total_credits, total_debits,
				(total_credits - total_debits) as imbalance,
				operation_count
			FROM transaction_balance
			WHERE total_credits != total_debits
			ORDER BY ABS(total_credits - total_debits) DESC
			LIMIT $1
		`

		rows, err := c.db.QueryContext(ctx, detailQuery, limit)
		if err != nil {
			return nil, fmt.Errorf("double-entry detail query failed: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var i domain.TransactionImbalance
			err := rows.Scan(
				&i.TransactionID, &i.Status, &i.AssetCode,
				&i.TotalCredits, &i.TotalDebits, &i.Imbalance, &i.OperationCount,
			)
			if err != nil {
				return nil, fmt.Errorf("double-entry row scan failed: %w", err)
			}
			result.Imbalances = append(result.Imbalances, i)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("double-entry row iteration failed: %w", err)
		}
	}

	return result, nil
}
