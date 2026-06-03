// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/logging"
)

// UsageCleanupWorkerConfig holds configuration for the cleanup worker.
type UsageCleanupWorkerConfig struct {
	// CleanupInterval is how often the cleanup runs (default: 24 hours).
	CleanupInterval time.Duration
}

// DefaultUsageCleanupWorkerConfig returns default configuration values.
func DefaultUsageCleanupWorkerConfig() UsageCleanupWorkerConfig {
	return UsageCleanupWorkerConfig{
		CleanupInterval: 24 * time.Hour,
	}
}

// UsageCleanupWorker periodically cleans up expired usage counters.
// It runs in the background and deletes counters that haven't been updated
// within the retention period.
// Implements libCommons.App interface for Launcher integration.
//
// In multi-tenant mode, tenantID scopes every cleanup to a single tenant
// (the context is enriched with the tenantID at the top of runLoop and
// runCleanupCycle, so the repository resolves the tenant-scoped connection).
// In single-tenant mode tenantID is "" and behaviour is identical to the
// pre-multi-tenant worker.
type UsageCleanupWorker struct {
	tenantID string
	repo     UsageCounterCleanupRepository
	config   UsageCleanupWorkerConfig
	logger   libLog.Logger
	clock    clock.Clock
	// poolResolver is non-nil in multi-tenant mode. Same contract as
	// RuleSyncWorker.poolResolver: each cleanup cycle resolves the
	// tenant-scoped pool and injects it onto the cycle context via
	// tmcore.ContextWithPG, so repo.DeleteExpiredCounters lands on the
	// tenant DB. In single-tenant mode this is nil and the cycle falls
	// through to the repository's static connection.
	poolResolver WorkerPoolResolver
}

// NewUsageCleanupWorker creates a new cleanup worker.
// Returns ErrNilRepository if repo is nil.
// Returns ErrNilLogger if logger is nil.
// Returns ErrInvalidCleanupInterval if CleanupInterval <= 0.
// The clk parameter is optional; if nil, uses clock.RealClock{}.
func NewUsageCleanupWorker(repo UsageCounterCleanupRepository, config UsageCleanupWorkerConfig, logger libLog.Logger, clk clock.Clock, tenantID string) (*UsageCleanupWorker, error) {
	return NewUsageCleanupWorkerWithPoolResolver(repo, config, logger, clk, tenantID, nil)
}

// NewUsageCleanupWorkerWithPoolResolver is the full constructor. MT callers
// pass a non-nil poolResolver so each cleanup cycle stashes the tenant DB on
// the context via tmcore.ContextWithPG. Single-tenant callers may use
// NewUsageCleanupWorker (poolResolver defaults to nil).
func NewUsageCleanupWorkerWithPoolResolver(
	repo UsageCounterCleanupRepository,
	config UsageCleanupWorkerConfig,
	logger libLog.Logger,
	clk clock.Clock,
	tenantID string,
	poolResolver WorkerPoolResolver,
) (*UsageCleanupWorker, error) {
	if repo == nil {
		return nil, ErrNilRepository
	}

	if logger == nil {
		return nil, ErrNilLogger
	}

	if config.CleanupInterval <= 0 {
		return nil, ErrInvalidCleanupInterval
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	return &UsageCleanupWorker{
		tenantID:     tenantID,
		repo:         repo,
		config:       config,
		logger:       logger,
		clock:        clk,
		poolResolver: poolResolver,
	}, nil
}

// Run implements the libCommons.App interface for Launcher integration.
// Handles OS signals (SIGINT, SIGTERM) for graceful shutdown.
// Cleanup errors are logged but do not stop the worker.
func (w *UsageCleanupWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.runLoop(ctx)
}

// RunWithContext runs the worker with a provided context.
// This is useful for testing or external orchestration (e.g., K8s CronJob).
func (w *UsageCleanupWorker) RunWithContext(ctx context.Context) error {
	return w.runLoop(ctx)
}

// runLoop is the internal loop that handles cleanup cycles.
func (w *UsageCleanupWorker) runLoop(ctx context.Context) error {
	if w.tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, w.tenantID)
	}

	w.logger.With(
		libLog.String("operation", "worker.usage_cleanup.run"),
		libLog.String("cleanup_interval", w.config.CleanupInterval.String()),
	).Log(ctx, libLog.LevelInfo, "Starting usage cleanup worker")

	// Use injected clock's ticker for deterministic testing
	tickerChan, stopTicker := w.clock.NewTicker(w.config.CleanupInterval)
	defer stopTicker()

	// Run cleanup immediately on start, then on interval
	// Check for cancellation before initial cleanup to avoid work after shutdown
	select {
	case <-ctx.Done():
		w.logger.With(
			libLog.String("operation", "worker.usage_cleanup.run"),
		).Log(ctx, libLog.LevelInfo, "Usage cleanup worker stopped before initial cycle")

		return nil
	default:
		w.runCleanupCycle(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.With(
				libLog.String("operation", "worker.usage_cleanup.run"),
			).Log(ctx, libLog.LevelInfo, "Usage cleanup worker stopped")

			return nil

		case <-tickerChan:
			w.runCleanupCycle(ctx)
		}
	}
}

