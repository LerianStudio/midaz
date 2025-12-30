package engine

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/pkg/safego"
)

// Logger interface
type Logger interface {
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
}

// checkResult holds the result of a single reconciliation check for channel-based collection
type checkResult struct {
	name             string
	balanceCheck     *domain.BalanceCheckResult
	doubleEntryCheck *domain.DoubleEntryCheckResult
	orphanCheck      *domain.OrphanCheckResult
	referentialCheck *domain.ReferentialCheckResult
	syncCheck        *domain.SyncCheckResult
	dlqCheck         *domain.DLQCheckResult
}

// ReconciliationEngine orchestrates all reconciliation checks
type ReconciliationEngine struct {
	// Checkers
	balanceChecker     *postgres.BalanceChecker
	doubleEntryChecker *postgres.DoubleEntryChecker
	orphanChecker      *postgres.OrphanChecker
	referentialChecker *postgres.ReferentialChecker
	syncChecker        *postgres.SyncChecker
	dlqChecker         *postgres.DLQChecker
	settlementDetector *postgres.SettlementDetector
	entityCounter      *postgres.EntityCounter

	// MongoDB
	onboardingMongo  *mongo.Database
	transactionMongo *mongo.Database

	// Config
	logger                Logger
	discrepancyThreshold  int64
	maxDiscrepancies      int
	settlementWaitSeconds int

	// State
	lastReport *domain.ReconciliationReport
	mu         sync.RWMutex
}

// NewReconciliationEngine creates a new engine
func NewReconciliationEngine(
	onboardingDB, transactionDB *sql.DB,
	onboardingMongo, transactionMongo *mongo.Database,
	logger Logger,
	discrepancyThreshold int64,
	maxDiscrepancies int,
	settlementWaitSeconds int,
) *ReconciliationEngine {
	return &ReconciliationEngine{
		balanceChecker:        postgres.NewBalanceChecker(transactionDB),
		doubleEntryChecker:    postgres.NewDoubleEntryChecker(transactionDB),
		orphanChecker:         postgres.NewOrphanChecker(transactionDB),
		referentialChecker:    postgres.NewReferentialChecker(onboardingDB, transactionDB),
		syncChecker:           postgres.NewSyncChecker(transactionDB),
		dlqChecker:            postgres.NewDLQChecker(transactionDB),
		settlementDetector:    postgres.NewSettlementDetector(transactionDB),
		entityCounter:         postgres.NewEntityCounter(onboardingDB, transactionDB),
		onboardingMongo:       onboardingMongo,
		transactionMongo:      transactionMongo,
		logger:                logger,
		discrepancyThreshold:  discrepancyThreshold,
		maxDiscrepancies:      maxDiscrepancies,
		settlementWaitSeconds: settlementWaitSeconds,
	}
}

