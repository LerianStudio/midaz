package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	reconcileLockKey       = "lock:{transactions}:reconcile_balance_status"
	reconcileLockTTL       = 55 * time.Minute
	reconcileDefaultPeriod = 1 * time.Hour
)

var releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

// PendingTransactionsReconciler runs a periodic job to reconcile transactions stuck in balance_status=PENDING.
type PendingTransactionsReconciler struct {
	redisConn *libRedis.RedisConnection
	logger    libLog.Logger
	useCase   *command.UseCase
	period    time.Duration
	instance  string
}

// NewPendingTransactionsReconciler creates a reconciler that periodically fixes transactions
// stuck in balance_status=PENDING by using proof-based reconciliation.
func NewPendingTransactionsReconciler(redisConn *libRedis.RedisConnection, logger libLog.Logger, useCase *command.UseCase, period time.Duration) *PendingTransactionsReconciler {
	if period <= 0 {
		period = reconcileDefaultPeriod
	}

	return &PendingTransactionsReconciler{
		redisConn: redisConn,
		logger:    logger,
		useCase:   useCase,
		period:    period,
		instance:  uuid.NewString(),
	}
}

// Run starts the periodic reconciliation loop.
func (w *PendingTransactionsReconciler) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Infof("PendingTransactionsReconciler started (period=%s)", w.period)

	rds, err := w.redisConn.GetClient(ctx)
	if err != nil {
		w.logger.Errorf("PendingTransactionsReconciler: failed to get redis client: %v", err)
		return pkg.ValidateInternalError(err, "PendingTransactionsReconciler")
	}

	// Run once on startup, then periodically.
	if err := w.runOnce(ctx, rds); err != nil {
		w.logger.Warnf("PendingTransactionsReconciler initial run failed: %v", err)
	}

	ticker := time.NewTicker(w.period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("PendingTransactionsReconciler: shutting down")
			return nil
		case <-ticker.C:
			if err := w.runOnce(ctx, rds); err != nil {
				w.logger.Warnf("PendingTransactionsReconciler run failed: %v", err)
			}
		}
	}
}

func (w *PendingTransactionsReconciler) runOnce(ctx context.Context, rds redis.UniversalClient) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "worker.transaction.reconcile_pending_transactions")
	defer span.End()

	locked, err := rds.SetNX(ctx, reconcileLockKey, w.instance, reconcileLockTTL).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to acquire reconcile lock", err)
		logger.Errorf("PendingTransactionsReconciler: failed to acquire reconcile lock: %v", err)

		return pkg.ValidateInternalError(err, "PendingTransactionsReconciler")
	}

	if !locked {
		logger.Infof("PendingTransactionsReconciler: lock not acquired (another instance running)")
		return nil
	}

	defer func() {
		// Best-effort release. If it fails, TTL will expire.
		if _, releaseErr := releaseLockScript.Run(ctx, rds, []string{reconcileLockKey}, w.instance).Result(); releaseErr != nil {
			if !errors.Is(releaseErr, context.Canceled) {
				w.logger.Warnf("PendingTransactionsReconciler: failed to release lock: %v", releaseErr)
			}
		}
	}()

	reconcileCfg := command.DefaultReconcilePendingTransactionsConfig()
	if err := w.useCase.ReconcilePendingTransactions(ctx, reconcileCfg); err != nil {
		libOpentelemetry.HandleSpanError(&span, "ReconcilePendingTransactions failed", err)
		logger.Errorf("PendingTransactionsReconciler: reconcile failed: %v", err)

		return fmt.Errorf("reconcile pending transactions: %w", err)
	}

	return nil
}
