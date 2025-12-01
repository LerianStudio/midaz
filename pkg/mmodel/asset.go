package mmodel

import "time"

// CreateAssetInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAssetInput
// @Description Request payload for creating a new asset within a ledger. Assets represent currencies, cryptocurrencies, commodities, or other financial instruments tracked in the ledger system.
//
//	@example {
//	  "name": "US Dollar",
//	  "type": "currency",
//	  "code": "USD",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "metadata": {
//	    "country": "United States",
//	    "symbol": "$",
//	    "isoNumeric": "840"
//	  }
//	}
type CreateAssetInput struct {
	// Human-readable name of the asset
	// required: true
	// example: US Dollar
	// maxLength: 256
	Name string `json:"name" validate:"required,max=256" example:"US Dollar" maxLength:"256"`

	// Type classification of the asset (e.g., currency, cryptocurrency, commodity, stock)
	// required: true
	// example: currency
	Type string `json:"type" validate:"required" example:"currency"`

	// Unique code/symbol for the asset (e.g., USD, BTC, GOLD)
	// required: true
	// example: USD
	// maxLength: 100
	Code string `json:"code" validate:"required,max=100" example:"USD" maxLength:"100"`

	// Current operating status of the asset
	// required: false
	Status Status `json:"status"`

	// Custom key-value pairs for extending the asset information
	// required: false
	// example: {"country": "United States", "symbol": "$", "isoNumeric": "840"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAssetInput

// UpdateAssetInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAssetInput
// @Description Request payload for updating an existing asset. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
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
	// Updated human-readable name of the asset
	// required: false
	// example: Bitcoin
	// maxLength: 256
	Name string `json:"name" validate:"max=256" example:"Bitcoin" maxLength:"256"`

	// Updated status of the asset
	// required: false
	Status Status `json:"status"`

	// Updated custom key-value pairs for extending the asset information
	// required: false
	// example: {"country": "United States", "symbol": "$", "isoNumeric": "840", "updated": true}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateAssetInput

// Asset is a struct designed to encapsulate payload data.
//
// swagger:model Asset
// @Description Complete asset entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for asset operations. Assets represent financial instruments within a ledger, such as currencies, cryptocurrencies, commodities, or other value units.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "name": "US Dollar",
//	  "type": "currency",
//	  "code": "USD",
//	  "status": {
//	    "code": "ACTIVE"
//	  },
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "country": "United States",
//	    "symbol": "$",
//	    "isoNumeric": "840"
//	  }
//	}
type Asset struct {
	// Unique identifier for the asset (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Human-readable name of the asset
	// example: US Dollar
	// maxLength: 256
	Name string `json:"name" example:"US Dollar" maxLength:"256"`

	// Type classification of the asset (e.g., currency, cryptocurrency, commodity, stock)
	// example: currency
	Type string `json:"type" example:"currency"`

	// Unique code/symbol for the asset (e.g., USD, BTC, GOLD)
	// example: USD
	// maxLength: 100
	Code string `json:"code" example:"USD" maxLength:"100"`

	// Current operating status of the asset
	Status Status `json:"status"`

	// ID of the ledger this asset belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this asset (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Timestamp when the asset was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the asset information
	// example: {"country": "United States", "symbol": "$", "isoNumeric": "840"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Asset

// Assets struct to return get all.
//
// swagger:model Assets
// @Description Paginated list of assets with metadata about the current page, limit, and the asset items themselves. Used for list operations.
//
//	@example {
//	  "items": [
//	    {
//	      "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	      "name": "US Dollar",
//	      "code": "USD",
//	      "type": "currency",
//	      "status": {
//	        "code": "ACTIVE"
//	      },
//	      "createdAt": "2022-04-15T09:30:00Z",
//	      "updatedAt": "2022-04-15T09:30:00Z"
//	    },
//	    {
//	      "id": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	      "name": "Bitcoin",
//	      "code": "BTC",
//	      "type": "cryptocurrency",
//	      "status": {
//	        "code": "ACTIVE"
//	      },
//	      "createdAt": "2022-04-16T10:15:00Z",
//	      "updatedAt": "2022-04-16T10:15:00Z"
//	    }
//	  ],
//	  "page": 1,
//	  "limit": 10
//	}
type Assets struct {
	// Array of asset records returned in this page
	// example: [{"id":"00000000-0000-0000-0000-000000000000","name":"US Dollar","code":"USD","type":"currency","status":{"code":"ACTIVE"}}]
	Items []Asset `json:"items"`

	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Assets

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
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`

		// Human-readable error message
		// example: Invalid input: field 'code' is required
		Message string `json:"message"`

		// Additional error details if available
		// example: {"field": "code", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}
