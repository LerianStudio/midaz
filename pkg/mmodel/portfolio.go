// Package mmodel defines domain models for the Midaz platform.
// This file contains Portfolio-related models and input/output structures.
package mmodel

import "time"

// CreatePortfolioInput represents the input data for creating a new portfolio.
//
// swagger:model CreatePortfolioInput
//
// @Description CreatePortfolioInput is the input payload to create a portfolio within a ledger, representing a collection of accounts grouped for specific purposes.
type CreatePortfolioInput struct {
	// An optional external identifier for the entity.
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The name of the portfolio.
	Name string `json:"name" validate:"required,max=256" example:"My Portfolio"`

	// The status of the portfolio (e.g., ACTIVE, INACTIVE, PENDING).
	Status Status `json:"status"`

	// Custom key-value pairs for extending the portfolio information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreatePortfolioInput

// UpdatePortfolioInput represents the input data for updating an existing portfolio.
//
// swagger:model UpdatePortfolioInput
//
// @Description UpdatePortfolioInput is the input payload to update an existing portfolio's properties such as name, entity ID, status, and metadata.
type UpdatePortfolioInput struct {
	// The updated external identifier for the entity.
	EntityID string `json:"entityId" validate:"omitempty,max=256" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`

	// The updated name of the portfolio.
	Name string `json:"name" validate:"max=256" example:"My Portfolio Updated"`

	// The updated status of the portfolio.
	Status Status `json:"status"`

	// The updated custom key-value pairs for extending the portfolio information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdatePortfolioInput

// Portfolio represents a collection of accounts.
//
// swagger:model Portfolio
//
// @Description Portfolio represents a collection of accounts grouped for specific purposes such as business units, departments, or client portfolios.
type Portfolio struct {
	// The unique identifier for the portfolio (UUID format).
	ID string `json:"id" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The name of the portfolio.
	Name string `json:"name" example:"My Portfolio" maxLength:"256"`

	// An optional external identifier for the entity.
	EntityID string `json:"entityId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" maxLength:"256"`

	// The ID of the ledger this portfolio belongs to (UUID format).
	LedgerID string `json:"ledgerId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The ID of the organization that owns this portfolio (UUID format).
	OrganizationID string `json:"organizationId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// The status of the portfolio.
	Status Status `json:"status"`

	// The timestamp when the portfolio was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the portfolio was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// The timestamp when the portfolio was soft-deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the portfolio information.
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Portfolio

// Portfolios represents a paginated list of portfolios.
//
// swagger:model Portfolios
//
// @Description Portfolios represents a paginated collection of portfolio records returned by list operations.
type Portfolios struct {
	// An array of portfolio records.
	// example: [{"id":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","name":"My Portfolio","ledgerId":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","status":{"code":"ACTIVE"}}]
	Items []Portfolio `json:"items"`

	// The current page number.
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// The maximum number of items per page.
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
		// The error code identifying the specific error.
		// example: 400001
		Code int `json:"code"`

		// A human-readable error message.
		// example: Invalid input: field 'name' is required
		Message string `json:"message"`

		// Additional error details if available.
		// example: {"field": "name", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
