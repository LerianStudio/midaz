// Package bootstrap provides initialization and server lifecycle management for the transaction service.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mretry"
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// metadataOutboxPollInterval is how often to poll for pending outbox entries.
	metadataOutboxPollInterval = 5 * time.Second

	// metadataOutboxCleanupInterval is how often to cleanup old entries.
	metadataOutboxCleanupInterval = 1 * time.Hour

	// metadataOutboxHealthCheckTimeout is the timeout for infrastructure health checks.
	metadataOutboxHealthCheckTimeout = 5 * time.Second

	// metadataOutboxProcessingTimeout is the timeout for processing a single metadata outbox entry.
	metadataOutboxProcessingTimeout = 30 * time.Second
)

// Metric names for observability (for future Prometheus integration)
const (
	MetricMetadataOutboxProcessed    = "metadata_outbox_processed_total"
	MetricMetadataOutboxFailed       = "metadata_outbox_failed_total"
	MetricMetadataOutboxDLQ          = "metadata_outbox_dlq_total"
	MetricMetadataOutboxProcessingMs = "metadata_outbox_processing_duration_ms"
)

// ErrMetadataOutboxPanicRecovered is returned when a panic is recovered during outbox processing.
var ErrMetadataOutboxPanicRecovered = errors.New("panic recovered during metadata outbox processing")

// MetadataOutboxWorker processes pending metadata outbox entries.
// It polls PostgreSQL for pending entries and creates metadata in MongoDB.
type MetadataOutboxWorker struct {
	logger        libLog.Logger
	outboxRepo    outbox.Repository
	metadataRepo  mongodb.Repository
	postgresConn  *libPostgres.PostgresConnection
	mongoConn     *libMongo.MongoConnection
	maxWorkers    int
	retentionDays int
	retryConfig   mretry.Config // Shared retry configuration
}

// NewMetadataOutboxWorker creates a new MetadataOutboxWorker instance.
// If no retry config is provided, defaults to mretry.DefaultMetadataOutboxConfig().
func NewMetadataOutboxWorker(
	logger libLog.Logger,
	outboxRepo outbox.Repository,
	metadataRepo mongodb.Repository,
	postgresConn *libPostgres.PostgresConnection,
	mongoConn *libMongo.MongoConnection,
	maxWorkers int,
	retentionDays int,
	config ...mretry.Config,
) *MetadataOutboxWorker {
	assert.NotNil(logger, "Logger required for MetadataOutboxWorker")
	assert.NotNil(outboxRepo, "OutboxRepository required for MetadataOutboxWorker")
	assert.NotNil(metadataRepo, "MetadataRepository required for MetadataOutboxWorker")
	assert.NotNil(postgresConn, "PostgresConnection required for MetadataOutboxWorker")

	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	if retentionDays <= 0 {
		retentionDays = 7
	}
	// maxWorkers and retentionDays are defaulted above to positive values when unset/invalid.

	// Use provided config or default
	retryConfig := mretry.DefaultMetadataOutboxConfig()
	if len(config) > 0 {
		retryConfig = config[0]
	}

	return &MetadataOutboxWorker{
		logger:        logger,
		outboxRepo:    outboxRepo,
		metadataRepo:  metadataRepo,
		postgresConn:  postgresConn,
		mongoConn:     mongoConn,
		maxWorkers:    maxWorkers,
		retentionDays: retentionDays,
		retryConfig:   retryConfig,
	}
}

// Run starts the metadata outbox worker loop.
// It blocks until the context is cancelled or an interrupt signal is received.
func (w *MetadataOutboxWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w.logger.Info("MetadataOutboxWorker started")

	pollTicker := time.NewTicker(metadataOutboxPollInterval)
	defer pollTicker.Stop()

	cleanupTicker := time.NewTicker(metadataOutboxCleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("MetadataOutboxWorker: shutting down...")
			return nil

		case <-pollTicker.C:
			w.processPendingEntries(ctx)

		case <-cleanupTicker.C:
			w.cleanupOldEntries(ctx)
		}
	}
}

