package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

// Transaction defines the interface for interacting with transaction data in the system
type Transaction interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateTransactionInput) (*mmodel.Transaction, error)
	CreateDSL(organizationID, ledgerID string, inp mmodel.CreateTransactionDSLInput) (*mmodel.Transaction, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Transactions, error)
	GetByID(organizationID, ledgerID, transactionID string) (*mmodel.Transaction, error)
	GetByParentID(organizationID, ledgerID, parentID string) (*mmodel.Transaction, error)
	ListByIDs(organizationID, ledgerID string, ids []string) ([]*mmodel.Transaction, error)
	Update(organizationID, ledgerID, transactionID string, inp mmodel.UpdateTransactionInput) (*mmodel.Transaction, error)
	Delete(organizationID, ledgerID, transactionID string) error
}