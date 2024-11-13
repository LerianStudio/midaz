package mmodel

import "time"

// CreateProductInput is a struct design to encapsulate request create payload data.
type CreateProductInput struct {
	Name     string         `json:"name" validate:"required,max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateProductInput is a struct design to encapsulate request update payload data.
type UpdateProductInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Product is a struct designed to encapsulate payload data.
type Product struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Products struct to return get all.
type Products struct {
	Items []Product `json:"items"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}