// processPendingEntries claims and processes a batch of pending outbox entries.
func (w *MetadataOutboxWorker) processPendingEntries(ctx context.Context) {
	// Setup context with correlation ID for this batch
	correlationID := libCommons.GenerateUUIDv7().String()

	log := w.logger.WithFields(
		libConstants.HeaderID, correlationID,
	).WithDefaultMessageTemplate(correlationID + " | ")

	ctx = libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, correlationID),
		log,
	)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata_outbox.worker.process_pending_entries")
	defer span.End()

	// Health check before processing
	if !w.isInfrastructureHealthy(ctx) {
		logger.Debug("METADATA_OUTBOX_HEALTH_CHECK_FAILED: Infrastructure not ready, skipping processing")
		return
	}

	// Claim a batch of pending entries
	entries, err := w.outboxRepo.ClaimPendingBatch(ctx, w.maxWorkers)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to claim pending batch", err)
		logger.Errorf("MetadataOutboxWorker: Failed to claim pending batch: %v", err)

		return
	}

	if len(entries) == 0 {
		return
	}

	span.SetAttributes(attribute.Int("metadata_outbox.batch_size", len(entries)))
	logger.Infof("MetadataOutboxWorker: Processing %d entries", len(entries))

	// Process entries concurrently with worker pool
	sem := make(chan struct{}, w.maxWorkers)

	var wg sync.WaitGroup

entriesLoop:
	for _, entry := range entries {
		e := entry // capture for goroutine

		// Increment before any potentially-blocking operation so wg.Wait() can't hang on cancellation.
		wg.Add(1)

		// Acquire a worker slot or bail if the context is already cancelled.
		select {
		case sem <- struct{}{}:
			// slot acquired
		case <-ctx.Done():
			wg.Done()
			break entriesLoop
		}

		// If we acquired a slot but context got cancelled before launching the goroutine,
		// release the slot and decrement the waitgroup to avoid leaks.
		select {
		case <-ctx.Done():
			<-sem
			wg.Done()

			break entriesLoop
		default:
		}

		mruntime.SafeGoWithContextAndComponent(ctx, logger, "transaction", "metadata_outbox_worker", mruntime.KeepRunning, func(ctx context.Context) {
			defer func() { <-sem }()
			defer wg.Done()

			w.processEntry(ctx, e)
		})
	}

	wg.Wait()
}

// processEntry processes a single outbox entry.
func (w *MetadataOutboxWorker) processEntry(ctx context.Context, entry *outbox.MetadataOutbox) {
	// Add processing timeout to prevent hanging on slow MongoDB operations
	ctx, cancel := context.WithTimeout(ctx, metadataOutboxProcessingTimeout)
	defer cancel()

	// Setup context with correlation ID for this entry
	correlationID := libCommons.GenerateUUIDv7().String()

	log := w.logger.WithFields(
		libConstants.HeaderID, correlationID,
		"outbox_id", entry.ID.String(),
		"entity_id", entry.EntityID,
		"entity_type", entry.EntityType,
	).WithDefaultMessageTemplate(correlationID + " | ")

	ctx = libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, correlationID),
		log,
	)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata_outbox.worker.process_entry")
	defer span.End()

	span.SetAttributes(
		attribute.String("metadata_outbox.entry_id", entry.ID.String()),
		attribute.String("metadata_outbox.entity_id", entry.EntityID),
		attribute.String("metadata_outbox.entity_type", entry.EntityType),
		attribute.Int("metadata_outbox.retry_count", entry.RetryCount),
	)

	// Panic recovery with span event recording
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			span.AddEvent("panic.recovered", trace.WithAttributes(
				attribute.String("panic.value", fmt.Sprintf("%v", rec)),
				attribute.String("panic.stack", string(stack)),
				attribute.String("entry_id", entry.ID.String()),
			))
			libOpentelemetry.HandleSpanError(&span, "Panic during metadata outbox processing", w.panicAsError(rec))
			// Re-panic so outer mruntime.SafeGo wrapper can record metrics
			//nolint:panicguardwarn // Intentional re-panic for observability chain
			panic(rec)
		}
	}()

	// Idempotency check: verify if metadata already exists in MongoDB
	existing, err := w.metadataRepo.FindByEntity(ctx, w.entityTypeToCollection(entry.EntityType), entry.EntityID)
	if err != nil {
		w.handleProcessingError(ctx, entry, err, logger, &span)
		return
	}

	if existing != nil {
		// Metadata already exists - mark as published (idempotent)
		logger.Infof("MetadataOutboxWorker: Metadata already exists for entity %s, marking as published", entry.EntityID)

		if err := w.outboxRepo.MarkPublished(ctx, entry.ID.String()); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as published (idempotent)", err)
			logger.Errorf("MetadataOutboxWorker: Failed to mark entry as published: %v", err)
		}

		return
	}

	// Create metadata in MongoDB
	metadata := &mongodb.Metadata{
		EntityID:   entry.EntityID,
		EntityName: entry.EntityType,
		Data:       mongodb.JSON(entry.Metadata),
		CreatedAt:  entry.CreatedAt,
		UpdatedAt:  time.Now(),
	}

	if err := w.metadataRepo.Create(ctx, w.entityTypeToCollection(entry.EntityType), metadata); err != nil {
		w.handleProcessingError(ctx, entry, err, logger, &span)
		return
	}

	// Mark as published
	if err := w.outboxRepo.MarkPublished(ctx, entry.ID.String()); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as published", err)
		logger.Errorf("MetadataOutboxWorker: Failed to mark entry as published: %v", err)

		return
	}

	logger.Infof("MetadataOutboxWorker: Successfully processed entry %s for entity %s", entry.ID.String(), entry.EntityID)
}

