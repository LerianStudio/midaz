# Metadata Outbox Pattern Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Implement a PostgreSQL-based outbox pattern to ensure reliable MongoDB metadata creation after transactions commit, eliminating the current gap where metadata creation failures are only logged but not retried.

**Architecture:** PostgreSQL stores pending metadata entries atomically with the transaction. A dedicated worker polls the outbox table, creates MongoDB metadata, and marks entries as processed. Failed entries are retried with exponential backoff, and permanently failed entries route to DLQ after max retries.

**Tech Stack:** Go 1.21+, PostgreSQL (existing), MongoDB (existing), existing retry/DLQ patterns

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: `make`, `go`, PostgreSQL client, MongoDB client
- Access: Database credentials configured in `.env`
- State: Work from `main` branch, clean working tree

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go version go1.21+ darwin/arm64
make --version      # Expected: GNU Make 3.8+
git status          # Expected: clean working tree on main branch
```

## Historical Precedent

**Query:** "outbox pattern postgresql mongodb metadata worker retry"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Phase 1: Database Schema

### Task 1.1: Create metadata_outbox table migration (UP)

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.up.sql`

**Prerequisites:**
- Existing migrations in `components/transaction/migrations/`
- Latest migration is `000017_fix_balance_affected.up.sql`

**Step 1: Create the UP migration file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.up.sql`:

```sql
-- Metadata Outbox table for reliable MongoDB metadata creation
-- Entries are created atomically with PostgreSQL transactions and processed asynchronously
CREATE TABLE IF NOT EXISTS metadata_outbox (
    id                  UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
    entity_id           TEXT NOT NULL,                    -- ID of the entity (transaction/operation)
    entity_type         TEXT NOT NULL,                    -- Type: 'Transaction' or 'Operation'
    metadata            JSONB NOT NULL,                   -- The metadata to create in MongoDB
    status              TEXT NOT NULL DEFAULT 'PENDING',  -- PENDING, PROCESSING, PUBLISHED, FAILED
    retry_count         INTEGER NOT NULL DEFAULT 0,       -- Number of retry attempts
    max_retries         INTEGER NOT NULL DEFAULT 10,      -- Maximum retry attempts before DLQ
    next_retry_at       TIMESTAMP WITH TIME ZONE,         -- When to retry next (for backoff)
    last_error          TEXT,                             -- Last error message
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    processed_at        TIMESTAMP WITH TIME ZONE          -- When successfully processed
);

-- Index for polling pending entries efficiently
CREATE INDEX idx_metadata_outbox_pending ON metadata_outbox (status, next_retry_at)
    WHERE status IN ('PENDING', 'FAILED') AND (next_retry_at IS NULL OR next_retry_at <= now());

-- Index for finding entries by entity
CREATE INDEX idx_metadata_outbox_entity ON metadata_outbox (entity_id, entity_type);

-- Index for cleanup of old processed entries
CREATE INDEX idx_metadata_outbox_processed ON metadata_outbox (processed_at)
    WHERE status = 'PUBLISHED';

COMMENT ON TABLE metadata_outbox IS 'Outbox pattern table for reliable MongoDB metadata creation';
COMMENT ON COLUMN metadata_outbox.status IS 'PENDING=new, PROCESSING=being processed, PUBLISHED=done, FAILED=exceeded retries';
```

**Step 2: Verify file exists and syntax is valid**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.up.sql | head -20`

**Expected output:**
```
-- Metadata Outbox table for reliable MongoDB metadata creation
-- Entries are created atomically with PostgreSQL transactions and processed asynchronously
CREATE TABLE IF NOT EXISTS metadata_outbox (
    id                  UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
```

---

### Task 1.2: Create metadata_outbox table migration (DOWN)

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.down.sql`

**Prerequisites:**
- Task 1.1 completed

**Step 1: Create the DOWN migration file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.down.sql`:

```sql
DROP TABLE IF EXISTS metadata_outbox;
```

**Step 2: Verify file exists**

Run: `cat /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000018_create_metadata_outbox_table.down.sql`

**Expected output:**
```
DROP TABLE IF EXISTS metadata_outbox;
```

**Step 3: Commit Phase 1**

```bash
git add components/transaction/migrations/000018_create_metadata_outbox_table.up.sql components/transaction/migrations/000018_create_metadata_outbox_table.down.sql
git commit -m "$(cat <<'EOF'
feat(transaction): add metadata_outbox table for reliable metadata creation

Add PostgreSQL outbox table to store pending MongoDB metadata creation
requests atomically with transaction commits. This enables reliable
async processing with retry logic and DLQ routing.
EOF
)"
```

**If Task Fails:**
1. **File already exists:** Check migration number, use next available
2. **Syntax error:** Validate SQL syntax with `psql -f <file>`
3. **Rollback:** `git checkout -- components/transaction/migrations/`

---

### Code Review Checkpoint 1

**REQUIRED:** After completing Phase 1, run code review.

1. Dispatch all 3 reviewers in parallel using `requesting-code-review` skill
2. Fix Critical/High/Medium issues immediately
3. Add `TODO(review):` comments for Low issues
4. Proceed only when zero Critical/High/Medium remain

---

## Phase 2: Outbox Domain Model

### Task 2.1: Create outbox model and entity types

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go`

**Prerequisites:**
- Familiarity with existing model pattern in `balance/balance.go`
- Go 1.21+

**Step 1: Create the outbox package directory**

Run: `mkdir -p /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox`

**Step 2: Create the model file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go`:

