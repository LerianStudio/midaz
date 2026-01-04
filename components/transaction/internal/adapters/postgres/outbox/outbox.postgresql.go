package outbox

import (
	"context"
	cryptoRand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	mathRand "math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
	"github.com/lib/pq"
	"go.opentelemetry.io/otel/trace"
)

// Default constants for outbox processing
const (
	DefaultBatchSize = 100
	// StaleProcessingThreshold is how long an entry can be in PROCESSING before being reclaimed.
	StaleProcessingThreshold = 5 * time.Minute
	// MaxErrorMessageLength limits error message size to prevent PII leakage.
	MaxErrorMessageLength = 500
)

// Static errors for outbox operations
var (
	ErrOutboxEntryNotFound = errors.New("outbox entry not found")
	ErrOutboxUpdateFailed  = errors.New("outbox update failed: no rows affected")
	ErrGetDBConnection     = errors.New("failed to get database connection")
	ErrScanRow             = errors.New("failed to scan row")
)

// secureRandLogger is used only for SecureRandomFloat64's (rare) crypto/rand fallback warning.
// It is initialized lazily to avoid repeated initialization and to respect global logger config timing.
var (
	secureRandLoggerOnce sync.Once
	secureRandLogger     libLog.Logger
)

func getSecureRandLogger() libLog.Logger {
	secureRandLoggerOnce.Do(func() {
		secureRandLogger = libZap.InitializeLogger()
	})

	return secureRandLogger
}