// runCleanupCycle executes a single cleanup and logs the result.
// Errors are logged but not returned - the worker continues running.
func (w *UsageCleanupWorker) runCleanupCycle(ctx context.Context) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "worker.usage_cleanup.run_cycle")
	defer span.End()

	// Use w.logger (guaranteed non-nil) instead of context logger which may be empty
	logger := logging.WithTrace(ctx, w.logger)

	// Multi-tenant: resolve the tenant's Postgres pool for this cycle and
	// stash it onto ctx so DeleteExpiredCounters lands on the tenant DB.
	// On resolution failure, skip this cycle — trying to clean up the wrong
	// database would corrupt root-pool counters.
	if w.tenantID != "" && w.poolResolver != nil {
		tenantDB, err := w.poolResolver.GetTenantDB(ctx, w.tenantID)
		if err != nil {
			// Mark the span as failed before returning so MT cleanup outages
			// surface in tracing dashboards. Without this, the early-return
			// path looks identical to a successful cycle in traces.
			libOtel.HandleSpanError(span, "Failed to resolve tenant pool", err)

			logger.With(
				libLog.String("operation", "worker.usage_cleanup.resolve_pool"),
				libLog.String("tenant_id", w.tenantID),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to resolve tenant pool; skipping cleanup cycle")

			return
		}

		ctx = tmcore.ContextWithPG(ctx, tenantDB)
	}

	logger.With(
		libLog.String("operation", "worker.usage_cleanup.run_cycle"),
	).Log(ctx, libLog.LevelInfo, "Running usage counter cleanup cycle")

	deleted, err := w.RunOnce(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Cleanup cycle failed", err)
		logger.With(
			libLog.String("operation", "worker.usage_cleanup.run_cycle"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to cleanup expired counters")

		return
	}

	logger.With(
		libLog.String("operation", "worker.usage_cleanup.run_cycle"),
		libLog.Any("deleted_count", deleted),
	).Log(ctx, libLog.LevelInfo, "Cleanup cycle completed successfully")
}

// RunOnce executes a single cleanup operation.
// Returns the number of deleted counters.
// This method can be called directly for manual/on-demand cleanup,
// or used by external schedulers (e.g., K8s CronJob).
//
//	Uses expires_at column for accurate cleanup timing.
func (w *UsageCleanupWorker) RunOnce(ctx context.Context) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "worker.usage_cleanup.run_once")
	defer span.End()

	logger := logging.WithTrace(ctx, w.logger)

	// Use current time for expires_at comparison
	// Counters with expires_at < now will be deleted
	// Counters with NULL expires_at are preserved (never deleted)
	now := w.clock.Now().UTC()

	logger.With(
		libLog.String("operation", "worker.usage_cleanup.run_once"),
		libLog.String("now", now.Format(time.RFC3339)),
	).Log(ctx, libLog.LevelInfo, "Deleting expired usage counters by expires_at")

	deleted, err := w.repo.DeleteExpiredCounters(ctx, now)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to delete expired counters", err)

		return 0, fmt.Errorf("failed to delete expired counters: %w", err)
	}

	logger.With(
		libLog.String("operation", "worker.usage_cleanup.run_once"),
		libLog.Any("deleted_count", deleted),
	).Log(ctx, libLog.LevelInfo, "Deleted expired usage counters")

	return deleted, nil
}