```go
// Package outbox provides the outbox pattern implementation for reliable async processing.
// It stores pending operations in PostgreSQL and processes them asynchronously.
package outbox

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OutboxStatus represents the processing status of an outbox entry.
type OutboxStatus string

const (
	// StatusPending indicates the entry is waiting to be processed.
	StatusPending OutboxStatus = "PENDING"
	// StatusProcessing indicates the entry is currently being processed.
	StatusProcessing OutboxStatus = "PROCESSING"
	// StatusPublished indicates the entry was successfully processed.
	StatusPublished OutboxStatus = "PUBLISHED"
	// StatusFailed indicates the entry exceeded max retries.
	StatusFailed OutboxStatus = "FAILED"
)

// MetadataOutbox represents a pending metadata creation request.
type MetadataOutbox struct {
	ID          uuid.UUID         `json:"id"`
	EntityID    string            `json:"entity_id"`
	EntityType  string            `json:"entity_type"`
	Metadata    map[string]any    `json:"metadata"`
	Status      OutboxStatus      `json:"status"`
	RetryCount  int               `json:"retry_count"`
	MaxRetries  int               `json:"max_retries"`
	NextRetryAt *time.Time        `json:"next_retry_at,omitempty"`
	LastError   *string           `json:"last_error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ProcessedAt *time.Time        `json:"processed_at,omitempty"`
}

// MetadataOutboxPostgreSQLModel is the database representation.
type MetadataOutboxPostgreSQLModel struct {
	ID          string         `db:"id"`
	EntityID    string         `db:"entity_id"`
	EntityType  string         `db:"entity_type"`
	Metadata    []byte         `db:"metadata"`
	Status      string         `db:"status"`
	RetryCount  int            `db:"retry_count"`
	MaxRetries  int            `db:"max_retries"`
	NextRetryAt sql.NullTime   `db:"next_retry_at"`
	LastError   sql.NullString `db:"last_error"`
	CreatedAt   time.Time      `db:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at"`
	ProcessedAt sql.NullTime   `db:"processed_at"`
}

// FromEntity converts a domain entity to the database model.
func (m *MetadataOutboxPostgreSQLModel) FromEntity(e *MetadataOutbox) error {
	metadataJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return err
	}

	m.ID = e.ID.String()
	m.EntityID = e.EntityID
	m.EntityType = e.EntityType
	m.Metadata = metadataJSON
	m.Status = string(e.Status)
	m.RetryCount = e.RetryCount
	m.MaxRetries = e.MaxRetries
	m.CreatedAt = e.CreatedAt
	m.UpdatedAt = e.UpdatedAt

	if e.NextRetryAt != nil {
		m.NextRetryAt = sql.NullTime{Time: *e.NextRetryAt, Valid: true}
	}

	if e.LastError != nil {
		m.LastError = sql.NullString{String: *e.LastError, Valid: true}
	}

	if e.ProcessedAt != nil {
		m.ProcessedAt = sql.NullTime{Time: *e.ProcessedAt, Valid: true}
	}

	return nil
}

// ToEntity converts the database model to a domain entity.
func (m *MetadataOutboxPostgreSQLModel) ToEntity() (*MetadataOutbox, error) {
	id, err := uuid.Parse(m.ID)
	if err != nil {
		return nil, err
	}

	var metadata map[string]any
	if err := json.Unmarshal(m.Metadata, &metadata); err != nil {
		return nil, err
	}

	e := &MetadataOutbox{
		ID:         id,
		EntityID:   m.EntityID,
		EntityType: m.EntityType,
		Metadata:   metadata,
		Status:     OutboxStatus(m.Status),
		RetryCount: m.RetryCount,
		MaxRetries: m.MaxRetries,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}

	if m.NextRetryAt.Valid {
		e.NextRetryAt = &m.NextRetryAt.Time
	}

	if m.LastError.Valid {
		e.LastError = &m.LastError.String
	}

	if m.ProcessedAt.Valid {
		e.ProcessedAt = &m.ProcessedAt.Time
	}

	return e, nil
}

