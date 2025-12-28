package outbox

import (
	"context"
	cryptoRand "crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	mathRand "math/rand"
	"regexp"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/lib/pq"
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
)

// piiPatterns defines patterns to sanitize from error messages
// TODO(review): Add IPv6 address patterns for complete IP sanitization
var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),            // email
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
	// TODO(review): Add early return for empty entityID or entityType parameters.
	FindByEntityID(ctx context.Context, entityID, entityType string) (*MetadataOutbox, error)

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
func SecureRandomFloat64() float64 {
	var b [8]byte
	if _, err := cryptoRand.Read(b[:]); err != nil {
		// Fallback to math/rand - less secure but acceptable for backoff jitter
		return mathRand.Float64() //nolint:gosec // Fallback for crypto failure
	}

	return float64(binary.BigEndian.Uint64(b[:])) / float64(^uint64(0))
}

// OutboxPostgreSQLRepository is a PostgreSQL implementation of the Repository.
type OutboxPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	tableName  string
}

// NewOutboxPostgreSQLRepository returns a new instance of OutboxPostgreSQLRepository.
func NewOutboxPostgreSQLRepository(pc *libPostgres.PostgresConnection) *OutboxPostgreSQLRepository {
	assert.NotNil(pc, "PostgreSQL connection must not be nil", "repository", "OutboxPostgreSQLRepository")

	db, err := pc.GetDB()
	assert.NoError(err, "database connection required for OutboxPostgreSQLRepository",
		"repository", "OutboxPostgreSQLRepository")
	assert.NotNil(db, "database handle must not be nil", "repository", "OutboxPostgreSQLRepository")

	return &OutboxPostgreSQLRepository{
		connection: pc,
		tableName:  "metadata_outbox",
	}
}

// getExecutor returns the transaction from context if present, otherwise the DB connection.
func (r *OutboxPostgreSQLRepository) getExecutor(ctx context.Context) (dbtx.Executor, error) {
	if tx := dbtx.TxFromContext(ctx); tx != nil {
		return tx, nil
	}

	return r.connection.GetDB()
}

