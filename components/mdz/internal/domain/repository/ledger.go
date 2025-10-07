// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Ledger repository interface.
package repository

import (
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// Ledger defines the interface for ledger data operations.
//
// This interface abstracts ledger CRUD operations, allowing CLI commands
// to work with ledgers without knowing the underlying HTTP implementation.
type Ledger interface {
	Create(organizationID string, inp mmodel.CreateLedgerInput) (*mmodel.Ledger, error)
	Get(organizationID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Ledgers, error)
	GetByID(organizationID, ledgerID string) (*mmodel.Ledger, error)
	Update(organizationID, ledgerID string, inp mmodel.UpdateLedgerInput) (*mmodel.Ledger, error)
	Delete(organizationID, ledgerID string) error
}
