package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

// Balance defines the interface for interacting with balance data in the system
type Balance interface {
	Create(organizationID, ledgerID, accountID string, inp mmodel.CreateBalanceInput) (*mmodel.Balance, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Balances, error)
	GetByID(organizationID, ledgerID, balanceID string) (*mmodel.Balance, error)
	GetByAccount(organizationID, ledgerID, accountID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Balances, error)
	ListByAccountIDs(organizationID, ledgerID string, accountIDs []string) ([]*mmodel.Balance, error)
	ListByAliases(organizationID, ledgerID string, aliases []string) ([]*mmodel.Balance, error)
	Update(organizationID, ledgerID, balanceID string, inp mmodel.UpdateBalance) (*mmodel.Balance, error)
	Delete(organizationID, ledgerID, balanceID string) error
}