package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Transaction interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateTransactionInput) (*mmodel.Transaction, error)
	CreateDSL(organizationID, ledgerID string, dslContent string) (*mmodel.Transaction, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Transactions, error)
	GetByID(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error)
	Revert(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error)
}
