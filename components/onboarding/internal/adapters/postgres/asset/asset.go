// Package asset provides PostgreSQL adapter implementations for asset entity persistence.
//
// This package implements the infrastructure layer for asset storage in PostgreSQL,
// following the hexagonal architecture pattern. Assets define the types of value
// that can be held in accounts (currencies, commodities, securities, etc.).
//
// Architecture Overview:
//
// The asset adapter provides:
//   - Full CRUD operations for asset entities
//   - Organization and ledger scoped queries
//   - Name and code uniqueness validation
//   - Soft delete support with audit timestamps
//   - Batch operations for efficient bulk lookups
//
// Domain Concepts:
//
// An Asset in the ledger system:
//   - Represents a type of value (USD, BTC, stock shares, etc.)
//   - Belongs to a ledger within an organization
//   - Has a unique code within the ledger (e.g., "USD", "BRL")
//   - Has a type classification (currency, crypto, commodity, etc.)
//   - Used to define what accounts hold balances in
//
// Data Flow:
//
//	Domain Entity (mmodel.Asset) → AssetPostgreSQLModel → PostgreSQL
//	PostgreSQL → AssetPostgreSQLModel → Domain Entity (mmodel.Asset)
//
// Related Packages:
//   - github.com/LerianStudio/midaz/v3/pkg/mmodel: Domain model definitions
//   - github.com/LerianStudio/lib-commons/v2/commons/postgres: PostgreSQL connection management
package asset

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AssetPostgreSQLModel represents the asset entity in PostgreSQL.
//
// This model maps directly to the 'asset' table with proper SQL types.
// It serves as the persistence layer representation, separate from the
// domain model to maintain hexagonal architecture boundaries.
//
// Table Schema:
//
//	CREATE TABLE asset (
//	    id UUID PRIMARY KEY,
//	    name VARCHAR(255) NOT NULL,
//	    type VARCHAR(50) NOT NULL,
//	    code VARCHAR(10) NOT NULL,
//	    status VARCHAR(50) NOT NULL,
//	    status_description TEXT,
//	    ledger_id UUID NOT NULL REFERENCES ledger(id),
//	    organization_id UUID NOT NULL REFERENCES organization(id),
//	    created_at TIMESTAMP WITH TIME ZONE,
//	    updated_at TIMESTAMP WITH TIME ZONE,
//	    deleted_at TIMESTAMP WITH TIME ZONE,
//	    UNIQUE(organization_id, ledger_id, code)
//	);
//
// Thread Safety:
//
// AssetPostgreSQLModel is not thread-safe. Each goroutine should work with
// its own instance.
type AssetPostgreSQLModel struct {
	ID                string
	Name              string
	Type              string
	Code              string
	Status            string
	StatusDescription *string
	LedgerID          string
	OrganizationID    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts an AssetPostgreSQLModel to the domain Asset model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Returns:
//   - *mmodel.Asset: Domain model with all fields mapped
func (t *AssetPostgreSQLModel) ToEntity() *mmodel.Asset {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	asset := &mmodel.Asset{
		ID:             t.ID,
		Name:           t.Name,
		Type:           t.Type,
		Code:           t.Code,
		Status:         status,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		asset.DeletedAt = &deletedAtCopy
	}

	return asset
}

// FromEntity converts a domain Asset model to AssetPostgreSQLModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Parameters:
//   - asset: Domain Asset model to convert
func (t *AssetPostgreSQLModel) FromEntity(asset *mmodel.Asset) {
	*t = AssetPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              asset.Name,
		Type:              asset.Type,
		Code:              asset.Code,
		Status:            asset.Status.Code,
		StatusDescription: asset.Status.Description,
		LedgerID:          asset.LedgerID,
		OrganizationID:    asset.OrganizationID,
		CreatedAt:         asset.CreatedAt,
		UpdatedAt:         asset.UpdatedAt,
	}

	if asset.DeletedAt != nil {
		deletedAtCopy := *asset.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