// piiPatterns defines patterns to sanitize from error messages
// TODO(review): Add IPv6 address patterns for complete IP sanitization
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),             // email
	regexp.MustCompile(`\+?\d{1,3}[-.\s]?\(?\d{1,4}\)?[-.\s]?\d{1,4}[-.\s]?\d{1,9}`), // phone (international)
	regexp.MustCompile(`\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b`),                 // credit card (with separators)
	regexp.MustCompile(`\b\d{4}[-\s]?\d{6}[-\s]?\d{5}\b`),                            // Amex (15 digits)
	regexp.MustCompile(`\b\d{3}[-\s]?\d{2}[-\s]?\d{4}\b`),                            // SSN (with separators)
	regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`),                                // IPv4 address
}

// Repository provides an interface for outbox operations.
//
//go:generate mockgen --destination=outbox.postgresql_mock.go --package=outbox . Repository
type Repository interface {
	// Create inserts a new outbox entry (participates in existing transaction if present).
	Create(ctx context.Context, entry *MetadataOutbox) error

	// ClaimPendingBatch atomically retrieves and marks entries as PROCESSING.
	// Uses FOR UPDATE SKIP LOCKED to prevent race conditions between workers.
	ClaimPendingBatch(ctx context.Context, batchSize int) ([]*MetadataOutbox, error)

	// FindByEntityID checks if an entry exists for the given entity (for idempotency).
	// Returns (nil, nil) if no entry exists - this is intentional for idempotency checks.
	// Returns (*MetadataOutbox, nil) if entry exists.
	// Returns (nil, error) on database errors.
	FindByEntityID(ctx context.Context, entityID, entityType string) (*MetadataOutbox, error)

	// FindMetadataByEntityIDs retrieves the latest metadata for each entity ID (if any).
	// Returns:
	// - metadataByID: map[entityID]metadata for IDs that have metadata
	// - errorsByID: map[entityID]error for IDs that failed to decode metadata (allows partial success)
	// - error: database-level error preventing the lookup entirely
	FindMetadataByEntityIDs(ctx context.Context, entityIDs []string, entityType string) (map[string]map[string]any, map[string]error, error)

	// MarkPublished marks an entry as successfully processed.
	// TODO(review): Add status precondition check for defense-in-depth.
	// TODO(review): Add UUID validation on id parameter.
	MarkPublished(ctx context.Context, id string) error

	// MarkFailed increments retry count and schedules next retry with backoff.
	// Error message is sanitized to remove PII before storage.
	MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error

	// MarkDLQ marks an entry as permanently failed (Dead Letter Queue).
	// TODO(review): Add status precondition check for defense-in-depth.
	// TODO(review): Add UUID validation on id parameter.
	MarkDLQ(ctx context.Context, id string, errMsg string) error

	// DeleteOldEntries removes old processed and DLQ entries (for cleanup).
	DeleteOldEntries(ctx context.Context, olderThan time.Time) (int64, error)
}

// SanitizeErrorMessage removes PII and truncates error messages for safe storage/logging.
func SanitizeErrorMessage(errMsg string) string {
	sanitized := errMsg
	for _, pattern := range piiPatterns {
		sanitized = pattern.ReplaceAllString(sanitized, "[REDACTED]")
	}
	// Truncate to max length
	if len(sanitized) > MaxErrorMessageLength {
		sanitized = sanitized[:MaxErrorMessageLength] + "...[truncated]"
	}
	// Remove any potential stack traces
	if idx := strings.Index(sanitized, "\n"); idx > 0 {
		sanitized = sanitized[:idx]
	}

	return sanitized
}

// SecureRandomFloat64 returns a cryptographically secure random float64 in [0,1).
// Exported for use by worker backoff calculation.
// Falls back to math/rand if crypto/rand fails (extremely rare, only in low-entropy environments).
// TODO(review): Consider adding a warning log when falling back to weak PRNG for observability.
func SecureRandomFloat64() float64 {
	var b [8]byte
	if _, err := cryptoRand.Read(b[:]); err != nil {
		// Observability: crypto/rand failures are rare and can indicate low entropy or system issues.
		// We keep execution unchanged (fallback remains) but emit a warning with non-sensitive context.
		getSecureRandLogger().WithFields(
			"component", "transaction",
			"subsystem", "outbox",
			"operation", "secure_random_float64",
			"function", "SecureRandomFloat64",
			"fallback", "math/rand",
			"fallback_at", time.Now().UTC().Format(time.RFC3339Nano),
			"error", SanitizeErrorMessage(err.Error()),
		).Warn("crypto/rand failed; falling back to weak PRNG for jitter/backoff")

		// Fallback to math/rand - less secure but acceptable for backoff jitter
		return mathRand.Float64() //nolint:gosec // Fallback for crypto failure
	}

	return float64(binary.BigEndian.Uint64(b[:])) / float64(^uint64(0))
}

// OutboxPostgreSQLRepository is a PostgreSQL implementation of the Repository.
type OutboxPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}

// NewOutboxPostgreSQLRepository returns a new instance of OutboxPostgreSQLRepository.
func NewOutboxPostgreSQLRepository(mw *mmigration.MigrationWrapper) *OutboxPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OutboxPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OutboxPostgreSQLRepository")

	return &OutboxPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "metadata_outbox",
	}
}

// getExecutor returns the transaction from context if present, otherwise the DB connection.
func (r *OutboxPostgreSQLRepository) getExecutor(ctx context.Context) (dbtx.Executor, error) {
	if tx := dbtx.TxFromContext(ctx); tx != nil {
		return tx, nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGetDBConnection, err)
	}

	return db, nil
}

// Create inserts a new outbox entry. If a transaction is in context, participates in it.
func (r *OutboxPostgreSQLRepository) Create(ctx context.Context, entry *MetadataOutbox) error {
	// Validate preconditions
	assert.NotNil(entry, "outbox entry must not be nil", "method", "Create")
	assert.That(entry.Status == StatusPending,
		"new outbox entry must have PENDING status",
		"actual_status", entry.Status, "method", "Create")
	assert.That(assert.NonNegative(int64(entry.RetryCount)),
		"retry_count must be non-negative", "retry_count", entry.RetryCount, "method", "Create")
	assert.That(entry.MaxRetries > 0,
		"max_retries must be greater than zero", "max_retries", entry.MaxRetries, "method", "Create")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.create")
	defer span.End()

	executor, err := r.getExecutor(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get executor", err)
		logger.Errorf("Failed to get executor: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	record := &MetadataOutboxPostgreSQLModel{}
	if err := record.FromEntity(entry); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert entity to model", err)
		logger.Errorf("Failed to convert entity to model: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		INSERT INTO metadata_outbox (id, entity_id, entity_type, metadata, status, retry_count, max_retries, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (entity_id, entity_type)
			WHERE status IN ('PENDING', 'PROCESSING')
		DO NOTHING
	`

	result, err := executor.ExecContext(ctx, query,
		record.ID,
		record.EntityID,
		record.EntityType,
		record.Metadata,
		record.Status,
		record.RetryCount,
		record.MaxRetries,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to insert outbox entry", err)
		logger.Errorf("Failed to insert outbox entry: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		// TODO(review): Consider returning an error or querying for existing entry to detect duplicates.
		// Currently we assume success and rely on downstream idempotency checks.
		logger.Warnf("Failed to read outbox insert rows affected: %v", rowsErr)
	} else {
		assert.That(rows == 0 || rows == 1,
			"outbox entry insert must affect at most one row",
			"rows_affected", rows,
			"entity_id", entry.EntityID,
			"entity_type", entry.EntityType)

		if rows == 0 {
			logger.Warnf("Duplicate outbox entry detected for entity_id=%s, entity_type=%s", entry.EntityID, entry.EntityType)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Duplicate outbox entry", ErrDuplicateOutboxEntry)
			// TODO(review): Consider using pkg.ValidateBusinessError for consistency with
			// other duplicate detection patterns (see pkg/errors.go for ErrDuplicateLedger).
			return ErrDuplicateOutboxEntry
		}
	}

	logger.Infof("Created outbox entry: entity_id=%s, entity_type=%s", entry.EntityID, entry.EntityType)

	return nil
}

