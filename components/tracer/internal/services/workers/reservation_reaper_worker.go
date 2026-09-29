// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// DefaultReservationReaperInterval is the sub-minute cadence at which the reaper
// sweeps expired RESERVED rows. It is intentionally far tighter than the 24h
// usage-counter cleanup worker: an abandoned reservation holds capacity until it
// is reaped, so the sweep must run often enough that a crashed ledger transaction
// frees its hold within an operator-tolerable window. 30s is the chosen default;
// operators tune it via RESERVATION_REAPER_INTERVAL_SECONDS.
const DefaultReservationReaperInterval = 30 * time.Second

// DefaultReservationReaperBatchSize is the number of expired reservations one
// sweep reads when RESERVATION_REAPER_BATCH_SIZE is unset.
const DefaultReservationReaperBatchSize = 500

// ReservationReaperWorkerConfig holds configuration for the reservation reaper.
type ReservationReaperWorkerConfig struct {
	// ReapInterval is how often the reaper sweeps expired reservations
	// (default: DefaultReservationReaperInterval, 30s).
	ReapInterval time.Duration
	// BatchSize caps the expired reservations one sweep reads (default:
	// DefaultReservationReaperBatchSize). A larger backlog drains over several
	// sweeps instead of one long burst of per-operation audit writes.
	BatchSize int
}

// DefaultReservationReaperWorkerConfig returns default configuration values.
func DefaultReservationReaperWorkerConfig() ReservationReaperWorkerConfig {
	return ReservationReaperWorkerConfig{
		ReapInterval: DefaultReservationReaperInterval,
		BatchSize:    DefaultReservationReaperBatchSize,
	}
}

// ReservationReaperWorker periodically expires RESERVED reservations whose TTL
// elapsed without a confirm or release. It runs in the background at a
// sub-minute cadence and expires each owning operation once, which returns the
// held capacity of all its reservations and audits the operation.
// Implements libCommons.App for Launcher integration.
//
// In multi-tenant mode tenantID scopes every sweep to a single tenant (the
// context is enriched with the tenantID at the top of runLoop and the cycle
// resolves the tenant-scoped pool), mirroring UsageCleanupWorker. In single-tenant
// mode tenantID is "" and behaviour is identical to the pre-multi-tenant worker.
type ReservationReaperWorker struct {
	tenantID string
	repo     ReservationReaperRepository
	// expirer closes the operations that own the expired reservations.
	expirer ReserveOperationExpirer
	config  ReservationReaperWorkerConfig
	logger  libLog.Logger
	clock   clock.Clock
	// poolResolver is non-nil in multi-tenant mode. Each sweep resolves the
	// tenant-scoped pool and injects it onto the cycle context via
	// tmcore.ContextWithPG so the find + per-row releases land on the tenant DB.
	// On resolution failure the cycle is SKIPPED — the worker never falls through
	// to the root pool, which would reap another database's reservations. In
	// single-tenant mode this is nil and the cycle falls through to the
	// repository's static connection.
	poolResolver WorkerPoolResolver

	// sweepMu serializes sweeps so resumeAfter advances one page at a time.
	sweepMu sync.Mutex
	// resumeAfter is where the next sweep starts: past the last row of a full
	// page, so rows that keep failing to expire cannot hold the head of the
	// expiry order. nil starts from the oldest expiry.
	resumeAfter *model.ReservationExpiryPosition
	// resumeOperationsAfter is resumeAfter for the operations that hold no
	// reservation, which are walked in their own expiry order.
	resumeOperationsAfter *model.OperationExpiryPosition
}

