// Package engine provides the reconciliation engine that orchestrates all reconciliation checks.
package engine

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	reconmetrics "github.com/LerianStudio/midaz/v3/components/reconciliation/internal/metrics"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/reportstore"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/pkg/safego"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	outboxCheck      *domain.OutboxCheckResult
	metadataCheck    *domain.MetadataCheckResult
	redisCheck       *domain.RedisCheckResult
	crossDBCheck     *domain.CrossDBCheckResult
	crmAliasCheck    *domain.CRMAliasCheckResult
	durationMs       int64
}

// checkerLabelMap maps checker names to their human-readable labels.
var checkerLabelMap = map[string]string{
	postgres.CheckerNameBalance:     "Balance",
	postgres.CheckerNameDoubleEntry: "Double-entry",
	postgres.CheckerNameOrphans:     "Orphan",
	postgres.CheckerNameReferential: "Referential",
	postgres.CheckerNameSync:        "Sync",
	postgres.CheckerNameDLQ:         "DLQ",
	postgres.CheckerNameOutbox:      "Outbox",
	postgres.CheckerNameMetadata:    "Metadata",
	postgres.CheckerNameRedis:       "Redis",
	postgres.CheckerNameCrossDB:     "CrossDB",
	postgres.CheckerNameCRMAlias:    "CRM Alias",
}

// errorCheckResultFactories maps checker names to their error result factory functions.
var errorCheckResultFactories = map[string]func() checkResult{
	postgres.CheckerNameBalance: func() checkResult {
		return checkResult{name: postgres.CheckerNameBalance, balanceCheck: &domain.BalanceCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameDoubleEntry: func() checkResult {
		return checkResult{name: postgres.CheckerNameDoubleEntry, doubleEntryCheck: &domain.DoubleEntryCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameOrphans: func() checkResult {
		return checkResult{name: postgres.CheckerNameOrphans, orphanCheck: &domain.OrphanCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameReferential: func() checkResult {
		return checkResult{name: postgres.CheckerNameReferential, referentialCheck: &domain.ReferentialCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameSync: func() checkResult {
		return checkResult{name: postgres.CheckerNameSync, syncCheck: &domain.SyncCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameDLQ: func() checkResult {
		return checkResult{name: postgres.CheckerNameDLQ, dlqCheck: &domain.DLQCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameOutbox: func() checkResult {
		return checkResult{name: postgres.CheckerNameOutbox, outboxCheck: &domain.OutboxCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameMetadata: func() checkResult {
		return checkResult{name: postgres.CheckerNameMetadata, metadataCheck: &domain.MetadataCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameRedis: func() checkResult {
		return checkResult{name: postgres.CheckerNameRedis, redisCheck: &domain.RedisCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameCrossDB: func() checkResult {
		return checkResult{name: postgres.CheckerNameCrossDB, crossDBCheck: &domain.CrossDBCheckResult{Status: domain.StatusError}}
	},
	postgres.CheckerNameCRMAlias: func() checkResult {
		return checkResult{name: postgres.CheckerNameCRMAlias, crmAliasCheck: &domain.CRMAliasCheckResult{Status: domain.StatusError}}
	},
}

// typedResultBuilders maps checker names to their typed result builder functions.
var typedResultBuilders = map[string]func(postgres.CheckResult, int64) (checkResult, bool){
	postgres.CheckerNameBalance: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameBalance, result, durationMs, func(r *checkResult, v *domain.BalanceCheckResult) { r.balanceCheck = v })
	},
	postgres.CheckerNameDoubleEntry: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameDoubleEntry, result, durationMs, func(r *checkResult, v *domain.DoubleEntryCheckResult) { r.doubleEntryCheck = v })
	},
	postgres.CheckerNameOrphans: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameOrphans, result, durationMs, func(r *checkResult, v *domain.OrphanCheckResult) { r.orphanCheck = v })
	},
	postgres.CheckerNameReferential: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameReferential, result, durationMs, func(r *checkResult, v *domain.ReferentialCheckResult) { r.referentialCheck = v })
	},
	postgres.CheckerNameSync: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameSync, result, durationMs, func(r *checkResult, v *domain.SyncCheckResult) { r.syncCheck = v })
	},
	postgres.CheckerNameDLQ: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameDLQ, result, durationMs, func(r *checkResult, v *domain.DLQCheckResult) { r.dlqCheck = v })
	},
	postgres.CheckerNameOutbox: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameOutbox, result, durationMs, func(r *checkResult, v *domain.OutboxCheckResult) { r.outboxCheck = v })
	},
	postgres.CheckerNameMetadata: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameMetadata, result, durationMs, func(r *checkResult, v *domain.MetadataCheckResult) { r.metadataCheck = v })
	},
	postgres.CheckerNameRedis: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameRedis, result, durationMs, func(r *checkResult, v *domain.RedisCheckResult) { r.redisCheck = v })
	},
	postgres.CheckerNameCrossDB: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameCrossDB, result, durationMs, func(r *checkResult, v *domain.CrossDBCheckResult) { r.crossDBCheck = v })
	},
	postgres.CheckerNameCRMAlias: func(result postgres.CheckResult, durationMs int64) (checkResult, bool) {
		return buildTypedResult(postgres.CheckerNameCRMAlias, result, durationMs, func(r *checkResult, v *domain.CRMAliasCheckResult) { r.crmAliasCheck = v })
	},
}

