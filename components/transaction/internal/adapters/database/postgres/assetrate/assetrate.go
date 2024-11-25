package assetrate

import (
	"github.com/LerianStudio/midaz/common"
	"time"
)

// AssetRatePostgreSQLModel represents the entity AssetRatePostgreSQLModel into SQL context in Database
type AssetRatePostgreSQLModel struct {
	ID               string
	BaseAssetCode    string
	CounterAssetCode string
	Amount           float64
	Scale            float64
	Source           string
	OrganizationID   string
	LedgerID         string
	CreatedAt        time.Time
	Metadata         map[string]any
}

// CreateAssetRateInput is a struct design to encapsulate payload data.
//
// swagger:model CreateAssetRateInput
// @Description CreateAssetRateInput is a struct design to encapsulate payload data.
type CreateAssetRateInput struct {
	BaseAssetCode    string         `json:"baseAssetCode" example:"BRL"`
	CounterAssetCode string         `json:"counterAssetCode" example:"USD"`
	Amount           float64        `json:"amount" example:"5000"`
	Scale            float64        `json:"scale" example:"2"`
	Source           string         `json:"source" example:"@person1"`
	Metadata         map[string]any `json:"metadata,omitempty"`
} // @name CreateAssetRateInput

// AssetRate is a struct designed to encapsulate response payload data.
//
// swagger:model AssetRate

type AssetRate struct {
	ID               string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	BaseAssetCode    string         `json:"baseAssetCode" example:"BRL"`
	CounterAssetCode string         `json:"counterAssetCode" example:"USD"`
	Amount           float64        `json:"amount" example:"5000"`
	Scale            float64        `json:"scale" example:"2"`
	Source           string         `json:"source" example:"@person1"`
	OrganizationID   string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID         string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt        time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	Metadata         map[string]any `json:"metadata"`
} // @name AssetRate

// ToEntity converts an TransactionPostgreSQLModel to entity Transaction
func (a *AssetRatePostgreSQLModel) ToEntity() *AssetRate {
	assetRate := &AssetRate{
		ID:               a.ID,
		BaseAssetCode:    a.BaseAssetCode,
		CounterAssetCode: a.CounterAssetCode,
		Amount:           a.Amount,
		Scale:            a.Scale,
		Source:           a.Source,
		OrganizationID:   a.OrganizationID,
		LedgerID:         a.LedgerID,
		CreatedAt:        a.CreatedAt,
	}

	return assetRate
}

// FromEntity converts an entity AssetRate to AssetRatePostgreSQLModel
func (a *AssetRatePostgreSQLModel) FromEntity(assetRate *AssetRate) {
	*a = AssetRatePostgreSQLModel{
		ID:               common.GenerateUUIDv7().String(),
		BaseAssetCode:    assetRate.BaseAssetCode,
		CounterAssetCode: assetRate.CounterAssetCode,
		Amount:           assetRate.Amount,
		Scale:            assetRate.Scale,
		Source:           assetRate.Source,
		OrganizationID:   assetRate.OrganizationID,
		LedgerID:         assetRate.LedgerID,
		CreatedAt:        assetRate.CreatedAt,
	}
}
