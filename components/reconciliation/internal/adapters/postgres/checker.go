package postgres

import (
	"context"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
)

// Checker name constants.
const (
	CheckerNameBalance     = "balance"
	CheckerNameDoubleEntry = "double-entry"
	CheckerNameOrphans     = "orphans"
	CheckerNameReferential = "referential"
	CheckerNameSync        = "sync"
	CheckerNameDLQ         = "dlq"
	CheckerNameOutbox      = "outbox"
	CheckerNameMetadata    = "metadata"
	CheckerNameRedis       = "redis"
	CheckerNameCrossDB     = "cross-db"
	CheckerNameCRMAlias    = "crm-alias"
)

// CheckerConfig holds configuration parameters for reconciliation checkers.
// This allows uniform passing of config to checkers with different signatures.
type CheckerConfig struct {
	// DiscrepancyThreshold is the minimum discrepancy amount to report (for balance checker).
	DiscrepancyThreshold int64

	// MaxResults limits the number of detailed results returned.
	MaxResults int

	// StaleThresholdSeconds is the staleness threshold for sync checks.
	StaleThresholdSeconds int

	// OutboxStaleSeconds is the staleness threshold for outbox PROCESSING entries.
	OutboxStaleSeconds int

	// LookbackDays limits metadata checks to recent outbox entries.
	LookbackDays int

	// CrossDBBatchSize controls batch size for cross-database checks.
	CrossDBBatchSize int

	// CrossDBMaxScan limits the maximum number of IDs scanned.
	CrossDBMaxScan int

	// RedisSampleSize limits Redis balance sampling.
	RedisSampleSize int

	// MetadataMaxScan limits the number of outbox entries scanned for metadata checks.
	MetadataMaxScan int
}

// CheckResult is the common interface for all check results.
// Each checker returns its specific result type that implements this interface.
type CheckResult interface {
	// GetStatus returns the status of the check (HEALTHY, WARNING, CRITICAL, ERROR).
	GetStatus() domain.ReconciliationStatus
}

// ReconciliationChecker is the common interface for all reconciliation checkers.
// It allows the engine to treat all checkers uniformly despite their different parameters.
type ReconciliationChecker interface {
	// Name returns the unique name of this checker.
	Name() string

	// Check performs the reconciliation check and returns the result.
	// The config provides all necessary parameters.
	Check(ctx context.Context, config CheckerConfig) (CheckResult, error)
}