// Batch size limits for claim operations
const (
	maxBatchSize = 1000
)

// normalizeBatchSize validates and normalizes the batch size within acceptable bounds.
func normalizeBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return DefaultBatchSize
	}

	if batchSize > maxBatchSize {
		return maxBatchSize
	}

	return batchSize
}

func assertValidOutboxStatus(status OutboxStatus, context string, kv ...any) {
	switch status {
	case StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ:
		return
	default:
		args := append([]any{"status", status, "context", context}, kv...)
		assert.Never("invalid outbox status", args...)
	}
}

// scanOutboxRow scans a single row into a MetadataOutboxPostgreSQLModel.
func scanOutboxRow(rows *sql.Rows) (*MetadataOutboxPostgreSQLModel, error) {
	var model MetadataOutboxPostgreSQLModel
	if err := rows.Scan(
		&model.ID,
		&model.EntityID,
		&model.EntityType,
		&model.Metadata,
		&model.Status,
		&model.RetryCount,
		&model.MaxRetries,
		&model.NextRetryAt,
		&model.ProcessingStartedAt,
		&model.LastError,
		&model.CreatedAt,
		&model.UpdatedAt,
		&model.ProcessedAt,
	); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrScanRow, err)
	}

	return &model, nil
}

// ClaimPendingBatch atomically retrieves and marks entries as PROCESSING.
// Uses FOR UPDATE SKIP LOCKED to prevent race conditions between concurrent workers.
// Also reclaims stale PROCESSING entries (older than StaleProcessingThreshold).
func (r *OutboxPostgreSQLRepository) ClaimPendingBatch(ctx context.Context, batchSize int) ([]*MetadataOutbox, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.claim_pending_batch")
	defer span.End()

	batchSize = normalizeBatchSize(batchSize)

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to begin transaction", err)
		logger.Errorf("Failed to begin transaction: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	entries, ids, err := r.queryAndScanPendingEntries(ctx, tx, batchSize, &span, logger)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		if err := tx.Rollback(); err != nil {
			logger.Errorf("Failed to rollback empty transaction: %v", err)
		}

		return entries, nil
	}

	now := time.Now()
	if err := r.markEntriesAsProcessing(ctx, tx, ids, now, &span, logger); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)
		logger.Errorf("Failed to commit transaction: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Update in-memory entries to reflect PROCESSING status
	// (the database was already updated in the transaction)
	for i, entry := range entries {
		assertValidOutboxStatus(entry.Status, "ClaimPendingBatch", "entry_id", entry.ID.String())
		assert.That(entry.Status == StatusPending || entry.Status == StatusProcessing || entry.Status == StatusFailed,
			"claimed entry must have been in claimable status",
			"index", i,
			"entry_id", entry.ID.String(),
			"status", entry.Status)
		assert.That(!now.Before(entry.CreatedAt),
			"processing_started_at must be >= created_at",
			"entry_id", entry.ID.String(),
			"created_at", entry.CreatedAt,
			"processing_started_at", now)
		entry.Status = StatusProcessing
		entry.ProcessingStartedAt = &now
	}

	// Postcondition: all returned entries must be in PROCESSING status
	for i, entry := range entries {
		assert.That(entry.Status == StatusProcessing,
			"claimed entry must be in PROCESSING status after claim",
			"index", i,
			"entry_id", entry.ID.String(),
			"actual_status", entry.Status)
	}

	logger.Infof("Claimed %d outbox entries for processing", len(entries))

	return entries, nil
}

