// Package domain contains the domain models and business logic for reconciliation.
package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

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
	RunID     string               `json:"run_id"`
	Timestamp time.Time            `json:"timestamp"`
	Duration  string               `json:"duration"`
	Status    ReconciliationStatus `json:"status"`
	// Previous status info (if available)
	PreviousRunID  string               `json:"previous_run_id,omitempty"`
	PreviousStatus ReconciliationStatus `json:"previous_status,omitempty"`
	StatusChanged  bool                 `json:"status_changed"`

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
	OutboxCheck      *OutboxCheckResult      `json:"outbox_check"`
	RedisCheck       *RedisCheckResult       `json:"redis_check"`
	CrossDBCheck     *CrossDBCheckResult     `json:"cross_db_check"`
	CRMAliasCheck    *CRMAliasCheckResult    `json:"crm_alias_check"`

	// Entity counts
	EntityCounts *EntityCounts `json:"entity_counts"`

	// Check execution durations by checker name (ms)
	CheckDurations map[string]int64 `json:"check_durations,omitempty"`

	// Delta from previous run (if available)
	Delta *ReconciliationDelta `json:"delta,omitempty"`
}

// BalanceCheckResult holds balance consistency results
type BalanceCheckResult struct {
	TotalBalances            int                  `json:"total_balances"`
	BalancesWithDiscrepancy  int                  `json:"balances_with_discrepancy"`
	DiscrepancyPercentage    float64              `json:"discrepancy_percentage"`
	TotalAbsoluteDiscrepancy decimal.Decimal      `json:"total_absolute_discrepancy"`
	Discrepancies            []BalanceDiscrepancy `json:"discrepancies,omitempty"`
	// On-hold validation
	OnHoldWithDiscrepancy  int                 `json:"on_hold_with_discrepancy"`
	OnHoldDiscrepancyPct   float64             `json:"on_hold_discrepancy_percentage"`
	TotalOnHoldDiscrepancy decimal.Decimal     `json:"total_on_hold_discrepancy"`
	OnHoldDiscrepancies    []OnHoldDiscrepancy `json:"on_hold_discrepancies,omitempty"`
	// Negative balances
	NegativeAvailable int                  `json:"negative_available"`
	NegativeOnHold    int                  `json:"negative_on_hold"`
	NegativeBalances  []NegativeBalance    `json:"negative_balances,omitempty"`
	Status            ReconciliationStatus `json:"status"`
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
	BalanceID       string          `json:"balance_id"`
	AccountID       string          `json:"account_id"`
	Alias           string          `json:"alias"`
	AssetCode       string          `json:"asset_code"`
	CurrentBalance  decimal.Decimal `json:"current_balance"`
	ExpectedBalance decimal.Decimal `json:"expected_balance"`
	Discrepancy     decimal.Decimal `json:"discrepancy"`
	OperationCount  int64           `json:"operation_count"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// OnHoldDiscrepancy represents an on-hold balance mismatch.
type OnHoldDiscrepancy struct {
	BalanceID      string          `json:"balance_id"`
	AccountID      string          `json:"account_id"`
	Alias          string          `json:"alias"`
	AssetCode      string          `json:"asset_code"`
	CurrentOnHold  decimal.Decimal `json:"current_on_hold"`
	ExpectedOnHold decimal.Decimal `json:"expected_on_hold"`
	Discrepancy    decimal.Decimal `json:"discrepancy"`
	OperationCount int64           `json:"operation_count"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// NegativeBalance represents a balance with negative values.
type NegativeBalance struct {
	BalanceID string          `json:"balance_id"`
	AccountID string          `json:"account_id"`
	Alias     string          `json:"alias"`
	AssetCode string          `json:"asset_code"`
	Available decimal.Decimal `json:"available"`
	OnHold    decimal.Decimal `json:"on_hold"`
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
	OrphanLedgers            int                  `json:"orphan_ledgers"`
	OrphanAssets             int                  `json:"orphan_assets"`
	OrphanAccounts           int                  `json:"orphan_accounts"`
	OrphanOperations         int                  `json:"orphan_operations"`
	OrphanPortfolios         int                  `json:"orphan_portfolios"`
	OperationsWithoutBalance int                  `json:"operations_without_balance"`
	OrphanUnknown            int                  `json:"orphan_unknown"`
	Orphans                  []OrphanEntity       `json:"orphans,omitempty"`
	Status                   ReconciliationStatus `json:"status"`
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
	PostgreSQLCount       int64                       `json:"postgresql_count"`
	MongoDBCount          int64                       `json:"mongodb_count"`
	MissingCount          int64                       `json:"missing_count"`
	MissingEntityIDs      int                         `json:"missing_entity_ids"`
	DuplicateEntityIDs    int                         `json:"duplicate_entity_ids"`
	MissingRequiredFields int                         `json:"missing_required_fields"`
	EmptyMetadata         int                         `json:"empty_metadata"`
	ScanLimited           bool                        `json:"scan_limited"`
	CollectionSummaries   []MetadataCollectionSummary `json:"collection_summaries,omitempty"`
	MissingEntities       []MetadataMissingEntity     `json:"missing_entities,omitempty"`
	Status                ReconciliationStatus        `json:"status"`
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

// OutboxCheckResult provides metadata outbox health beyond DLQ.
type OutboxCheckResult struct {
	Pending         int64                `json:"pending"`
	Processing      int64                `json:"processing"`
	Failed          int64                `json:"failed"`
	DLQ             int64                `json:"dlq"`
	StaleProcessing int64                `json:"stale_processing"`
	MaxRetryCount   int64                `json:"max_retry_count"`
	OldestPendingAt *time.Time           `json:"oldest_pending_at,omitempty"`
	OldestFailedAt  *time.Time           `json:"oldest_failed_at,omitempty"`
	Entries         []OutboxEntry        `json:"entries,omitempty"`
	Status          ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *OutboxCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// OutboxEntry represents a single metadata outbox entry (summary view).
type OutboxEntry struct {
	ID         string    `json:"id"`
	EntityID   string    `json:"entity_id"`
	EntityType string    `json:"entity_type"`
	Status     string    `json:"status"`
	RetryCount int       `json:"retry_count"`
	LastError  string    `json:"last_error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// RedisCheckResult compares Redis and PostgreSQL balance snapshots.
type RedisCheckResult struct {
	SampledBalances   int                       `json:"sampled_balances"`
	MissingRedis      int                       `json:"missing_redis"`
	ValueMismatches   int                       `json:"value_mismatches"`
	VersionMismatches int                       `json:"version_mismatches"`
	Discrepancies     []RedisBalanceDiscrepancy `json:"discrepancies,omitempty"`
	SyncQueueDepth    int64                     `json:"sync_queue_depth"`
	OldestSyncScore   int64                     `json:"oldest_sync_score"`
	OldestSyncAt      *time.Time                `json:"oldest_sync_at,omitempty"`
	Status            ReconciliationStatus      `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *RedisCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// RedisBalanceDiscrepancy captures Redis vs DB mismatch for a balance.
type RedisBalanceDiscrepancy struct {
	BalanceID      string          `json:"balance_id"`
	AccountID      string          `json:"account_id"`
	Alias          string          `json:"alias"`
	AssetCode      string          `json:"asset_code"`
	Key            string          `json:"key"`
	DBAvailable    decimal.Decimal `json:"db_available"`
	DBOnHold       decimal.Decimal `json:"db_on_hold"`
	DBVersion      int64           `json:"db_version"`
	RedisAvailable decimal.Decimal `json:"redis_available"`
	RedisOnHold    decimal.Decimal `json:"redis_on_hold"`
	RedisVersion   int64           `json:"redis_version"`
}

// CrossDBCheckResult validates transaction references against onboarding records.
type CrossDBCheckResult struct {
	MissingAccounts      int                  `json:"missing_accounts"`
	MissingLedgers       int                  `json:"missing_ledgers"`
	MissingOrganizations int                  `json:"missing_organizations"`
	Samples              []CrossDBMissing     `json:"samples,omitempty"`
	ScanLimited          bool                 `json:"scan_limited"`
	Status               ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *CrossDBCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// CrossDBMissing captures a cross-database missing reference.
type CrossDBMissing struct {
	Source   string `json:"source"`
	EntityID string `json:"entity_id"`
	RefType  string `json:"ref_type"`
	RefID    string `json:"ref_id"`
}

// CRMAliasCheckResult validates CRM alias references to ledger/account.
type CRMAliasCheckResult struct {
	MissingLedgerIDs  int                  `json:"missing_ledger_ids"`
	MissingAccountIDs int                  `json:"missing_account_ids"`
	Samples           []CRMAliasMissing    `json:"samples,omitempty"`
	ScanLimited       bool                 `json:"scan_limited"`
	Status            ReconciliationStatus `json:"status"`
}

// GetStatus returns the reconciliation status for this check result.
func (r *CRMAliasCheckResult) GetStatus() ReconciliationStatus {
	if r == nil {
		return StatusUnknown
	}

	return r.Status
}

// CRMAliasMissing captures a CRM alias with missing references.
type CRMAliasMissing struct {
	AliasID   string `json:"alias_id"`
	LedgerID  string `json:"ledger_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Issue     string `json:"issue"`
}

// MetadataCollectionSummary captures basic collection-level metadata stats.
type MetadataCollectionSummary struct {
	Collection            string           `json:"collection"`
	TotalDocuments        int64            `json:"total_documents"`
	EmptyMetadata         int64            `json:"empty_metadata"`
	MissingEntityIDs      int64            `json:"missing_entity_ids"`
	DuplicateEntityIDs    int64            `json:"duplicate_entity_ids"`
	MissingRequiredFields map[string]int64 `json:"missing_required_fields,omitempty"`
}

// MetadataMissingEntity captures an outbox entity missing in MongoDB.
type MetadataMissingEntity struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
	Reason     string `json:"reason"`
}

// EntityCounts holds PostgreSQL entity counts
type EntityCounts struct {
	// Onboarding
	Organizations int64 `json:"organizations"`
	Ledgers       int64 `json:"ledgers"`
	Assets        int64 `json:"assets"`
	Accounts      int64 `json:"accounts"`
	Portfolios    int64 `json:"portfolios"`
	Segments      int64 `json:"segments"`

	// Transaction
	Transactions int64 `json:"transactions"`
	Operations   int64 `json:"operations"`
	Balances     int64 `json:"balances"`
	AssetRates   int64 `json:"asset_rates"`
}

// ReconciliationDelta captures change from previous run.
type ReconciliationDelta struct {
	BalanceDiscrepancies  int `json:"balance_discrepancies"`
	DoubleEntryUnbalanced int `json:"double_entry_unbalanced"`
	OrphanTransactions    int `json:"orphan_transactions"`
	ReferentialOrphans    int `json:"referential_orphans"`
	OutboxPending         int `json:"outbox_pending"`
	OutboxFailed          int `json:"outbox_failed"`
	DLQEntries            int `json:"dlq_entries"`
	RedisMismatches       int `json:"redis_mismatches"`
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

	if r.OutboxCheck != nil {
		statuses = append(statuses, r.OutboxCheck.GetStatus())
	}

	if r.RedisCheck != nil {
		statuses = append(statuses, r.RedisCheck.GetStatus())
	}

	if r.CrossDBCheck != nil {
		statuses = append(statuses, r.CrossDBCheck.GetStatus())
	}

	if r.CRMAliasCheck != nil {
		statuses = append(statuses, r.CRMAliasCheck.GetStatus())
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
