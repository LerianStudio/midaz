package asset

import (
	"database/sql"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
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