// NewReservationReaperWorkerWithPoolResolver creates a reservation reaper
// worker. MT callers pass a non-nil poolResolver so each sweep stashes the tenant
// DB on the context via tmcore.ContextWithPG; single-tenant callers pass nil.
// Returns ErrNilRepository, ErrNilOperationExpirer
// or ErrNilLogger for a missing dependency, and ErrInvalidReaperInterval if
// ReapInterval <= 0 or ErrInvalidReaperBatchSize if BatchSize <= 0. A nil clk
// uses clock.RealClock{}.
func NewReservationReaperWorkerWithPoolResolver(
	repo ReservationReaperRepository,
	expirer ReserveOperationExpirer,
	config ReservationReaperWorkerConfig,
	logger libLog.Logger,
	clk clock.Clock,
	tenantID string,
	poolResolver WorkerPoolResolver,
) (*ReservationReaperWorker, error) {
	if repo == nil {
		return nil, ErrNilRepository
	}

	if expirer == nil {
		return nil, ErrNilOperationExpirer
	}

	if logger == nil {
		return nil, ErrNilLogger
	}

	if config.ReapInterval <= 0 {
		return nil, ErrInvalidReaperInterval
	}

	if config.BatchSize <= 0 {
		return nil, ErrInvalidReaperBatchSize
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	return &ReservationReaperWorker{
		tenantID:     tenantID,
		repo:         repo,
		expirer:      expirer,
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
	).Log(ctx, libLog.LevelDebug, "Reap cycle completed successfully")
}

// RunOnce executes a single reap sweep: find the expired RESERVED reservations
// and the expired OPEN operations that hold none, and expire each operation
// once through the expirer. Returns the number of reservations the expirer
// moved; an operation without reservations expires and audits without moving
// any. At most config.BatchSize reservations and config.BatchSize operations
// without reservations are read; the rest stay expired for the next sweep.
//
// Sweeps walk the expiry order (expiry, id) page by page. After a full page the
// next sweep resumes past its last row, whatever the page's outcome; a short
// page, or an empty page past the resume position, returns the walk to the
// oldest expiry. An operation that fails on every sweep is therefore retried
// once per pass instead of holding every sweep's head, so newer rows behind it
// still expire. Operations without reservations are paged the same way over
// their own position. A failed operation read or an operation that fails to
// expire does not stop the others; the failures are returned together so the
// cycle is logged as failed.
func (w *ReservationReaperWorker) RunOnce(ctx context.Context) (int, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled

	ctx, span := tracer.Start(ctx, "worker.reservation_reaper.run_once")
	defer span.End()

	logger := logging.WithTrace(ctx, w.logger)

	now := w.clock.Now().UTC()

	w.sweepMu.Lock()
	defer w.sweepMu.Unlock()

	expired, err := w.findPage(ctx, now)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to find expired reservations", err)

		return 0, fmt.Errorf("failed to find expired reservations: %w", err)
	}

	w.resumeAfter = nil
	if len(expired) == w.config.BatchSize {
		position := expired[len(expired)-1].Position()
		w.resumeAfter = &position
	}

	var findErr error

	unheld, err := w.findOperationPage(ctx, now)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to find expired operations", err)

		findErr = fmt.Errorf("failed to find expired operations: %w", err)
	}

	w.advanceOperations(unheld, err)

	operations := make([]model.ReserveOperationIdentity, 0, len(expired)+len(unheld))
	seen := make(map[model.ReserveOperationIdentity]struct{}, len(expired)+len(unheld))

	for _, reservation := range expired {
		if _, ok := seen[reservation.Operation]; !ok {
			seen[reservation.Operation] = struct{}{}
			operations = append(operations, reservation.Operation)
		}
	}

	for _, operation := range unheld {
		if _, ok := seen[operation.Operation]; !ok {
			seen[operation.Operation] = struct{}{}
			operations = append(operations, operation.Operation)
		}
	}

	if len(operations) == 0 {
		return 0, findErr
	}

	released, err := w.expireOperations(ctx, span, operations, now)
	if err != nil || findErr != nil {
		return released, errors.Join(findErr, err)
	}

	logger.With(
		libLog.String("operation", "worker.reservation_reaper.run_once"),
		libLog.String("now", now.Format(time.RFC3339)),
		libLog.Int("released_count", released),
	).Log(ctx, libLog.LevelDebug, "Released expired reservations")

	return released, nil
}

// findPage reads the sweep's page from the resume position, returning to the
// oldest expiry when nothing is left past it. It keeps the resume position on
// error so a failed read retries the same page.
func (w *ReservationReaperWorker) findPage(ctx context.Context, now time.Time) ([]model.ExpiredReservation, error) {
	expired, err := w.repo.FindExpiredReservations(ctx, now, w.resumeAfter, w.config.BatchSize)
	if err != nil || len(expired) > 0 || w.resumeAfter == nil {
		return expired, err
	}

	return w.repo.FindExpiredReservations(ctx, now, nil, w.config.BatchSize)
}

// findOperationPage is findPage for the operations that hold no reservation.
func (w *ReservationReaperWorker) findOperationPage(ctx context.Context, now time.Time) ([]model.ExpiredOperation, error) {
	expired, err := w.repo.FindExpiredOperations(ctx, now, w.resumeOperationsAfter, w.config.BatchSize)
	if err != nil || len(expired) > 0 || w.resumeOperationsAfter == nil {
		return expired, err
	}

	return w.repo.FindExpiredOperations(ctx, now, nil, w.config.BatchSize)
}

// advanceOperations moves the operation walk past a full page and back to the
// oldest expiry after a short one, keeping the position when the read failed.
func (w *ReservationReaperWorker) advanceOperations(page []model.ExpiredOperation, err error) {
	if err != nil {
		return
	}

	w.resumeOperationsAfter = nil

	if len(page) == w.config.BatchSize {
		position := page[len(page)-1].Position()
		w.resumeOperationsAfter = &position
	}
}

// expireOperations expires each owning operation once and counts the
// reservations the expirer moved. The expirer settles every reservation of the
// operation, including any the sweep's batch cap left unread, so the count is
// what moved rather than what this sweep happened to read.
func (w *ReservationReaperWorker) expireOperations(ctx context.Context, span trace.Span, operations []model.ReserveOperationIdentity, now time.Time) (int, error) {
	released := 0

	var errs []error

	for _, operation := range operations {
		moved, err := w.expirer.Execute(ctx, operation, now)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to expire reserve operation", err)

			errs = append(errs, fmt.Errorf("failed to expire reserve operation for transaction %s: %w", operation.TransactionID, err))

			continue
		}

		released += moved
	}

	return released, errors.Join(errs...)
}
