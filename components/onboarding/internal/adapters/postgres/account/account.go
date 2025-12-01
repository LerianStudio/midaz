// Package account provides PostgreSQL adapter implementations for account entity persistence.
//
// This package implements the infrastructure layer for account storage in PostgreSQL,
// following the hexagonal architecture pattern. Accounts are fundamental entities in
// the ledger system, representing balance-holding containers for assets.
//
// Architecture Overview:
//
// The account adapter provides:
//   - Full CRUD operations for account entities
//   - Multi-tenant scoping (organization, ledger, portfolio)
//   - Soft delete support with audit timestamps
//   - Alias-based lookups for human-readable account identification
//   - Batch operations for transaction processing
//
// Domain Concepts:
//
// An Account in the ledger system:
//   - Holds balances for a specific asset
//   - Belongs to a ledger within an organization
//   - May optionally belong to a portfolio
//   - Can have parent-child relationships (chart of accounts)
//   - Supports blocking for compliance/risk management
//
// Data Flow:
//
//	Domain Entity (mmodel.Account) → AccountPostgreSQLModel → PostgreSQL
//	PostgreSQL → AccountPostgreSQLModel → Domain Entity (mmodel.Account)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package account

import (
	"database/sql"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AccountPostgreSQLModel represents the account entity in PostgreSQL.
//
// This model maps directly to the 'account' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE account (
//	    id UUID PRIMARY KEY,
//	    name VARCHAR(255) NOT NULL,
//	    parent_account_id UUID REFERENCES account(id),
//	    entity_id UUID,
//	    asset_code VARCHAR(10) NOT NULL,
//	    organization_id UUID NOT NULL,
//	    ledger_id UUID NOT NULL,
//	    portfolio_id UUID,
//	    segment_id UUID,
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    alias VARCHAR(255) UNIQUE,
//	    type VARCHAR(50) NOT NULL,
//	    blocked BOOLEAN DEFAULT FALSE,
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE
//	);
//
// Indexing Strategy:
//
// Key indexes for performance:
//   - (organization_id, ledger_id, id): Primary lookup path
//   - (organization_id, ledger_id, alias): Alias lookups
//   - (organization_id, ledger_id, asset_code): Asset filtering
//   - (deleted_at): Soft delete filtering
//
// Thread Safety:
//
// AccountPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance. The repository handles concurrent access at the database level.
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
	Blocked           bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts an AccountPostgreSQLModel to the domain Account model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Convert status fields to Status value object
//  2. Map all direct fields
//  3. Handle nullable DeletedAt for soft delete support
//
// Returns:
//   - *mmodel.Account: Domain model with all fields mapped
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
		Blocked:         &t.Blocked,
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

// FromEntity converts a domain Account model to AccountPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Generate UUID v7 if ID not provided (new account)
//  2. Map all direct fields with type conversions
//  3. Normalize type to lowercase for consistency
//  4. Handle optional fields (Blocked, PortfolioID, DeletedAt)
//
// Parameters:
//   - account: Domain Account model to convert
//
// ID Generation:
//
// Uses UUID v7 which provides:
//   - Time-ordered IDs for index efficiency
//   - Globally unique identifiers
//   - Sortable by creation time
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
		Type:              strings.ToLower(account.Type),
		CreatedAt:         account.CreatedAt,
		UpdatedAt:         account.UpdatedAt,
	}

	if account.Blocked != nil {
		t.Blocked = *account.Blocked
	}

	if !libCommons.IsNilOrEmpty(account.PortfolioID) {
		t.PortfolioID = account.PortfolioID
	}

	if account.DeletedAt != nil {
		deletedAtCopy := *account.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
