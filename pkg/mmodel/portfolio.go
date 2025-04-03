package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreatePortfolioInput
//
// @Description CreatePortfolioInput is the input payload to create a portfolio within a ledger, representing a collection of accounts grouped for specific purposes.
type CreatePortfolioInput struct {
	// Optional external entity identifier (max length 256 characters)
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	
	// Name of the portfolio (required, max length 256 characters)
	Name string `json:"name" validate:"required,max=256" example:"My Portfolio"`
	
	// Status of the portfolio (active, inactive, pending)
	Status Status `json:"status"`
	
	// Additional custom attributes for the portfolio 
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreatePortfolioInput

// UpdatePortfolioInput is a struct design to encapsulate payload data.
//
// swagger:model UpdatePortfolioInput
//
// @Description UpdatePortfolioInput is the input payload to update an existing portfolio's properties such as name, entity ID, status, and metadata.
type UpdatePortfolioInput struct {
	// Updated external entity identifier (optional, max length 256 characters)
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	
	// Updated name of the portfolio (optional, max length 256 characters)
	Name string `json:"name" validate:"max=256" example:"My Portfolio Updated"`
	
	// Updated status of the portfolio (active, inactive, pending)
	Status Status `json:"status"`
	
	// Updated or additional custom attributes for the portfolio
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdatePortfolioInput

// Portfolio is a struct designed to encapsulate request update payload data.
//
// swagger:model Portfolio
//
// @Description Portfolio represents a collection of accounts grouped for specific purposes such as business units, departments, or client portfolios.
type Portfolio struct {
	// Unique identifier for the portfolio (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Name of the portfolio (max length 256 characters)
	Name string `json:"name" example:"My Portfolio" maxLength:"256"`
	
	// Optional external entity identifier (max length 256 characters)
	EntityID string `json:"entityId,omitempty" example:"00000000-0000-0000-0000-000000000000" maxLength:"256"`
	
	// ID of the ledger this portfolio belongs to (UUID format)
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the organization that owns this portfolio (UUID format)
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Status of the portfolio (active, inactive, pending)
	Status Status `json:"status"`
	
	// Timestamp when the portfolio was created
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the portfolio was last updated
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the portfolio was deleted (null if not deleted)
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Additional custom attributes for the portfolio
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Portfolio

// Portfolios struct to return get all.
//
// swagger:model Portfolios
//
// @Description Portfolios represents a paginated collection of portfolio records returned by list operations.
type Portfolios struct {
	// Array of portfolio records
	Items []Portfolio `json:"items"`
	
	// Current page number
	Page int `json:"page" example:"1" minimum:"1"`
	
	// Maximum number of items per page
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Portfolios
