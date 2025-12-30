package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// ReferentialChecker finds orphan entities
type ReferentialChecker struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
}

// NewReferentialChecker creates a new referential checker
func NewReferentialChecker(onboardingDB, transactionDB *sql.DB) *ReferentialChecker {
	return &ReferentialChecker{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
	}
}

// Check finds orphan entities across databases
func (c *ReferentialChecker) Check(ctx context.Context) (*domain.ReferentialCheckResult, error) {
	result := &domain.ReferentialCheckResult{}

	// Check onboarding DB orphans
	if err := c.checkOnboardingOrphans(ctx, result); err != nil {
		return nil, fmt.Errorf("onboarding orphan check failed: %w", err)
	}

	// Check transaction DB orphans
	if err := c.checkTransactionOrphans(ctx, result); err != nil {
		return nil, fmt.Errorf("transaction orphan check failed: %w", err)
	}

	// Determine status
	total := result.OrphanLedgers + result.OrphanAssets + result.OrphanAccounts +
		result.OrphanOperations + result.OrphanPortfolios
	if total == 0 {
		result.Status = domain.StatusHealthy
	} else if total < 10 {
		result.Status = domain.StatusWarning
	} else {
		result.Status = domain.StatusCritical
	}

	return result, nil
}

func (c *ReferentialChecker) checkOnboardingOrphans(ctx context.Context, result *domain.ReferentialCheckResult) error {
	query := `
		WITH orphan_ledgers AS (
			SELECT l.id::text as entity_id, 'ledger' as entity_type,
				   'organization' as reference_type, l.organization_id::text as reference_id
			FROM ledger l
			LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL
			WHERE l.deleted_at IS NULL AND o.id IS NULL
		),
		orphan_assets AS (
			SELECT a.id::text as entity_id, 'asset' as entity_type,
				   'ledger' as reference_type, a.ledger_id::text as reference_id
			FROM asset a
			LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE a.deleted_at IS NULL AND l.id IS NULL
		),
		orphan_accounts AS (
			SELECT a.id::text as entity_id, 'account' as entity_type,
				   'ledger' as reference_type, a.ledger_id::text as reference_id
			FROM account a
			LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE a.deleted_at IS NULL AND l.id IS NULL
		),
		orphan_portfolios AS (
			SELECT p.id::text as entity_id, 'portfolio' as entity_type,
				   'ledger' as reference_type, p.ledger_id::text as reference_id
			FROM portfolio p
			LEFT JOIN ledger l ON p.ledger_id = l.id AND l.deleted_at IS NULL
			WHERE p.deleted_at IS NULL AND l.id IS NULL
		)
		SELECT * FROM orphan_ledgers
		UNION ALL SELECT * FROM orphan_assets
		UNION ALL SELECT * FROM orphan_accounts
		UNION ALL SELECT * FROM orphan_portfolios
	`

	rows, err := c.onboardingDB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return err
		}
		result.Orphans = append(result.Orphans, o)

		switch o.EntityType {
		case "ledger":
			result.OrphanLedgers++
		case "asset":
			result.OrphanAssets++
		case "account":
			result.OrphanAccounts++
		case "portfolio":
			result.OrphanPortfolios++
		}
	}

	return rows.Err()
}

func (c *ReferentialChecker) checkTransactionOrphans(ctx context.Context, result *domain.ReferentialCheckResult) error {
	query := `
		SELECT o.id::text as entity_id, 'operation' as entity_type,
			   'transaction' as reference_type, o.transaction_id::text as reference_id
		FROM operation o
		LEFT JOIN transaction t ON o.transaction_id = t.id AND t.deleted_at IS NULL
		WHERE o.deleted_at IS NULL AND t.id IS NULL
	`

	rows, err := c.transactionDB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return err
		}
		result.Orphans = append(result.Orphans, o)
		result.OrphanOperations++
	}

	return rows.Err()
}
