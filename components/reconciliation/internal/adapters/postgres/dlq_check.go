package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// DLQ check threshold constants.
const (
	dlqWarningThreshold = 10
)

// DLQChecker reports metadata outbox entries stuck in DLQ.
type DLQChecker struct {
	db *sql.DB
}

// NewDLQChecker creates a new DLQ checker.
func NewDLQChecker(db *sql.DB) *DLQChecker {
	return &DLQChecker{db: db}
}

// Name returns the unique name of this checker.
func (c *DLQChecker) Name() string {
	return CheckerNameDLQ
}

// Check returns DLQ summary and a limited set of entries.
func (c *DLQChecker) Check(ctx context.Context, config CheckerConfig) (CheckResult, error) {
	// Guard against negative limits. In PostgreSQL, a negative LIMIT means "no limit",
	// which could accidentally return all DLQ rows.
	if config.MaxResults < 0 {
		config.MaxResults = 0
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
		return nil, fmt.Errorf("%w: %w", ErrDLQSummaryQuery, err)
	}

	result.Status = DetermineStatus(int(result.Total), StatusThresholds{
		WarningThreshold:          dlqWarningThreshold,
		WarningThresholdExclusive: true,
	})

	if result.Total > 0 && config.MaxResults > 0 {
		entries, err := c.fetchDLQEntries(ctx, config.MaxResults)
		if err != nil {
			return nil, err
		}

		result.Entries = entries
	}

	return result, nil
}

// fetchDLQEntries retrieves detailed DLQ entry records.
func (c *DLQChecker) fetchDLQEntries(ctx context.Context, limit int) ([]domain.DLQEntry, error) {
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
		return nil, fmt.Errorf("%w: %w", ErrDLQDetailQuery, err)
	}
	defer rows.Close()

	var entries []domain.DLQEntry

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
			return nil, fmt.Errorf("%w: %w", ErrDLQRowScan, err)
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDLQRowIteration, err)
	}

	return entries, nil
}
