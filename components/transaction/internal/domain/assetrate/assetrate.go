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
type CreateAssetRateInput struct {
	BaseAssetCode    string         `json:"baseAssetCode"`
	CounterAssetCode string         `json:"counterAssetCode"`
	Amount           float64        `json:"amount"`
	Scale            float64        `json:"scale"`
	Source           string         `json:"source"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// AssetRate is a struct designed to encapsulate response payload data.
type AssetRate struct {
	ID               string         `json:"id"`
	BaseAssetCode    string         `json:"baseAssetCode"`
	CounterAssetCode string         `json:"counterAssetCode"`
	Amount           float64        `json:"amount"`
	Scale            float64        `json:"scale"`
	Source           string         `json:"source"`
	OrganizationID   string         `json:"organizationId"`
	LedgerID         string         `json:"ledgerId"`
	CreatedAt        time.Time      `json:"createdAt"`
	Metadata         map[string]any `json:"metadata"`
}

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
