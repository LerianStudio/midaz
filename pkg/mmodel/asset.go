package mmodel

import "time"

// CreateAssetInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAssetInput
//
//	@Description	CreateAssetInput is the input payload to create an asset within a ledger, such as a currency, cryptocurrency, or other financial instrument.
type CreateAssetInput struct {
	// Name of the asset (required, max length 256 characters)
	Name string `json:"name" validate:"required,max=256" example:"Brazilian Real"`

	// Type of the asset (e.g., currency, cryptocurrency, commodity, stock)
	Type string `json:"type" example:"currency"`

	// Unique code/symbol for the asset (required, max length 100 characters)
	Code string `json:"code" validate:"required,max=100" example:"BRL"`

	// Status of the asset (active, inactive, pending)
	Status Status `json:"status"`

	// Additional custom attributes for the asset
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} //	@name	CreateAssetInput

// UpdateAssetInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAssetInput
//
//	@Description	UpdateAssetInput is the input payload to update an existing asset's properties such as name, status, and metadata.
type UpdateAssetInput struct {
	// Updated name of the asset (optional, max length 256 characters)
	Name string `json:"name" validate:"max=256" example:"Bitcoin"`

	// Updated status of the asset (active, inactive, pending)
	Status Status `json:"status"`

	// Updated or additional custom attributes for the asset
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} //	@name	UpdateAssetInput

// Asset is a struct designed to encapsulate payload data.
//
// swagger:model Asset
//
//	@Description	Asset represents a financial instrument within a ledger, such as a currency, cryptocurrency, commodity, or other asset type.
type Asset struct {
	// Unique identifier for the asset (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Name of the asset (max length 256 characters)
	Name string `json:"name" example:"Brazilian Real" maxLength:"256"`

	// Type of the asset (e.g., currency, cryptocurrency, commodity, stock)
	Type string `json:"type" example:"currency"`

	// Unique code/symbol for the asset (max length 100 characters)
	Code string `json:"code" example:"BRL" maxLength:"100"`

	// Status of the asset (active, inactive, pending)
	Status Status `json:"status"`

	// ID of the ledger this asset belongs to (UUID format)
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this asset (UUID format)
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Timestamp when the asset was created
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was last updated
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was deleted (null if not deleted)
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes for the asset
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	Asset

// Assets struct to return get all.
//
// swagger:model Assets
//
//	@Description	Assets represents a paginated collection of asset records returned by list operations.
type Assets struct {
	// Array of asset records
	Items []Asset `json:"items"`

	// Current page number
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} //	@name	Assets
