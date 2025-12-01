package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreatePortfolioInput
// @Description Request payload for creating a new portfolio within a ledger. Portfolios represent collections of accounts grouped for specific purposes such as business units, departments, or client portfolios.
//
//	@example {
//	  "entityId": "EXT-PORT-001",
//	  "name": "Corporate Treasury Portfolio",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "department": "Treasury",
//	    "purpose": "Operating Accounts",
//	    "region": "North America"
//	  }
//	}
type CreatePortfolioInput struct {
	// Optional external entity identifier for linking to external systems
	// required: false
	// example: EXT-PORT-001
	// maxLength: 256
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"EXT-PORT-001" maxLength:"256"`

	// Human-readable name of the portfolio
	// required: true
	// example: Corporate Treasury Portfolio
	// maxLength: 256
	Name string `json:"name" validate:"required,max=256" example:"Corporate Treasury Portfolio" maxLength:"256"`

	// Current operating status of the portfolio (defaults to ACTIVE if not specified)
	// required: false
	Status Status `json:"status"`

	// Custom key-value pairs for extending the portfolio information
	// required: false
	// example: {"department": "Treasury", "purpose": "Operating Accounts", "region": "North America"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreatePortfolioInput

// UpdatePortfolioInput is a struct design to encapsulate payload data.
//
// swagger:model UpdatePortfolioInput
// @Description Request payload for updating an existing portfolio. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "entityId": "EXT-PORT-002",
//	  "name": "Updated Treasury Portfolio",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "department": "Global Treasury",
//	    "purpose": "Primary Operations",
//	    "region": "Global"
//	  }
//	}
type UpdatePortfolioInput struct {
	// Updated external entity identifier
	// required: false
	// example: EXT-PORT-002
	// maxLength: 256
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"EXT-PORT-002" maxLength:"256"`

	// Updated name of the portfolio
	// required: false
	// example: Updated Treasury Portfolio
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Updated Treasury Portfolio" maxLength:"256"`

	// Updated status of the portfolio
	// required: false
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the portfolio information
	// required: false
	// example: {"department": "Global Treasury", "purpose": "Primary Operations", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdatePortfolioInput

// Portfolio is a struct designed to encapsulate request update payload data.
//
// swagger:model Portfolio
// @Description Complete portfolio entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for portfolio operations. Portfolios represent collections of accounts grouped for specific purposes.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "name": "Corporate Treasury Portfolio",
//	  "entityId": "EXT-PORT-001",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "department": "Treasury",
//	    "purpose": "Operating Accounts"
//	  }
//	}
type Portfolio struct {
	// Unique identifier for the portfolio (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable name of the portfolio
	// example: Corporate Treasury Portfolio
	// maxLength: 256
	Name string `json:"name" example:"Corporate Treasury Portfolio" maxLength:"256"`

	// Optional external entity identifier for linking to external systems
	// example: EXT-PORT-001
	// maxLength: 256
	EntityID string `json:"entityId,omitempty" example:"EXT-PORT-001" maxLength:"256"`

	// ID of the ledger this portfolio belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this portfolio (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Current operating status of the portfolio
	Status Status `json:"status"`

	// Timestamp when the portfolio was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the portfolio was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the portfolio was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the portfolio information
	// example: {"department": "Treasury", "purpose": "Operating Accounts"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Portfolio

// Portfolios struct to return get all.
//
// swagger:model Portfolios
//
// @Description Portfolios represents a paginated collection of portfolio records returned by list operations.
type Portfolios struct {
	// Array of portfolio records
	// example: [{"id":"00000000-0000-0000-0000-000000000000","name":"My Portfolio","ledgerId":"00000000-0000-0000-0000-000000000000","status":{"code":"ACTIVE"}}]
	Items []Portfolio `json:"items"`

	// Current page number
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Portfolios

// PortfolioResponse represents a success response containing a single portfolio.
//
// swagger:response PortfolioResponse
// @Description Successful response containing a single portfolio entity.
type PortfolioResponse struct {
	// in: body
	Body Portfolio
}

// PortfoliosResponse represents a success response containing a paginated list of portfolios.
//
// swagger:response PortfoliosResponse
// @Description Successful response containing a paginated list of portfolios.
type PortfoliosResponse struct {
	// in: body
	Body Portfolios
}

// PortfolioErrorResponse represents an error response for portfolio operations.
//
// swagger:response PortfolioErrorResponse
// @Description Error response for portfolio operations with error code and message.
type PortfolioErrorResponse struct {
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
