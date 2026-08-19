// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libBackoff "github.com/LerianStudio/lib-commons/v6/commons/backoff"
	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type TracerOutcomeWorkerConfig struct {
	PollInterval    time.Duration
	PreparedTimeout time.Duration
	LeaderTTL       time.Duration
	RetryBase       time.Duration
	RetryMax        time.Duration
	DeliveredTTL    time.Duration
	BatchSize       int64
}

func (c TracerOutcomeWorkerConfig) withDefaults() TracerOutcomeWorkerConfig {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}

	if c.PreparedTimeout <= 0 {
		c.PreparedTimeout = 30 * time.Second
	}

	if c.LeaderTTL <= 0 {
		c.LeaderTTL = 15 * time.Second
	}

	if c.RetryBase <= 0 {
		c.RetryBase = 250 * time.Millisecond
	}

	if c.RetryMax <= 0 {
		c.RetryMax = 30 * time.Second
	}

	if c.DeliveredTTL <= 0 {
		c.DeliveredTTL = 7 * 24 * time.Hour
	}

	if c.BatchSize <= 0 {
		c.BatchSize = 100
	}

	return c
}

// TracerOutcomeWorker is the durable Ledger->Tracer dispatcher and PREPARED
// recovery owner. PENDING_HELD is deliberately never recovered or delivered.
type TracerOutcomeWorker struct {
	logger         libLog.Logger
	repo           redisTransaction.RedisRepository
	applier        in.TracerOutcomeApplier
	config         TracerOutcomeWorkerConfig
	tenantCache    *tenantcache.TenantCache
	mtEnabled      bool
	owner          string
	now            func() time.Time
	backoff        func(int) time.Duration
	metricsFactory *metrics.MetricsFactory
}

func (w *TracerOutcomeWorker) WithMetricsFactory(factory *metrics.MetricsFactory) *TracerOutcomeWorker {
	w.metricsFactory = factory
	return w
}

func NewTracerOutcomeWorker(
	logger libLog.Logger,
	repo redisTransaction.RedisRepository,
	applier in.TracerOutcomeApplier,
	config TracerOutcomeWorkerConfig,
) *TracerOutcomeWorker {
	config = config.withDefaults()

	return &TracerOutcomeWorker{
		logger: logger, repo: repo, applier: applier, config: config,
		owner: podIdentifier() + ":" + uuid.NewString(), now: time.Now,
		backoff: func(attempt int) time.Duration {
			return min(libBackoff.ExponentialWithJitter(config.RetryBase, attempt), config.RetryMax)
		},
	}
}

func NewTracerOutcomeWorkerMT(
	logger libLog.Logger,
	repo redisTransaction.RedisRepository,
	applier in.TracerOutcomeApplier,
	config TracerOutcomeWorkerConfig,
	cache *tenantcache.TenantCache,
) *TracerOutcomeWorker {
	w := NewTracerOutcomeWorker(logger, repo, applier, config)
	w.mtEnabled = true
	w.tenantCache = cache

	return w
}

func (w *TracerOutcomeWorker) Ready() bool {
	return w != nil && w.repo != nil && w.applier != nil && (!w.mtEnabled || w.tenantCache != nil)
}

func (w *TracerOutcomeWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.RunContext(ctx)
}

func (w *TracerOutcomeWorker) RunContext(ctx context.Context) error {
	if !w.Ready() {
		return fmt.Errorf("tracer outcome worker is not ready")
	}

	w.runCycle(ctx)

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.runCycle(ctx)
		}
	}
}

func (w *TracerOutcomeWorker) runCycle(ctx context.Context) {
	if !w.mtEnabled {
		w.runTenantCycle(ctx)
		return
	}

	for _, tenantID := range w.tenantCache.TenantIDs() {
		if ctx.Err() != nil {
			return
		}

		w.runTenantCycle(tmcore.ContextWithTenantID(ctx, tenantID))
	}
}

func (w *TracerOutcomeWorker) runTenantCycle(ctx context.Context) {
	acquired, err := w.repo.AcquireOwnedKey(ctx, utils.TracerOutcomeDispatcherLock, w.owner,
		time.Duration(max(int64(1), int64(w.config.LeaderTTL/time.Second))))
	if err != nil || !acquired {
		return
	}
	defer func() {
		if _, releaseErr := w.repo.ReleaseOwnedKey(ctx, utils.TracerOutcomeDispatcherLock, w.owner); releaseErr != nil && w.logger != nil {
			w.logger.Log(ctx, libLog.LevelWarn, "Failed to release tracer outcome dispatcher leadership", libLog.Err(releaseErr))
		}
	}()

	now := w.now().UTC()

	keys, err := w.repo.ListDueTracerOutcomes(ctx, now, w.config.BatchSize)
	if err != nil {
		w.logFailure(ctx, "Failed to list due tracer outcomes", err)
		return
	}

	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}

		w.dispatchOne(ctx, key, now)
	}
}

