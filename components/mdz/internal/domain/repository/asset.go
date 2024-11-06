package repository

import "github.com/LerianStudio/midaz/common/mmodel"

type Asset interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAssetInput) (*mmodel.Asset, error)
}
