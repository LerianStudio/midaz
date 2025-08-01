package assetrate

import (
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
)

// AssetRatePostgreSQLModel represents the entity AssetRatePostgreSQLModel into SQL context in Database
//
// @Description Database model for storing asset rate information in PostgreSQL
type AssetRatePostgreSQLModel struct {
	ID             string         // Unique identifier (UUID format)
	OrganizationID string         // Organization that owns this asset rate
	LedgerID       string         // Ledger containing this asset rate
	ExternalID     string         // External identifier for integration
	From           string         // Source asset code
	To             string         // Target asset code
	Rate           float64        // Conversion rate value
	RateScale      float64        // Decimal places for the rate
	Source         *string        // Source of rate information (e.g., "External System")
	TTL            int            // Time-to-live in seconds
	CreatedAt      time.Time      // Timestamp when created
	UpdatedAt      time.Time      // Timestamp when last updated
	Metadata       map[string]any // Additional custom attributes
}

// CreateAssetRateInput is a struct design to encapsulate payload data.
//
// swagger:model CreateAssetRateInput
// @Description CreateAssetRateInput is the input payload to create an asset rate. Contains required fields for setting up asset conversion rates, including source and target assets, rate value, scale, and optional metadata.
type CreateAssetRateInput struct {
	// Source asset code (required)
	// example: USD
	// required: true
	// minLength: 2
	// maxLength: 10
	From string `json:"from" validate:"required" example:"USD" minLength:"2" maxLength:"10"`

	// Target asset code (required)
	// example: BRL
	// required: true
	// minLength: 2
	// maxLength: 10
	To string `json:"to" validate:"required" example:"BRL" minLength:"2" maxLength:"10"`

	// Conversion rate value (required)
	// example: 100
	// required: true
	Rate int `json:"rate" validate:"required" example:"100"`

	// Decimal places for the rate (optional)
	// example: 2
	// minimum: 0
	Scale int `json:"scale,omitempty" validate:"gte=0" example:"2" minimum:"0"`

	// Source of rate information (optional)
	// example: External System
	// maxLength: 200
	Source *string `json:"source,omitempty" example:"External System" maxLength:"200"`

	// Time-to-live in seconds (optional)
	// example: 3600
	// minimum: 0
	TTL *int `json:"ttl,omitempty" example:"3600" minimum:"0"`

	// External identifier for integration (optional)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ExternalID *string `json:"externalId,omitempty" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Additional custom attributes (optional)
	// example: {"provider": "Central Bank", "rateName": "Official Exchange Rate"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name CreateAssetRateInput

// AssetRate is a struct designed to encapsulate response payload data.
//
// swagger:model AssetRate
// @Description AssetRate is a struct designed to store asset rate data. Represents a complete asset rate entity containing conversion information between two assets, including all system-generated fields.
type AssetRate struct {
	// Unique identifier for the asset rate
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization that owns this asset rate
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Ledger containing this asset rate
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// External identifier for integration with third-party systems
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ExternalID string `json:"externalId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Source asset code
	// example: USD
	// minLength: 2
	// maxLength: 10
	From string `json:"from" example:"USD" minLength:"2" maxLength:"10"`

	// Target asset code
	// example: BRL
	// minLength: 2
	// maxLength: 10
	To string `json:"to" example:"BRL" minLength:"2" maxLength:"10"`

	// Conversion rate value
	// example: 100
	Rate float64 `json:"rate" example:"100"`

	// Decimal places for the rate
	// example: 2
	// minimum: 0
	Scale *float64 `json:"scale" example:"2" minimum:"0"`

	// Source of rate information
	// example: External System
	// maxLength: 200
	Source *string `json:"source" example:"External System" maxLength:"200"`

	// Time-to-live in seconds
	// example: 3600
	// minimum: 0
	TTL int `json:"ttl" example:"3600" minimum:"0"`

	// Timestamp when the asset rate was created
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the asset rate was last updated
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Additional custom attributes
	// example: {"provider": "Central Bank", "rateName": "Official Exchange Rate"}
	Metadata map[string]any `json:"metadata"`
} // @name AssetRate

// ToEntity converts an TransactionPostgreSQLModel to entity Transaction
func (a *AssetRatePostgreSQLModel) ToEntity() *AssetRate {
	assetRate := &AssetRate{
		ID:             a.ID,
		OrganizationID: a.OrganizationID,
		LedgerID:       a.LedgerID,
		ExternalID:     a.ExternalID,
		From:           a.From,
		To:             a.To,
		Rate:           a.Rate,
		Scale:          &a.RateScale,
		Source:         a.Source,
		TTL:            a.TTL,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
	}

	return assetRate
}

// FromEntity converts an entity AssetRate to AssetRatePostgreSQLModel
func (a *AssetRatePostgreSQLModel) FromEntity(assetRate *AssetRate) {
	*a = AssetRatePostgreSQLModel{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: assetRate.OrganizationID,
		LedgerID:       assetRate.LedgerID,
		ExternalID:     assetRate.ExternalID,
		From:           assetRate.From,
		To:             assetRate.To,
		Rate:           assetRate.Rate,
		RateScale:      *assetRate.Scale,
		Source:         assetRate.Source,
		TTL:            assetRate.TTL,
		CreatedAt:      assetRate.CreatedAt,
		UpdatedAt:      assetRate.UpdatedAt,
	}
}

// AssetRateResponse represents a success response containing a single asset rate.
//
// swagger:response AssetRateResponse
// @Description Successful response containing a single asset rate entity.
type AssetRateResponse struct {
	// in: body
	Body AssetRate
}

// AssetRatesResponse represents a success response containing a paginated list of asset rates.
//
// swagger:response AssetRatesResponse
// @Description Successful response containing a paginated list of asset rates.
type AssetRatesResponse struct {
	// in: body
	Body struct {
		Items      []AssetRate `json:"items"`
		Pagination struct {
			Limit      int     `json:"limit"`
			NextCursor *string `json:"next_cursor,omitempty"`
			PrevCursor *string `json:"prev_cursor,omitempty"`
		} `json:"pagination"`
	}
}
