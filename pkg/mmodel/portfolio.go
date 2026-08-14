// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
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
}

// UpdatePortfolioInput is a struct design to encapsulate payload data.
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
}

// Portfolio is a struct designed to encapsulate request update payload data.
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
}

// Portfolios struct to return get all.
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
}

// PortfolioResponse represents a success response containing a single portfolio.
type PortfolioResponse struct {
	Body Portfolio
}

// PortfoliosResponse represents a success response containing a paginated list of portfolios.
type PortfoliosResponse struct {
	Body Portfolios
}

// PortfolioErrorResponse represents an error response for portfolio operations.
type PortfolioErrorResponse struct {
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
