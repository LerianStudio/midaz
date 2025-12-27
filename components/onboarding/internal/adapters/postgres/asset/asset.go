// Package asset provides PostgreSQL database adapters for asset entity
// persistence and retrieval operations.
package asset

import (
	"database/sql"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// AssetPostgreSQLModel represents the entity Asset into SQL context in Database
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

// ToEntity converts an AssetPostgreSQLModel to entity response Asset
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

// FromEntity converts a request entity Asset to AssetPostgreSQLModel
func (t *AssetPostgreSQLModel) FromEntity(asset *mmodel.Asset) {
	ID := libCommons.GenerateUUIDv7().String()
	if asset.ID != "" {
		ID = asset.ID
	}

	assert.That(assert.ValidUUID(ID),
		"resolved asset ID must be valid UUID",
		"asset_code", asset.Code,
		"has_existing_id", asset.ID != "")

	*t = AssetPostgreSQLModel{
		ID:                ID,
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
