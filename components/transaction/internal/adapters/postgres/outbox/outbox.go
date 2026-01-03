// Package outbox provides the outbox pattern implementation for reliable async processing.
// It stores pending operations in PostgreSQL and processes them asynchronously.
package outbox

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mretry"
	"github.com/google/uuid"
)

// OutboxStatus represents the processing status of an outbox entry.
//
// Status transitions:
//
//	PENDING -> PROCESSING (worker claims entry)
//	PROCESSING -> PUBLISHED (success)
//	PROCESSING -> FAILED (error, will retry if retry_count < max_retries)
//	FAILED -> PROCESSING (retry attempt)
//	FAILED -> DLQ (max retries exceeded, requires manual intervention)
type OutboxStatus string

const (
	// StatusPending indicates the entry is waiting to be processed.
	StatusPending OutboxStatus = "PENDING"
	// StatusProcessing indicates the entry is currently being processed.
	StatusProcessing OutboxStatus = "PROCESSING"
	// StatusPublished indicates the entry was successfully processed.
	StatusPublished OutboxStatus = "PUBLISHED"
	// StatusFailed indicates the entry failed but can be retried.
	StatusFailed OutboxStatus = "FAILED"
	// StatusDLQ indicates the entry permanently failed after max retries (Dead Letter Queue).
	StatusDLQ OutboxStatus = "DLQ"
)

// ValidOutboxTransitions defines the allowed status transitions.
// This enforces the state machine at runtime to catch bugs early.
//
// Valid transitions:
//   - PENDING -> PROCESSING (worker claims entry)
//   - PROCESSING -> PUBLISHED (success)
//   - PROCESSING -> FAILED (error, will retry)
//   - PROCESSING -> DLQ (final attempt failed, max retries exceeded)
//   - FAILED -> PROCESSING (retry attempt)
//   - FAILED -> DLQ (max retries exceeded)
//
// Terminal states (no outgoing transitions):
//   - PUBLISHED: Successfully processed
//   - DLQ: Permanently failed, requires manual intervention
var ValidOutboxTransitions = map[OutboxStatus][]OutboxStatus{
	StatusPending:    {StatusProcessing},
	StatusProcessing: {StatusPublished, StatusFailed, StatusDLQ},
	StatusFailed:     {StatusProcessing, StatusDLQ},
	StatusPublished:  {}, // Terminal state
	StatusDLQ:        {}, // Terminal state
}

// CanTransitionTo returns true if transitioning from this status to the target is valid.
func (s OutboxStatus) CanTransitionTo(target OutboxStatus) bool {
	allowed, exists := ValidOutboxTransitions[s]
	if !exists {
		return false
	}

	for _, a := range allowed {
		if a == target {
			return true
		}
	}

	return false
}

// IsTerminal returns true if this status is a terminal state (no valid outgoing transitions).
func (s OutboxStatus) IsTerminal() bool {
	return s == StatusPublished || s == StatusDLQ
}

// Entity type constants - use these instead of reflection for type safety.
const (
	EntityTypeTransaction = "Transaction"
	EntityTypeOperation   = "Operation"
)

// Validation constants
const (
	// MaxMetadataSize is the maximum allowed size for metadata in bytes (64KB).
	MaxMetadataSize = 64 * 1024
	// MaxEntityIDLength is the maximum allowed length for entity IDs (UUID is 36 chars).
	MaxEntityIDLength = 255
)

// Errors for validation
var (
	ErrInvalidEntityType    = errors.New("invalid entity type: must be 'Transaction' or 'Operation'")
	ErrMetadataTooLarge     = errors.New("metadata exceeds maximum size limit")
	ErrMetadataNil          = errors.New("metadata cannot be nil")
	ErrEntityIDEmpty        = errors.New("entity ID cannot be empty")
	ErrEntityIDTooLong      = errors.New("entity ID exceeds maximum length")
	ErrMarshalMetadata      = errors.New("failed to marshal metadata")
	ErrUnmarshalMetadata    = errors.New("failed to unmarshal metadata")
	ErrParseUUID            = errors.New("failed to parse UUID")
	ErrDuplicateOutboxEntry = errors.New("duplicate outbox entry: entry already exists for this entity in PENDING or PROCESSING status")
)