// reportAppliers maps checker names to functions that apply results to a report.
var reportAppliers = map[string]func(*domain.ReconciliationReport, checkResult){
	postgres.CheckerNameBalance:     func(r *domain.ReconciliationReport, res checkResult) { r.BalanceCheck = res.balanceCheck },
	postgres.CheckerNameDoubleEntry: func(r *domain.ReconciliationReport, res checkResult) { r.DoubleEntryCheck = res.doubleEntryCheck },
	postgres.CheckerNameOrphans:     func(r *domain.ReconciliationReport, res checkResult) { r.OrphanCheck = res.orphanCheck },
	postgres.CheckerNameReferential: func(r *domain.ReconciliationReport, res checkResult) { r.ReferentialCheck = res.referentialCheck },
	postgres.CheckerNameSync:        func(r *domain.ReconciliationReport, res checkResult) { r.SyncCheck = res.syncCheck },
	postgres.CheckerNameDLQ:         func(r *domain.ReconciliationReport, res checkResult) { r.DLQCheck = res.dlqCheck },
	postgres.CheckerNameOutbox:      func(r *domain.ReconciliationReport, res checkResult) { r.OutboxCheck = res.outboxCheck },
	postgres.CheckerNameMetadata:    func(r *domain.ReconciliationReport, res checkResult) { r.MetadataCheck = res.metadataCheck },
	postgres.CheckerNameRedis:       func(r *domain.ReconciliationReport, res checkResult) { r.RedisCheck = res.redisCheck },
	postgres.CheckerNameCrossDB:     func(r *domain.ReconciliationReport, res checkResult) { r.CrossDBCheck = res.crossDBCheck },
	postgres.CheckerNameCRMAlias:    func(r *domain.ReconciliationReport, res checkResult) { r.CRMAliasCheck = res.crmAliasCheck },
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
	reportStore           reportstore.Store
	metrics               *reconmetrics.ReconciliationMetrics

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
	reportStore reportstore.Store,
	metrics *reconmetrics.ReconciliationMetrics,
) *ReconciliationEngine {
	if checkerConfigs == nil {
		checkerConfigs = CheckerConfigMap{}
	}

	checkerMap := buildCheckerMap(checkers, logger)

	engine := &ReconciliationEngine{
		checkers:              checkerMap,
		checkerConfigs:        checkerConfigs,
		settlementDetector:    postgres.NewSettlementDetector(transactionDB),
		entityCounter:         postgres.NewEntityCounter(onboardingDB, transactionDB),
		onboardingMongo:       onboardingMongo,
		transactionMongo:      transactionMongo,
		logger:                logger,
		settlementWaitSeconds: settlementWaitSeconds,
		reportStore:           reportStore,
		metrics:               metrics,
	}

	loadLastReport(engine, reportStore)

	return engine
}