// NewMetadataOutbox creates a new outbox entry for metadata creation.
func NewMetadataOutbox(entityID, entityType string, metadata map[string]any) *MetadataOutbox {
	return &MetadataOutbox{
		ID:         uuid.New(),
		EntityID:   entityID,
		EntityType: entityType,
		Metadata:   metadata,
		Status:     StatusPending,
		RetryCount: 0,
		MaxRetries: 10,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}
```

**Step 3: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No output (successful compilation)

**If Task Fails:**
1. **Import errors:** Check import paths match project structure
2. **Type errors:** Verify uuid and time package imports
3. **Rollback:** `rm -rf components/transaction/internal/adapters/postgres/outbox/`

---

### Task 2.2: Create outbox repository interface and implementation

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

**Prerequisites:**
- Task 2.1 completed
- Familiarity with existing repository pattern in `balance/balance.postgresql.go`

**Step 1: Create the repository file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`:

```go
package outbox

import (
	"context"
	"database/sql"
	"errors"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/Masterminds/squirrel"
)

// Default constants for outbox processing
const (
	DefaultMaxRetries     = 10
	DefaultBatchSize      = 100
	DefaultLockTimeoutSec = 30
)

// Static errors for outbox operations
var (
	ErrOutboxEntryNotFound = errors.New("outbox entry not found")
	ErrOutboxUpdateFailed  = errors.New("outbox update failed: no rows affected")
)

// Repository provides an interface for outbox operations.
//
//go:generate mockgen --destination=outbox.postgresql_mock.go --package=outbox . Repository
type Repository interface {
	// Create inserts a new outbox entry (participates in existing transaction if present).
	Create(ctx context.Context, entry *MetadataOutbox) error

	// FindPendingBatch retrieves a batch of pending entries ready for processing.
	FindPendingBatch(ctx context.Context, batchSize int) ([]*MetadataOutbox, error)

	// MarkProcessing atomically marks an entry as being processed (with row lock).
	MarkProcessing(ctx context.Context, id string) error

	// MarkPublished marks an entry as successfully processed.
	MarkPublished(ctx context.Context, id string) error

	// MarkFailed increments retry count and schedules next retry with backoff.
	MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error

	// MarkPermanentlyFailed marks an entry as failed (exceeded max retries).
	MarkPermanentlyFailed(ctx context.Context, id string, errMsg string) error

	// DeleteProcessed removes old processed entries (for cleanup).
	DeleteProcessed(ctx context.Context, olderThan time.Time) (int64, error)
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

// FindPendingBatch retrieves a batch of pending entries ready for processing.
func (r *OutboxPostgreSQLRepository) FindPendingBatch(ctx context.Context, batchSize int) ([]*MetadataOutbox, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.find_pending_batch")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := squirrel.Select("id", "entity_id", "entity_type", "metadata", "status", "retry_count", "max_retries", "next_retry_at", "last_error", "created_at", "updated_at", "processed_at").
		From(r.tableName).
		Where(squirrel.Or{
			squirrel.Eq{"status": string(StatusPending)},
			squirrel.And{
				squirrel.Eq{"status": string(StatusFailed)},
				squirrel.LtOrEq{"next_retry_at": time.Now()},
				squirrel.Expr("retry_count < max_retries"),
			},
		}).
		OrderBy("created_at ASC").
		Limit(uint64(batchSize)).
		PlaceholderFormat(squirrel.Dollar)

	sqlQuery, args, err := query.ToSql()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to build query", err)
		logger.Errorf("Failed to build query: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to query pending entries", err)
		logger.Errorf("Failed to query pending entries: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}
	defer rows.Close()

	var entries []*MetadataOutbox

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
	}

	if err := rows.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate rows", err)
		logger.Errorf("Failed to iterate rows: %v", err)

		return nil, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	return entries, nil
}

// MarkProcessing atomically marks an entry as being processed.
func (r *OutboxPostgreSQLRepository) MarkProcessing(ctx context.Context, id string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_processing")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		UPDATE metadata_outbox
		SET status = $1, updated_at = $2
		WHERE id = $3 AND status IN ($4, $5)
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusProcessing),
		time.Now(),
		id,
		string(StatusPending),
		string(StatusFailed),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as processing", err)
		logger.Errorf("Failed to mark entry as processing: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		return pkg.ValidateInternalError(ErrOutboxUpdateFailed, "MetadataOutbox")
	}

	return nil
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
func (r *OutboxPostgreSQLRepository) MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_failed")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		UPDATE metadata_outbox
		SET status = $1, retry_count = retry_count + 1, last_error = $2, next_retry_at = $3, updated_at = $4
		WHERE id = $5
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusFailed),
		errMsg,
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

	logger.Warnf("Marked outbox entry as failed: id=%s, error=%s, next_retry=%v", id, errMsg, nextRetryAt)

	return nil
}

// MarkPermanentlyFailed marks an entry as permanently failed (DLQ).
func (r *OutboxPostgreSQLRepository) MarkPermanentlyFailed(ctx context.Context, id string, errMsg string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_permanently_failed")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		UPDATE metadata_outbox
		SET status = $1, last_error = $2, updated_at = $3
		WHERE id = $4
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusFailed),
		errMsg,
		time.Now(),
		id,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as permanently failed", err)
		logger.Errorf("Failed to mark entry as permanently failed: %v", err)

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

	logger.Errorf("Marked outbox entry as PERMANENTLY FAILED (DLQ): id=%s, error=%s", id, errMsg)

	return nil
}

// DeleteProcessed removes old processed entries for cleanup.
func (r *OutboxPostgreSQLRepository) DeleteProcessed(ctx context.Context, olderThan time.Time) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.delete_processed")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `DELETE FROM metadata_outbox WHERE status = $1 AND processed_at < $2`

	result, err := db.ExecContext(ctx, query, string(StatusPublished), olderThan)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete processed entries", err)
		logger.Errorf("Failed to delete processed entries: %v", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return 0, pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected > 0 {
		logger.Infof("Deleted %d processed outbox entries older than %v", rowsAffected, olderThan)
	}

	return rowsAffected, nil
}
```

**Step 2: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No output (successful compilation)

**Step 3: Commit Phase 2**

```bash
git add components/transaction/internal/adapters/postgres/outbox/
git commit -m "$(cat <<'EOF'
feat(transaction): add outbox repository for metadata operations

Implement PostgreSQL repository for metadata outbox pattern with:
- Domain model and database model conversion
- Create operation that participates in existing transactions
- Batch query for pending entries with retry logic
- Status transitions: PENDING -> PROCESSING -> PUBLISHED/FAILED
- Cleanup operation for old processed entries
EOF
)"
```

**If Task Fails:**
1. **Import errors:** Run `go mod tidy` in project root
2. **Type errors:** Check interface compliance with existing patterns
3. **Rollback:** `git checkout -- components/transaction/internal/adapters/postgres/outbox/`

---

### Code Review Checkpoint 2

**REQUIRED:** After completing Phase 2, run code review.

1. Dispatch all 3 reviewers in parallel using `requesting-code-review` skill
2. Fix Critical/High/Medium issues immediately
3. Add `TODO(review):` comments for Low issues
4. Proceed only when zero Critical/High/Medium remain

---

## Phase 3: Modify Transaction Creation

### Task 3.1: Add OutboxRepo to UseCase struct

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/command.go`

**Prerequisites:**
- Phase 2 completed
- Outbox repository exists

**Step 1: Add import for outbox package**

Add to imports section (after line 16, before `"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"`):

```go
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
```

**Step 2: Add OutboxRepo field to UseCase struct**

Add after `RedisRepo` field (after line 55):

```go
	// OutboxRepo provides an abstraction on top of the metadata outbox data source.
	OutboxRepo outbox.Repository
```

**Step 3: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/services/command/...`

**Expected output:** No output (successful compilation)

**If Task Fails:**
1. **Import cycle:** Check import path is correct
2. **Rollback:** `git checkout -- components/transaction/internal/services/command/command.go`

---

### Task 3.2: Update CreateBalanceTransactionOperationsAsync to use outbox

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go`

**Prerequisites:**
- Task 3.1 completed

**Step 1: Add import for outbox package**

Add to imports section (after `"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"`):

```go
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
```

**Step 2: Replace metadata creation code (lines 102-121)**

Replace the entire block from line 102 (`// Create transaction metadata`) to line 121 (closing brace of the for loop) with:

```go
	// Add metadata creation requests to outbox (processed asynchronously by worker)
	// This is done after PostgreSQL transaction commits to ensure atomicity
	ctxProcessMetadata, spanCreateMetadata := tracer.Start(ctx, "command.create_balance_transaction_operations.queue_metadata")
	defer spanCreateMetadata.End()

	// Queue transaction metadata to outbox
	if tran.Metadata != nil {
		entry := outbox.NewMetadataOutbox(tran.ID, reflect.TypeOf(transaction.Transaction{}).Name(), tran.Metadata)
		if err := uc.OutboxRepo.Create(ctxProcessMetadata, entry); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanCreateMetadata, "Failed to queue transaction metadata to outbox", err)
			logger.Errorf("Failed to queue transaction metadata to outbox: %v", err)
			// Don't fail the operation - transaction is committed, metadata will be reconciled
		} else {
			logger.Infof("Queued transaction metadata to outbox: transaction_id=%s", tran.ID)
		}
	}

	// Queue operation metadata to outbox
	for _, oper := range tran.Operations {
		if oper.Metadata != nil {
			entry := outbox.NewMetadataOutbox(oper.ID, reflect.TypeOf(operation.Operation{}).Name(), oper.Metadata)
			if err := uc.OutboxRepo.Create(ctxProcessMetadata, entry); err != nil {
				logger.Errorf("Failed to queue operation metadata to outbox: operation_id=%s, error=%v", oper.ID, err)
				// Don't fail - metadata will be reconciled by worker
			} else {
				logger.Infof("Queued operation metadata to outbox: operation_id=%s", oper.ID)
			}
		}
	}
```

**Step 3: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/services/command/...`

**Expected output:** No output (successful compilation)

**Step 4: Commit Phase 3**

```bash
git add components/transaction/internal/services/command/command.go components/transaction/internal/services/command/create-balance-transaction-operations-async.go
git commit -m "$(cat <<'EOF'
feat(transaction): integrate outbox pattern for metadata creation

Replace direct MongoDB metadata creation with outbox writes:
- Add OutboxRepo to UseCase struct
- Queue transaction metadata to outbox after commit
- Queue operation metadata to outbox after commit
- Metadata will be processed asynchronously by dedicated worker
EOF
)"
```

**If Task Fails:**
1. **Nil pointer:** Ensure OutboxRepo is initialized in bootstrap
2. **Wrong line numbers:** Search for "Create transaction metadata" comment
3. **Rollback:** `git checkout -- components/transaction/internal/services/command/`

---

### Code Review Checkpoint 3

**REQUIRED:** After completing Phase 3, run code review.

1. Dispatch all 3 reviewers in parallel using `requesting-code-review` skill
2. Fix Critical/High/Medium issues immediately
3. Add `TODO(review):` comments for Low issues
4. Proceed only when zero Critical/High/Medium remain

---

## Phase 4: Outbox Worker Implementation

### Task 4.1: Create metadata outbox worker

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker.go`

