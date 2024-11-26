package mmodel

import "time"

// CreateProductInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateProductInput
// @Description CreateProductInput is the input payload to create a product.
type CreateProductInput struct {
	Name     string         `json:"name" validate:"required,max=256" example:"My Product"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateProductInput

// UpdateProductInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateProductInput
// @Description UpdateProductInput is the input payload to update a product.
type UpdateProductInput struct {
	Name     string         `json:"name" validate:"max=256" example:"My Product Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateProductInput

// Product is a struct designed to encapsulate payload data.
//
// swagger:model Product
// @Description Product is a struct designed to store product data.
type Product struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"My Product"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} // @name Product

// Products struct to return get all.
//
// swagger:model Products
// @Description Products is the struct designed to return a list of products with pagination.
type Products struct {
	Items []Product `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
} // @name Products
