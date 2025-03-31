package assetrate

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"time"
)

// AssetRatePostgreSQLModel represents the entity AssetRatePostgreSQLModel into SQL context in Database
type AssetRatePostgreSQLModel struct {
	ID             string
	OrganizationID string
	LedgerID       string
	ExternalID     string
	From           string
	To             string
	Rate           float64
	RateScale      float64
	Source         *string
	TTL            int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Metadata       map[string]any
}

// CreateAssetRateInput is a struct design to encapsulate payload data.
//
// swagger:model CreateAssetRateInput
// @Description CreateAssetRateInput is the input payload to create an asset rate.
type CreateAssetRateInput struct {
	From       string         `json:"from" validate:"required" example:"USD"`
	To         string         `json:"to" validate:"required" example:"BRL"`
	Rate       int            `json:"rate" validate:"required" example:"100"`
	Scale      int            `json:"scale,omitempty" validate:"gte=0" example:"2"`
	Source     *string        `json:"source,omitempty" example:"External System"`
	TTL        *int           `json:"ttl,omitempty" example:"3600"`
	ExternalID *string        `json:"externalId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Metadata   map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name CreateAssetRateInput

// AssetRate is a struct designed to encapsulate response payload data.
//
// swagger:model AssetRate
// @Description AssetRate is a struct designed to store asset rate data.
type AssetRate struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	ExternalID     string         `json:"externalId" example:"00000000-0000-0000-0000-000000000000"`
	From           string         `json:"from" example:"USD"`
	To             string         `json:"to" example:"BRL"`
	Rate           float64        `json:"rate" example:"100"`
	Scale          *float64       `json:"scale" example:"2"`
	Source         *string        `json:"source" example:"External System"`
	TTL            int            `json:"ttl" example:"3600"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata"`
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
