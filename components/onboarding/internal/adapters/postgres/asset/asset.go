// Package asset provides the repository implementation for asset entity persistence.
//
// This package implements the Repository pattern for the Asset entity, providing
// PostgreSQL-based data access. Assets represent currencies, cryptocurrencies,
// commodities, or other value types tracked in the ledger.
package asset

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AssetPostgreSQLModel represents the PostgreSQL database model for assets.
//
// This model maps to the "asset" table and provides the database representation
// of asset entities. Assets define what types of value can be held in accounts.
//
// Key Features:
//   - Type classification (currency, crypto, commodities, others)
//   - Unique code within ledger (e.g., USD, BTC, GOLD)
//   - Status tracking with description
//   - Soft delete support (DeletedAt)
//   - Automatic external account creation
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

// ToEntity converts a PostgreSQL model to a domain Asset entity.
//
// Transforms database representation to business logic representation,
// handling status decomposition and DeletedAt conversion.
//
// Returns:
//   - *mmodel.Asset: Domain model with all fields populated
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

// FromEntity converts a domain Asset entity to a PostgreSQL model.
//
// Transforms business logic representation to database representation,
// handling UUID generation, status composition, and DeletedAt conversion.
//
// Parameters:
//   - asset: Domain model to convert
//
// Side Effects:
//   - Modifies the receiver (*t) in place
//   - Generates new UUIDv7 for ID field
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