// handleProcessingError handles errors during entry processing.
// It applies exponential backoff and marks entries as failed or DLQ.
func (w *MetadataOutboxWorker) handleProcessingError(
	ctx context.Context,
	entry *outbox.MetadataOutbox,
	err error,
	logger libLog.Logger,
	span *trace.Span,
) {
	libOpentelemetry.HandleSpanError(span, "Failed to process metadata outbox entry", err)

	// Calculate what the new retry count would be after this failure
	newRetryCount := entry.RetryCount + 1

	// Check if this failure would exceed max retries
	// If newRetryCount >= MaxRetries, we've exhausted all retries and must move to DLQ
	if newRetryCount >= entry.MaxRetries {
		logger.Errorf("MetadataOutboxWorker: Max retries exceeded for entry %s (retry %d/%d), moving to DLQ",
			entry.ID.String(), newRetryCount, entry.MaxRetries)

		if dlqErr := w.outboxRepo.MarkDLQ(ctx, entry.ID.String(), err.Error()); dlqErr != nil {
			logger.Errorf("MetadataOutboxWorker: Failed to mark entry as DLQ: %v", dlqErr)
			// Entry will be reclaimed as stale - acceptable since MarkDLQ is idempotent
			// TODO(review): Add metrics for monitoring persistent DLQ failures
		}

		return
	}

	// Calculate backoff and schedule retry
	backoff := w.calculateBackoff(newRetryCount)
	nextRetryAt := time.Now().Add(backoff)

	logger.Warnf("MetadataOutboxWorker: Entry %s failed, scheduling retry at %v (attempt %d/%d)",
		entry.ID.String(), nextRetryAt, newRetryCount, entry.MaxRetries)

	if failErr := w.outboxRepo.MarkFailed(ctx, entry.ID.String(), err.Error(), nextRetryAt); failErr != nil {
		logger.Errorf("MetadataOutboxWorker: Failed to mark entry as failed: %v", failErr)
	}
}

