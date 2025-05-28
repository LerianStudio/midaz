package events

import (
	"time"

	"github.com/google/uuid"
)

// AccountCreatedEvent represents the event when an account is created
type AccountCreatedEvent struct {
	DomainEvent
	AccountID       string                 `json:"account_id"`
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`
	Status          string                 `json:"status"`
	AllowSending    bool                   `json:"allow_sending"`
	AllowReceiving  bool                   `json:"allow_receiving"`
	AssetCode       string                 `json:"asset_code"`
	Alias           *string                `json:"alias,omitempty"`
	ProductID       *string                `json:"product_id,omitempty"`
	PortfolioID     *string                `json:"portfolio_id,omitempty"`
	SegmentID       *string                `json:"segment_id,omitempty"`
	AccountMetadata map[string]interface{} `json:"account_metadata,omitempty"`
}

// NewAccountCreatedEvent creates a new account created event
func NewAccountCreatedEvent(organizationID, ledgerID, accountID uuid.UUID) AccountCreatedEvent {
	base := NewDomainEvent(AccountCreated, accountID, "Account", organizationID).WithLedger(ledgerID)
	return AccountCreatedEvent{
		DomainEvent: base,
		AccountID:   accountID.String(),
	}
}

// AccountActivatedEvent represents the event when an account is activated
type AccountActivatedEvent struct {
	DomainEvent
	AccountID    string    `json:"account_id"`
	ActivatedBy  *string   `json:"activated_by,omitempty"`
	ActivatedAt  time.Time `json:"activated_at"`
	PreviousStatus string  `json:"previous_status"`
}

// NewAccountActivatedEvent creates a new account activated event
func NewAccountActivatedEvent(organizationID, ledgerID, accountID uuid.UUID, previousStatus string) AccountActivatedEvent {
	base := NewDomainEvent(AccountActivated, accountID, "Account", organizationID).WithLedger(ledgerID)
	return AccountActivatedEvent{
		DomainEvent:    base,
		AccountID:      accountID.String(),
		ActivatedAt:    time.Now(),
		PreviousStatus: previousStatus,
	}
}

// AccountFrozenEvent represents the event when an account is frozen
type AccountFrozenEvent struct {
	DomainEvent
	AccountID      string    `json:"account_id"`
	FrozenBy       *string   `json:"frozen_by,omitempty"`
	FrozenAt       time.Time `json:"frozen_at"`
	Reason         string    `json:"reason"`
	PreviousStatus string    `json:"previous_status"`
}

// NewAccountFrozenEvent creates a new account frozen event
func NewAccountFrozenEvent(organizationID, ledgerID, accountID uuid.UUID, reason, previousStatus string) AccountFrozenEvent {
	base := NewDomainEvent(AccountFrozen, accountID, "Account", organizationID).WithLedger(ledgerID)
	return AccountFrozenEvent{
		DomainEvent:    base,
		AccountID:      accountID.String(),
		FrozenAt:       time.Now(),
		Reason:         reason,
		PreviousStatus: previousStatus,
	}
}

// AccountUpdatedEvent represents the event when an account is updated
type AccountUpdatedEvent struct {
	DomainEvent
	AccountID      string                 `json:"account_id"`
	ChangedFields  []string               `json:"changed_fields"`
	OldValues      map[string]interface{} `json:"old_values,omitempty"`
	NewValues      map[string]interface{} `json:"new_values,omitempty"`
	UpdatedBy      *string                `json:"updated_by,omitempty"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// NewAccountUpdatedEvent creates a new account updated event
func NewAccountUpdatedEvent(organizationID, ledgerID, accountID uuid.UUID) AccountUpdatedEvent {
	base := NewDomainEvent(AccountUpdated, accountID, "Account", organizationID).WithLedger(ledgerID)
	return AccountUpdatedEvent{
		DomainEvent:   base,
		AccountID:     accountID.String(),
		UpdatedAt:     time.Now(),
		ChangedFields: []string{},
		OldValues:     make(map[string]interface{}),
		NewValues:     make(map[string]interface{}),
	}
}

// AccountDeletedEvent represents the event when an account is deleted (soft delete)
type AccountDeletedEvent struct {
	DomainEvent
	AccountID  string    `json:"account_id"`
	DeletedBy  *string   `json:"deleted_by,omitempty"`
	DeletedAt  time.Time `json:"deleted_at"`
	Reason     *string   `json:"reason,omitempty"`
}

// NewAccountDeletedEvent creates a new account deleted event
func NewAccountDeletedEvent(organizationID, ledgerID, accountID uuid.UUID) AccountDeletedEvent {
	base := NewDomainEvent(AccountDeleted, accountID, "Account", organizationID).WithLedger(ledgerID)
	return AccountDeletedEvent{
		DomainEvent: base,
		AccountID:   accountID.String(),
		DeletedAt:   time.Now(),
	}
}

// OrganizationCreatedEvent represents the event when an organization is created
type OrganizationCreatedEvent struct {
	DomainEvent
	OrganizationName     string                 `json:"organization_name"`
	LegalName            string                 `json:"legal_name"`
	LegalDocument        string                 `json:"legal_document"`
	Status               string                 `json:"status"`
	OrganizationMetadata map[string]interface{} `json:"organization_metadata,omitempty"`
}

// NewOrganizationCreatedEvent creates a new organization created event
func NewOrganizationCreatedEvent(organizationID uuid.UUID) OrganizationCreatedEvent {
	// Organization events don't have a ledger
	base := NewDomainEvent(OrganizationCreated, organizationID, "Organization", organizationID)
	return OrganizationCreatedEvent{
		DomainEvent: base,
	}
}

// LedgerCreatedEvent represents the event when a ledger is created
type LedgerCreatedEvent struct {
	DomainEvent
	LedgerName     string                 `json:"ledger_name"`
	Status         string                 `json:"status"`
	LedgerMetadata map[string]interface{} `json:"ledger_metadata,omitempty"`
}

// NewLedgerCreatedEvent creates a new ledger created event
func NewLedgerCreatedEvent(organizationID, ledgerID uuid.UUID) LedgerCreatedEvent {
	base := NewDomainEvent(LedgerCreated, ledgerID, "Ledger", organizationID).WithLedger(ledgerID)
	return LedgerCreatedEvent{
		DomainEvent: base,
	}
}

// AssetCreatedEvent represents the event when an asset is created
type AssetCreatedEvent struct {
	DomainEvent
	AssetCode     string                 `json:"asset_code"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Precision     int                    `json:"precision"`
	AssetMetadata map[string]interface{} `json:"asset_metadata,omitempty"`
}

// NewAssetCreatedEvent creates a new asset created event
func NewAssetCreatedEvent(organizationID, ledgerID, assetID uuid.UUID) AssetCreatedEvent {
	base := NewDomainEvent(AssetCreated, assetID, "Asset", organizationID).WithLedger(ledgerID)
	return AssetCreatedEvent{
		DomainEvent: base,
	}
}