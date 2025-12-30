// Package domain contains the domain models and business logic for reconciliation.
package domain

import "time"

// ReconciliationStatus represents the health status of a reconciliation check.
type ReconciliationStatus string

// Reconciliation status constants.
const (
	StatusHealthy  ReconciliationStatus = "HEALTHY"
	StatusWarning  ReconciliationStatus = "WARNING"
	StatusCritical ReconciliationStatus = "CRITICAL"
	StatusError    ReconciliationStatus = "ERROR"
	StatusSkipped  ReconciliationStatus = "SKIPPED"
	StatusUnknown  ReconciliationStatus = "UNKNOWN"
)

// ReconciliationReport is the complete output of a reconciliation run
type ReconciliationReport struct {
	Timestamp time.Time            `json:"timestamp"`
	Duration  string               `json:"duration"`
	Status    ReconciliationStatus `json:"status"`

	// Settlement info
	UnsettledTransactions int `json:"unsettled_transactions"`
	SettledTransactions   int `json:"settled_transactions"`

	// Check results
	BalanceCheck     *BalanceCheckResult     `json:"balance_check"`
	DoubleEntryCheck *DoubleEntryCheckResult `json:"double_entry_check"`
	ReferentialCheck *ReferentialCheckResult `json:"referential_check"`
	SyncCheck        *SyncCheckResult        `json:"sync_check"`
	OrphanCheck      *OrphanCheckResult      `json:"orphan_check"`
	MetadataCheck    *MetadataCheckResult    `json:"metadata_check"`
	DLQCheck         *DLQCheckResult         `json:"dlq_check"`

	// Entity counts
	EntityCounts *EntityCounts `json:"entity_counts"`
}

