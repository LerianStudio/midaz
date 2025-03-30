package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type AssetRate interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error)
	Update(organizationID, ledgerID, assetRateID string, inp mmodel.UpdateAssetRateInput) (*mmodel.AssetRate, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.AssetRates, error)
	GetByID(organizationID, ledgerID, assetRateID string) (*mmodel.AssetRate, error)
	GetByAssetCode(organizationID, ledgerID, assetCode string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.AssetRates, error)
}
