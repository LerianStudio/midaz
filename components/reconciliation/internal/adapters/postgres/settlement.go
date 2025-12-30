package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// SettlementDetector checks if transactions have fully settled
type SettlementDetector struct {
	db *sql.DB
}

// NewSettlementDetector creates a new settlement detector
func NewSettlementDetector(db *sql.DB) *SettlementDetector {
	return &SettlementDetector{db: db}
}

// GetUnsettledCount returns the count of transactions still processing
// NOTE: NOTED transactions don't affect balances and are excluded from settlement counts.
func (s *SettlementDetector) GetUnsettledCount(ctx context.Context, settlementWaitSeconds int) (int, error) {
	query := `
		WITH txn_ops AS (
			SELECT
				t.id,
				t.status,
				t.created_at,
				COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			GROUP BY t.id, t.status, t.created_at
		)
		SELECT COUNT(*)
		FROM txn_ops
	 WHERE status = 'PENDING'
	   OR (
		   status IN ('APPROVED', 'CANCELED')
		   AND (
			   created_at >= NOW() - INTERVAL '1 second' * $1
			   OR operation_count < 2
		   )
	   )
	`

	var count int
	if err := s.db.QueryRowContext(ctx, query, settlementWaitSeconds).Scan(&count); err != nil {
		return 0, fmt.Errorf("GetUnsettledCount failed: %w", err)
	}

	return count, nil
}

// GetSettledCount returns the count of settled transactions
// NOTE: Only APPROVED and CANCELED affect balances, NOTED transactions don't need settlement
func (s *SettlementDetector) GetSettledCount(ctx context.Context, settlementWaitSeconds int) (int, error) {
	query := `
		WITH txn_ops AS (
			SELECT
				t.id,
				t.status,
				t.created_at,
				COUNT(o.id) as operation_count
			FROM transaction t
			LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
			WHERE t.deleted_at IS NULL
			GROUP BY t.id, t.status, t.created_at
		)
		SELECT COUNT(*)
	FROM txn_ops
	WHERE status IN ('APPROVED', 'CANCELED')
	  AND created_at < NOW() - INTERVAL '1 second' * $1
	  AND operation_count >= 2
	`

	var count int
	if err := s.db.QueryRowContext(ctx, query, settlementWaitSeconds).Scan(&count); err != nil {
		return 0, fmt.Errorf("GetSettledCount failed: %w", err)
	}

	return count, nil
}