func (w *TracerOutcomeWorker) dispatchOne(ctx context.Context, key string, now time.Time) {
	record, err := w.repo.ReadTracerOutcomeByKey(ctx, key)
	if err != nil {
		w.logFailure(ctx, "Failed to read tracer outcome", err)
		return
	}

	if record == nil {
		_ = w.repo.RemoveTracerOutcomeSchedule(ctx, key)
		return
	}

	if record.State == mmodel.TracerOutcomePrepared {
		if now.UnixMilli()-record.PreparedAtUnixMS < w.config.PreparedTimeout.Milliseconds() {
			_ = w.repo.RescheduleTracerOutcome(ctx, key, record.OutcomeID, record.State, "",
				now, time.UnixMilli(record.PreparedAtUnixMS).Add(w.config.PreparedTimeout))

			return
		}

		record, err = w.repo.AbortPreparedTracerOutcome(ctx, record.OrganizationID, record.LedgerID,
			record.TransactionID, record.Owner, record.OutcomeID, now)
		if err != nil {
			w.emit(ctx, utils.TracerOutcomePreparedRecoveryTotal, map[string]string{"result": "conflict"})
			// A concurrent Lua terminal transition wins the CAS. The next schedule
			// pass observes it; recovery never overwrites that economic fact.
			return
		}

		w.emit(ctx, utils.TracerOutcomePreparedRecoveryTotal, map[string]string{"result": "aborted"})
	}

	switch record.State {
	case mmodel.TracerOutcomePendingHeld:
		_ = w.repo.RemoveTracerOutcomeSchedule(ctx, key)
		return
	case mmodel.TracerOutcomeDelivered:
		_ = w.repo.RemoveTracerOutcomeSchedule(ctx, key)
		return
	case mmodel.TracerOutcomeCommitted, mmodel.TracerOutcomeAborted:
	default:
		w.logFailure(ctx, "Invalid tracer outcome state", fmt.Errorf("state %q", record.State))
		return
	}

	outcome := tracerclient.ReservationOutcomeCommitted
	if record.State == mmodel.TracerOutcomeAborted {
		outcome = tracerclient.ReservationOutcomeAborted
	}

	_, err = w.applier.ApplyOutcome(ctx, tracerclient.ApplyOutcomeRequest{
		TransactionID: record.TransactionID, OutcomeID: record.OutcomeID, Outcome: outcome,
	})
	if err != nil {
		w.emit(ctx, utils.TracerOutcomeDispatchTotal, map[string]string{"outcome": string(outcome), "result": "retry"})

		next := now.Add(w.backoff(record.DeliveryAttempts))
		if rescheduleErr := w.repo.RescheduleTracerOutcome(ctx, key, record.OutcomeID, record.State,
			err.Error(), now, next); rescheduleErr != nil {
			w.logFailure(ctx, "Failed to reschedule tracer outcome", rescheduleErr)
		}

		return
	}

	if _, err := w.repo.MarkTracerOutcomeDelivered(ctx, key, record.OutcomeID, record.State, now, w.config.DeliveredTTL); err != nil {
		w.emit(ctx, utils.TracerOutcomeDispatchTotal, map[string]string{"outcome": string(outcome), "result": "ack_persist_failed"})
		w.logFailure(ctx, "Failed to persist tracer outcome acknowledgement", err)

		return
	}

	w.emit(ctx, utils.TracerOutcomeDispatchTotal, map[string]string{"outcome": string(outcome), "result": "delivered"})
}

func (w *TracerOutcomeWorker) logFailure(ctx context.Context, message string, err error) {
	if w.logger != nil {
		w.logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))
	}
}

func (w *TracerOutcomeWorker) emit(ctx context.Context, metric metrics.Metric, labels map[string]string) {
	if w.metricsFactory == nil {
		return
	}

	counter, err := w.metricsFactory.Counter(metric)
	if err == nil {
		err = counter.WithLabels(labels).AddOne(ctx)
	}

	if err != nil {
		w.logFailure(ctx, "Failed to emit tracer outcome metric", err)
	}
}
