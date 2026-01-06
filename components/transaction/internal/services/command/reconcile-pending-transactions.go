package command

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

const (
	reconcileThresholdAgeDefault       = 25 * time.Hour // 24h retry window + 1h buffer
	reconcileBatchSizeDefault          = 100
	reconcileMaxTotalPerRunDefault     = 500
	reconcileMaxPerOrgPerRunDefault    = 200
	reconcileMaxErrorRateDefault       = 0.3
	reconcileSleepBetweenMinDefault    = 50 * time.Millisecond
	reconcileSleepBetweenJitterDefault = 50 * time.Millisecond
	reconcileMinProcessedForErrorRate  = 10
)

var errReconcileAbortedHighErrorRate = errors.New("reconciliation aborted due to high error rate")

// ReconcilePendingTransactionsConfig configures the reconciliation job.
type ReconcilePendingTransactionsConfig struct {
	ThresholdAge time.Duration
	BatchSize    int

	// Guardrails
	MaxTotalPerRun  int
	MaxPerOrgPerRun int
	MaxErrorRate    float64

	SleepBetweenMin    time.Duration
	SleepBetweenJitter time.Duration
}

// DefaultReconcilePendingTransactionsConfig returns a safe default configuration for
// reconciling transactions stuck in balance_status=PENDING.
func DefaultReconcilePendingTransactionsConfig() ReconcilePendingTransactionsConfig {
	return ReconcilePendingTransactionsConfig{
		ThresholdAge: reconcileThresholdAgeDefault,
		BatchSize:    reconcileBatchSizeDefault,

		MaxTotalPerRun:  reconcileMaxTotalPerRunDefault,
		MaxPerOrgPerRun: reconcileMaxPerOrgPerRunDefault,
		MaxErrorRate:    reconcileMaxErrorRateDefault,

		SleepBetweenMin:    reconcileSleepBetweenMinDefault,
		SleepBetweenJitter: reconcileSleepBetweenJitterDefault,
	}
}

// ReconcilePendingTransactions fixes transactions stuck in balance_status=PENDING beyond the retry window.
//
// Proof-based rule (no guessing):
// - If balance_persisted_at IS NOT NULL => mark CONFIRMED
// - If balance_persisted_at IS NULL     => mark FAILED (terminal)
//
//nolint:cyclop // Reconciliation has multiple guardrails (limits, error-rate cutoff, per-org caps).
func (uc *UseCase) ReconcilePendingTransactions(ctx context.Context, cfg ReconcilePendingTransactionsConfig) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.transaction.reconcile_pending_transactions")
	defer span.End()

	cfg = normalizeReconcilePendingTransactionsConfig(cfg)

	candidates, err := uc.TransactionRepo.FindPendingForReconciliation(ctx, cfg.ThresholdAge, cfg.BatchSize)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find PENDING reconciliation candidates", err)
		logger.Errorf("Failed to find PENDING reconciliation candidates: %v", err)

		return fmt.Errorf("find pending transactions for reconciliation: %w", err)
	}

	if len(candidates) == 0 {
		logger.Info("No PENDING reconciliation candidates found")
		return nil
	}

	seenPerOrg := map[string]int{}
	processed := 0
	confirmed := 0
	failed := 0
	errorsCount := 0

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Non-crypto jitter

	for _, c := range candidates {
		if processed >= cfg.MaxTotalPerRun {
			break
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("reconcile pending transactions interrupted: %w", ctx.Err())
		default:
		}

		orgKey := c.OrganizationID.String()
		if seenPerOrg[orgKey] >= cfg.MaxPerOrgPerRun {
			continue
		}

		if err := sleepWithJitter(ctx, cfg.SleepBetweenMin, cfg.SleepBetweenJitter, rng); err != nil {
			return err
		}

		targetStatus := constant.BalanceStatusFailed
		if c.BalancePersistedAt != nil {
			targetStatus = constant.BalanceStatusConfirmed
		}

		if err := uc.TransactionRepo.UpdateBalanceStatus(ctx, c.OrganizationID, c.LedgerID, c.ID, targetStatus); err != nil {
			errorsCount++

			logger.Warnf("Failed to reconcile tx %s to %s: %v", c.ID, targetStatus, err)
		} else {
			if targetStatus == constant.BalanceStatusConfirmed {
				confirmed++
			} else {
				failed++
			}
		}

		processed++
		seenPerOrg[orgKey]++

		// Avoid aborting too early on small samples.
		if processed > reconcileMinProcessedForErrorRate && float64(errorsCount)/float64(processed) > cfg.MaxErrorRate {
			abortErr := fmt.Errorf("%w: errors=%d processed=%d", errReconcileAbortedHighErrorRate, errorsCount, processed)
			libOpentelemetry.HandleSpanError(&span, "Reconciliation aborting due to high error rate", abortErr)
			logger.Errorf("%v", abortErr)

			break
		}
	}

	logger.Infof(
		"Reconciliation completed: total_found=%d processed=%d confirmed=%d failed=%d errors=%d",
		len(candidates),
		processed,
		confirmed,
		failed,
		errorsCount,
	)

	return nil
}

func normalizeReconcilePendingTransactionsConfig(cfg ReconcilePendingTransactionsConfig) ReconcilePendingTransactionsConfig {
	if cfg.ThresholdAge <= 0 {
		cfg.ThresholdAge = reconcileThresholdAgeDefault
	}

	if cfg.BatchSize <= 0 {
		cfg.BatchSize = reconcileBatchSizeDefault
	}

	if cfg.MaxTotalPerRun <= 0 {
		cfg.MaxTotalPerRun = reconcileMaxTotalPerRunDefault
	}

	if cfg.MaxPerOrgPerRun <= 0 {
		cfg.MaxPerOrgPerRun = reconcileMaxPerOrgPerRunDefault
	}

	if cfg.MaxErrorRate <= 0 {
		cfg.MaxErrorRate = reconcileMaxErrorRateDefault
	}

	if cfg.SleepBetweenMin < 0 {
		cfg.SleepBetweenMin = 0
	}

	if cfg.SleepBetweenJitter < 0 {
		cfg.SleepBetweenJitter = 0
	}

	return cfg
}

func sleepWithJitter(ctx context.Context, baseDelay, jitterWindow time.Duration, rng *rand.Rand) error {
	if baseDelay <= 0 && jitterWindow <= 0 {
		return nil
	}

	j := time.Duration(0)
	if jitterWindow > 0 {
		j = time.Duration(rng.Int63n(int64(jitterWindow)))
	}

	d := baseDelay + j
	if d <= 0 {
		return nil
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("sleep interrupted: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}
