package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Balance interface {
	Get(organizationID, ledgerID string, limit int, cursor, sortOrder, startDate, endDate string) (*mmodel.Balances, error)
	GetByID(organizationID, ledgerID, balanceID string) (*mmodel.Balance, error)
	GetByAccount(organizationID, ledgerID, accountID string, limit int, cursor, sortOrder, startDate, endDate string) (*mmodel.Balances, error)
	Delete(organizationID, ledgerID, balanceID string) error
}
