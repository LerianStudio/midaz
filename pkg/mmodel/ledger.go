package mmodel

import "time"

// CreateLedgerInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateLedgerInput
// @Description Request payload for creating a new ledger. Contains the ledger name (required), status, and optional metadata.
type CreateLedgerInput struct {
	// Display name of the ledger (required)
	Name string `json:"name" validate:"required,max=256" example:"Lerian Studio" maxLength:"256"`

	// Current operating status of the ledger (defaults to ACTIVE if not specified)
	Status Status `json:"status"`

	// Custom key-value pairs for extending the ledger information
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateLedgerInput

// UpdateLedgerInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateLedgerInput
// @Description Request payload for updating an existing ledger. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
type UpdateLedgerInput struct {
	// Updated display name of the ledger (optional)
	Name string `json:"name" validate:"max=256" example:"Lerian Studio Updated" maxLength:"256"`

	// Updated status of the ledger (optional)
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the ledger information (optional)
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateLedgerInput

// Ledger is a struct designed to encapsulate payload data.
//
// swagger:model Ledger
// @Description Complete ledger entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for ledger operations.
type Ledger struct {
	// Unique identifier for the ledger (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Display name of the ledger
	Name string `json:"name" example:"Lerian Studio" maxLength:"256"`

	// Reference to the organization that owns this ledger
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Current operating status of the ledger
	Status Status `json:"status"`

	// Timestamp when the ledger was created (RFC3339 format)
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the ledger was last updated (RFC3339 format)
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the ledger was soft deleted, null if not deleted (RFC3339 format)
	DeletedAt *time.Time `json:"deletedAt" sql:"index" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the ledger information
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Ledger

// Ledgers struct to return get all.
//
// swagger:model Ledgers
// @Description Paginated list of ledgers with metadata about the current page, limit, and the ledger items themselves.
type Ledgers struct {
	// List of ledger items in the current page
	Items []Ledger `json:"items"`

	// Current page number
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Ledgers
