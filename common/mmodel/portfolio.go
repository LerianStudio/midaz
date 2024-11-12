package mmodel

import "time"

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
type CreatePortfolioInput struct {
	EntityID string         `json:"entityId" validate:"required,max=256"`
	Name     string         `json:"name" validate:"required,max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdatePortfolioInput is a struct design to encapsulate payload data.
type UpdatePortfolioInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Portfolio is a struct designed to encapsulate request update payload data.
type Portfolio struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	EntityID       string         `json:"entityId"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Portfolios is a struct designed to encapsulate pagination response payload data.
type Portfolios struct {
	Items []Portfolio `json:"items"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}