// buildCheckerMap creates a map of checkers from the provided slice.
func buildCheckerMap(checkers []postgres.ReconciliationChecker, logger Logger) map[string]postgres.ReconciliationChecker {
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

		if _, exists := checkerMap[name]; exists && logger != nil {
			logger.Warnf("Overriding checker with duplicate name: %s", name)
		}

		checkerMap[name] = checker
	}

	return checkerMap
}

// loadLastReport loads the last report from the store if available.
func loadLastReport(engine *ReconciliationEngine, reportStore reportstore.Store) {
	if reportStore == nil {
		return
	}

	if last, err := reportStore.LoadLatest(context.Background()); err == nil && last != nil {
		engine.lastReport = last
	}
}

// RunReconciliation executes all reconciliation checks
func (e *ReconciliationEngine) RunReconciliation(ctx context.Context) (*domain.ReconciliationReport, error) {
	startTime := time.Now()

	runID := uuid.NewString()

	e.logger.Info("Starting reconciliation run")

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only tracer needed here
	ctx, span := tracer.Start(ctx, "reconciliation.run")

	span.SetAttributes(attribute.String("reconciliation.run_id", runID))
	defer span.End()

	var previous *domain.ReconciliationReport

	e.mu.RLock()

	if e.lastReport != nil {
		previous = e.lastReport
	}

	e.mu.RUnlock()

	report := &domain.ReconciliationReport{
		RunID:        runID,
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

	if previous != nil {
		report.PreviousRunID = previous.RunID
		report.PreviousStatus = previous.Status
		report.StatusChanged = previous.Status != report.Status

		report.Delta = computeDelta(previous, report)
		if report.StatusChanged && e.logger != nil {
			e.logger.Infof("Reconciliation status changed: %s -> %s", previous.Status, report.Status)
		}
	}

	// Store last report
	e.mu.Lock()
	e.lastReport = report
	e.mu.Unlock()

	e.logger.Infof("Reconciliation complete: status=%s, duration=%s", report.Status, report.Duration)

	if e.metrics != nil {
		e.metrics.RecordRun(ctx, report, time.Since(startTime).Milliseconds())
	}

	if e.reportStore != nil {
		if err := e.reportStore.Save(ctx, report); err != nil && e.logger != nil {
			e.logger.Errorf("Failed to persist reconciliation report: %v", err)
		}
	}

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

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only tracer needed here

	resultCh := make(chan checkResult, checkerCount)

	var wg sync.WaitGroup
	wg.Add(checkerCount)

	for name, checker := range e.checkers {
		checkerName := name
		checkerInstance := checker

		safego.Go(e.logger, checkerName+"-check", func() {
			defer wg.Done()

			start := time.Now()
			checkCtx, span := tracer.Start(ctx, "reconciliation.check",
				trace.WithAttributes(attribute.String("reconciliation.check_name", checkerName)),
			)

			cfg := e.checkerConfigs[checkerName]

			result, err := checkerInstance.Check(checkCtx, cfg)
			if err != nil {
				span.RecordError(err)
			}

			span.End()

			durationMs := time.Since(start).Milliseconds()
			resultCh <- e.buildCheckResult(checkerName, result, err, durationMs)
		})
	}

	wg.Wait()
	close(resultCh)

	// Drain channel and populate report on single goroutine (no race)
	for res := range resultCh {
		e.applyCheckResult(report, res)

		if res.durationMs > 0 {
			if report.CheckDurations == nil {
				report.CheckDurations = make(map[string]int64, checkerCount)
			}

			report.CheckDurations[res.name] = res.durationMs
		}
	}
}

// buildTypedCheckResult attempts to build a typed check result for the given name and result.
// Returns the check result and true if successful, otherwise returns empty result and false.
func buildTypedCheckResult(name string, result postgres.CheckResult, durationMs int64) (checkResult, bool) {
	if builder, ok := typedResultBuilders[name]; ok {
		return builder(result, durationMs)
	}

	return checkResult{}, false
}

func (e *ReconciliationEngine) buildCheckResult(name string, result postgres.CheckResult, err error, durationMs int64) checkResult {
	if err != nil {
		e.logCheckError(name, err)
		res := errorCheckResult(name)
		res.durationMs = durationMs

		return res
	}

	if result == nil {
		if e.logger != nil {
			e.logger.Errorf("%s check returned nil result", checkerLabel(name))
		}

		res := errorCheckResult(name)
		res.durationMs = durationMs

		return res
	}

	if res, ok := buildTypedCheckResult(name, result, durationMs); ok {
		return res
	}

	// Handle unknown checker or type mismatch
	if e.logger != nil {
		e.logger.Warnf("Unknown checker name or type mismatch: %s", name)
	}

	res := e.unexpectedResult(name, result)
	res.durationMs = durationMs

	return res
}

func buildTypedResult[T postgres.CheckResult](
	name string,
	result postgres.CheckResult,
	durationMs int64,
	set func(*checkResult, T),
) (checkResult, bool) {
	typed, ok := result.(T)
	if !ok {
		return checkResult{}, false
	}

	res := checkResult{name: name, durationMs: durationMs}
	set(&res, typed)

	return res, true
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
	if applier, ok := reportAppliers[res.name]; ok {
		applier(report, res)
	}
}

// setRequiredCheckDefaults sets error status for required checks that are nil.
func setRequiredCheckDefaults(report *domain.ReconciliationReport) {
	if report.BalanceCheck == nil {
		report.BalanceCheck = &domain.BalanceCheckResult{Status: domain.StatusError}
	}

	if report.DoubleEntryCheck == nil {
		report.DoubleEntryCheck = &domain.DoubleEntryCheckResult{Status: domain.StatusError}
	}

	if report.OrphanCheck == nil {
		report.OrphanCheck = &domain.OrphanCheckResult{Status: domain.StatusError}
	}

	if report.ReferentialCheck == nil {
		report.ReferentialCheck = &domain.ReferentialCheckResult{Status: domain.StatusError}
	}

	if report.SyncCheck == nil {
		report.SyncCheck = &domain.SyncCheckResult{Status: domain.StatusError}
	}
}

// checkerStatus returns StatusError if checker exists, StatusSkipped otherwise.
func (e *ReconciliationEngine) checkerStatus(name string) domain.ReconciliationStatus {
	if _, ok := e.checkers[name]; ok {
		return domain.StatusError
	}

	return domain.StatusSkipped
}

// setOptionalCheckDefaults sets status for optional checks that are nil.
func (e *ReconciliationEngine) setOptionalCheckDefaults(report *domain.ReconciliationReport) {
	if report.MetadataCheck == nil {
		report.MetadataCheck = &domain.MetadataCheckResult{Status: e.checkerStatus(postgres.CheckerNameMetadata)}
	}

	if report.DLQCheck == nil {
		report.DLQCheck = &domain.DLQCheckResult{Status: e.checkerStatus(postgres.CheckerNameDLQ)}
	}

	if report.OutboxCheck == nil {
		report.OutboxCheck = &domain.OutboxCheckResult{Status: e.checkerStatus(postgres.CheckerNameOutbox)}
	}

	if report.RedisCheck == nil {
		report.RedisCheck = &domain.RedisCheckResult{Status: e.checkerStatus(postgres.CheckerNameRedis)}
	}

	if report.CrossDBCheck == nil {
		report.CrossDBCheck = &domain.CrossDBCheckResult{Status: e.checkerStatus(postgres.CheckerNameCrossDB)}
	}

	if report.CRMAliasCheck == nil {
		report.CRMAliasCheck = &domain.CRMAliasCheckResult{Status: e.checkerStatus(postgres.CheckerNameCRMAlias)}
	}
}

func (e *ReconciliationEngine) setCheckDefaults(report *domain.ReconciliationReport) {
	setRequiredCheckDefaults(report)
	e.setOptionalCheckDefaults(report)
}

func checkerLabel(name string) string {
	if label, ok := checkerLabelMap[name]; ok {
		return label
	}

	return name
}

func errorCheckResult(name string) checkResult {
	if factory, ok := errorCheckResultFactories[name]; ok {
		return factory()
	}

	return checkResult{name: name}
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

// computeReferentialOrphans calculates total orphans from referential check result.
func computeReferentialOrphans(r *domain.ReferentialCheckResult) int {
	return r.OrphanLedgers +
		r.OrphanAssets +
		r.OrphanAccounts +
		r.OrphanOperations +
		r.OrphanPortfolios +
		r.OrphanUnknown +
		r.OperationsWithoutBalance
}

// computeRedisMismatches calculates total mismatches from redis check result.
func computeRedisMismatches(r *domain.RedisCheckResult) int {
	return r.MissingRedis + r.ValueMismatches + r.VersionMismatches
}

// computeCoreDelta computes delta for balance, double-entry, and orphan checks.
func computeCoreDelta(prev, curr *domain.ReconciliationReport, delta *domain.ReconciliationDelta) {
	if prev.BalanceCheck != nil && curr.BalanceCheck != nil {
		delta.BalanceDiscrepancies = curr.BalanceCheck.BalancesWithDiscrepancy - prev.BalanceCheck.BalancesWithDiscrepancy
	}

	if prev.DoubleEntryCheck != nil && curr.DoubleEntryCheck != nil {
		delta.DoubleEntryUnbalanced = curr.DoubleEntryCheck.UnbalancedTransactions - prev.DoubleEntryCheck.UnbalancedTransactions
	}

	if prev.OrphanCheck != nil && curr.OrphanCheck != nil {
		delta.OrphanTransactions = curr.OrphanCheck.OrphanTransactions - prev.OrphanCheck.OrphanTransactions
	}

	if prev.ReferentialCheck != nil && curr.ReferentialCheck != nil {
		delta.ReferentialOrphans = computeReferentialOrphans(curr.ReferentialCheck) - computeReferentialOrphans(prev.ReferentialCheck)
	}
}

// computeInfraDelta computes delta for outbox, DLQ, and redis checks.
func computeInfraDelta(prev, curr *domain.ReconciliationReport, delta *domain.ReconciliationDelta) {
	if prev.OutboxCheck != nil && curr.OutboxCheck != nil {
		delta.OutboxPending = int(curr.OutboxCheck.Pending - prev.OutboxCheck.Pending)
		delta.OutboxFailed = int(curr.OutboxCheck.Failed - prev.OutboxCheck.Failed)
	}

	if prev.DLQCheck != nil && curr.DLQCheck != nil {
		delta.DLQEntries = int(curr.DLQCheck.Total - prev.DLQCheck.Total)
	}

	if prev.RedisCheck != nil && curr.RedisCheck != nil {
		delta.RedisMismatches = computeRedisMismatches(curr.RedisCheck) - computeRedisMismatches(prev.RedisCheck)
	}
}

func computeDelta(prev, curr *domain.ReconciliationReport) *domain.ReconciliationDelta {
	if prev == nil || curr == nil {
		return nil
	}

	delta := &domain.ReconciliationDelta{}
	computeCoreDelta(prev, curr, delta)
	computeInfraDelta(prev, curr, delta)

	return delta
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
