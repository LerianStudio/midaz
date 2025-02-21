package mmodel

import "time"

// CreateAssetInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAssetInput
//
//	@Description	CreateAssetInput is the input payload to create an asset.
type CreateAssetInput struct {
	Name     string         `json:"name" validate:"required,max=256" example:"Brazilian Real"`
	Type     string         `json:"type" example:"currency"`
	Code     string         `json:"code" validate:"required,max=100" example:"BRL"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} //	@name	CreateAssetInput

// UpdateAssetInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAssetInput
//
//	@Description	UpdateAssetInput is the input payload to update an asset.
type UpdateAssetInput struct {
	Name     string         `json:"name" validate:"max=256" example:"Bitcoin"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} //	@name	UpdateAssetInput

// Asset is a struct designed to encapsulate payload data.
//
// swagger:model Asset
//
//	@Description	Asset is a struct designed to store asset data.
type Asset struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"Brazilian Real"`
	Type           string         `json:"type" example:"currency"`
	Code           string         `json:"code" example:"BRL"`
	Status         Status         `json:"status"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} //	@name	Asset

// Assets struct to return get all.
//
// swagger:model Assets
//
//	@Description	Assets is the struct designed to return a list of assets with pagination.
type Assets struct {
	Items []Asset `json:"items"`
	Page  int     `json:"page" example:"1"`
	Limit int     `json:"limit" example:"10"`
} //	@name	Assets