// queryAndScanPendingEntries queries and scans pending outbox entries.
func (r *OutboxPostgreSQLRepository) queryAndScanPendingEntries(
	ctx context.Context,
	tx dbtx.Tx,
	batchSize int,
	span *trace.Span,
	logger libLog.Logger,
) ([]*MetadataOutbox, []string, error) {
	now := time.Now()
	staleThreshold := now.Add(-StaleProcessingThreshold)

	query := `
		SELECT id, entity_id, entity_type, metadata, status, retry_count, max_retries,
		       next_retry_at, processing_started_at, last_error, created_at, updated_at, processed_at
		FROM metadata_outbox
		WHERE (status = $1)
		   OR (status = $2 AND next_retry_at <= $3 AND retry_count < max_retries)
		   OR (status = $4 AND processing_started_at < $5)
		ORDER BY created_at DESC
		LIMIT $6
		FOR UPDATE SKIP LOCKED
	`

	fetchSize := batchSize * 3
	if fetchSize < batchSize {
		fetchSize = batchSize
	}

	if fetchSize > maxBatchSize {
		fetchSize = maxBatchSize
	}

	rows, err := tx.QueryContext(ctx, query,
		string(StatusPending),
		string(StatusFailed),
		now,
		string(StatusProcessing),
		staleThreshold,
		fetchSize,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to query pending entries", err)
		logger.Errorf("Failed to query pending entries: %v", err)

		return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	entries, _, err := r.collectEntriesFromRows(rows, fetchSize, span, logger)
	if err != nil {
		return nil, nil, err
	}

	dedupedEntries, dedupedIDs := dedupePendingEntries(entries, batchSize)

	return dedupedEntries, dedupedIDs, nil
}

// collectEntriesFromRows iterates over rows and collects entries and IDs.
func (r *OutboxPostgreSQLRepository) collectEntriesFromRows(
	rows *sql.Rows,
	batchSize int,
	span *trace.Span,
	logger libLog.Logger,
) ([]*MetadataOutbox, []string, error) {
	entries := make([]*MetadataOutbox, 0, batchSize)
	ids := make([]string, 0, batchSize)

	for rows.Next() {
		model, err := scanOutboxRow(rows)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
		}

		entry, err := model.ToEntity()
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert model to entity", err)
			logger.Errorf("Failed to convert model to entity: %v", err)

			return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
		}

		entries = append(entries, entry)
		ids = append(ids, model.ID)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	return entries, ids, nil
}

