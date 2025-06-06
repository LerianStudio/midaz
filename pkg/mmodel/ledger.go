package mmodel

import "time"

// CreateLedgerInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreateLedgerInput
// @Description Request payload for creating a new ledger. Contains the ledger name (required), status, and optional metadata. Ledgers are organizational units within an organization that group related financial accounts and assets together.
type CreateLedgerInput struct {
	// Display name of the ledger
	// required: true
	// maxLength: 256
	Name string `json:"name" validate:"required,max=256" maxLength:"256"`

	// Current operating status of the ledger (defaults to ACTIVE if not specified)
	// required: false
	Status Status `json:"status"`

	// Custom key-value pairs for extending the ledger information
	// required: false
	// example: {"department": "Finance", "currency": "USD", "region": "North America"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateLedgerInput

// UpdateLedgerInput is a struct designed to encapsulate request update payload data.
//
// swagger:model UpdateLedgerInput
// @Description Request payload for updating an existing ledger. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
type UpdateLedgerInput struct {
	// Updated display name of the ledger
	// required: false
	// example: Treasury Operations Global
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Treasury Operations Global" maxLength:"256"`

	// Updated status of the ledger
	// required: false
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the ledger information
	// required: false
	// example: {"department": "Global Finance", "currency": "USD", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateLedgerInput

// Ledger is a struct designed to encapsulate response payload data.
//
// swagger:model Ledger
// @Description Complete ledger entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for ledger operations. Ledgers are organizational units within an organization that group related financial accounts and assets together.
type Ledger struct {
	// Unique identifier for the ledger (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Display name of the ledger
	// example: Treasury Operations
	// maxLength: 256
	Name string `json:"name" example:"Treasury Operations" maxLength:"256"`

	// Reference to the organization that owns this ledger (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Current operating status of the ledger
	Status Status `json:"status"`

	// Timestamp when the ledger was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the ledger was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the ledger was soft deleted, null if not deleted (RFC3339 format)
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" sql:"index" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the ledger information
	// example: {"department": "Finance", "currency": "USD", "region": "North America"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Ledger

// Ledgers struct designed to return a paginated list of ledgers.
//
// swagger:model Ledgers
// @Description Paginated list of ledgers with metadata about the current page, limit, and the ledger items themselves. Used for list operations.
type Ledgers struct {
	// Array of ledger records returned in this page
	// example: [{"id":"00000000-0000-0000-0000-000000000000","name":"Treasury Operations","status":{"code":"ACTIVE"}}]
	Items []Ledger `json:"items"`

	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Ledgers

// LedgerResponse represents a success response containing a single ledger.
//
// swagger:response LedgerResponse
// @Description Successful response containing a single ledger entity.
type LedgerResponse struct {
	// in: body
	Body Ledger
}

// LedgersResponse represents a success response containing a paginated list of ledgers.
//
// swagger:response LedgersResponse
// @Description Successful response containing a paginated list of ledgers.
type LedgersResponse struct {
	// in: body
	Body Ledgers
}

// LedgerErrorResponse represents an error response for ledger operations.
//
// swagger:response LedgerErrorResponse
// @Description Error response for ledger operations with error code and message.
type LedgerErrorResponse struct {
	// in: body
	Body struct {
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`

		// Human-readable error message
		// example: Invalid input: field 'name' is required
		Message string `json:"message"`

		// Additional error details if available
		// example: {"field": "name", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