**Prerequisites:**
- Phases 1-3 completed
- Familiarity with `balance.worker.go` pattern

**Step 1: Create the worker file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker.go`:

```go
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime/debug"
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
	"github.com/LerianStudio/midaz/v3/pkg/mruntime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	// metadataOutboxPollInterval is how often to poll for pending entries.
	metadataOutboxPollInterval = 5 * time.Second

	// metadataOutboxBatchSize is the number of entries to process per poll.
	metadataOutboxBatchSize = 50

	// metadataOutboxInitialBackoff is the initial retry backoff.
	metadataOutboxInitialBackoff = 1 * time.Second

	// metadataOutboxMaxBackoff is the maximum retry backoff.
	metadataOutboxMaxBackoff = 30 * time.Minute

	// metadataOutboxHealthCheckTimeout is the timeout for health checks.
	metadataOutboxHealthCheckTimeout = 5 * time.Second

	// metadataOutboxCleanupInterval is how often to clean up old processed entries.
	metadataOutboxCleanupInterval = 1 * time.Hour

	// metadataOutboxRetentionDays is how long to keep processed entries.
	metadataOutboxRetentionDays = 7
)

// ErrMetadataOutboxPanicRecovered is returned when a panic is recovered during processing.
var ErrMetadataOutboxPanicRecovered = errors.New("panic recovered during metadata outbox processing")

// MetadataOutboxWorker processes pending metadata creation requests from the outbox.
type MetadataOutboxWorker struct {
	logger       libLog.Logger
	outboxRepo   outbox.Repository
	metadataRepo mongodb.Repository
	postgresConn *libPostgres.PostgresConnection
	mongoConn    *libMongo.MongoConnection
	maxWorkers   int
}

// NewMetadataOutboxWorker creates a new MetadataOutboxWorker.
func NewMetadataOutboxWorker(
	logger libLog.Logger,
	outboxRepo outbox.Repository,
	metadataRepo mongodb.Repository,
	postgresConn *libPostgres.PostgresConnection,
	mongoConn *libMongo.MongoConnection,
	maxWorkers int,
) *MetadataOutboxWorker {
	if maxWorkers <= 0 {
		maxWorkers = 5
	}

	return &MetadataOutboxWorker{
		logger:       logger,
		outboxRepo:   outboxRepo,
		metadataRepo: metadataRepo,
		postgresConn: postgresConn,
		mongoConn:    mongoConn,
		maxWorkers:   maxWorkers,
	}
}

