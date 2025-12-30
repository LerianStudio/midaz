package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
)

// ReconciliationWorker runs reconciliation checks on a schedule
type ReconciliationWorker struct {
	engine  *engine.ReconciliationEngine
	logger  libLog.Logger
	config  *Config
	running atomic.Bool // Guard against concurrent runs
}

// NewReconciliationWorker creates a new reconciliation worker
func NewReconciliationWorker(
	eng *engine.ReconciliationEngine,
	logger libLog.Logger,
	config *Config,
) *ReconciliationWorker {
	return &ReconciliationWorker{
		engine: eng,
		logger: logger,
		config: config,
	}
}

// Run starts the reconciliation worker
func (w *ReconciliationWorker) Run(l *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := w.config.GetReconciliationInterval()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.logger.Infof("Reconciliation worker started, interval: %s", interval)

	// Run initial reconciliation
	w.runReconciliation(ctx)

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("Reconciliation worker shutting down")
			return nil

		case <-ticker.C:
			w.runReconciliation(ctx)
		}
	}
}

// runReconciliation executes a single reconciliation run with concurrency guard
func (w *ReconciliationWorker) runReconciliation(ctx context.Context) {
	// Guard against concurrent runs
	if !w.running.CompareAndSwap(false, true) {
		w.logger.Warnf("Skipping reconciliation - previous run still in progress")
		return
	}
	defer w.running.Store(false)

	report, err := w.engine.RunReconciliation(ctx)
	if err != nil {
		w.logger.Errorf("Reconciliation failed: %v", err)
		return
	}

	if report == nil {
		w.logger.Warnf("Reconciliation returned nil report without error - skipping result processing")
		return
	}

	// Log summary with nil checks
	balanceTotal, balanceDisc := 0, 0
	txnTotal, txnUnbal := 0, 0

	if report.BalanceCheck != nil {
		balanceTotal = report.BalanceCheck.TotalBalances
		balanceDisc = report.BalanceCheck.BalancesWithDiscrepancy
	}

	if report.DoubleEntryCheck != nil {
		txnTotal = report.DoubleEntryCheck.TotalTransactions
		txnUnbal = report.DoubleEntryCheck.UnbalancedTransactions
	}

	w.logger.Infof(
		"Reconciliation: status=%s, balances=%d (disc=%d), txns=%d (unbal=%d), settled=%d, unsettled=%d",
		report.Status,
		balanceTotal,
		balanceDisc,
		txnTotal,
		txnUnbal,
		report.SettledTransactions,
		report.UnsettledTransactions,
	)
}
