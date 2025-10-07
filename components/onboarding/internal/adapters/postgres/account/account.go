// Package account provides the repository implementation for account entity persistence.
//
// This package implements the Repository pattern for the Account entity, providing
// PostgreSQL-based data access with support for hierarchical accounts, asset tracking,
// and portfolio/segment organization.
package account

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AccountPostgreSQLModel represents the PostgreSQL database model for accounts.
//
// This model maps to the "account" table and provides the database representation
// of account entities. Accounts are the fundamental units in the ledger system where
// balances are tracked.
//
// Key Features:
//   - Hierarchical structure (parent-child relationships via ParentAccountID)
//   - Asset-specific (each account holds one asset type)
//   - Optional portfolio and segment grouping
//   - Soft delete support (DeletedAt)
//   - Status tracking with description
//   - Unique alias for account identification
type AccountPostgreSQLModel struct {
	ID                string
	Name              string
	ParentAccountID   *string
	EntityID          *string
	AssetCode         string
	OrganizationID    string
	LedgerID          string
	PortfolioID       *string
	SegmentID         *string
	Status            string
	StatusDescription *string
	Alias             *string
	Type              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts a PostgreSQL model to a domain Account entity.
//
// This method transforms the database representation into the business logic representation,
// handling:
//   - Status decomposition (code + description → Status struct)
//   - DeletedAt conversion (sql.NullTime → *time.Time)
//   - All field mappings
//
// Returns:
//   - *mmodel.Account: Domain model with all fields populated
func (t *AccountPostgreSQLModel) ToEntity() *mmodel.Account {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	acc := &mmodel.Account{
		ID:              t.ID,
		Name:            t.Name,
		ParentAccountID: t.ParentAccountID,
		EntityID:        t.EntityID,
		AssetCode:       t.AssetCode,
		OrganizationID:  t.OrganizationID,
		LedgerID:        t.LedgerID,
		PortfolioID:     t.PortfolioID,
		SegmentID:       t.SegmentID,
		Status:          status,
		Alias:           t.Alias,
		Type:            t.Type,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		DeletedAt:       nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		acc.DeletedAt = &deletedAtCopy
	}

	return acc
}

// FromEntity converts a domain Account entity to a PostgreSQL model.
//
// This method transforms the business logic representation into the database representation,
// handling:
//   - UUID generation (UUIDv7 if ID not provided)
//   - Status composition (Status struct → code + description fields)
//   - DeletedAt conversion (*time.Time → sql.NullTime)
//   - Portfolio ID handling (only set if not nil/empty)
//
// Parameters:
//   - account: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*t) in place
//   - Generates new UUIDv7 if account.ID is empty
func (t *AccountPostgreSQLModel) FromEntity(account *mmodel.Account) {
	ID := libCommons.GenerateUUIDv7().String()
	if account.ID != "" {
		ID = account.ID
	}

	*t = AccountPostgreSQLModel{
		ID:                ID,
		Name:              account.Name,
		ParentAccountID:   account.ParentAccountID,
		EntityID:          account.EntityID,
		AssetCode:         account.AssetCode,
		OrganizationID:    account.OrganizationID,
		LedgerID:          account.LedgerID,
		SegmentID:         account.SegmentID,
		Status:            account.Status.Code,
		StatusDescription: account.Status.Description,
		Alias:             account.Alias,
		Type:              account.Type,
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}

	if !libCommons.IsNilOrEmpty(account.PortfolioID) {
		t.PortfolioID = account.PortfolioID
	}

	if account.DeletedAt != nil {
		deletedAtCopy := *account.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