// allowedEntityTypes defines valid entity types for validation.
var allowedEntityTypes = map[string]bool{
	EntityTypeTransaction: true,
	EntityTypeOperation:   true,
}

// MetadataOutbox represents a pending metadata creation request.
type MetadataOutbox struct {
	ID                  uuid.UUID      `json:"id"`
	EntityID            string         `json:"entity_id"`
	EntityType          string         `json:"entity_type"`
	Metadata            map[string]any `json:"metadata"`
	Status              OutboxStatus   `json:"status"`
	RetryCount          int            `json:"retry_count"`
	MaxRetries          int            `json:"max_retries"`
	NextRetryAt         *time.Time     `json:"next_retry_at,omitempty"`
	ProcessingStartedAt *time.Time     `json:"processing_started_at,omitempty"`
	LastError           *string        `json:"last_error,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	ProcessedAt         *time.Time     `json:"processed_at,omitempty"`
}

// MetadataOutboxPostgreSQLModel is the database representation.
type MetadataOutboxPostgreSQLModel struct {
	ID                  string         `db:"id"`
	EntityID            string         `db:"entity_id"`
	EntityType          string         `db:"entity_type"`
	Metadata            []byte         `db:"metadata"`
	Status              string         `db:"status"`
	RetryCount          int            `db:"retry_count"`
	MaxRetries          int            `db:"max_retries"`
	NextRetryAt         sql.NullTime   `db:"next_retry_at"`
	ProcessingStartedAt sql.NullTime   `db:"processing_started_at"`
	LastError           sql.NullString `db:"last_error"`
	CreatedAt           time.Time      `db:"created_at"`
	UpdatedAt           time.Time      `db:"updated_at"`
	ProcessedAt         sql.NullTime   `db:"processed_at"`
}

// FromEntity converts a domain entity to the database model.
func (m *MetadataOutboxPostgreSQLModel) FromEntity(e *MetadataOutbox) error {
	metadataJSON, err := json.Marshal(e.Metadata)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMarshalMetadata, err)
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

	if e.ProcessingStartedAt != nil {
		m.ProcessingStartedAt = sql.NullTime{Time: *e.ProcessingStartedAt, Valid: true}
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
		return nil, fmt.Errorf("%w: %w", ErrParseUUID, err)
	}

	var metadata map[string]any
	if err := json.Unmarshal(m.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnmarshalMetadata, err)
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

	if m.ProcessingStartedAt.Valid {
		e.ProcessingStartedAt = &m.ProcessingStartedAt.Time
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
// Returns error if validation fails (invalid entity type, entity ID, or metadata too large).
func NewMetadataOutbox(entityID, entityType string, metadata map[string]any) (*MetadataOutbox, error) {
	// Validate entityID is not empty
	if entityID == "" {
		return nil, ErrEntityIDEmpty
	}

	// Validate entityID length to prevent resource exhaustion
	if len(entityID) > MaxEntityIDLength {
		return nil, fmt.Errorf("%w: length %d exceeds max %d", ErrEntityIDTooLong, len(entityID), MaxEntityIDLength)
	}

	// Validate entity type
	if !allowedEntityTypes[entityType] {
		return nil, fmt.Errorf("%w: got '%s'", ErrInvalidEntityType, entityType)
	}

	// Validate metadata is not nil
	if metadata == nil {
		return nil, ErrMetadataNil
	}

	// Validate metadata size
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMarshalMetadata, err)
	}

	if len(metadataJSON) > MaxMetadataSize {
		return nil, fmt.Errorf("%w: size %d exceeds max %d bytes", ErrMetadataTooLarge, len(metadataJSON), MaxMetadataSize)
	}

	return &MetadataOutbox{
		ID:         uuid.New(),
		EntityID:   entityID,
		EntityType: entityType,
		Metadata:   metadata,
		Status:     StatusPending,
		RetryCount: 0,
		MaxRetries: mretry.DefaultMaxRetries,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}
