package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreatePortfolioInput
// @Description CreatePortfolioInput is the input payload to create a portfolio.
type CreatePortfolioInput struct {
	EntityID string         `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name" validate:"required,max=256" example:"My Portfolio"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreatePortfolioInput

// UpdatePortfolioInput is a struct design to encapsulate payload data.
//
// swagger:model UpdatePortfolioInput
// @Description UpdatePortfolioInput is the input payload to update a portfolio.
type UpdatePortfolioInput struct {
	EntityID string         `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name" validate:"max=256" example:"My Portfolio Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdatePortfolioInput

// Portfolio is a struct designed to encapsulate request update payload data.
//
// swagger:model Portfolio
// @Description Portfolio is a struct designed to store portfolio data.
type Portfolio struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"My Portfolio"`
	EntityID       string         `json:"entityId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} // @name Portfolio

// Portfolios struct to return get all.
//
// swagger:model Portfolios
// @Description Portfolios is the struct designed to return a list of portfolios with pagination.
type Portfolios struct {
	Items []Portfolio `json:"items"`
	Page  int         `json:"page" example:"1"`
	Limit int         `json:"limit" example:"10"`
} // @name Portfolios