// RunReconciliation executes all reconciliation checks
func (e *ReconciliationEngine) RunReconciliation(ctx context.Context) (*domain.ReconciliationReport, error) {
	startTime := time.Now()
	e.logger.Info("Starting reconciliation run")

	report := &domain.ReconciliationReport{
		Timestamp:    startTime,
		EntityCounts: &domain.EntityCounts{},
	}

	// Get settlement info
	unsettled, err := e.settlementDetector.GetUnsettledCount(ctx, e.settlementWaitSeconds)
	if err != nil {
		e.logger.Warnf("Failed to get unsettled count: %v", err)
	} else {
		report.UnsettledTransactions = unsettled
	}

	settled, err := e.settlementDetector.GetSettledCount(ctx, e.settlementWaitSeconds)
	if err != nil {
		e.logger.Warnf("Failed to get settled count: %v", err)
	} else {
		report.SettledTransactions = settled
	}

	// Get entity counts
	e.collectEntityCounts(ctx, report)

	// Run all checks in parallel with panic recovery
	// Use channel-based collection to avoid data race on report fields
	const numChecks = 6
	resultCh := make(chan checkResult, numChecks)

	var wg sync.WaitGroup
	wg.Add(numChecks)

	safego.Go(e.logger, "balance-check", func() {
		defer wg.Done()
		result, err := e.balanceChecker.Check(ctx, e.discrepancyThreshold, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Balance check failed: %v", err)
			resultCh <- checkResult{name: "balance", balanceCheck: &domain.BalanceCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "balance", balanceCheck: result}
		}
	})

	safego.Go(e.logger, "double-entry-check", func() {
		defer wg.Done()
		result, err := e.doubleEntryChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Double-entry check failed: %v", err)
			resultCh <- checkResult{name: "double-entry", doubleEntryCheck: &domain.DoubleEntryCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "double-entry", doubleEntryCheck: result}
		}
	})

	safego.Go(e.logger, "orphan-check", func() {
		defer wg.Done()
		result, err := e.orphanChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Orphan check failed: %v", err)
			resultCh <- checkResult{name: "orphan", orphanCheck: &domain.OrphanCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "orphan", orphanCheck: result}
		}
	})

	safego.Go(e.logger, "referential-check", func() {
		defer wg.Done()
		result, err := e.referentialChecker.Check(ctx)
		if err != nil {
			e.logger.Errorf("Referential check failed: %v", err)
			resultCh <- checkResult{name: "referential", referentialCheck: &domain.ReferentialCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "referential", referentialCheck: result}
		}
	})

	safego.Go(e.logger, "sync-check", func() {
		defer wg.Done()
		result, err := e.syncChecker.Check(ctx, e.settlementWaitSeconds, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("Sync check failed: %v", err)
			resultCh <- checkResult{name: "sync", syncCheck: &domain.SyncCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "sync", syncCheck: result}
		}
	})

	safego.Go(e.logger, "dlq-check", func() {
		defer wg.Done()
		result, err := e.dlqChecker.Check(ctx, e.maxDiscrepancies)
		if err != nil {
			e.logger.Errorf("DLQ check failed: %v", err)
			resultCh <- checkResult{name: "dlq", dlqCheck: &domain.DLQCheckResult{Status: domain.StatusError}}
		} else {
			resultCh <- checkResult{name: "dlq", dlqCheck: result}
		}
	})

	wg.Wait()
	close(resultCh)

	// Drain channel and populate report on single goroutine (no race)
	for res := range resultCh {
		switch res.name {
		case "balance":
			report.BalanceCheck = res.balanceCheck
		case "double-entry":
			report.DoubleEntryCheck = res.doubleEntryCheck
		case "orphan":
			report.OrphanCheck = res.orphanCheck
		case "referential":
			report.ReferentialCheck = res.referentialCheck
		case "sync":
			report.SyncCheck = res.syncCheck
		case "dlq":
			report.DLQCheck = res.dlqCheck
		}
	}

	// Set defaults for nil checks
	if report.MetadataCheck == nil {
		report.MetadataCheck = &domain.MetadataCheckResult{Status: domain.StatusSkipped}
	}

	if report.DLQCheck == nil {
		report.DLQCheck = &domain.DLQCheckResult{Status: domain.StatusSkipped}
	}

	// Determine overall status
	report.DetermineOverallStatus()
	report.Duration = time.Since(startTime).String()

	// Store last report
	e.mu.Lock()
	e.lastReport = report
	e.mu.Unlock()

	e.logger.Infof("Reconciliation complete: status=%s, duration=%s", report.Status, report.Duration)
	return report, nil
}

func (e *ReconciliationEngine) collectEntityCounts(ctx context.Context, report *domain.ReconciliationReport) {
	// Onboarding counts
	onboarding, err := e.entityCounter.GetOnboardingCounts(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get onboarding counts: %v", err)
	} else {
		report.EntityCounts.Organizations = onboarding.Organizations
		report.EntityCounts.Ledgers = onboarding.Ledgers
		report.EntityCounts.Assets = onboarding.Assets
		report.EntityCounts.Accounts = onboarding.Accounts
		report.EntityCounts.Portfolios = onboarding.Portfolios
	}

	// Transaction counts
	transaction, err := e.entityCounter.GetTransactionCounts(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get transaction counts: %v", err)
	} else {
		report.EntityCounts.Transactions = transaction.Transactions
		report.EntityCounts.Operations = transaction.Operations
		report.EntityCounts.Balances = transaction.Balances
	}
}

// GetLastReport returns the most recent reconciliation report
func (e *ReconciliationEngine) GetLastReport() *domain.ReconciliationReport {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastReport
}

// IsHealthy returns true if the last report showed no critical issues
func (e *ReconciliationEngine) IsHealthy() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.lastReport == nil {
		return false
	}
	return e.lastReport.Status != domain.StatusCritical
}
