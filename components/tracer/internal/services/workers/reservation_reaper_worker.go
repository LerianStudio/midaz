// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=reservation_reaper_worker.go -destination=mocks/reservation_reaper_worker_mock.go -package=mocks

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

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
)

// DefaultReservationReaperInterval is the sub-minute cadence at which the reaper
// sweeps expired RESERVED rows. It is intentionally far tighter than the 24h
// usage-counter cleanup worker: an abandoned reservation holds capacity until it
// is reaped, so the sweep must run often enough that a crashed ledger transaction
// frees its hold within an operator-tolerable window. 30s is the chosen default;
// operators tune it via RESERVATION_REAPER_INTERVAL_SECONDS.
const DefaultReservationReaperInterval = 30 * time.Second

// ReservationExpiryAuditor records the single batch-summary audit row per reaper
// sweep. It is the narrow slice of the audit writer the reaper needs: per-row
// reserve/confirm/release transitions are audited individually elsewhere, but the
// high-volume / low-forensic-value expiry path collapses to ONE summary row per
// sweep to cap hash-chain advisory-lock contention (Q11). Implemented by
// command.RecordAuditEventCommand.
type ReservationExpiryAuditor interface {
	// RecordReservationExpiryBatch writes ONE audit row summarizing a sweep of N
	// expired reservations. Called once per cycle AFTER the per-row EXPIRED
	// releases have committed individually.
	RecordReservationExpiryBatch(ctx context.Context, summary command.ReservationExpiryBatchSummary) error
}

// ReservationReaperWorkerConfig holds configuration for the reservation reaper.
type ReservationReaperWorkerConfig struct {
	// ReapInterval is how often the reaper sweeps expired reservations
	// (default: DefaultReservationReaperInterval, 30s).
	ReapInterval time.Duration
}

// DefaultReservationReaperWorkerConfig returns default configuration values.
func DefaultReservationReaperWorkerConfig() ReservationReaperWorkerConfig {
	return ReservationReaperWorkerConfig{
		ReapInterval: DefaultReservationReaperInterval,
	}
}

// ReservationReaperWorker periodically releases expired RESERVED reservations.
// It runs in the background at a sub-minute cadence and, for each reservation
// whose TTL has elapsed without a confirm or release, returns the held amount to
// the counter (status -> EXPIRED). One batch-summary audit row is written per
// sweep rather than one per expired row.
// Implements libCommons.App for Launcher integration.
//
// In multi-tenant mode tenantID scopes every sweep to a single tenant (the
// context is enriched with the tenantID at the top of runLoop and the cycle
// resolves the tenant-scoped pool), mirroring UsageCleanupWorker. In single-tenant
// mode tenantID is "" and behaviour is identical to the pre-multi-tenant worker.
type ReservationReaperWorker struct {
	tenantID string
	repo     ReservationReaperRepository
	auditor  ReservationExpiryAuditor
	config   ReservationReaperWorkerConfig
	logger   libLog.Logger
	clock    clock.Clock
	// poolResolver is non-nil in multi-tenant mode. Each sweep resolves the
	// tenant-scoped pool and injects it onto the cycle context via
	// tmcore.ContextWithPG so the find + per-row releases land on the tenant DB.
	// On resolution failure the cycle is SKIPPED — the worker never falls through
	// to the root pool, which would reap another database's reservations. In
	// single-tenant mode this is nil and the cycle falls through to the
	// repository's static connection.
	poolResolver WorkerPoolResolver
}

// NewReservationReaperWorker creates a new reservation reaper worker.
// Returns ErrNilRepository if repo is nil.
// Returns ErrNilReservationAuditor if auditor is nil.
// Returns ErrNilLogger if logger is nil.
// Returns ErrInvalidReaperInterval if ReapInterval <= 0.
// The clk parameter is optional; if nil, uses clock.RealClock{}.
func NewReservationReaperWorker(
	repo ReservationReaperRepository,
	auditor ReservationExpiryAuditor,
	config ReservationReaperWorkerConfig,
	logger libLog.Logger,
	clk clock.Clock,
	tenantID string,
) (*ReservationReaperWorker, error) {
	return NewReservationReaperWorkerWithPoolResolver(repo, auditor, config, logger, clk, tenantID, nil)
}

