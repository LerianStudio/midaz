package repository

import "github.com/LerianStudio/midaz/components/mdz/internal/model"

type Ledger interface {
	Create(organizationID string, inp model.LedgerInput) (*model.LedgerCreate, error)
	Get(organizationID string, limit, page int) (*model.LedgerList, error)
}
