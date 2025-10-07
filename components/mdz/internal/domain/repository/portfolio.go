// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Portfolio repository interface.
package repository

import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

// Portfolio defines the interface for portfolio data operations.
//
// This interface abstracts portfolio CRUD operations, allowing CLI commands
// to work with portfolios without knowing the underlying HTTP implementation.
type Portfolio interface {
	Create(organizationID, ledgerID string, inp mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Portfolios, error)
	GetByID(organizationID, ledgerID, portfolioID string) (*mmodel.Portfolio, error)
	Update(organizationID, ledgerID, portfolioID string, inp mmodel.UpdatePortfolioInput) (*mmodel.Portfolio, error)
	Delete(organizationID, ledgerID, portfolioID string) error
}
