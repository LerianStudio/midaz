package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

// AssetRate defines the interface for interacting with asset rate data in the system
type AssetRate interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error)
	GetByExternalID(organizationID, ledgerID, externalID string) (*mmodel.AssetRate, error)
	GetByAssetCode(organizationID, ledgerID, assetCode string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.AssetRates, error)
}