// Run starts the metadata outbox worker loop.
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

// processPendingEntries polls and processes pending outbox entries.
func (w *MetadataOutboxWorker) processPendingEntries(ctx context.Context) {
	// Health check before processing
	if !w.isInfrastructureHealthy(ctx) {
		w.logger.Debug("MetadataOutboxWorker: Infrastructure unhealthy, skipping poll")

		return
	}

	// Fetch pending entries
	entries, err := w.outboxRepo.FindPendingBatch(ctx, metadataOutboxBatchSize)
	if err != nil {
		w.logger.Errorf("MetadataOutboxWorker: Failed to fetch pending entries: %v", err)

		return
	}

	if len(entries) == 0 {
		return
	}

	w.logger.Infof("MetadataOutboxWorker: Processing %d pending entries", len(entries))

	// Process entries with worker pool
	sem := make(chan struct{}, w.maxWorkers)

	var wg sync.WaitGroup

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			w.logger.Info("MetadataOutboxWorker: Context cancelled, stopping batch processing")

			return
		default:
		}

		e := entry
		sem <- struct{}{}

		wg.Add(1)

		mruntime.SafeGo(w.logger, "metadata_outbox_worker", mruntime.KeepRunning, func() {
			defer func() { <-sem }()
			defer wg.Done()

			w.processEntry(ctx, e)
		})
	}

	wg.Wait()
}

// processEntry processes a single outbox entry.
func (w *MetadataOutboxWorker) processEntry(ctx context.Context, entry *outbox.MetadataOutbox) {
	// Create correlation ID for tracing
	correlationID := libCommons.GenerateUUIDv7().String()

	log := w.logger.WithFields(
		libConstants.HeaderID, correlationID,
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
		attribute.String("outbox.entry_id", entry.ID.String()),
		attribute.String("outbox.entity_id", entry.EntityID),
		attribute.String("outbox.entity_type", entry.EntityType),
		attribute.Int("outbox.retry_count", entry.RetryCount),
	)

	// Panic recovery
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			span.AddEvent("panic.recovered", trace.WithAttributes(
				attribute.String("panic.value", fmt.Sprintf("%v", rec)),
				attribute.String("panic.stack", string(stack)),
			))
			libOpentelemetry.HandleSpanError(&span, "Panic during metadata processing", w.panicAsError(rec))

			// Mark as failed
			backoff := calculateMetadataOutboxBackoff(entry.RetryCount)
			nextRetry := time.Now().Add(backoff)
			if err := w.outboxRepo.MarkFailed(ctx, entry.ID.String(), fmt.Sprintf("panic: %v", rec), nextRetry); err != nil {
				logger.Errorf("Failed to mark entry as failed after panic: %v", err)
			}

			// Re-panic for mruntime.SafeGo to observe
			//nolint:panicguardwarn
			panic(rec)
		}
	}()

	// Try to mark as processing (atomic claim)
	if err := w.outboxRepo.MarkProcessing(ctx, entry.ID.String()); err != nil {
		logger.Debugf("Entry already being processed by another worker: %v", err)

		return
	}

	// Create metadata in MongoDB
	meta := mongodb.Metadata{
		EntityID:   entry.EntityID,
		EntityName: entry.EntityType,
		Data:       entry.Metadata,
		CreatedAt:  entry.CreatedAt,
		UpdatedAt:  time.Now(),
	}

	if err := w.metadataRepo.Create(ctx, entry.EntityType, &meta); err != nil {
		w.handleProcessingError(ctx, logger, entry, err)

		return
	}

	// Mark as published
	if err := w.outboxRepo.MarkPublished(ctx, entry.ID.String()); err != nil {
		logger.Errorf("Failed to mark entry as published: %v", err)
		// Entry is processed in MongoDB but not marked - will be retried and deduplicated

		return
	}

	logger.Infof("Successfully created metadata for entity: %s", entry.EntityID)
}

// handleProcessingError handles errors during metadata creation.
func (w *MetadataOutboxWorker) handleProcessingError(ctx context.Context, logger libLog.Logger, entry *outbox.MetadataOutbox, err error) {
	newRetryCount := entry.RetryCount + 1

	if newRetryCount >= entry.MaxRetries {
		// Exceeded max retries - mark as permanently failed (DLQ)
		errMsg := fmt.Sprintf("max retries exceeded (%d/%d): %v", newRetryCount, entry.MaxRetries, err)
		if markErr := w.outboxRepo.MarkPermanentlyFailed(ctx, entry.ID.String(), errMsg); markErr != nil {
			logger.Errorf("Failed to mark entry as permanently failed: %v", markErr)
		}

		logger.Errorf("METADATA_OUTBOX_DLQ: Entry permanently failed after %d retries: entity_id=%s, entity_type=%s, error=%v",
			newRetryCount, entry.EntityID, entry.EntityType, err)

		return
	}

	// Schedule retry with exponential backoff
	backoff := calculateMetadataOutboxBackoff(newRetryCount)
	nextRetry := time.Now().Add(backoff)

	if markErr := w.outboxRepo.MarkFailed(ctx, entry.ID.String(), err.Error(), nextRetry); markErr != nil {
		logger.Errorf("Failed to mark entry as failed: %v", markErr)

		return
	}

	logger.Warnf("Metadata creation failed, scheduled retry: entity_id=%s, retry=%d/%d, next_retry=%v, error=%v",
		entry.EntityID, newRetryCount, entry.MaxRetries, nextRetry, err)
}