// calculateBackoff calculates exponential backoff with jitter using the worker's retry config.
func (w *MetadataOutboxWorker) calculateBackoff(attempt int) time.Duration {
	cfg := w.retryConfig
	if attempt <= 0 {
		return cfg.InitialBackoff
	}

	// Exponential backoff: initial * 2^(attempt-1)
	backoff := cfg.InitialBackoff
	shift := attempt - 1

	// Compute multiplier = 1 << (attempt-1) safely.
	// Shifting by >= 63 would overflow int64/time.Duration, so treat it as overflow and cap early.
	if shift >= 63 {
		backoff = cfg.MaxBackoff
	} else {
		multiplier := uint64(1) << uint(shift)

		// Avoid intermediate overflow when multiplying durations by doing a capped multiplication.
		// If initialBackoff > maxBackoff/multiplier, the product would exceed maxBackoff, so cap.
		if cfg.InitialBackoff > 0 && cfg.MaxBackoff > 0 {
			initialU := uint64(cfg.InitialBackoff)
			maxU := uint64(cfg.MaxBackoff)
			if initialU > maxU/multiplier {
				backoff = cfg.MaxBackoff
			} else {
				backoff = time.Duration(initialU * multiplier)
			}
		}
	}

	// Add jitter using config's jitter factor
	jitter := time.Duration(float64(backoff) * cfg.JitterFactor * outbox.SecureRandomFloat64())
	backoff += jitter

	// Cap at max backoff
	if backoff > cfg.MaxBackoff {
		backoff = cfg.MaxBackoff
	}

	return backoff
}

// isInfrastructureHealthy checks if PostgreSQL and MongoDB are available.
func (w *MetadataOutboxWorker) isInfrastructureHealthy(ctx context.Context) bool {
	healthCtx, cancel := context.WithTimeout(ctx, metadataOutboxHealthCheckTimeout)
	defer cancel()

	// Check PostgreSQL
	if w.postgresConn != nil {
		db, err := w.postgresConn.GetDB()
		if err != nil {
			w.logger.Warnf("METADATA_OUTBOX_HEALTH_CHECK: PostgreSQL connection failed: %v", err)
			return false
		}

		if err := db.PingContext(healthCtx); err != nil {
			w.logger.Warnf("METADATA_OUTBOX_HEALTH_CHECK: PostgreSQL unhealthy: %v", err)
			return false
		}
	}

	// Check MongoDB
	if w.mongoConn != nil {
		db, err := w.mongoConn.GetDB(healthCtx)
		if err != nil {
			w.logger.Warnf("METADATA_OUTBOX_HEALTH_CHECK: MongoDB connection failed: %v", err)
			return false
		}

		if err := db.Ping(healthCtx, nil); err != nil {
			w.logger.Warnf("METADATA_OUTBOX_HEALTH_CHECK: MongoDB unhealthy: %v", err)
			return false
		}
	}

	return true
}

// cleanupOldEntries removes old processed and DLQ entries.
func (w *MetadataOutboxWorker) cleanupOldEntries(ctx context.Context) {
	correlationID := libCommons.GenerateUUIDv7().String()

	log := w.logger.WithFields(
		libConstants.HeaderID, correlationID,
	).WithDefaultMessageTemplate(correlationID + " | ")

	ctx = libCommons.ContextWithLogger(
		libCommons.ContextWithHeaderID(ctx, correlationID),
		log,
	)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "metadata_outbox.worker.cleanup_old_entries")
	defer span.End()

	retentionCutoff := time.Now().AddDate(0, 0, -w.retentionDays)

	deleted, err := w.outboxRepo.DeleteOldEntries(ctx, retentionCutoff)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to cleanup old entries", err)
		logger.Errorf("MetadataOutboxWorker: Failed to cleanup old entries: %v", err)

		return
	}

	if deleted > 0 {
		logger.Infof("MetadataOutboxWorker: Cleaned up %d old entries older than %v", deleted, retentionCutoff)
	}
}

// entityTypeToCollection converts entity type to MongoDB collection name.
// Current implementation: simple lowercase (e.g., "Transaction" -> "transaction").
// Note: If collection naming conventions diverge from entity types, update this mapping.
func (w *MetadataOutboxWorker) entityTypeToCollection(entityType string) string {
	return strings.ToLower(entityType)
}

// panicAsError converts a recovered panic value to an error.
func (w *MetadataOutboxWorker) panicAsError(rec any) error {
	var panicErr error

	if err, ok := rec.(error); ok {
		panicErr = fmt.Errorf("%w: %w", ErrMetadataOutboxPanicRecovered, err)
	} else {
		panicErr = fmt.Errorf("%w: %s", ErrMetadataOutboxPanicRecovered, fmt.Sprint(rec))
	}

	return pkg.ValidateInternalError(panicErr, "MetadataOutboxWorker")
}
