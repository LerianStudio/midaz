// Package mmodel defines domain models for the Midaz platform.
// This file contains Asset-related models and input/output structures.
package mmodel

import "time"

// CreateAssetInput represents the input data for creating a new asset.
//
// swagger:model CreateAssetInput
//
//	@Description	CreateAssetInput is the input payload to create an asset within a ledger, such as a currency, cryptocurrency, or other financial instrument.
//
//	@example		{
//	  "name": "US Dollar",
//	  "type": "currency",
//	  "code": "USD",
//	  "status": "ACTIVE",
//	  "metadata": {
//	    "country": "United States",
//	    "symbol": "$",
//	    "isoNumeric": "840"
//	  }
//	}
type CreateAssetInput struct {
	// Human-readable name of the asset.
	Name string `json:"name" validate:"required,max=256" example:"US Dollar"`

	// Type of the asset (e.g., currency, cryptocurrency, commodity, stock).
	Type string `json:"type" validate:"required" example:"currency"`

	// Unique code/symbol for the asset.
	Code string `json:"code" validate:"required,max=100" example:"USD"`

	// Status of the asset (e.g., ACTIVE, INACTIVE, PENDING).
	Status Status `json:"status"`

	// Custom key-value pairs for extending the asset information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} //	@name	CreateAssetInput

// UpdateAssetInput represents the input data for updating an existing asset.
//
// swagger:model UpdateAssetInput
//
//	@Description	UpdateAssetInput is the input payload to update an existing asset's properties such as name, status, and metadata.
//
//	@example		{
//	  "name": "US Dollar Updated",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "country": "United States",
//	    "symbol": "$",
//	    "isoNumeric": "840",
//	    "updated": true
//	  }
//	}
type UpdateAssetInput struct {
	// Updated name of the asset.
	Name string `json:"name" validate:"max=256" example:"Bitcoin"`

	// Updated status of the asset.
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the asset information.
	// Note: Nested structures are not supported.
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} //	@name	UpdateAssetInput

// Asset represents a financial instrument in the ledger system.
//
// swagger:model Asset
//
//	@Description	Asset represents a financial instrument within a ledger, such as a currency, cryptocurrency, commodity, or other asset type.
type Asset struct {
	// Unique identifier for the asset (UUID format).
	ID string `json:"id" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// Human-readable name of the asset.
	Name string `json:"name" example:"US Dollar" maxLength:"256"`

	// Type of the asset (e.g., currency, cryptocurrency, commodity, stock).
	Type string `json:"type" example:"currency"`

	// Unique code/symbol for the asset.
	Code string `json:"code" example:"USD" maxLength:"100"`

	// Current status of the asset.
	Status Status `json:"status"`

	// ID of the ledger this asset belongs to (UUID format).
	LedgerID string `json:"ledgerId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// ID of the organization that owns this asset (UUID format).
	OrganizationID string `json:"organizationId" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a" format:"uuid"`

	// Timestamp when the asset was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was soft-deleted (null if not deleted).
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the asset information.
	Metadata map[string]any `json:"metadata,omitempty"`
} //	@name	Asset

// Assets represents a paginated list of assets.
//
// swagger:model Assets
//
//	@Description	Assets represents a paginated collection of asset records returned by list operations.
type Assets struct {
	// Array of asset records for the current page.
	// example: [{"id":"01965ed9-7fa4-75b2-8872-fc9e8509ab0a","name":"US Dollar","code":"USD","type":"currency"}]
	Items []Asset `json:"items"`

	// Current page number in the pagination.
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page.
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} //	@name	Assets

// AssetResponse represents a success response containing a single asset.
//
// swagger:response AssetResponse
// @Description Successful response containing a single asset entity.
type AssetResponse struct {
	// in: body
	Body Asset
}

// AssetsResponse represents a success response containing a paginated list of assets.
//
// swagger:response AssetsResponse
// @Description Successful response containing a paginated list of assets.
type AssetsResponse struct {
	// in: body
	Body Assets
}

// AssetErrorResponse represents an error response for asset operations.
//
// swagger:response AssetErrorResponse
// @Description Error response for asset operations with error code and message.
type AssetErrorResponse struct {
	// in: body
	Body struct {
		// Error code identifying the specific error.
		// example: 400001
		Code int `json:"code"`

		// Human-readable error message.
		// example: Invalid input: field 'code' is required
		Message string `json:"message"`

		// Additional error details if available.
		// example: {"field": "code", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