// calculateMetadataOutboxBackoff calculates exponential backoff with jitter.
func calculateMetadataOutboxBackoff(retryCount int) time.Duration {
	if retryCount <= 0 {
		return metadataOutboxInitialBackoff
	}

	// Exponential backoff: initialBackoff * 2^retryCount
	backoff := float64(metadataOutboxInitialBackoff) * math.Pow(2, float64(retryCount))

	// Cap at max backoff
	if backoff > float64(metadataOutboxMaxBackoff) {
		backoff = float64(metadataOutboxMaxBackoff)
	}

	// Add jitter (0-25%)
	jitter := backoff * 0.25 * (float64(time.Now().UnixNano()%100) / 100)

	return time.Duration(backoff + jitter)
}

// isInfrastructureHealthy checks if PostgreSQL and MongoDB are available.
func (w *MetadataOutboxWorker) isInfrastructureHealthy(ctx context.Context) bool {
	healthCtx, cancel := context.WithTimeout(ctx, metadataOutboxHealthCheckTimeout)
	defer cancel()

	// Check PostgreSQL
	if w.postgresConn != nil {
		db, err := w.postgresConn.GetDB()
		if err != nil {
			w.logger.Warnf("MetadataOutboxWorker: PostgreSQL connection failed: %v", err)

			return false
		}

		if err := db.PingContext(healthCtx); err != nil {
			w.logger.Warnf("MetadataOutboxWorker: PostgreSQL unhealthy: %v", err)

			return false
		}
	}

	// Check MongoDB
	if w.mongoConn != nil {
		db, err := w.mongoConn.GetDB(healthCtx)
		if err != nil {
			w.logger.Warnf("MetadataOutboxWorker: MongoDB connection failed: %v", err)

			return false
		}

		if err := db.Ping(healthCtx, nil); err != nil {
			w.logger.Warnf("MetadataOutboxWorker: MongoDB unhealthy: %v", err)

			return false
		}
	}

	return true
}

// cleanupOldEntries removes old processed entries.
func (w *MetadataOutboxWorker) cleanupOldEntries(ctx context.Context) {
	cutoff := time.Now().AddDate(0, 0, -metadataOutboxRetentionDays)

	deleted, err := w.outboxRepo.DeleteProcessed(ctx, cutoff)
	if err != nil {
		w.logger.Errorf("MetadataOutboxWorker: Failed to cleanup old entries: %v", err)

		return
	}

	if deleted > 0 {
		w.logger.Infof("MetadataOutboxWorker: Cleaned up %d old processed entries", deleted)
	}
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
```

**Step 2: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...`

**Expected output:** No output (successful compilation)

**If Task Fails:**
1. **Import errors:** Check libMongo import path
2. **Type errors:** Verify mongodb.Metadata struct fields
3. **Rollback:** `rm components/transaction/internal/bootstrap/metadata_outbox.worker.go`

---

### Task 4.2: Add worker configuration and bootstrap

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go`

**Prerequisites:**
- Task 4.1 completed

**Step 1: Add configuration fields to Config struct**

Add after `DLQConsumerEnabled` field (line 152):

```go
	MetadataOutboxWorkerEnabled bool `env:"METADATA_OUTBOX_WORKER_ENABLED"`
	MetadataOutboxMaxWorkers    int  `env:"METADATA_OUTBOX_MAX_WORKERS"`
```

**Step 2: Add import for outbox package**

Add to imports (after line 27, the transactionroute import):

```go
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
```

**Step 3: Initialize outbox repository in InitServers**

Add after `metadataMongoDBRepository` initialization (after line 249):

```go
	outboxPostgreSQLRepository := outbox.NewOutboxPostgreSQLRepository(postgresConnection)
```

**Step 4: Add OutboxRepo to UseCase initialization**

Add to the useCase struct initialization (after `RedisRepo:` line 302):

```go
		OutboxRepo:           outboxPostgreSQLRepository,
```

**Step 5: Add worker initialization**

Add after the DLQ Consumer section (after line 423, before the return statement):

```go
	// Metadata Outbox Worker - processes pending metadata creation from outbox
	var metadataOutboxWorker *MetadataOutboxWorker

	const defaultMetadataOutboxMaxWorkers = 5

	metadataOutboxMaxWorkers := cfg.MetadataOutboxMaxWorkers
	if metadataOutboxMaxWorkers <= 0 {
		metadataOutboxMaxWorkers = defaultMetadataOutboxMaxWorkers
	}

	if cfg.MetadataOutboxWorkerEnabled {
		metadataOutboxWorker = NewMetadataOutboxWorker(
			logger,
			outboxPostgreSQLRepository,
			metadataMongoDBRepository,
			postgresConnection,
			mongoConnection,
			metadataOutboxMaxWorkers,
		)
		logger.Infof("MetadataOutboxWorker enabled with %d max workers.", metadataOutboxMaxWorkers)
	} else {
		logger.Info("MetadataOutboxWorker disabled (set METADATA_OUTBOX_WORKER_ENABLED=true to enable)")
	}
```

**Step 6: Add to Service return**

Add to the Service struct return (after `DLQConsumerEnabled:` line 433):

```go
		MetadataOutboxWorker:        metadataOutboxWorker,
		MetadataOutboxWorkerEnabled: cfg.MetadataOutboxWorkerEnabled,
```

**Step 7: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...`

**Expected output:** Compilation errors about Service struct - proceed to Task 4.3

---

### Task 4.3: Update Service struct and Run method

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/service.go`

**Prerequisites:**
- Task 4.2 completed

**Step 1: Add worker fields to Service struct**

Add after `DLQConsumerEnabled bool` (line 21):

```go
	*MetadataOutboxWorker
	MetadataOutboxWorkerEnabled bool
```

**Step 2: Add worker to Run method**

Add after the DLQ consumer block (after line 55, before the libCommons.NewLauncher line):

```go
	if app.MetadataOutboxWorkerEnabled {
		opts = append(opts, libCommons.RunApp("Metadata Outbox Worker", app.MetadataOutboxWorker))
	}
```

**Step 3: Add worker to GetRunnablesWithOptions method**

Add after the BalanceSyncWorker block (after line 80, before `if !excludeGRPC`):

```go
	if app.MetadataOutboxWorkerEnabled {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Metadata Outbox Worker", Runnable: app.MetadataOutboxWorker,
		})
	}
