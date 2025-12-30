package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// SyncChecker detects Redis-PostgreSQL version mismatches
type SyncChecker struct {
	db *sql.DB
}

// NewSyncChecker creates a new sync checker
func NewSyncChecker(db *sql.DB) *SyncChecker {
	return &SyncChecker{db: db}
}

// Check detects version mismatches and stale balances
func (c *SyncChecker) Check(ctx context.Context, staleThresholdSeconds int, limit int) (*domain.SyncCheckResult, error) {
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

	rows, err := c.db.QueryContext(ctx, query, staleThresholdSeconds, limit)
	if err != nil {
		return nil, fmt.Errorf("sync check query failed: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s domain.SyncIssue
		if err := rows.Scan(
			&s.BalanceID, &s.Alias, &s.AssetCode, &s.DBVersion,
			&s.MaxOpVersion, &s.StalenessSeconds,
		); err != nil {
			return nil, fmt.Errorf("sync row scan failed: %w", err)
		}
		result.Issues = append(result.Issues, s)

		if s.DBVersion != s.MaxOpVersion {
			result.VersionMismatches++
		}
		if s.StalenessSeconds > int64(staleThresholdSeconds) {
			result.StaleBalances++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sync row iteration failed: %w", err)
	}

	// Status based on distinct issue count (not sum of categories which double-counts)
	total := 0
	if result.Issues != nil {
		total = len(result.Issues)
	}

	if total == 0 {
		result.Status = domain.StatusHealthy
	} else if total < 10 {
		result.Status = domain.StatusWarning
	} else {
		result.Status = domain.StatusCritical
	}

	return result, nil
}