func dedupePendingEntries(entries []*MetadataOutbox, batchSize int) ([]*MetadataOutbox, []string) {
	if len(entries) == 0 {
		return nil, nil
	}

	selected := make([]*MetadataOutbox, 0, batchSize)
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		key := entry.EntityID + ":" + entry.EntityType
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		selected = append(selected, entry)
		if len(selected) >= batchSize {
			break
		}
	}

	sort.Slice(selected, func(i, j int) bool {
		return selected[i].CreatedAt.Before(selected[j].CreatedAt)
	})

	ids := make([]string, 0, len(selected))
	for _, entry := range selected {
		ids = append(ids, entry.ID.String())
	}

	return selected, ids
}

// markEntriesAsProcessing updates entries status to PROCESSING.
func (r *OutboxPostgreSQLRepository) markEntriesAsProcessing(
	ctx context.Context,
	tx dbtx.Tx,
	ids []string,
	now time.Time,
	span *trace.Span,
	logger libLog.Logger,
) error {
	updateQuery := `
		UPDATE metadata_outbox
		SET status = $1,
		    processing_started_at = $2,
		    updated_at = $2
		WHERE id = ANY($3)
	`

	if _, err := tx.ExecContext(ctx, updateQuery, string(StatusProcessing), now, pq.Array(ids)); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to mark entries as processing", err)
		logger.Errorf("Failed to mark entries as processing: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	return nil
}

// FindByEntityID checks if an entry exists for the given entity (for idempotency checks).
func (r *OutboxPostgreSQLRepository) FindByEntityID(ctx context.Context, entityID, entityType string) (*MetadataOutbox, error) {
	// Runtime validation (defense-in-depth): return a clear error instead of panicking on invalid inputs.
	// Keep assertions below for development/debug signal.
	if strings.TrimSpace(entityID) == "" {
		return nil, pkg.ValidationError{
			EntityType: "MetadataOutbox",
			Code:       constant.ErrBadRequest.Error(),
			Title:      "Invalid Argument",
			Message:    "entityID must not be empty",
		}
	}

	if strings.TrimSpace(entityType) == "" {
		return nil, pkg.ValidationError{
			EntityType: "MetadataOutbox",
			Code:       constant.ErrBadRequest.Error(),
			Title:      "Invalid Argument",
			Message:    "entityType must not be empty",
		}
	}

	// Validate preconditions (assertions for debug builds)
	assert.NotEmpty(entityID, "entityID must not be empty", "method", "FindByEntityID")
	assert.NotEmpty(entityType, "entityType must not be empty",
		"entityID", entityID, "method", "FindByEntityID")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.find_by_entity_id")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		SELECT id, entity_id, entity_type, metadata, status, retry_count, max_retries,
		       next_retry_at, processing_started_at, last_error, created_at, updated_at, processed_at
		FROM metadata_outbox
		WHERE entity_id = $1 AND entity_type = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var model MetadataOutboxPostgreSQLModel

	err = db.QueryRowContext(ctx, query, entityID, entityType).Scan(
		&model.ID,
		&model.EntityID,
		&model.EntityType,
		&model.Metadata,
		&model.Status,
		&model.RetryCount,
		&model.MaxRetries,
		&model.NextRetryAt,
		&model.ProcessingStartedAt,
		&model.LastError,
		&model.CreatedAt,
		&model.UpdatedAt,
		&model.ProcessedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // Not found is not an error for idempotency checks
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find entry by entity ID", err)
		logger.Errorf("Failed to find entry by entity ID: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	entry, err := model.ToEntity()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert model to entity", err)
		logger.Errorf("Failed to convert model to entity: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	assertValidOutboxStatus(entry.Status, "FindByEntityID", "entry_id", entry.ID.String())
	assert.That(assert.NonNegative(int64(entry.RetryCount)),
		"retry_count must be non-negative", "retry_count", entry.RetryCount, "method", "FindByEntityID")
	assert.That(entry.MaxRetries > 0,
		"max_retries must be greater than zero", "max_retries", entry.MaxRetries, "method", "FindByEntityID")

	return entry, nil
}

// FindMetadataByEntityIDs retrieves the latest metadata for each entity ID (if any) in a single query.
func (r *OutboxPostgreSQLRepository) FindMetadataByEntityIDs(ctx context.Context, entityIDs []string, entityType string) (map[string]map[string]any, map[string]error, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.find_metadata_by_entity_ids")
	defer span.End()

	metadataByID := make(map[string]map[string]any)
	errorsByID := make(map[string]error)

	if len(entityIDs) == 0 {
		return metadataByID, errorsByID, nil
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// DISTINCT ON picks the newest row per entity_id; ORDER BY ensures newest by created_at.
	query := `
		SELECT DISTINCT ON (entity_id) entity_id, metadata
		FROM metadata_outbox
		WHERE entity_type = $1 AND entity_id = ANY($2)
		ORDER BY entity_id, created_at DESC
	`

	rows, err := db.QueryContext(ctx, query, entityType, pq.Array(entityIDs))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to query outbox metadata by entity IDs", err)
		logger.Errorf("Failed to query outbox metadata by entity IDs: %v", err)

		return nil, nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			entityID    string
			rawMetadata []byte
		)

		if scanErr := rows.Scan(&entityID, &rawMetadata); scanErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan outbox metadata row", scanErr)
			logger.Errorf("Failed to scan outbox metadata row: %v", scanErr)

			return nil, nil, pkg.ValidateInternalError(scanErr, "MetadataOutbox")
		}

		// Decode only the metadata blob (avoid materializing full outbox entity).
		var metadata map[string]any
		if unmarshalErr := json.Unmarshal(rawMetadata, &metadata); unmarshalErr != nil {
			// Keep partial-success behavior (skip bad row) but make the issue observable for operators.
			// Avoid logging raw metadata to reduce PII exposure; length is useful for debugging corruption/truncation.
			logger.Warnf(
				"Failed to unmarshal outbox metadata JSON: entity_id=%s, entity_type=%s, metadata_len=%d, err=%v",
				entityID,
				entityType,
				len(rawMetadata),
				unmarshalErr,
			)
			errorsByID[entityID] = fmt.Errorf("%w: %w", ErrUnmarshalMetadata, unmarshalErr)

			continue
		}

		if len(metadata) != 0 {
			metadataByID[entityID] = metadata
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed iterating outbox metadata rows", rowsErr)
		logger.Errorf("Failed iterating outbox metadata rows: %v", rowsErr)

		return nil, nil, pkg.ValidateInternalError(rowsErr, "MetadataOutbox")
	}

	return metadataByID, errorsByID, nil
}

// MarkPublished marks an entry as successfully processed.
func (r *OutboxPostgreSQLRepository) MarkPublished(ctx context.Context, id string) error {
	// Validate UUID format
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkPublished")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_published")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	now := time.Now()
	// Enforce state machine: only PROCESSING -> PUBLISHED is valid
	query := `
		UPDATE metadata_outbox
		SET status = $1, updated_at = $2, processed_at = $3
		WHERE id = $4 AND status = $5 AND processing_started_at IS NOT NULL
		RETURNING processing_started_at
	`

	rows, err := db.QueryContext(ctx, query, string(StatusPublished), now, now, id, string(StatusProcessing))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as published", err)
		logger.Errorf("Failed to mark entry as published: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	var (
		processingStartedAt time.Time
		rowsAffected        int64
	)
	for rows.Next() {
		if scanErr := rows.Scan(&processingStartedAt); scanErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan marked published row", scanErr)
			logger.Errorf("Failed to scan marked published row: %v", scanErr)

			return pkg.ValidateInternalError(scanErr, "MetadataOutbox")
		}

		rowsAffected++
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate marked published rows", rowsErr)
		logger.Errorf("Failed to iterate marked published rows: %v", rowsErr)

		return pkg.ValidateInternalError(rowsErr, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING status
		logger.Warnf("MarkPublished: no rows affected - entry may not exist or not in PROCESSING status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Postcondition: processing must have started before publish
	assert.That(!processingStartedAt.IsZero(), "processing_started_at must be set when publishing",
		"id", id)
	assert.That(!now.Before(processingStartedAt), "processed_at must be >= processing_started_at",
		"id", id, "processed_at", now, "processing_started_at", processingStartedAt)

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark published must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	logger.Infof("Marked outbox entry as published: id=%s", id)

	return nil
}

// MarkFailed increments retry count and schedules next retry.
// Error message is sanitized to remove PII before storage.
func (r *OutboxPostgreSQLRepository) MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error {
	// Validate preconditions
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkFailed")
	assert.NotEmpty(errMsg, "error message must not be empty",
		"id", id, "method", "MarkFailed")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_failed")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Sanitize error message to remove PII before storing
	sanitizedErr := SanitizeErrorMessage(errMsg)
	assert.NotEmpty(sanitizedErr, "sanitized error message must not be empty",
		"id", id, "method", "MarkFailed")

	now := time.Now()
	assert.That(!nextRetryAt.Before(now), "next_retry_at must not be in the past",
		"id", id, "next_retry_at", nextRetryAt, "now", now)

	// Enforce state machine: only PROCESSING -> FAILED is valid
	query := `
		UPDATE metadata_outbox
		SET status = $1, retry_count = retry_count + 1, last_error = $2, next_retry_at = $3, updated_at = $4
		WHERE id = $5 AND status = $6 AND processing_started_at IS NOT NULL AND retry_count < max_retries
		RETURNING retry_count, max_retries, processing_started_at
	`

	rows, err := db.QueryContext(ctx, query,
		string(StatusFailed),
		sanitizedErr,
		nextRetryAt,
		now,
		id,
		string(StatusProcessing),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as failed", err)
		logger.Errorf("Failed to mark entry as failed: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	var (
		retryCount          int
		maxRetries          int
		processingStartedAt time.Time
		rowsAffected        int64
	)
	for rows.Next() {
		if scanErr := rows.Scan(&retryCount, &maxRetries, &processingStartedAt); scanErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan marked failed row", scanErr)
			logger.Errorf("Failed to scan marked failed row: %v", scanErr)

			return pkg.ValidateInternalError(scanErr, "MetadataOutbox")
		}

		rowsAffected++
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate marked failed rows", rowsErr)
		logger.Errorf("Failed to iterate marked failed rows: %v", rowsErr)

		return pkg.ValidateInternalError(rowsErr, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING status
		logger.Warnf("MarkFailed: no rows affected - entry may not exist or not in PROCESSING status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	assert.That(retryCount <= maxRetries, "retry_count must not exceed max_retries",
		"id", id, "retry_count", retryCount, "max_retries", maxRetries)
	assert.That(!processingStartedAt.IsZero(), "processing_started_at must be set when failing",
		"id", id)
	assert.That(!processingStartedAt.After(now), "processing_started_at must be <= now",
		"id", id, "processing_started_at", processingStartedAt, "now", now)

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark failed must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	// Log with correlation ID only, not the error message (to avoid PII in logs)
	logger.Warnf("Marked outbox entry as failed: id=%s, next_retry=%v", id, nextRetryAt)

	return nil
}

// MarkDLQ marks an entry as permanently failed (Dead Letter Queue).
// Error message is sanitized to remove PII before storage.
// Also increments retry_count to reflect the final failed attempt.
func (r *OutboxPostgreSQLRepository) MarkDLQ(ctx context.Context, id string, errMsg string) error {
	// Validate preconditions
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkDLQ")
	assert.NotEmpty(errMsg, "DLQ reason must not be empty",
		"id", id, "method", "MarkDLQ")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_dlq")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Sanitize error message to remove PII before storing
	sanitizedErr := SanitizeErrorMessage(errMsg)
	assert.NotEmpty(sanitizedErr, "sanitized DLQ reason must not be empty",
		"id", id, "method", "MarkDLQ")

	now := time.Now()

	// Enforce state machine: PROCESSING -> DLQ or FAILED -> DLQ are valid
	// Note: PROCESSING -> DLQ happens when processing fails and this is the final attempt
	// We accept entries where retry_count = max_retries - 1 (final attempt) or retry_count >= max_retries
	// The retry_count is incremented to reflect the final failed attempt
	query := `
		UPDATE metadata_outbox
		SET status = $1, last_error = $2, updated_at = $3, retry_count = retry_count + 1
		WHERE id = $4 AND status IN ($5, $6) AND retry_count >= max_retries - 1 AND processed_at IS NULL
		RETURNING retry_count, max_retries, processed_at
	`

	rows, err := db.QueryContext(ctx, query,
		string(StatusDLQ),
		sanitizedErr,
		now,
		id,
		string(StatusProcessing),
		string(StatusFailed),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as DLQ", err)
		logger.Errorf("Failed to mark entry as DLQ: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	var (
		retryCount   int
		maxRetries   int
		processedAt  sql.NullTime
		rowsAffected int64
	)
	for rows.Next() {
		if scanErr := rows.Scan(&retryCount, &maxRetries, &processedAt); scanErr != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to scan marked DLQ row", scanErr)
			logger.Errorf("Failed to scan marked DLQ row: %v", scanErr)

			return pkg.ValidateInternalError(scanErr, "MetadataOutbox")
		}

		rowsAffected++
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate marked DLQ rows", rowsErr)
		logger.Errorf("Failed to iterate marked DLQ rows: %v", rowsErr)

		return pkg.ValidateInternalError(rowsErr, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING/FAILED status
		logger.Warnf("MarkDLQ: no rows affected - entry may not exist or in invalid status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// After increment, retry_count should be >= max_retries
	assert.That(retryCount >= maxRetries, "retry_count must be >= max_retries after DLQ (post-increment)",
		"id", id, "retry_count", retryCount, "max_retries", maxRetries)
	assert.That(!processedAt.Valid, "processed_at must be nil for DLQ entries",
		"id", id)

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark DLQ must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	// Log DLQ event for alerting (no PII in message)
	// TODO(review): Consider adding metrics/alerting hook for DLQ entries
	logger.Warnf("METADATA_OUTBOX_DLQ: Entry moved to Dead Letter Queue: id=%s, final_retry_count=%d", id, retryCount)

	return nil
}

// DeleteOldEntries removes old processed and DLQ entries for cleanup.
// Cleans up both PUBLISHED (successful) and DLQ (permanently failed) entries.
func (r *OutboxPostgreSQLRepository) DeleteOldEntries(ctx context.Context, olderThan time.Time) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.delete_old_entries")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Delete both PUBLISHED and DLQ entries older than threshold
	query := `
		DELETE FROM metadata_outbox
		WHERE (status = $1 AND processed_at < $3)
		   OR (status = $2 AND updated_at < $3)
	`

	result, err := db.ExecContext(ctx, query, string(StatusPublished), string(StatusDLQ), olderThan)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete old entries", err)
		logger.Errorf("Failed to delete old entries: %v", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected > 0 {
		logger.Infof("Deleted %d old outbox entries (PUBLISHED/DLQ) older than %v", rowsAffected, olderThan)
	}

	return rowsAffected, nil
}
