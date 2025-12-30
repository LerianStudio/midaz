// Package engine provides the reconciliation engine that orchestrates all reconciliation checks.
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
	Info(args ...any)
	Infof(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Warnf(format string, args ...any)
}

// CheckerConfigMap maps checker names to their configuration.
type CheckerConfigMap map[string]postgres.CheckerConfig

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
	checkers           map[string]postgres.ReconciliationChecker
	checkerConfigs     CheckerConfigMap
	settlementDetector *postgres.SettlementDetector
	entityCounter      *postgres.EntityCounter

	// MongoDB
	onboardingMongo  *mongo.Database
	transactionMongo *mongo.Database

	// Config
	logger                Logger
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
	settlementWaitSeconds int,
	checkers []postgres.ReconciliationChecker,
	checkerConfigs CheckerConfigMap,
) *ReconciliationEngine {
	if checkerConfigs == nil {
		checkerConfigs = CheckerConfigMap{}
	}

	checkerMap := make(map[string]postgres.ReconciliationChecker, len(checkers))
	for _, checker := range checkers {
		if checker == nil {
			continue
		}

		name := checker.Name()
		if name == "" {
			if logger != nil {
				logger.Warnf("Ignoring checker with empty name")
			}
			continue
		}

		if _, exists := checkerMap[name]; exists {
			if logger != nil {
				logger.Warnf("Overriding checker with duplicate name: %s", name)
			}
		}

		checkerMap[name] = checker
	}

	return &ReconciliationEngine{
		checkers:              checkerMap,
		checkerConfigs:        checkerConfigs,
		settlementDetector:    postgres.NewSettlementDetector(transactionDB),
		entityCounter:         postgres.NewEntityCounter(onboardingDB, transactionDB),
		onboardingMongo:       onboardingMongo,
		transactionMongo:      transactionMongo,
		logger:                logger,
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

	// Get settlement info and entity counts
	e.collectSettlementInfo(ctx, report)
	e.collectEntityCounts(ctx, report)

	// Run all checks in parallel
	e.runParallelChecks(ctx, report)

	// Set defaults for nil checks
	e.setCheckDefaults(report)

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

func (e *ReconciliationEngine) collectSettlementInfo(ctx context.Context, report *domain.ReconciliationReport) {
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
}

func (e *ReconciliationEngine) runParallelChecks(ctx context.Context, report *domain.ReconciliationReport) {
	checkerCount := len(e.checkers)
	if checkerCount == 0 {
		return
	}

	resultCh := make(chan checkResult, checkerCount)

	var wg sync.WaitGroup
	wg.Add(checkerCount)

	for name, checker := range e.checkers {
		checkerName := name
		checkerInstance := checker

		safego.Go(e.logger, checkerName+"-check", func() {
			defer wg.Done()

			cfg := e.checkerConfigs[checkerName]
			result, err := checkerInstance.Check(ctx, cfg)
			resultCh <- e.buildCheckResult(checkerName, result, err)
		})
	}

	wg.Wait()
	close(resultCh)

	// Drain channel and populate report on single goroutine (no race)
	for res := range resultCh {
		e.applyCheckResult(report, res)
	}
}

func (e *ReconciliationEngine) buildCheckResult(name string, result postgres.CheckResult, err error) checkResult {
	if err != nil {
		e.logCheckError(name, err)
		return errorCheckResult(name)
	}

	if result == nil {
		if e.logger != nil {
			e.logger.Errorf("%s check returned nil result", checkerLabel(name))
		}
		return errorCheckResult(name)
	}

	switch name {
	case postgres.CheckerNameBalance:
		typedResult, ok := result.(*domain.BalanceCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, balanceCheck: typedResult}
	case postgres.CheckerNameDoubleEntry:
		typedResult, ok := result.(*domain.DoubleEntryCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, doubleEntryCheck: typedResult}
	case postgres.CheckerNameOrphans:
		typedResult, ok := result.(*domain.OrphanCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, orphanCheck: typedResult}
	case postgres.CheckerNameReferential:
		typedResult, ok := result.(*domain.ReferentialCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, referentialCheck: typedResult}
	case postgres.CheckerNameSync:
		typedResult, ok := result.(*domain.SyncCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, syncCheck: typedResult}
	case postgres.CheckerNameDLQ:
		typedResult, ok := result.(*domain.DLQCheckResult)
		if !ok {
			return e.unexpectedResult(name, result)
		}
		return checkResult{name: name, dlqCheck: typedResult}
	default:
		if e.logger != nil {
			e.logger.Warnf("Unknown checker name: %s", name)
		}
		return checkResult{name: name}
	}
}

func (e *ReconciliationEngine) unexpectedResult(name string, result postgres.CheckResult) checkResult {
	if e.logger != nil {
		e.logger.Errorf("%s check returned unexpected result type: %T", checkerLabel(name), result)
	}
	return errorCheckResult(name)
}

func (e *ReconciliationEngine) logCheckError(name string, err error) {
	if e.logger != nil {
		e.logger.Errorf("%s check failed: %v", checkerLabel(name), err)
	}
}

func (e *ReconciliationEngine) applyCheckResult(report *domain.ReconciliationReport, res checkResult) {
	switch res.name {
	case postgres.CheckerNameBalance:
		report.BalanceCheck = res.balanceCheck
	case postgres.CheckerNameDoubleEntry:
		report.DoubleEntryCheck = res.doubleEntryCheck
	case postgres.CheckerNameOrphans:
		report.OrphanCheck = res.orphanCheck
	case postgres.CheckerNameReferential:
		report.ReferentialCheck = res.referentialCheck
	case postgres.CheckerNameSync:
		report.SyncCheck = res.syncCheck
	case postgres.CheckerNameDLQ:
		report.DLQCheck = res.dlqCheck
	}
}

func (e *ReconciliationEngine) setCheckDefaults(report *domain.ReconciliationReport) {
	if report.MetadataCheck == nil {
		report.MetadataCheck = &domain.MetadataCheckResult{Status: domain.StatusSkipped}
	}

	// DLQCheck is usually set by the checker. This fallback is defensive (e.g. missing checker).
	if report.DLQCheck == nil {
		report.DLQCheck = &domain.DLQCheckResult{Status: domain.StatusSkipped}
	}
}

func checkerLabel(name string) string {
	switch name {
	case postgres.CheckerNameBalance:
		return "Balance"
	case postgres.CheckerNameDoubleEntry:
		return "Double-entry"
	case postgres.CheckerNameOrphans:
		return "Orphan"
	case postgres.CheckerNameReferential:
		return "Referential"
	case postgres.CheckerNameSync:
		return "Sync"
	case postgres.CheckerNameDLQ:
		return "DLQ"
	default:
		return name
	}
}

func errorCheckResult(name string) checkResult {
	switch name {
	case postgres.CheckerNameBalance:
		return checkResult{name: name, balanceCheck: &domain.BalanceCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameDoubleEntry:
		return checkResult{name: name, doubleEntryCheck: &domain.DoubleEntryCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameOrphans:
		return checkResult{name: name, orphanCheck: &domain.OrphanCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameReferential:
		return checkResult{name: name, referentialCheck: &domain.ReferentialCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameSync:
		return checkResult{name: name, syncCheck: &domain.SyncCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameDLQ:
		return checkResult{name: name, dlqCheck: &domain.DLQCheckResult{Status: domain.StatusError}}
	default:
		return checkResult{name: name}
	}
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
		report.EntityCounts.Segments = onboarding.Segments
	}

	// Transaction counts
	transaction, err := e.entityCounter.GetTransactionCounts(ctx)
	if err != nil {
		e.logger.Warnf("Failed to get transaction counts: %v", err)
	} else {
		report.EntityCounts.Transactions = transaction.Transactions
		report.EntityCounts.Operations = transaction.Operations
		report.EntityCounts.Balances = transaction.Balances
		report.EntityCounts.AssetRates = transaction.AssetRates
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