// NewReservationReaperWorkerWithPoolResolver is the full constructor. MT callers
// pass a non-nil poolResolver so each sweep stashes the tenant DB on the context
// via tmcore.ContextWithPG. Single-tenant callers may use NewReservationReaperWorker
// (poolResolver defaults to nil).
func NewReservationReaperWorkerWithPoolResolver(
	repo ReservationReaperRepository,
	auditor ReservationExpiryAuditor,
	config ReservationReaperWorkerConfig,
	logger libLog.Logger,
	clk clock.Clock,
	tenantID string,
	poolResolver WorkerPoolResolver,
) (*ReservationReaperWorker, error) {
	if repo == nil {
		return nil, ErrNilRepository
	}

	if auditor == nil {
		return nil, ErrNilReservationAuditor
	}

	if logger == nil {
		return nil, ErrNilLogger
	}

	if config.ReapInterval <= 0 {
		return nil, ErrInvalidReaperInterval
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	return &ReservationReaperWorker{
		tenantID:     tenantID,
		repo:         repo,
		auditor:      auditor,
		config:       config,
		logger:       logger,
		clock:        clk,
		poolResolver: poolResolver,
	}, nil
}

// Run implements the libCommons.App interface for Launcher integration.
// Handles OS signals (SIGINT, SIGTERM) for graceful shutdown.
// Sweep errors are logged but do not stop the worker.
func (w *ReservationReaperWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.runLoop(ctx)
}

// RunWithContext runs the worker with a provided context.
// Useful for testing or external orchestration.
func (w *ReservationReaperWorker) RunWithContext(ctx context.Context) error {
	return w.runLoop(ctx)
}

// runLoop is the internal loop that drives reap cycles.
func (w *ReservationReaperWorker) runLoop(ctx context.Context) error {
	if w.tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, w.tenantID)
	}

	w.logger.With(
		libLog.String("operation", "worker.reservation_reaper.run"),
		libLog.String("reap_interval", w.config.ReapInterval.String()),
	).Log(ctx, libLog.LevelInfo, "Starting reservation reaper worker")

	// Injected clock's ticker keeps the cadence deterministic in tests.
	tickerChan, stopTicker := w.clock.NewTicker(w.config.ReapInterval)
	defer stopTicker()

	// Sweep immediately on start, then on interval. Check for cancellation first
	// so a worker stopped before its first tick does no work.
	select {
	case <-ctx.Done():
		w.logger.With(
			libLog.String("operation", "worker.reservation_reaper.run"),
		).Log(ctx, libLog.LevelInfo, "Reservation reaper worker stopped before initial cycle")

		return nil
	default:
		w.runReapCycle(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.With(
				libLog.String("operation", "worker.reservation_reaper.run"),
			).Log(ctx, libLog.LevelInfo, "Reservation reaper worker stopped")

			return nil

		case <-tickerChan:
			w.runReapCycle(ctx)
		}
	}
}

// runReapCycle resolves the tenant pool (MT), runs a single sweep, and logs the
// result. Errors are logged but not returned — the worker continues running.
func (w *ReservationReaperWorker) runReapCycle(ctx context.Context) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "worker.reservation_reaper.run_cycle")
	defer span.End()

	logger := logging.WithTrace(ctx, w.logger)

	// Multi-tenant: resolve the tenant's pool for this cycle and stash it on ctx
	// so the find + releases land on the tenant DB. On resolution failure SKIP the
	// cycle — reaping against the wrong (root) database would corrupt another
	// tenant's reserved_usage. NEVER fall back to the root pool.
	if w.tenantID != "" && w.poolResolver != nil {
		tenantDB, err := w.poolResolver.GetTenantDB(ctx, w.tenantID)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to resolve tenant pool", err)

			logger.With(
				libLog.String("operation", "worker.reservation_reaper.resolve_pool"),
				libLog.String("tenant_id", w.tenantID),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to resolve tenant pool; skipping reap cycle")

			return
		}

		ctx = tmcore.ContextWithPG(ctx, tenantDB)
	}

	released, err := w.RunOnce(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Reap cycle failed", err)
		logger.With(
			libLog.String("operation", "worker.reservation_reaper.run_cycle"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to reap expired reservations")

		return
	}

	logger.With(
		libLog.String("operation", "worker.reservation_reaper.run_cycle"),
		libLog.Int("released_count", released),
	).Log(ctx, libLog.LevelInfo, "Reap cycle completed successfully")
}

// RunOnce executes a single reap sweep: find the expired RESERVED reservations,
// release each as EXPIRED in its own transaction, then write ONE batch-summary
// audit row for the sweep. Returns the number of reservations released.
//
// A per-row release that hits an already-terminal reservation (a confirm/release
// raced the sweep) is an idempotent no-op handled inside the repository, so the
// reaper does not special-case it here. A genuine release failure aborts the
// remaining releases for the cycle and is returned so the cycle is logged as
// failed; the next tick retries the still-expired rows.
//
// The batch audit is only written when at least one reservation expired — an
// empty sweep produces no audit row.
func (w *ReservationReaperWorker) RunOnce(ctx context.Context) (int, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "worker.reservation_reaper.run_once")
	defer span.End()

	logger := logging.WithTrace(ctx, w.logger)

	now := w.clock.Now().UTC()

	expired, err := w.repo.FindExpiredReservations(ctx, now)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to find expired reservations", err)

		return 0, fmt.Errorf("failed to find expired reservations: %w", err)
	}

	if len(expired) == 0 {
		return 0, nil
	}

	released := 0

	for _, reservationID := range expired {
		if err := w.repo.ReleaseExpired(ctx, reservationID); err != nil {
			libOtel.HandleSpanError(span, "Failed to release expired reservation", err)

			return released, fmt.Errorf("failed to release expired reservation %s: %w", reservationID, err)
		}

		released++
	}

	summary := command.ReservationExpiryBatchSummary{
		ExpiredCount: released,
		SweptAt:      now,
	}

	if err := w.auditor.RecordReservationExpiryBatch(ctx, summary); err != nil {
		// The counter moves already committed per row; a failed batch audit is a
		// forensic gap, not a correctness fault, so it does not unwind the sweep.
		// Surface it so the cycle is logged as failed and the audit chain gap is
		// visible in tracing.
		libOtel.HandleSpanError(span, "Failed to record reservation expiry batch", err)

		return released, fmt.Errorf("failed to record reservation expiry batch: %w", err)
	}

	logger.With(
		libLog.String("operation", "worker.reservation_reaper.run_once"),
		libLog.String("now", now.Format(time.RFC3339)),
		libLog.Int("released_count", released),
	).Log(ctx, libLog.LevelInfo, "Released expired reservations")

	return released, nil
}
