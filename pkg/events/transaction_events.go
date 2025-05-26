package events

import (
	"time"

	"github.com/google/uuid"
)

// TransactionCreatedEvent represents the event when a transaction is created
type TransactionCreatedEvent struct {
	DomainEvent
	TransactionID       string                 `json:"transaction_id"`
	ParentTransactionID *string                `json:"parent_transaction_id,omitempty"`
	Description         string                 `json:"description"`
	Status              string                 `json:"status"`
	Amount              *float64               `json:"amount,omitempty"`
	AmountScale         *int                   `json:"amount_scale,omitempty"`
	AssetCode           string                 `json:"asset_code"`
	Template            string                 `json:"template,omitempty"`
	ChartOfAccounts     string                 `json:"chart_of_accounts,omitempty"`
	Body                map[string]interface{} `json:"body,omitempty"`
}

// NewTransactionCreatedEvent creates a new transaction created event
func NewTransactionCreatedEvent(organizationID, ledgerID, transactionID uuid.UUID) TransactionCreatedEvent {
	base := NewDomainEvent(TransactionCreated, transactionID, "Transaction", organizationID).WithLedger(ledgerID)
	return TransactionCreatedEvent{
		DomainEvent:   base,
		TransactionID: transactionID.String(),
	}
}

// TransactionApprovedEvent represents the event when a transaction is approved
type TransactionApprovedEvent struct {
	DomainEvent
	TransactionID string    `json:"transaction_id"`
	ApprovedBy    *string   `json:"approved_by,omitempty"`
	ApprovedAt    time.Time `json:"approved_at"`
	Comments      *string   `json:"comments,omitempty"`
}

// NewTransactionApprovedEvent creates a new transaction approved event
func NewTransactionApprovedEvent(organizationID, ledgerID, transactionID uuid.UUID) TransactionApprovedEvent {
	base := NewDomainEvent(TransactionApproved, transactionID, "Transaction", organizationID).WithLedger(ledgerID)
	return TransactionApprovedEvent{
		DomainEvent:   base,
		TransactionID: transactionID.String(),
		ApprovedAt:    time.Now(),
	}
}

// TransactionRejectedEvent represents the event when a transaction is rejected
type TransactionRejectedEvent struct {
	DomainEvent
	TransactionID string    `json:"transaction_id"`
	RejectedBy    *string   `json:"rejected_by,omitempty"`
	RejectedAt    time.Time `json:"rejected_at"`
	Reason        string    `json:"reason"`
}

// NewTransactionRejectedEvent creates a new transaction rejected event
func NewTransactionRejectedEvent(organizationID, ledgerID, transactionID uuid.UUID, reason string) TransactionRejectedEvent {
	base := NewDomainEvent(TransactionRejected, transactionID, "Transaction", organizationID).WithLedger(ledgerID)
	return TransactionRejectedEvent{
		DomainEvent:   base,
		TransactionID: transactionID.String(),
		RejectedAt:    time.Now(),
		Reason:        reason,
	}
}

// OperationCreatedEvent represents the event when an operation is created
type OperationCreatedEvent struct {
	DomainEvent
	OperationID      string                 `json:"operation_id"`
	TransactionID    string                 `json:"transaction_id"`
	AccountID        string                 `json:"account_id"`
	Type             string                 `json:"type"` // credit or debit
	Amount           float64                `json:"amount"`
	AmountScale      int                    `json:"amount_scale"`
	AssetCode        string                 `json:"asset_code"`
	Balance          *float64               `json:"balance,omitempty"`
	BalanceScale     *int                   `json:"balance_scale,omitempty"`
	ChartOfAccounts  string                 `json:"chart_of_accounts,omitempty"`
	AccountPath      string                 `json:"account_path,omitempty"`
	PortfolioID      *string                `json:"portfolio_id,omitempty"`
	SegmentID        *string                `json:"segment_id,omitempty"`
	SourceID         *string                `json:"source_id,omitempty"`
	OperationMetadata map[string]interface{} `json:"operation_metadata,omitempty"`
}

// NewOperationCreatedEvent creates a new operation created event
func NewOperationCreatedEvent(organizationID, ledgerID, operationID, transactionID, accountID uuid.UUID) OperationCreatedEvent {
	base := NewDomainEvent(OperationCreated, operationID, "Operation", organizationID).WithLedger(ledgerID)
	return OperationCreatedEvent{
		DomainEvent:   base,
		OperationID:   operationID.String(),
		TransactionID: transactionID.String(),
		AccountID:     accountID.String(),
	}
}

// BalanceUpdatedEvent represents the event when a balance is updated
type BalanceUpdatedEvent struct {
	DomainEvent
	BalanceID    string   `json:"balance_id"`
	AccountID    string   `json:"account_id"`
	AssetCode    string   `json:"asset_code"`
	OldBalance   float64  `json:"old_balance"`
	NewBalance   float64  `json:"new_balance"`
	BalanceScale int      `json:"balance_scale"`
	Change       float64  `json:"change"`
	OperationID  string   `json:"operation_id"`
	Reason       string   `json:"reason"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NewBalanceUpdatedEvent creates a new balance updated event
func NewBalanceUpdatedEvent(organizationID, ledgerID, balanceID, accountID uuid.UUID) BalanceUpdatedEvent {
	base := NewDomainEvent(BalanceUpdated, balanceID, "Balance", organizationID).WithLedger(ledgerID)
	return BalanceUpdatedEvent{
		DomainEvent: base,
		BalanceID:   balanceID.String(),
		AccountID:   accountID.String(),
		UpdatedAt:   time.Now(),
	}
}

// TransactionCompletedEvent represents the event when a transaction and all its operations are completed
type TransactionCompletedEvent struct {
	DomainEvent
	TransactionID    string    `json:"transaction_id"`
	OperationCount   int       `json:"operation_count"`
	TotalDebits      float64   `json:"total_debits"`
	TotalCredits     float64   `json:"total_credits"`
	CompletedAt      time.Time `json:"completed_at"`
	ProcessingTimeMs int64     `json:"processing_time_ms"`
}

// NewTransactionCompletedEvent creates a new transaction completed event
func NewTransactionCompletedEvent(organizationID, ledgerID, transactionID uuid.UUID) TransactionCompletedEvent {
	base := NewDomainEvent(TransactionCompleted, transactionID, "Transaction", organizationID).WithLedger(ledgerID)
	return TransactionCompletedEvent{
		DomainEvent:   base,
		TransactionID: transactionID.String(),
		CompletedAt:   time.Now(),
	}
}