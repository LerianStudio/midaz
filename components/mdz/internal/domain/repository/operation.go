package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Operation interface {
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Operations, error)
	GetByID(organizationID, ledgerID, operationID string) (*mmodel.Operation, error)
	GetByAccount(organizationID, ledgerID, accountID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Operations, error)
}
