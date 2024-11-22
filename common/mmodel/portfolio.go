package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreatePortfolioInput
// @Description CreatePortfolioInput is a struct design to encapsulate request create payload data.
type CreatePortfolioInput struct {
	EntityID string         `json:"entityId" validate:"required,max=256" example:"00000000-0000-0000-0000-000000000000"`
	Name     string         `json:"name" validate:"required,max=256" example:"My Portfolio"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreatePortfolioInput

// UpdatePortfolioInput is a struct design to encapsulate payload data.
//
// swagger:model UpdatePortfolioInput
// @Description UpdatePortfolioInput is a struct design to encapsulate payload data.
type UpdatePortfolioInput struct {
	Name     string         `json:"name" validate:"max=256" example:"My Portfolio Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdatePortfolioInput

// Portfolio is a struct designed to encapsulate request update payload data.
//
// swagger:model Portfolio
// @Description Portfolio is a struct designed to encapsulate request update payload data.
type Portfolio struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"My Portfolio"`
	EntityID       string         `json:"entityId" example:"00000000-0000-0000-0000-000000000000"`
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
// @Description Portfolios struct to return get all.
type Portfolios struct {
	Items []Portfolio `json:"items"`
	Page  int         `json:"page" example:"1"`
	Limit int         `json:"limit" example:"10"`
} // @name Portfolios
