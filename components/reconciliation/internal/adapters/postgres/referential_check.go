package postgres

import (
	"context"
	"database/sql"
	"fmt"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Referential check threshold constants.
const (
	referentialWarningThreshold = 10
)

// ReferentialChecker finds orphan entities
type ReferentialChecker struct {
	onboardingDB  *sql.DB
	transactionDB *sql.DB
	logger        libLog.Logger
}

// NewReferentialChecker creates a new referential checker
func NewReferentialChecker(onboardingDB, transactionDB *sql.DB, logger libLog.Logger) *ReferentialChecker {
	if logger == nil {
		logger = &libLog.NoneLogger{}
	}

	return &ReferentialChecker{
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
		logger:        logger,
	}
}

// Name returns the unique name of this checker.
func (c *ReferentialChecker) Name() string {
	return CheckerNameReferential
}

// Check finds orphan entities across databases
func (c *ReferentialChecker) Check(ctx context.Context, _ CheckerConfig) (CheckResult, error) {
	result := &domain.ReferentialCheckResult{}

	// Check onboarding DB orphans
	if err := c.checkOnboardingOrphans(ctx, result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReferentialOnboardingCheck, err)
	}

	// Check transaction DB orphans
	if err := c.checkTransactionOrphans(ctx, result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReferentialTransactionCheck, err)
	}

	// Determine status
	total := result.OrphanLedgers + result.OrphanAssets + result.OrphanAccounts +
		result.OrphanOperations + result.OrphanPortfolios + result.OrphanUnknown +
		result.OperationsWithoutBalance

	result.Status = DetermineStatus(total, StatusThresholds{
		WarningThreshold:          referentialWarningThreshold,
		WarningThresholdExclusive: true,
	})

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
		return fmt.Errorf("%w: %w", ErrOnboardingQuery, err)
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return fmt.Errorf("%w: %w", ErrOnboardingRowScan, err)
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
		default:
			// Count + log unknown entity types so schema changes aren't silently ignored.
			result.OrphanUnknown++

			c.logger.Warnf(
				"reconciliation referential check: unexpected orphan entity_type=%q entity_id=%q reference_type=%q reference_id=%q",
				o.EntityType, o.EntityID, o.ReferenceType, o.ReferenceID,
			)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrOnboardingRowIteration, err)
	}

	return nil
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
		return fmt.Errorf("%w: %w", ErrTransactionQuery, err)
	}
	defer rows.Close()

	for rows.Next() {
		var o domain.OrphanEntity
		if err := rows.Scan(&o.EntityID, &o.EntityType, &o.ReferenceType, &o.ReferenceID); err != nil {
			return fmt.Errorf("%w: %w", ErrTransactionRowScan, err)
		}

		result.Orphans = append(result.Orphans, o)
		result.OrphanOperations++
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionRowIteration, err)
	}

	// Count operations that reference missing balances
	balanceQuery := `
		SELECT COUNT(*)
		FROM operation o
		LEFT JOIN balance b ON o.balance_id = b.id AND b.deleted_at IS NULL
		WHERE o.deleted_at IS NULL
		  AND o.balance_id IS NOT NULL
		  AND b.id IS NULL
	`

	if err := c.transactionDB.QueryRowContext(ctx, balanceQuery).Scan(&result.OperationsWithoutBalance); err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionQuery, err)
	}

	return nil
}
