package repository

import "github.com/LerianStudio/midaz/components/mdz/internal/model"

type Ledger interface {
	Create(organizationID string, inp model.LedgerInput) (*model.LedgerCreate, error)
}
