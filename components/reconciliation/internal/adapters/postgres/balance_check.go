// Package postgres provides PostgreSQL database adapters for reconciliation checks.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Balance check threshold constants.
const (
	balanceWarningThreshold = 10
	percentageMultiplier    = 100
)

// BalanceChecker performs balance consistency checks
type BalanceChecker struct {
	db *sql.DB
}

// NewBalanceChecker creates a new balance checker
func NewBalanceChecker(db *sql.DB) *BalanceChecker {
	return &BalanceChecker{db: db}
}

// determineBalanceStatus determines the reconciliation status based on discrepancy count.
func determineBalanceStatus(discrepancyCount int) domain.ReconciliationStatus {
	switch {
	case discrepancyCount == 0:
		return domain.StatusHealthy
	case discrepancyCount <= balanceWarningThreshold:
		return domain.StatusWarning
	default:
		return domain.StatusCritical
	}
}

// Check verifies balance = sum(credits) - sum(debits)
func (c *BalanceChecker) Check(ctx context.Context, threshold int64, limit int) (*domain.BalanceCheckResult, error) {
	result := &domain.BalanceCheckResult{}

	// Summary query - using explicit DECIMAL cast for comparison
	summaryQuery := `
		WITH balance_calc AS (
			SELECT
				b.id,
				b.available::DECIMAL as current_balance,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
				COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND o.balance_affected AND t.status = 'PENDING' THEN o.amount ELSE 0 END), 0)::DECIMAL as total_on_hold,
				COUNT(o.id) as operation_count
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			LEFT JOIN transaction t ON o.transaction_id = t.id
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.available
		)
		SELECT
			COUNT(*) as total_balances,
			COUNT(*) FILTER (WHERE ABS(current_balance - (total_credits - total_debits - total_on_hold)) > $1) as discrepancies,
			COALESCE(SUM(ABS(current_balance - (total_credits - total_debits - total_on_hold)))
				FILTER (WHERE ABS(current_balance - (total_credits - total_debits - total_on_hold)) > $1), 0)::BIGINT as total_discrepancy
		FROM balance_calc
	`

	var totalDiscrepancy int64

	err := c.db.QueryRowContext(ctx, summaryQuery, threshold).Scan(
		&result.TotalBalances,
		&result.BalancesWithDiscrepancy,
		&totalDiscrepancy,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceSummaryQuery, err)
	}

	result.TotalAbsoluteDiscrepancy = totalDiscrepancy
	if result.TotalBalances > 0 {
		result.DiscrepancyPercentage = float64(result.BalancesWithDiscrepancy) / float64(result.TotalBalances) * percentageMultiplier
	}

	result.Status = determineBalanceStatus(result.BalancesWithDiscrepancy)

	// Get detailed discrepancies
	if result.BalancesWithDiscrepancy > 0 && limit > 0 {
		discrepancies, err := c.fetchBalanceDiscrepancies(ctx, threshold, limit)
		if err != nil {
			return nil, err
		}

		result.Discrepancies = discrepancies
	}

	return result, nil
}

// fetchBalanceDiscrepancies retrieves detailed balance discrepancy records.
func (c *BalanceChecker) fetchBalanceDiscrepancies(ctx context.Context, threshold int64, limit int) ([]domain.BalanceDiscrepancy, error) {
	detailQuery := `
		WITH balance_calc AS (
			SELECT
				b.id as balance_id,
				b.account_id,
				b.alias,
				b.asset_code,
				b.available::DECIMAL as current_balance,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
				COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND o.balance_affected AND t.status = 'PENDING' THEN o.amount ELSE 0 END), 0)::DECIMAL as total_on_hold,
				COUNT(o.id) as operation_count,
				b.updated_at
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			LEFT JOIN transaction t ON o.transaction_id = t.id
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.available, b.updated_at
		)
		SELECT
			balance_id, account_id, alias, asset_code,
			current_balance::BIGINT, (total_credits - total_debits - total_on_hold)::BIGINT as expected_balance,
			(current_balance - (total_credits - total_debits - total_on_hold))::BIGINT as discrepancy,
			operation_count, updated_at
		FROM balance_calc
		WHERE ABS(current_balance - (total_credits - total_debits - total_on_hold)) > $1
		ORDER BY ABS(current_balance - (total_credits - total_debits - total_on_hold)) DESC
		LIMIT $2
	`

	rows, err := c.db.QueryContext(ctx, detailQuery, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceDetailQuery, err)
	}
	defer rows.Close()

	var discrepancies []domain.BalanceDiscrepancy

	for rows.Next() {
		var d domain.BalanceDiscrepancy

		err := rows.Scan(
			&d.BalanceID, &d.AccountID, &d.Alias, &d.AssetCode,
			&d.CurrentBalance, &d.ExpectedBalance, &d.Discrepancy,
			&d.OperationCount, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBalanceRowScan, err)
		}

		discrepancies = append(discrepancies, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceRowIteration, err)
	}

	return discrepancies, nil
}
