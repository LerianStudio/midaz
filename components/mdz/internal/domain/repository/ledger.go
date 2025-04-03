package repository

import (
	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// \1 represents an entity
type Ledger interface {
	Create(organizationID string, inp mmodel.CreateLedgerInput) (*mmodel.Ledger, error)
	Get(organizationID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Ledgers, error)
	GetByID(organizationID, ledgerID string) (*mmodel.Ledger, error)
	Update(organizationID, ledgerID string, inp mmodel.UpdateLedgerInput) (*mmodel.Ledger, error)
	Delete(organizationID, ledgerID string) error
}
