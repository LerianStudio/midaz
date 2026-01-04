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

	if reportStore != nil {
		if last, err := reportStore.LoadLatest(context.Background()); err == nil && last != nil {
			engine.lastReport = last
		}
	}

	return engine
}

// RunReconciliation executes all reconciliation checks
func (e *ReconciliationEngine) RunReconciliation(ctx context.Context) (*domain.ReconciliationReport, error) {
	startTime := time.Now()

	runID := uuid.NewString()

	e.logger.Info("Starting reconciliation run")

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
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

	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

	switch name {
	case postgres.CheckerNameBalance:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.BalanceCheckResult) {
			r.balanceCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameDoubleEntry:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.DoubleEntryCheckResult) {
			r.doubleEntryCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameOrphans:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.OrphanCheckResult) {
			r.orphanCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameReferential:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.ReferentialCheckResult) {
			r.referentialCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameSync:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.SyncCheckResult) {
			r.syncCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameDLQ:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.DLQCheckResult) {
			r.dlqCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameOutbox:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.OutboxCheckResult) {
			r.outboxCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameMetadata:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.MetadataCheckResult) {
			r.metadataCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameRedis:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.RedisCheckResult) {
			r.redisCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameCrossDB:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.CrossDBCheckResult) {
			r.crossDBCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	case postgres.CheckerNameCRMAlias:
		if res, ok := buildTypedResult(name, result, durationMs, func(r *checkResult, v *domain.CRMAliasCheckResult) {
			r.crmAliasCheck = v
		}); ok {
			return res
		}

		res := e.unexpectedResult(name, result)
		res.durationMs = durationMs

		return res
	default:
		if e.logger != nil {
			e.logger.Warnf("Unknown checker name: %s", name)
		}

		return checkResult{name: name, durationMs: durationMs}
	}
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
	case postgres.CheckerNameOutbox:
		report.OutboxCheck = res.outboxCheck
	case postgres.CheckerNameMetadata:
		report.MetadataCheck = res.metadataCheck
	case postgres.CheckerNameRedis:
		report.RedisCheck = res.redisCheck
	case postgres.CheckerNameCrossDB:
		report.CrossDBCheck = res.crossDBCheck
	case postgres.CheckerNameCRMAlias:
		report.CRMAliasCheck = res.crmAliasCheck
	}
}

func (e *ReconciliationEngine) setCheckDefaults(report *domain.ReconciliationReport) {
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

	if report.MetadataCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameMetadata]; ok {
			report.MetadataCheck = &domain.MetadataCheckResult{Status: domain.StatusError}
		} else {
			report.MetadataCheck = &domain.MetadataCheckResult{Status: domain.StatusSkipped}
		}
	}

	// DLQCheck is usually set by the checker. This fallback is defensive (e.g. missing checker).
	if report.DLQCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameDLQ]; ok {
			report.DLQCheck = &domain.DLQCheckResult{Status: domain.StatusError}
		} else {
			report.DLQCheck = &domain.DLQCheckResult{Status: domain.StatusSkipped}
		}
	}

	if report.OutboxCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameOutbox]; ok {
			report.OutboxCheck = &domain.OutboxCheckResult{Status: domain.StatusError}
		} else {
			report.OutboxCheck = &domain.OutboxCheckResult{Status: domain.StatusSkipped}
		}
	}

	if report.RedisCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameRedis]; ok {
			report.RedisCheck = &domain.RedisCheckResult{Status: domain.StatusError}
		} else {
			report.RedisCheck = &domain.RedisCheckResult{Status: domain.StatusSkipped}
		}
	}

	if report.CrossDBCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameCrossDB]; ok {
			report.CrossDBCheck = &domain.CrossDBCheckResult{Status: domain.StatusError}
		} else {
			report.CrossDBCheck = &domain.CrossDBCheckResult{Status: domain.StatusSkipped}
		}
	}

	if report.CRMAliasCheck == nil {
		if _, ok := e.checkers[postgres.CheckerNameCRMAlias]; ok {
			report.CRMAliasCheck = &domain.CRMAliasCheckResult{Status: domain.StatusError}
		} else {
			report.CRMAliasCheck = &domain.CRMAliasCheckResult{Status: domain.StatusSkipped}
		}
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
	case postgres.CheckerNameOutbox:
		return "Outbox"
	case postgres.CheckerNameMetadata:
		return "Metadata"
	case postgres.CheckerNameRedis:
		return "Redis"
	case postgres.CheckerNameCrossDB:
		return "CrossDB"
	case postgres.CheckerNameCRMAlias:
		return "CRM Alias"
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
	case postgres.CheckerNameOutbox:
		return checkResult{name: name, outboxCheck: &domain.OutboxCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameMetadata:
		return checkResult{name: name, metadataCheck: &domain.MetadataCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameRedis:
		return checkResult{name: name, redisCheck: &domain.RedisCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameCrossDB:
		return checkResult{name: name, crossDBCheck: &domain.CrossDBCheckResult{Status: domain.StatusError}}
	case postgres.CheckerNameCRMAlias:
		return checkResult{name: name, crmAliasCheck: &domain.CRMAliasCheckResult{Status: domain.StatusError}}
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

func computeDelta(prev, curr *domain.ReconciliationReport) *domain.ReconciliationDelta {
	if prev == nil || curr == nil {
		return nil
	}

	delta := &domain.ReconciliationDelta{}

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
		prevOrphans := prev.ReferentialCheck.OrphanLedgers +
			prev.ReferentialCheck.OrphanAssets +
			prev.ReferentialCheck.OrphanAccounts +
			prev.ReferentialCheck.OrphanOperations +
			prev.ReferentialCheck.OrphanPortfolios +
			prev.ReferentialCheck.OrphanUnknown +
			prev.ReferentialCheck.OperationsWithoutBalance
		currOrphans := curr.ReferentialCheck.OrphanLedgers +
			curr.ReferentialCheck.OrphanAssets +
			curr.ReferentialCheck.OrphanAccounts +
			curr.ReferentialCheck.OrphanOperations +
			curr.ReferentialCheck.OrphanPortfolios +
			curr.ReferentialCheck.OrphanUnknown +
			curr.ReferentialCheck.OperationsWithoutBalance
		delta.ReferentialOrphans = currOrphans - prevOrphans
	}

	if prev.OutboxCheck != nil && curr.OutboxCheck != nil {
		delta.OutboxPending = int(curr.OutboxCheck.Pending - prev.OutboxCheck.Pending)
		delta.OutboxFailed = int(curr.OutboxCheck.Failed - prev.OutboxCheck.Failed)
	}

	if prev.DLQCheck != nil && curr.DLQCheck != nil {
		delta.DLQEntries = int(curr.DLQCheck.Total - prev.DLQCheck.Total)
	}

	if prev.RedisCheck != nil && curr.RedisCheck != nil {
		prevMismatch := prev.RedisCheck.MissingRedis + prev.RedisCheck.ValueMismatches + prev.RedisCheck.VersionMismatches
		currMismatch := curr.RedisCheck.MissingRedis + curr.RedisCheck.ValueMismatches + curr.RedisCheck.VersionMismatches
		delta.RedisMismatches = currMismatch - prevMismatch
	}

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
