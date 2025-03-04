package model

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

// AssetRate represents the API model for an asset rate
type AssetRate struct {
	ID             string                 `json:"id"`
	OrganizationID string                 `json:"organizationId"`
	LedgerID       string                 `json:"ledgerId"`
	ExternalID     string                 `json:"externalId"`
	From           string                 `json:"from"`
	To             string                 `json:"to"`
	Rate           float64                `json:"rate"`
	Scale          *float64               `json:"scale,omitempty"`
	Source         *string                `json:"source,omitempty"`
	TTL            int                    `json:"ttl"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// AsAssetRate converts a mmodel.AssetRate to an API AssetRate
func AsAssetRate(assetRate *mmodel.AssetRate) *AssetRate {
	if assetRate == nil {
		return nil
	}

	return &AssetRate{
		ID:             assetRate.ID,
		OrganizationID: assetRate.OrganizationID,
		LedgerID:       assetRate.LedgerID,
		ExternalID:     assetRate.ExternalID,
		From:           assetRate.From,
		To:             assetRate.To,
		Rate:           assetRate.Rate,
		Scale:          assetRate.Scale,
		Source:         assetRate.Source,
		TTL:            assetRate.TTL,
		CreatedAt:      assetRate.CreatedAt,
		UpdatedAt:      assetRate.UpdatedAt,
		Metadata:       assetRate.Metadata,
	}
}