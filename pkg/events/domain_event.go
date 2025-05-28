package events

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// EventType represents the type of domain event
type EventType string

const (
	// Transaction events
	TransactionCreated   EventType = "transaction.created"
	TransactionUpdated   EventType = "transaction.updated"
	TransactionDeleted   EventType = "transaction.deleted"
	TransactionApproved  EventType = "transaction.approved"
	TransactionRejected  EventType = "transaction.rejected"
	TransactionCompleted EventType = "transaction.completed"

	// Operation events
	OperationCreated EventType = "operation.created"
	OperationUpdated EventType = "operation.updated"
	OperationDeleted EventType = "operation.deleted"

	// Balance events
	BalanceCreated      EventType = "balance.created"
	BalanceUpdated      EventType = "balance.updated"
	BalanceDeleted      EventType = "balance.deleted"
	BalanceRecalculated EventType = "balance.recalculated"

	// Account events
	AccountCreated   EventType = "account.created"
	AccountUpdated   EventType = "account.updated"
	AccountDeleted   EventType = "account.deleted"
	AccountActivated EventType = "account.activated"
	AccountFrozen    EventType = "account.frozen"

	// Organization events
	OrganizationCreated EventType = "organization.created"
	OrganizationUpdated EventType = "organization.updated"
	OrganizationDeleted EventType = "organization.deleted"

	// Ledger events
	LedgerCreated EventType = "ledger.created"
	LedgerUpdated EventType = "ledger.updated"
	LedgerDeleted EventType = "ledger.deleted"

	// Asset events
	AssetCreated EventType = "asset.created"
	AssetUpdated EventType = "asset.updated"
	AssetDeleted EventType = "asset.deleted"
)

// DomainEvent represents a base domain event with common fields
type DomainEvent struct {
	// ID is the unique identifier for this event
	ID uuid.UUID `json:"id"`

	// Type is the type of event
	Type EventType `json:"type"`

	// AggregateID is the ID of the aggregate that this event is for
	AggregateID uuid.UUID `json:"aggregate_id"`

	// AggregateType is the type of aggregate (e.g., "Transaction", "Account")
	AggregateType string `json:"aggregate_type"`

	// Version is the version of the aggregate after this event
	Version int `json:"version"`

	// OccurredAt is when the event occurred
	OccurredAt time.Time `json:"occurred_at"`

	// Metadata contains additional context about the event
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// OrganizationID is the organization this event belongs to
	OrganizationID uuid.UUID `json:"organization_id"`

	// LedgerID is the ledger this event belongs to (if applicable)
	LedgerID *uuid.UUID `json:"ledger_id,omitempty"`

	// UserID is the user who triggered this event (if applicable)
	UserID *string `json:"user_id,omitempty"`

	// CorrelationID for tracing related events
	CorrelationID *string `json:"correlation_id,omitempty"`

	// CausationID links to the event that caused this one
	CausationID *uuid.UUID `json:"causation_id,omitempty"`
}

// NewDomainEvent creates a new domain event with common fields populated
func NewDomainEvent(eventType EventType, aggregateID uuid.UUID, aggregateType string, organizationID uuid.UUID) DomainEvent {
	return DomainEvent{
		ID:             uuid.New(),
		Type:           eventType,
		AggregateID:    aggregateID,
		AggregateType:  aggregateType,
		Version:        1,
		OccurredAt:     time.Now(),
		OrganizationID: organizationID,
		Metadata:       make(map[string]interface{}),
	}
}

// WithLedger adds ledger context to the event
func (e DomainEvent) WithLedger(ledgerID uuid.UUID) DomainEvent {
	e.LedgerID = &ledgerID
	return e
}

// WithUser adds user context to the event
func (e DomainEvent) WithUser(userID string) DomainEvent {
	e.UserID = &userID
	return e
}

// WithCorrelation adds correlation context to the event
func (e DomainEvent) WithCorrelation(correlationID string) DomainEvent {
	e.CorrelationID = &correlationID
	return e
}

// WithCausation links this event to a causing event
func (e DomainEvent) WithCausation(causationID uuid.UUID) DomainEvent {
	e.CausationID = &causationID
	return e
}

// WithMetadata adds or updates metadata
func (e DomainEvent) WithMetadata(key string, value interface{}) DomainEvent {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// EventHandler defines the interface for handling domain events
type EventHandler interface {
	// Handle processes the domain event
	Handle(ctx context.Context, event DomainEvent) error

	// CanHandle returns true if this handler can process the given event type
	CanHandle(eventType EventType) bool
}

// EventPublisher defines the interface for publishing domain events
type EventPublisher interface {
	// Publish sends an event to be processed
	Publish(ctx context.Context, event DomainEvent) error

	// PublishBatch sends multiple events to be processed
	PublishBatch(ctx context.Context, events []DomainEvent) error
}

// EventStore defines the interface for storing domain events
type EventStore interface {
	// Save persists an event to the store
	Save(ctx context.Context, event DomainEvent) error

	// SaveBatch persists multiple events to the store
	SaveBatch(ctx context.Context, events []DomainEvent) error

	// GetByAggregate retrieves all events for a specific aggregate
	GetByAggregate(ctx context.Context, aggregateID uuid.UUID) ([]DomainEvent, error)

	// GetByType retrieves events of a specific type
	GetByType(ctx context.Context, eventType EventType, limit int) ([]DomainEvent, error)

	// GetAfter retrieves events that occurred after a specific time
	GetAfter(ctx context.Context, after time.Time, limit int) ([]DomainEvent, error)
}

// EventBus defines the interface for an event bus that routes events to handlers
type EventBus interface {
	EventPublisher

	// Subscribe registers a handler for specific event types
	Subscribe(handler EventHandler, eventTypes ...EventType) error

	// Unsubscribe removes a handler
	Unsubscribe(handler EventHandler) error

	// Start begins processing events
	Start(ctx context.Context) error

	// Stop gracefully shuts down the event bus
	Stop(ctx context.Context) error
}