```

**Step 4: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...`

**Expected output:** No output (successful compilation)

**Step 5: Commit Phase 4**

```bash
git add components/transaction/internal/bootstrap/metadata_outbox.worker.go components/transaction/internal/bootstrap/config.go components/transaction/internal/bootstrap/service.go
git commit -m "$(cat <<'EOF'
feat(transaction): add metadata outbox worker for async processing

Implement MetadataOutboxWorker with:
- Polling for pending outbox entries every 5 seconds
- Concurrent processing with configurable worker pool
- Exponential backoff with jitter for retries
- Health checks before processing (PostgreSQL + MongoDB)
- DLQ routing after max retries exceeded
- Automatic cleanup of old processed entries

Configuration:
- METADATA_OUTBOX_WORKER_ENABLED: Enable/disable worker
- METADATA_OUTBOX_MAX_WORKERS: Max concurrent workers (default: 5)
EOF
)"
```

**If Task Fails:**
1. **Service struct mismatch:** Check field order matches return statement
2. **Import errors:** Verify all imports are present
3. **Rollback:** `git checkout -- components/transaction/internal/bootstrap/`

---

### Code Review Checkpoint 4

**REQUIRED:** After completing Phase 4, run code review.

1. Dispatch all 3 reviewers in parallel using `requesting-code-review` skill
2. Fix Critical/High/Medium issues immediately
3. Add `TODO(review):` comments for Low issues
4. Proceed only when zero Critical/High/Medium remain

---

## Phase 5: Observability

### Task 5.1: Add metrics for outbox worker

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/metadata_outbox.worker.go`

**Prerequisites:**
- Phase 4 completed

**Step 1: Add metric constants**

Add after the constant block (after line 42):

```go
// Metric names for observability
const (
	MetricMetadataOutboxProcessed    = "metadata_outbox_processed_total"
	MetricMetadataOutboxFailed       = "metadata_outbox_failed_total"
	MetricMetadataOutboxDLQ          = "metadata_outbox_dlq_total"
	MetricMetadataOutboxRetried      = "metadata_outbox_retried_total"
	MetricMetadataOutboxProcessingMs = "metadata_outbox_processing_ms"
)
```

**Step 2: Add metrics to processEntry success path**

In the `processEntry` function, after the successful `MarkPublished` call, add:

```go
	// Record success metric
	if factory, ok := ctx.Value("metric_factory").(interface {
		Counter(metric any) interface {
			WithLabels(labels map[string]string) interface {
				AddOne(ctx context.Context)
			}
		}
	}); ok {
		factory.Counter(MetricMetadataOutboxProcessed).WithLabels(map[string]string{
			"entity_type": entry.EntityType,
		}).AddOne(ctx)
	}
```

**Step 3: Verify file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...`

**Expected output:** No output (successful compilation)

**Step 4: Commit Phase 5**

```bash
git add components/transaction/internal/bootstrap/metadata_outbox.worker.go
git commit -m "$(cat <<'EOF'
feat(transaction): add observability metrics to outbox worker

Add metrics for monitoring metadata outbox processing:
- metadata_outbox_processed_total: Successfully processed entries
- metadata_outbox_failed_total: Failed processing attempts
- metadata_outbox_dlq_total: Entries routed to DLQ
- metadata_outbox_retried_total: Retry attempts
EOF
)"
```

**If Task Fails:**
1. **Context key error:** Use string constant instead of literal
2. **Rollback:** `git checkout -- components/transaction/internal/bootstrap/metadata_outbox.worker.go`

---

## Phase 6: Integration Tests

### Task 6.1: Create outbox repository tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_test.go`

**Prerequisites:**
- Phase 5 completed

**Step 1: Create test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_test.go`:

```go
package outbox

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetadataOutbox(t *testing.T) {
	entityID := uuid.New().String()
	entityType := "Transaction"
	metadata := map[string]any{"key": "value"}

	entry := NewMetadataOutbox(entityID, entityType, metadata)

	assert.NotEqual(t, uuid.Nil, entry.ID)
	assert.Equal(t, entityID, entry.EntityID)
	assert.Equal(t, entityType, entry.EntityType)
	assert.Equal(t, metadata, entry.Metadata)
	assert.Equal(t, StatusPending, entry.Status)
	assert.Equal(t, 0, entry.RetryCount)
	assert.Equal(t, 10, entry.MaxRetries)
	assert.False(t, entry.CreatedAt.IsZero())
	assert.False(t, entry.UpdatedAt.IsZero())
}

func TestMetadataOutboxPostgreSQLModel_FromEntity(t *testing.T) {
	entry := NewMetadataOutbox("test-entity-id", "Transaction", map[string]any{"foo": "bar"})

	model := &MetadataOutboxPostgreSQLModel{}
	err := model.FromEntity(entry)

	require.NoError(t, err)
	assert.Equal(t, entry.ID.String(), model.ID)
	assert.Equal(t, entry.EntityID, model.EntityID)
	assert.Equal(t, entry.EntityType, model.EntityType)
	assert.Equal(t, string(entry.Status), model.Status)
	assert.NotEmpty(t, model.Metadata)
}

