// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Account repository interface.
package repository

import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

// Account defines the interface for account data operations.
//
// This interface abstracts account CRUD operations, allowing CLI commands
// to work with accounts without knowing the underlying HTTP implementation.
type Account interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAccountInput) (*mmodel.Account, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Accounts, error)
	GetByID(organizationID, ledgerID, accountID string) (*mmodel.Account, error)
	Update(organizationID, ledgerID, accountID string, inp mmodel.UpdateAccountInput) (*mmodel.Account, error)
	Delete(organizationID, ledgerID, accountID string) error
}
