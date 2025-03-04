package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

// Operation defines the interface for interacting with operation data in the system
type Operation interface {
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Operations, error)
	GetByID(organizationID, ledgerID, operationID string) (*mmodel.Operation, error)
	GetByAccount(organizationID, ledgerID, accountID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Operations, error)
	GetByAccountAndID(organizationID, ledgerID, accountID, operationID string) (*mmodel.Operation, error)
	GetByTransaction(organizationID, ledgerID, transactionID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Operations, error)
	ListByIDs(organizationID, ledgerID string, ids []string) ([]*mmodel.Operation, error)
	Update(organizationID, ledgerID, transactionID, operationID string, inp mmodel.UpdateOperationInput) (*mmodel.Operation, error)
}
