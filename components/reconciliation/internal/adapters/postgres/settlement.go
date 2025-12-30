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
// NOTE: Only APPROVED and CANCELED affect balances, NOTED transactions don't need settlement
func (s *SettlementDetector) GetUnsettledCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(DISTINCT t.id)
		FROM transaction t
		WHERE t.deleted_at IS NULL
		  AND t.status IN ('APPROVED', 'CANCELED')
		  AND EXISTS (
			  SELECT 1
			  FROM metadata_outbox o
			  WHERE o.entity_id = t.id::text
				AND o.status IN ('PENDING', 'PROCESSING', 'FAILED')
		  )
	`

	var count int
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("GetUnsettledCount failed: %w", err)
	}

	return count, nil
}

// GetSettledCount returns the count of settled transactions
// NOTE: Only APPROVED and CANCELED affect balances, NOTED transactions don't need settlement
func (s *SettlementDetector) GetSettledCount(ctx context.Context, settlementWaitSeconds int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM transaction t
		WHERE t.deleted_at IS NULL
		  AND t.status IN ('APPROVED', 'CANCELED')
		  AND t.created_at < NOW() - INTERVAL '1 second' * $1
		  AND NOT EXISTS (
			  SELECT 1
			  FROM metadata_outbox o
			  WHERE o.entity_id = t.id::text
				AND o.status IN ('PENDING', 'PROCESSING', 'FAILED')
		  )
	`

	var count int
	if err := s.db.QueryRowContext(ctx, query, settlementWaitSeconds).Scan(&count); err != nil {
		return 0, fmt.Errorf("GetSettledCount failed: %w", err)
	}

	return count, nil
}
