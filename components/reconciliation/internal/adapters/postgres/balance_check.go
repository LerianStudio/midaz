// Package postgres provides PostgreSQL database adapters for reconciliation checks.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/shopspring/decimal"
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

// Name returns the unique name of this checker.
func (c *BalanceChecker) Name() string {
	return CheckerNameBalance
}

// Check verifies balance = sum(credits) - sum(debits)
func (c *BalanceChecker) Check(ctx context.Context, config CheckerConfig) (CheckResult, error) {
	result := &domain.BalanceCheckResult{}

	if err := c.fetchSummary(ctx, result, config.DiscrepancyThreshold); err != nil {
		return nil, err
	}

	c.calculatePercentages(result)
	c.determineInitialStatus(result)

	if err := c.fetchDetailedResults(ctx, result, config); err != nil {
		return nil, err
	}

	return result, nil
}

// fetchSummary executes the summary query and populates the result.
func (c *BalanceChecker) fetchSummary(ctx context.Context, result *domain.BalanceCheckResult, threshold int64) error {
	summaryQuery := `
		WITH balance_calc AS (
			SELECT
				b.id,
				b.account_type,
				b.available::DECIMAL as current_balance,
				b.on_hold::DECIMAL as current_on_hold,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
				COALESCE(SUM(CASE
					WHEN o.type = 'ON_HOLD' AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN o.amount
					WHEN o.type IN ('RELEASE', 'DEBIT') AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN -o.amount
					ELSE 0
				END), 0)::DECIMAL as expected_on_hold,
				COUNT(o.id) as operation_count
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			LEFT JOIN transaction t ON o.transaction_id = t.id
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.account_type, b.available, b.on_hold
		)
		SELECT
			COUNT(*) as total_balances,
			COUNT(*) FILTER (WHERE ABS(current_balance - (total_credits - total_debits - expected_on_hold)) > $1) as discrepancies,
			COALESCE(SUM(ABS(current_balance - (total_credits - total_debits - expected_on_hold)))
				FILTER (WHERE ABS(current_balance - (total_credits - total_debits - expected_on_hold)) > $1), 0)::DECIMAL as total_discrepancy,
			COUNT(*) FILTER (WHERE ABS(current_on_hold - expected_on_hold) > $1) as on_hold_discrepancies,
			COALESCE(SUM(ABS(current_on_hold - expected_on_hold))
				FILTER (WHERE ABS(current_on_hold - expected_on_hold) > $1), 0)::DECIMAL as total_on_hold_discrepancy,
			COUNT(*) FILTER (WHERE current_balance < 0 AND account_type <> 'external') as negative_available,
			COUNT(*) FILTER (WHERE current_on_hold < 0 AND account_type <> 'external') as negative_on_hold
		FROM balance_calc
	`

	var (
		totalDiscrepancy       decimal.Decimal
		totalOnHoldDiscrepancy decimal.Decimal
	)

	err := c.db.QueryRowContext(ctx, summaryQuery, threshold).Scan(
		&result.TotalBalances,
		&result.BalancesWithDiscrepancy,
		&totalDiscrepancy,
		&result.OnHoldWithDiscrepancy,
		&totalOnHoldDiscrepancy,
		&result.NegativeAvailable,
		&result.NegativeOnHold,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBalanceSummaryQuery, err)
	}

	result.TotalAbsoluteDiscrepancy = totalDiscrepancy
	result.TotalOnHoldDiscrepancy = totalOnHoldDiscrepancy

	return nil
}

// calculatePercentages computes discrepancy percentages.
func (c *BalanceChecker) calculatePercentages(result *domain.BalanceCheckResult) {
	if result.TotalBalances > 0 {
		result.DiscrepancyPercentage = float64(result.BalancesWithDiscrepancy) / float64(result.TotalBalances) * percentageMultiplier
		result.OnHoldDiscrepancyPct = float64(result.OnHoldWithDiscrepancy) / float64(result.TotalBalances) * percentageMultiplier
	}
}

// determineInitialStatus sets the initial status based on discrepancy counts.
func (c *BalanceChecker) determineInitialStatus(result *domain.BalanceCheckResult) {
	result.Status = DetermineStatus(result.BalancesWithDiscrepancy+result.OnHoldWithDiscrepancy, StatusThresholds{
		WarningThreshold: balanceWarningThreshold,
	})
}

// fetchDetailedResults retrieves detailed discrepancy records if needed.
func (c *BalanceChecker) fetchDetailedResults(ctx context.Context, result *domain.BalanceCheckResult, config CheckerConfig) error {
	if err := c.fetchBalanceDiscrepanciesIfNeeded(ctx, result, config); err != nil {
		return err
	}

	if err := c.fetchOnHoldDiscrepanciesIfNeeded(ctx, result, config); err != nil {
		return err
	}

	return c.fetchNegativeBalancesIfNeeded(ctx, result, config)
}

// fetchBalanceDiscrepanciesIfNeeded retrieves balance discrepancies if needed.
func (c *BalanceChecker) fetchBalanceDiscrepanciesIfNeeded(ctx context.Context, result *domain.BalanceCheckResult, config CheckerConfig) error {
	if result.BalancesWithDiscrepancy == 0 || config.MaxResults <= 0 {
		return nil
	}

	discrepancies, err := c.fetchBalanceDiscrepancies(ctx, config.DiscrepancyThreshold, config.MaxResults)
	if err != nil {
		return err
	}

	result.Discrepancies = discrepancies

	return nil
}

