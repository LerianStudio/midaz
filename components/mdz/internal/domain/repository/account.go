package repository

import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

type Account interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAccountInput) (*mmodel.Account, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Accounts, error)
	GetByID(organizationID, ledgerID, accountID string) (*mmodel.Account, error)
	Update(organizationID, ledgerID, accountID string, inp mmodel.UpdateAccountInput) (*mmodel.Account, error)
	Delete(organizationID, ledgerID, accountID string) error
}
