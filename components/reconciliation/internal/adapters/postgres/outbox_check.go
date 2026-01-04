package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Outbox check threshold constants.
const (
	outboxWarningThreshold = 10
)

// OutboxChecker reports metadata outbox backlog and staleness.
type OutboxChecker struct {
	db *sql.DB
}

// NewOutboxChecker creates a new outbox checker.
func NewOutboxChecker(db *sql.DB) *OutboxChecker {
	return &OutboxChecker{db: db}
}

// Name returns the unique name of this checker.
func (c *OutboxChecker) Name() string {
	return CheckerNameOutbox
}

// Check returns outbox backlog summary and limited entry details.
func (c *OutboxChecker) Check(ctx context.Context, config CheckerConfig) (CheckResult, error) {
	if config.MaxResults < 0 {
		config.MaxResults = 0
	}

	staleSeconds := config.OutboxStaleSeconds
	if staleSeconds <= 0 {
		staleSeconds = config.StaleThresholdSeconds
	}

	if staleSeconds <= 0 {
		staleSeconds = 600
	}

	result := &domain.OutboxCheckResult{}

	summaryQuery := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'PENDING') as pending,
			COUNT(*) FILTER (WHERE status = 'PROCESSING') as processing,
			COUNT(*) FILTER (WHERE status = 'FAILED') as failed,
			COUNT(*) FILTER (WHERE status = 'DLQ') as dlq,
			COUNT(*) FILTER (
				WHERE status = 'PROCESSING'
				  AND processing_started_at IS NOT NULL
				  AND processing_started_at < NOW() - INTERVAL '1 second' * $1
			) as stale_processing,
			COALESCE(MAX(retry_count), 0) as max_retry_count,
			MIN(created_at) FILTER (WHERE status = 'PENDING') as oldest_pending,
			MIN(created_at) FILTER (WHERE status = 'FAILED') as oldest_failed
		FROM metadata_outbox
	`

	var oldestPending, oldestFailed sql.NullTime

	if err := c.db.QueryRowContext(ctx, summaryQuery, staleSeconds).Scan(
		&result.Pending,
		&result.Processing,
		&result.Failed,
		&result.DLQ,
		&result.StaleProcessing,
		&result.MaxRetryCount,
		&oldestPending,
		&oldestFailed,
	); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDLQSummaryQuery, err)
	}

	if oldestPending.Valid {
		ts := oldestPending.Time
		result.OldestPendingAt = &ts
	}

	if oldestFailed.Valid {
		ts := oldestFailed.Time
		result.OldestFailedAt = &ts
	}

	if result.Pending+result.Failed+result.Processing > 0 && config.MaxResults > 0 {
		entries, err := c.fetchOutboxEntries(ctx, config.MaxResults)
		if err != nil {
			return nil, err
		}

		result.Entries = entries
	}

	issueCount := int(result.Pending + result.Failed + result.Processing)
	result.Status = DetermineStatus(issueCount, StatusThresholds{
		WarningThreshold:          outboxWarningThreshold,
		WarningThresholdExclusive: true,
	})

	if result.StaleProcessing > 0 {
		result.Status = domain.StatusCritical
	}

	return result, nil
}

func (c *OutboxChecker) fetchOutboxEntries(ctx context.Context, limit int) ([]domain.OutboxEntry, error) {
	query := `
		SELECT
			id::text,
			entity_id,
			entity_type,
			status,
			retry_count,
			COALESCE(last_error, ''),
			created_at
		FROM metadata_outbox
		WHERE status IN ('PENDING', 'FAILED', 'PROCESSING')
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := c.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDLQDetailQuery, err)
	}
	defer rows.Close()

	var entries []domain.OutboxEntry

	for rows.Next() {
		var entry domain.OutboxEntry
		if err := rows.Scan(
			&entry.ID,
			&entry.EntityID,
			&entry.EntityType,
			&entry.Status,
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
