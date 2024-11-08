package repository

import "github.com/LerianStudio/midaz/common/mmodel"

type Asset interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAssetInput) (*mmodel.Asset, error)
	Get(organizationID, ledgerID string, limit, page int) (*mmodel.Assets, error)
	GetByID(organizationID, ledgerID, assetID string) (*mmodel.Asset, error)
	Update(organizationID, ledgerID, assetID string, inp mmodel.UpdateAssetInput) (*mmodel.Asset, error)
}
