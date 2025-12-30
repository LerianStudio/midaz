package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// EntityCounter counts entities in databases
type EntityCounter struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
}

// NewEntityCounter creates a new entity counter
func NewEntityCounter(onboardingDB, transactionDB *sql.DB) *EntityCounter {
	return &EntityCounter{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
	}
}

// OnboardingCounts holds onboarding entity counts
type OnboardingCounts struct {
	Organizations int64
	Ledgers       int64
	Assets        int64
	Accounts      int64
	Portfolios    int64
}

// TransactionCounts holds transaction entity counts
type TransactionCounts struct {
	Transactions int64
	Operations   int64
	Balances     int64
}

// GetOnboardingCounts returns counts from onboarding DB
func (c *EntityCounter) GetOnboardingCounts(ctx context.Context) (*OnboardingCounts, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM organization WHERE deleted_at IS NULL) as organizations,
			(SELECT COUNT(*) FROM ledger WHERE deleted_at IS NULL) as ledgers,
			(SELECT COUNT(*) FROM asset WHERE deleted_at IS NULL) as assets,
			(SELECT COUNT(*) FROM account WHERE deleted_at IS NULL) as accounts,
			(SELECT COUNT(*) FROM portfolio WHERE deleted_at IS NULL) as portfolios
	`

	counts := &OnboardingCounts{}
	err := c.onboardingDB.QueryRowContext(ctx, query).Scan(
		&counts.Organizations,
		&counts.Ledgers,
		&counts.Assets,
		&counts.Accounts,
		&counts.Portfolios,
	)
	if err != nil {
		return nil, fmt.Errorf("GetOnboardingCounts: query failed: %w", err)
	}

	return counts, nil
}

// GetTransactionCounts returns counts from transaction DB
func (c *EntityCounter) GetTransactionCounts(ctx context.Context) (*TransactionCounts, error) {
	query := `
		SELECT
			(SELECT COUNT(*) FROM transaction WHERE deleted_at IS NULL) as transactions,
			(SELECT COUNT(*) FROM operation WHERE deleted_at IS NULL) as operations,
			(SELECT COUNT(*) FROM balance WHERE deleted_at IS NULL) as balances
	`

	counts := &TransactionCounts{}
	err := c.transactionDB.QueryRowContext(ctx, query).Scan(
		&counts.Transactions,
		&counts.Operations,
		&counts.Balances,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction counts: %w", err)
	}

	return counts, nil
}