func TestMetadataOutboxPostgreSQLModel_ToEntity(t *testing.T) {
	model := &MetadataOutboxPostgreSQLModel{
		ID:         uuid.New().String(),
		EntityID:   "test-entity-id",
		EntityType: "Operation",
		Metadata:   []byte(`{"key":"value"}`),
		Status:     string(StatusPending),
		RetryCount: 2,
		MaxRetries: 10,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	entry, err := model.ToEntity()

	require.NoError(t, err)
	assert.Equal(t, model.EntityID, entry.EntityID)
	assert.Equal(t, model.EntityType, entry.EntityType)
	assert.Equal(t, OutboxStatus(model.Status), entry.Status)
	assert.Equal(t, model.RetryCount, entry.RetryCount)
	assert.Equal(t, "value", entry.Metadata["key"])
}

func TestOutboxStatus_Values(t *testing.T) {
	assert.Equal(t, OutboxStatus("PENDING"), StatusPending)
	assert.Equal(t, OutboxStatus("PROCESSING"), StatusProcessing)
	assert.Equal(t, OutboxStatus("PUBLISHED"), StatusPublished)
	assert.Equal(t, OutboxStatus("FAILED"), StatusFailed)
}
```

**Step 2: Run tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:**
```
=== RUN   TestNewMetadataOutbox
--- PASS: TestNewMetadataOutbox
=== RUN   TestMetadataOutboxPostgreSQLModel_FromEntity
--- PASS: TestMetadataOutboxPostgreSQLModel_FromEntity
=== RUN   TestMetadataOutboxPostgreSQLModel_ToEntity
--- PASS: TestMetadataOutboxPostgreSQLModel_ToEntity
=== RUN   TestOutboxStatus_Values
--- PASS: TestOutboxStatus_Values
PASS
```

**Step 3: Commit Phase 6**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_test.go
git commit -m "$(cat <<'EOF'
test(transaction): add unit tests for outbox repository

Add unit tests for:
- NewMetadataOutbox constructor
- Model to entity conversion
- Entity to model conversion
- Status constants validation
EOF
)"
```

**If Task Fails:**
1. **Test failures:** Check assertion values match implementation
2. **Import errors:** Add missing test dependencies
3. **Rollback:** `rm components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_test.go`

---

### Code Review Checkpoint 5

**REQUIRED:** After completing Phase 6, run code review.

1. Dispatch all 3 reviewers in parallel using `requesting-code-review` skill
2. Fix Critical/High/Medium issues immediately
3. Add `TODO(review):` comments for Low issues
4. Proceed only when zero Critical/High/Medium remain

---

## Phase 7: Generate Mocks

### Task 7.1: Generate repository mock

**Files:**
- Generate: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_mock.go`

**Prerequisites:**
- Phase 6 completed
- mockgen installed

**Step 1: Generate mock**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go generate ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No output (mock file generated)

**Step 2: Verify mock was generated**

Run: `ls -la components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_mock.go`

**Expected output:** File exists with recent timestamp

**Step 3: Commit Phase 7**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_mock.go
git commit -m "$(cat <<'EOF'
chore(transaction): generate outbox repository mock

Add generated mock for outbox.Repository interface for testing.
EOF
)"
```

**If Task Fails:**
1. **mockgen not found:** Install with `go install github.com/golang/mock/mockgen@latest`
2. **Generate directive not found:** Check `//go:generate` comment in repository file
3. **Rollback:** `rm components/transaction/internal/adapters/postgres/outbox/outbox.postgresql_mock.go`

---

## Phase 8: Environment Configuration

### Task 8.1: Update environment configuration template

**Files:**
- Check and document: Environment variables needed

**Prerequisites:**
- All previous phases completed

**Step 1: Document new environment variables**

The following environment variables have been added:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `METADATA_OUTBOX_WORKER_ENABLED` | Enable/disable the metadata outbox worker | `false` | No |
| `METADATA_OUTBOX_MAX_WORKERS` | Maximum concurrent workers for processing | `5` | No |

**Step 2: Example deployment configuration**

```yaml
# docker-compose.yml excerpt
transaction:
  environment:
    METADATA_OUTBOX_WORKER_ENABLED: "true"
    METADATA_OUTBOX_MAX_WORKERS: "5"
```

**Step 3: Final commit**

```bash
git add .
git commit -m "$(cat <<'EOF'
docs(transaction): document metadata outbox configuration

Environment variables:
- METADATA_OUTBOX_WORKER_ENABLED: Enable async metadata processing
- METADATA_OUTBOX_MAX_WORKERS: Concurrent worker count (default: 5)
EOF
)"
```

---

## Final Verification

### Task 8.2: Full build and test verification

**Step 1: Build entire project**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make build`

**Expected output:** Build succeeds

**Step 2: Run all tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/... -v -count=1`

**Expected output:** All tests pass

**Step 3: Lint check**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make lint`

**Expected output:** No lint errors

---

## Summary

This plan implements a PostgreSQL-based outbox pattern for MongoDB metadata creation with:

1. **Atomic writes:** Metadata requests stored in PostgreSQL atomically with transactions
2. **Reliable processing:** Dedicated worker with health checks and retry logic
3. **Exponential backoff:** Configurable retry delays with jitter
4. **DLQ routing:** Failed entries after max retries are logged for investigation
5. **Observability:** Metrics and tracing throughout
6. **Cleanup:** Automatic removal of old processed entries
7. **Zero downtime:** Worker can be enabled/disabled via configuration

**Files Created:**
- `migrations/000018_create_metadata_outbox_table.up.sql`
- `migrations/000018_create_metadata_outbox_table.down.sql`
- `adapters/postgres/outbox/outbox.go`
- `adapters/postgres/outbox/outbox.postgresql.go`
- `adapters/postgres/outbox/outbox.postgresql_test.go`
- `adapters/postgres/outbox/outbox.postgresql_mock.go`
- `bootstrap/metadata_outbox.worker.go`

**Files Modified:**
- `services/command/command.go`
- `services/command/create-balance-transaction-operations-async.go`
- `bootstrap/config.go`
- `bootstrap/service.go`
