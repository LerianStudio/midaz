package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Sync check threshold constants.
const (
	syncWarningThreshold = 10
)

// SyncChecker detects Redis-PostgreSQL version mismatches
type SyncChecker struct {
	db *sql.DB
}

// NewSyncChecker creates a new sync checker
func NewSyncChecker(db *sql.DB) *SyncChecker {
	return &SyncChecker{db: db}
}

// Name returns the unique name of this checker.
func (c *SyncChecker) Name() string {
	return CheckerNameSync
}

// processSyncIssue updates result counters based on the sync issue type.
func processSyncIssue(result *domain.SyncCheckResult, issue domain.SyncIssue, staleThresholdSeconds int) {
	result.Issues = append(result.Issues, issue)

	if issue.DBVersion != issue.MaxOpVersion {
		result.VersionMismatches++
	}

	if issue.StalenessSeconds > int64(staleThresholdSeconds) {
		result.StaleBalances++
	}
}

// Check detects version mismatches and stale balances
func (c *SyncChecker) Check(ctx context.Context, config CheckerConfig) (CheckResult, error) {
	result := &domain.SyncCheckResult{}

	// Query includes balance_key in join to ensure correct operation matching
	query := `
		WITH balance_ops AS (
			SELECT
				b.id as balance_id,
				b.account_id,
				b.asset_code,
				b.version as db_version,
				MAX(o.balance_version_after) as max_op_version,
				EXTRACT(EPOCH FROM (MAX(o.created_at) - b.updated_at))::bigint as staleness_seconds
			FROM balance b
			LEFT JOIN operation o ON b.account_id = o.account_id
				AND b.asset_code = o.asset_code
				AND b.key = o.balance_key
				AND o.deleted_at IS NULL
				AND o.balance_affected = true
			WHERE b.deleted_at IS NULL
			GROUP BY b.id, b.account_id, b.asset_code, b.version, b.updated_at
		)
		SELECT
			balance_id, account_id as alias, asset_code, db_version,
			COALESCE(max_op_version, 0)::int as max_op_version,
			COALESCE(staleness_seconds, 0) as staleness_seconds
		FROM balance_ops
		WHERE db_version != COALESCE(max_op_version, 0)
		   OR staleness_seconds > $1
		ORDER BY staleness_seconds DESC NULLS LAST
		LIMIT $2
	`

	rows, err := c.db.QueryContext(ctx, query, config.StaleThresholdSeconds, config.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSyncCheckQuery, err)
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.SyncIssue
		if err := rows.Scan(
			&s.BalanceID, &s.Alias, &s.AssetCode, &s.DBVersion,
			&s.MaxOpVersion, &s.StalenessSeconds,
		); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSyncRowScan, err)
		}

		processSyncIssue(result, s, config.StaleThresholdSeconds)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSyncRowIteration, err)
	}

	result.Status = DetermineStatus(len(result.Issues), StatusThresholds{
		WarningThreshold:          syncWarningThreshold,
		WarningThresholdExclusive: true,
	})

	return result, nil
}