// BalanceCheckResult holds balance consistency results
type BalanceCheckResult struct {
	TotalBalances            int                  `json:"total_balances"`
	BalancesWithDiscrepancy  int                  `json:"balances_with_discrepancy"`
	DiscrepancyPercentage    float64              `json:"discrepancy_percentage"`
	TotalAbsoluteDiscrepancy int64                `json:"total_absolute_discrepancy"`
	Discrepancies            []BalanceDiscrepancy `json:"discrepancies,omitempty"`
	Status                   ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *BalanceCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// BalanceDiscrepancy represents a balance that doesn't match its operations
type BalanceDiscrepancy struct {
	BalanceID       string    `json:"balance_id"`
	AccountID       string    `json:"account_id"`
	Alias           string    `json:"alias"`
	AssetCode       string    `json:"asset_code"`
	CurrentBalance  int64     `json:"current_balance"`
	ExpectedBalance int64     `json:"expected_balance"`
	Discrepancy     int64     `json:"discrepancy"`
	OperationCount  int64     `json:"operation_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DoubleEntryCheckResult holds double-entry validation results
type DoubleEntryCheckResult struct {
	TotalTransactions        int                    `json:"total_transactions"`
	UnbalancedTransactions   int                    `json:"unbalanced_transactions"`
	UnbalancedPercentage     float64                `json:"unbalanced_percentage"`
	TransactionsNoOperations int                    `json:"transactions_without_operations"`
	Imbalances               []TransactionImbalance `json:"imbalances,omitempty"`
	Status                   ReconciliationStatus   `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *DoubleEntryCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// TransactionImbalance represents a transaction where credits != debits
type TransactionImbalance struct {
	TransactionID  string `json:"transaction_id"`
	Status         string `json:"status"`
	AssetCode      string `json:"asset_code"`
	TotalCredits   int64  `json:"total_credits"`
	TotalDebits    int64  `json:"total_debits"`
	Imbalance      int64  `json:"imbalance"`
	OperationCount int64  `json:"operation_count"`
}

// ReferentialCheckResult holds referential integrity results
type ReferentialCheckResult struct {
	OrphanLedgers    int                  `json:"orphan_ledgers"`
	OrphanAssets     int                  `json:"orphan_assets"`
	OrphanAccounts   int                  `json:"orphan_accounts"`
	OrphanOperations int                  `json:"orphan_operations"`
	OrphanPortfolios int                  `json:"orphan_portfolios"`
	Orphans          []OrphanEntity       `json:"orphans,omitempty"`
	Status           ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *ReferentialCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// OrphanEntity represents an entity with missing references
type OrphanEntity struct {
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	ReferenceType string `json:"reference_type"`
	ReferenceID   string `json:"reference_id"`
}

// SyncCheckResult holds Redis-PostgreSQL sync results
type SyncCheckResult struct {
	VersionMismatches int                  `json:"version_mismatches"`
	StaleBalances     int                  `json:"stale_balances"`
	Issues            []SyncIssue          `json:"issues,omitempty"`
	Status            ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *SyncCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// SyncIssue represents a Redis-PostgreSQL sync problem
type SyncIssue struct {
	BalanceID        string `json:"balance_id"`
	Alias            string `json:"alias"`
	AssetCode        string `json:"asset_code"`
	DBVersion        int32  `json:"db_version"`
	MaxOpVersion     int32  `json:"max_op_version"`
	StalenessSeconds int64  `json:"staleness_seconds"`
}

// OrphanCheckResult holds orphan transaction results
type OrphanCheckResult struct {
	OrphanTransactions  int                  `json:"orphan_transactions"`
	PartialTransactions int                  `json:"partial_transactions"`
	Orphans             []OrphanTransaction  `json:"orphans,omitempty"`
	Status              ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *OrphanCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// OrphanTransaction represents a transaction without operations
type OrphanTransaction struct {
	TransactionID  string    `json:"transaction_id"`
	OrganizationID string    `json:"organization_id"`
	LedgerID       string    `json:"ledger_id"`
	Status         string    `json:"status"`
	Amount         int64     `json:"amount"`
	AssetCode      string    `json:"asset_code"`
	CreatedAt      time.Time `json:"created_at"`
	OperationCount int32     `json:"operation_count"`
}

// MetadataCheckResult holds metadata sync results
type MetadataCheckResult struct {
	PostgreSQLCount int64                `json:"postgresql_count"`
	MongoDBCount    int64                `json:"mongodb_count"`
	MissingCount    int64                `json:"missing_count"`
	Status          ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *MetadataCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// DLQCheckResult holds metadata outbox DLQ results
type DLQCheckResult struct {
	Total              int64                `json:"total"`
	TransactionEntries int64                `json:"transaction_entries"`
	OperationEntries   int64                `json:"operation_entries"`
	Entries            []DLQEntry           `json:"entries,omitempty"`
	Status             ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *DLQCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// DLQEntry represents a single DLQ entry
type DLQEntry struct {
	ID         string    `json:"id"`
	EntityID   string    `json:"entity_id"`
	EntityType string    `json:"entity_type"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// EntityCounts holds PostgreSQL entity counts
type EntityCounts struct {
	// Onboarding
	Organizations int64 `json:"organizations"`
	Ledgers       int64 `json:"ledgers"`
	Assets        int64 `json:"assets"`
	Accounts      int64 `json:"accounts"`
	Portfolios    int64 `json:"portfolios"`

	// Transaction
	Transactions int64 `json:"transactions"`
	Operations   int64 `json:"operations"`
	Balances     int64 `json:"balances"`
}

// DetermineOverallStatus calculates the overall status from individual checks
func (r *ReconciliationReport) DetermineOverallStatus() {
	checkStatuses := r.collectCheckStatuses()
	r.Status = calculateWorstStatus(checkStatuses)
}

// collectCheckStatuses gathers all non-nil check statuses into a slice.
func (r *ReconciliationReport) collectCheckStatuses() []ReconciliationStatus {
	var statuses []ReconciliationStatus

	if r.BalanceCheck != nil {
		statuses = append(statuses, r.BalanceCheck.GetStatus())
	}

	if r.DoubleEntryCheck != nil {
		statuses = append(statuses, r.DoubleEntryCheck.GetStatus())
	}

	if r.ReferentialCheck != nil {
		statuses = append(statuses, r.ReferentialCheck.GetStatus())
	}

	if r.SyncCheck != nil {
		statuses = append(statuses, r.SyncCheck.GetStatus())
	}

	if r.OrphanCheck != nil {
		statuses = append(statuses, r.OrphanCheck.GetStatus())
	}

	if r.MetadataCheck != nil {
		statuses = append(statuses, r.MetadataCheck.GetStatus())
	}

	if r.DLQCheck != nil {
		statuses = append(statuses, r.DLQCheck.GetStatus())
	}

	return statuses
}

// calculateWorstStatus returns the worst status from a slice of statuses.
// Priority: Critical > Error > Warning > Healthy
func calculateWorstStatus(statuses []ReconciliationStatus) ReconciliationStatus {
	for _, status := range statuses {
		if status == StatusCritical {
			return StatusCritical
		}
	}

	for _, status := range statuses {
		if status == StatusError {
			return StatusError
		}
	}

	for _, status := range statuses {
		if status == StatusWarning {
			return StatusWarning
		}
	}

	return StatusHealthy
}
