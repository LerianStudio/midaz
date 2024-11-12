package repository

import "github.com/LerianStudio/midaz/common/mmodel"

type Portfolio interface {
	Create(organizationID, ledgerID string, inp mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error)
	Get(organizationID, ledgerID string, limit, page int) (*mmodel.Portfolios, error)
}
