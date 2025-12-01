// Package portfolio provides PostgreSQL adapter implementations for portfolio entity persistence.
//
// This package implements the infrastructure layer for portfolio storage in PostgreSQL,
// following the hexagonal architecture pattern. Portfolios group accounts that belong
// to a specific entity (customer, department, etc.) within a ledger.
//
// Architecture Overview:
//
// The portfolio adapter provides:
//   - Full CRUD operations for portfolio entities
//   - Organization and ledger scoped queries
//   - Entity-based lookups for account grouping
//   - Soft delete support with audit timestamps
//   - Batch operations for efficient bulk lookups
//
// Domain Concepts:
//
// A Portfolio in the ledger system:
//   - Groups accounts belonging to a specific entity
//   - Belongs to a ledger within an organization
//   - Has an EntityID linking to external entity (customer, holder)
//   - Enables aggregated balance views across accounts
//   - Supports logical separation of account ownership
//
// Data Flow:
//
//	Domain Entity (mmodel.Portfolio) → PortfolioPostgreSQLModel → PostgreSQL
//	PostgreSQL → PortfolioPostgreSQLModel → Domain Entity (mmodel.Portfolio)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package portfolio

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// PortfolioPostgreSQLModel represents the portfolio entity in PostgreSQL.
//
// This model maps directly to the 'portfolio' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE portfolio (
//	    id UUID PRIMARY KEY,
//	    name VARCHAR(255) NOT NULL,
//	    entity_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL REFERENCES ledger(id),
//	    organization_id UUID NOT NULL REFERENCES organization(id),
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Thread Safety:
//
// PortfolioPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance.
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

// ToEntity converts a PortfolioPostgreSQLModel to the domain Portfolio model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Returns:
//   - *mmodel.Portfolio: Domain model with all fields mapped
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

// FromEntity converts a domain Portfolio model to PortfolioPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Parameters:
//   - portfolio: Domain Portfolio model to convert
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