// Create inserts a new outbox entry. If a transaction is in context, participates in it.
func (r *OutboxPostgreSQLRepository) Create(ctx context.Context, entry *MetadataOutbox) error {
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
	`

	_, err = executor.ExecContext(ctx, query,
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

	logger.Infof("Created outbox entry: entity_id=%s, entity_type=%s", entry.EntityID, entry.EntityType)

	return nil
}

// ClaimPendingBatch atomically retrieves and marks entries as PROCESSING.
// Uses FOR UPDATE SKIP LOCKED to prevent race conditions between concurrent workers.
// Also reclaims stale PROCESSING entries (older than StaleProcessingThreshold).
func (r *OutboxPostgreSQLRepository) ClaimPendingBatch(ctx context.Context, batchSize int) ([]*MetadataOutbox, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.claim_pending_batch")
	defer span.End()

	// Validate and normalize batchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	if batchSize > 1000 {
		batchSize = 1000 // Cap to prevent memory issues
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Start transaction for atomic select + update
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

	now := time.Now()
	staleThreshold := now.Add(-StaleProcessingThreshold)

	// Select entries with FOR UPDATE SKIP LOCKED to atomically claim them
	// Includes: PENDING, retriable FAILED, and stale PROCESSING entries
	query := `
		SELECT id, entity_id, entity_type, metadata, status, retry_count, max_retries,
		       next_retry_at, processing_started_at, last_error, created_at, updated_at, processed_at
		FROM metadata_outbox
		WHERE (status = $1)
		   OR (status = $2 AND next_retry_at <= $3 AND retry_count < max_retries)
		   OR (status = $4 AND processing_started_at < $5)
		ORDER BY created_at ASC
		LIMIT $6
		FOR UPDATE SKIP LOCKED
	`

	rows, err := tx.QueryContext(ctx, query,
		string(StatusPending),
		string(StatusFailed),
		now,
		string(StatusProcessing),
		staleThreshold,
		batchSize,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to query pending entries", err)
		logger.Errorf("Failed to query pending entries: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	entries := make([]*MetadataOutbox, 0, batchSize)
	ids := make([]string, 0, batchSize)

	for rows.Next() {
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
			libOpentelemetry.HandleSpanError(&span, "Failed to scan row", err)
			logger.Errorf("Failed to scan row: %v", err)

			return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
		}

		entry, err := model.ToEntity()
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to convert model to entity", err)
			logger.Errorf("Failed to convert model to entity: %v", err)

			return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
		}

		entries = append(entries, entry)
		ids = append(ids, model.ID)
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// If no entries found, commit empty transaction and return
	if len(entries) == 0 {
		if err := tx.Commit(); err != nil {
			logger.Errorf("Failed to commit empty transaction: %v", err)
		}

		return entries, nil
	}

	// Atomically mark all selected entries as PROCESSING within the same transaction
	// Note: We do NOT increment retry_count here - MarkFailed handles the increment when processing fails.
	// This avoids double-increment for stale entries that get reclaimed and then fail again.
	updateQuery := `
		UPDATE metadata_outbox
		SET status = $1,
		    processing_started_at = $2,
		    updated_at = $2
		WHERE id = ANY($3)
	`
	if _, err := tx.ExecContext(ctx, updateQuery, string(StatusProcessing), now, pq.Array(ids)); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entries as processing", err)
		logger.Errorf("Failed to mark entries as processing: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to commit transaction", err)
		logger.Errorf("Failed to commit transaction: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	logger.Infof("Claimed %d outbox entries for processing", len(entries))

	return entries, nil
}

// FindByEntityID checks if an entry exists for the given entity (for idempotency checks).
func (r *OutboxPostgreSQLRepository) FindByEntityID(ctx context.Context, entityID, entityType string) (*MetadataOutbox, error) {
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

	return model.ToEntity()
}

// MarkPublished marks an entry as successfully processed.
func (r *OutboxPostgreSQLRepository) MarkPublished(ctx context.Context, id string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_published")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	now := time.Now()
	query := `
		UPDATE metadata_outbox
		SET status = $1, updated_at = $2, processed_at = $3
		WHERE id = $4
	`

	result, err := db.ExecContext(ctx, query, string(StatusPublished), now, now, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as published", err)
		logger.Errorf("Failed to mark entry as published: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	logger.Infof("Marked outbox entry as published: id=%s", id)

	return nil
}

// MarkFailed increments retry count and schedules next retry.
// Error message is sanitized to remove PII before storage.
func (r *OutboxPostgreSQLRepository) MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error {
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

	query := `
		UPDATE metadata_outbox
		SET status = $1, retry_count = retry_count + 1, last_error = $2, next_retry_at = $3, updated_at = $4
		WHERE id = $5
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusFailed),
		sanitizedErr,
		nextRetryAt,
		time.Now(),
		id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as failed", err)
		logger.Errorf("Failed to mark entry as failed: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Log with correlation ID only, not the error message (to avoid PII in logs)
	logger.Warnf("Marked outbox entry as failed: id=%s, next_retry=%v", id, nextRetryAt)

	return nil
}

// MarkDLQ marks an entry as permanently failed (Dead Letter Queue).
// Error message is sanitized to remove PII before storage.
func (r *OutboxPostgreSQLRepository) MarkDLQ(ctx context.Context, id string, errMsg string) error {
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

	query := `
		UPDATE metadata_outbox
		SET status = $1, last_error = $2, updated_at = $3
		WHERE id = $4
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusDLQ),
		sanitizedErr,
		time.Now(),
		id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as DLQ", err)
		logger.Errorf("Failed to mark entry as DLQ: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Log DLQ event for alerting (no PII in message)
	// TODO(review): Consider adding metrics/alerting hook for DLQ entries
	logger.Warnf("METADATA_OUTBOX_DLQ: Entry moved to Dead Letter Queue: id=%s", id)

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
