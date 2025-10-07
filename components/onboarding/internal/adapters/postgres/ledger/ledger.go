// Package ledger provides the repository implementation for ledger entity persistence.
//
// This package implements the Repository pattern for the Ledger entity, providing
// PostgreSQL-based data access. Ledgers are the top-level containers within an
// organization that group assets, accounts, and transactions.
package ledger

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// LedgerPostgreSQLModel represents the PostgreSQL database model for ledgers.
//
// This model maps to the "ledger" table and provides the database representation
// of ledger entities. Ledgers organize all financial data within an organization.
//
// Key Features:
//   - Organization scoping (one ledger belongs to one organization)
//   - Status tracking with description
//   - Soft delete support (DeletedAt)
//   - Unique name within organization
type LedgerPostgreSQLModel struct {
	ID                string
	Name              string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts a PostgreSQL model to a domain Ledger entity.
//
// Transforms database representation to business logic representation,
// handling status decomposition and DeletedAt conversion.
//
// Returns:
//   - *mmodel.Ledger: Domain model with all fields populated
func (t *LedgerPostgreSQLModel) ToEntity() *mmodel.Ledger {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	ledger := &mmodel.Ledger{
		ID:             t.ID,
		Name:           t.Name,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		ledger.DeletedAt = &deletedAtCopy
	}

	return ledger
}

// FromEntity converts a domain Ledger entity to a PostgreSQL model.
//
// Transforms business logic representation to database representation,
// handling UUID generation, status composition, and DeletedAt conversion.
//
// Parameters:
//   - ledger: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*t) in place
//   - Generates new UUIDv7 for ID field
func (t *LedgerPostgreSQLModel) FromEntity(ledger *mmodel.Ledger) {
	*t = LedgerPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              ledger.Name,
		OrganizationID:    ledger.OrganizationID,
		Status:            ledger.Status.Code,
		StatusDescription: ledger.Status.Description,
		CreatedAt:         ledger.CreatedAt,
		UpdatedAt:         ledger.UpdatedAt,
	}

	if ledger.DeletedAt != nil {
		deletedAtCopy := *ledger.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
