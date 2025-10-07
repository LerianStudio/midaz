// Package portfolio provides the repository implementation for portfolio entity persistence.
//
// This package implements the Repository pattern for the Portfolio entity, providing
// PostgreSQL-based data access. Portfolios group related accounts for organizational
// and reporting purposes.
package portfolio

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// PortfolioPostgreSQLModel represents the PostgreSQL database model for portfolios.
//
// Portfolios provide logical grouping of accounts within a ledger for organizational
// purposes (e.g., grouping accounts by department, project, or customer).
type PortfolioPostgreSQLModel struct {
	ID                string
	Name              string
	EntityID          string
	LedgerID          string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts a PostgreSQL model to a domain Portfolio entity.
func (t *PortfolioPostgreSQLModel) ToEntity() *mmodel.Portfolio {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	portfolio := &mmodel.Portfolio{
		ID:             t.ID,
		Name:           t.Name,
		EntityID:       t.EntityID,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		portfolio.DeletedAt = &deletedAtCopy
	}

	return portfolio
}

// FromEntity converts a domain Portfolio entity to a PostgreSQL model.
func (t *PortfolioPostgreSQLModel) FromEntity(portfolio *mmodel.Portfolio) {
	*t = PortfolioPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              portfolio.Name,
		EntityID:          portfolio.EntityID,
		LedgerID:          portfolio.LedgerID,
		OrganizationID:    portfolio.OrganizationID,
		Status:            portfolio.Status.Code,
		StatusDescription: portfolio.Status.Description,
		CreatedAt:         portfolio.CreatedAt,
		UpdatedAt:         portfolio.UpdatedAt,
	}

	if portfolio.DeletedAt != nil {
		deletedAtCopy := *portfolio.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
