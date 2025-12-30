package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// DLQChecker reports metadata outbox entries stuck in DLQ.
type DLQChecker struct {
	db *sql.DB
}

// NewDLQChecker creates a new DLQ checker.
func NewDLQChecker(db *sql.DB) *DLQChecker {
	return &DLQChecker{db: db}
}

// Check returns DLQ summary and a limited set of entries.
func (c *DLQChecker) Check(ctx context.Context, limit int) (*domain.DLQCheckResult, error) {
	// Guard against negative limits. In PostgreSQL, a negative LIMIT means "no limit",
	// which could accidentally return all DLQ rows.
	if limit < 0 {
		limit = 0
	}

	result := &domain.DLQCheckResult{}

	summaryQuery := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'DLQ') as total,
			COUNT(*) FILTER (WHERE status = 'DLQ' AND entity_type = 'Transaction') as transaction_count,
			COUNT(*) FILTER (WHERE status = 'DLQ' AND entity_type = 'Operation') as operation_count
		FROM metadata_outbox
	`

	if err := c.db.QueryRowContext(ctx, summaryQuery).Scan(
		&result.Total,
		&result.TransactionEntries,
		&result.OperationEntries,
	); err != nil {
		return nil, fmt.Errorf("dlq summary query failed: %w", err)
	}

	switch {
	case result.Total == 0:
		result.Status = domain.StatusHealthy
	case result.Total <= 10:
		result.Status = domain.StatusWarning
	default:
		result.Status = domain.StatusCritical
	}

	if result.Total > 0 && limit > 0 {
		detailQuery := `
			SELECT
				id::text,
				entity_id,
				entity_type,
				retry_count,
				COALESCE(last_error, ''),
				created_at
			FROM metadata_outbox
			WHERE status = 'DLQ'
			ORDER BY created_at DESC
			LIMIT $1
		`

		rows, err := c.db.QueryContext(ctx, detailQuery, limit)
		if err != nil {
			return nil, fmt.Errorf("dlq detail query failed: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var entry domain.DLQEntry
			if err := rows.Scan(
				&entry.ID,
				&entry.EntityID,
				&entry.EntityType,
				&entry.RetryCount,
				&entry.LastError,
				&entry.CreatedAt,
			); err != nil {
				return nil, fmt.Errorf("dlq row scan failed: %w", err)
			}
			result.Entries = append(result.Entries, entry)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("dlq row iteration failed: %w", err)
		}
	}

	return result, nil
}
