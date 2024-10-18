package asset

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
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

// CreateAssetInput is a struct design to encapsulate request create payload data.
type CreateAssetInput struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Code     string         `json:"code" validate:"required"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// UpdateAssetInput is a struct design to encapsulate request update payload data.
type UpdateAssetInput struct {
	Name     string         `json:"name"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// Asset is a struct designed to encapsulate payload data.
type Asset struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Code           string         `json:"code"`
	Status         Status         `json:"status"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code"`
	Description *string `json:"description"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// ToEntity converts an AssetPostgreSQLModel to entity response Asset
func (t *AssetPostgreSQLModel) ToEntity() *Asset {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	asset := &Asset{
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
func (t *AssetPostgreSQLModel) FromEntity(asset *Asset) {
	*t = AssetPostgreSQLModel{
		ID:                uuid.New().String(),
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