// fetchOnHoldDiscrepanciesIfNeeded retrieves on-hold discrepancies if needed.
func (c *BalanceChecker) fetchOnHoldDiscrepanciesIfNeeded(ctx context.Context, result *domain.BalanceCheckResult, config CheckerConfig) error {
	if result.OnHoldWithDiscrepancy == 0 || config.MaxResults <= 0 {
		return nil
	}

	onHoldDiscrepancies, err := c.fetchOnHoldDiscrepancies(ctx, config.DiscrepancyThreshold, config.MaxResults)
	if err != nil {
		return err
	}

	result.OnHoldDiscrepancies = onHoldDiscrepancies

	return nil
}

// fetchNegativeBalancesIfNeeded retrieves negative balances if needed.
func (c *BalanceChecker) fetchNegativeBalancesIfNeeded(ctx context.Context, result *domain.BalanceCheckResult, config CheckerConfig) error {
	if (result.NegativeAvailable == 0 && result.NegativeOnHold == 0) || config.MaxResults <= 0 {
		return nil
	}

	negativeBalances, err := c.fetchNegativeBalances(ctx, config.MaxResults)
	if err != nil {
		return err
	}

	result.NegativeBalances = negativeBalances
	result.Status = domain.StatusCritical

	return nil
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
				b.on_hold::DECIMAL as current_on_hold,
				COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_credits,
				COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL as total_debits,
				COALESCE(SUM(CASE
					WHEN o.type = 'ON_HOLD' AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN o.amount
					WHEN o.type IN ('RELEASE', 'DEBIT') AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN -o.amount
					ELSE 0
				END), 0)::DECIMAL as expected_on_hold,
				COUNT(o.id) as operation_count,
				b.updated_at
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			LEFT JOIN transaction t ON o.transaction_id = t.id
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.available, b.on_hold, b.updated_at
		)
		SELECT
			balance_id, account_id, alias, asset_code,
			current_balance, (total_credits - total_debits - expected_on_hold) as expected_balance,
			(current_balance - (total_credits - total_debits - expected_on_hold)) as discrepancy,
			operation_count, updated_at
		FROM balance_calc
		WHERE ABS(current_balance - (total_credits - total_debits - expected_on_hold)) > $1
		ORDER BY ABS(current_balance - (total_credits - total_debits - expected_on_hold)) DESC
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

// fetchOnHoldDiscrepancies retrieves detailed on-hold discrepancy records.
func (c *BalanceChecker) fetchOnHoldDiscrepancies(ctx context.Context, threshold int64, limit int) ([]domain.OnHoldDiscrepancy, error) {
	detailQuery := `
		WITH balance_calc AS (
			SELECT
				b.id as balance_id,
				b.account_id,
				b.alias,
				b.asset_code,
				b.on_hold::DECIMAL as current_on_hold,
				COALESCE(SUM(CASE
					WHEN o.type = 'ON_HOLD' AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN o.amount
					WHEN o.type IN ('RELEASE', 'DEBIT') AND o.balance_affected
						AND COALESCE((t.body->>'pending')::boolean, false)
						THEN -o.amount
					ELSE 0
				END), 0)::DECIMAL as expected_on_hold,
				COUNT(o.id) as operation_count,
				b.updated_at
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
			LEFT JOIN transaction t ON o.transaction_id = t.id
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.on_hold, b.updated_at
		)
		SELECT
			balance_id, account_id, alias, asset_code,
			current_on_hold, expected_on_hold,
			(current_on_hold - expected_on_hold) as discrepancy,
			operation_count, updated_at
		FROM balance_calc
		WHERE ABS(current_on_hold - expected_on_hold) > $1
		ORDER BY ABS(current_on_hold - expected_on_hold) DESC
		LIMIT $2
	`

	rows, err := c.db.QueryContext(ctx, detailQuery, threshold, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceDetailQuery, err)
	}
	defer rows.Close()

	var discrepancies []domain.OnHoldDiscrepancy

	for rows.Next() {
		var d domain.OnHoldDiscrepancy

		err := rows.Scan(
			&d.BalanceID, &d.AccountID, &d.Alias, &d.AssetCode,
			&d.CurrentOnHold, &d.ExpectedOnHold, &d.Discrepancy,
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

// fetchNegativeBalances retrieves balances with negative available or on-hold values.
func (c *BalanceChecker) fetchNegativeBalances(ctx context.Context, limit int) ([]domain.NegativeBalance, error) {
	query := `
		SELECT
			id::text,
			account_id::text,
			alias,
			asset_code,
			available::DECIMAL,
			on_hold::DECIMAL
		FROM balance
		WHERE deleted_at IS NULL
		  AND (available < 0 OR on_hold < 0)
		  AND account_type <> 'external'
		ORDER BY updated_at DESC
		LIMIT $1
	`

	rows, err := c.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceDetailQuery, err)
	}
	defer rows.Close()

	var negatives []domain.NegativeBalance

	for rows.Next() {
		var n domain.NegativeBalance
		if err := rows.Scan(&n.BalanceID, &n.AccountID, &n.Alias, &n.AssetCode, &n.Available, &n.OnHold); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBalanceRowScan, err)
		}

		negatives = append(negatives, n)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBalanceRowIteration, err)
	}

	return negatives, nil
}
