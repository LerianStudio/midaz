package postgres

import "errors"

// Sentinel errors for database operations in reconciliation checks.
var (
	// Balance check errors
	ErrBalanceSummaryQuery = errors.New("balance summary query failed")
	ErrBalanceDetailQuery  = errors.New("balance detail query failed")
	ErrBalanceRowScan      = errors.New("balance row scan failed")
	ErrBalanceRowIteration = errors.New("balance row iteration failed")

	// Counts errors
	ErrOnboardingCountsQuery  = errors.New("onboarding counts query failed")
	ErrTransactionCountsQuery = errors.New("transaction counts query failed")

	// DLQ check errors
	ErrDLQSummaryQuery = errors.New("dlq summary query failed")
	ErrDLQDetailQuery  = errors.New("dlq detail query failed")
	ErrDLQRowScan      = errors.New("dlq row scan failed")
	ErrDLQRowIteration = errors.New("dlq row iteration failed")

	// Double-entry check errors
	ErrDoubleEntrySummaryQuery = errors.New("double-entry summary query failed")
	ErrDoubleEntryDetailQuery  = errors.New("double-entry detail query failed")
	ErrDoubleEntryRowScan      = errors.New("double-entry row scan failed")
	ErrDoubleEntryRowIteration = errors.New("double-entry row iteration failed")

	// Orphan check errors
	ErrOrphanSummaryQuery = errors.New("orphan summary query failed")
	ErrOrphanDetailQuery  = errors.New("orphan detail query failed")
	ErrOrphanRowScan      = errors.New("orphan row scan failed")
	ErrOrphanRowIteration = errors.New("orphan row iteration failed")

	// Referential check errors
	ErrReferentialOnboardingCheck  = errors.New("referential onboarding check failed")
	ErrReferentialTransactionCheck = errors.New("referential transaction check failed")
	ErrOnboardingQuery             = errors.New("onboarding query failed")
	ErrOnboardingRowScan           = errors.New("onboarding row scan failed")
	ErrOnboardingRowIteration      = errors.New("onboarding row iteration failed")
	ErrTransactionQuery            = errors.New("transaction query failed")
	ErrTransactionRowScan          = errors.New("transaction row scan failed")
	ErrTransactionRowIteration     = errors.New("transaction row iteration failed")

	// Settlement errors
	ErrUnsettledCountQuery = errors.New("unsettled count query failed")
	ErrSettledCountQuery   = errors.New("settled count query failed")

	// Sync check errors
	ErrSyncCheckQuery   = errors.New("sync check query failed")
	ErrSyncRowScan      = errors.New("sync row scan failed")
	ErrSyncRowIteration = errors.New("sync row iteration failed")
)
