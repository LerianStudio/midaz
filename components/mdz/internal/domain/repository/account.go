package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Account interface {
	Create(organizationID, ledgerID, portfolioID string, inp mmodel.CreateAccountInput) (*mmodel.Account, error)
	Get(organizationID, ledgerID, portfolioID string, limit, page int) (*mmodel.Accounts, error)
	GetByID(organizationID, ledgerID, portfolioID, accountID string) (*mmodel.Account, error)
	Update(organizationID, ledgerID, portfolioID, accountID string, inp mmodel.UpdateAccountInput) (*mmodel.Account, error)
	Delete(organizationID, ledgerID, portfolioID, accountID string) error
}
