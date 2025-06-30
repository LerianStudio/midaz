package settings

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// SettingsPostgreSQLModel represents the database model for settings
type SettingsPostgreSQLModel struct {
	ID             uuid.UUID    `db:"id"`
	OrganizationID uuid.UUID    `db:"organization_id"`
	LedgerID       uuid.UUID    `db:"ledger_id"`
	Key            string       `db:"key"`
	Value          string       `db:"value"`
	Description    string       `db:"description"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
	DeletedAt      sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *SettingsPostgreSQLModel) ToEntity() *mmodel.Settings {
	e := &mmodel.Settings{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Key:            m.Key,
		Value:          m.Value,
		Description:    m.Description,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// FromEntity converts a domain model to the database model
func (m *SettingsPostgreSQLModel) FromEntity(settings *mmodel.Settings) {
	m.ID = settings.ID
	m.OrganizationID = settings.OrganizationID
	m.LedgerID = settings.LedgerID
	m.Key = settings.Key
	m.Value = settings.Value
	m.Description = settings.Description
	m.CreatedAt = settings.CreatedAt
	m.UpdatedAt = settings.UpdatedAt

	if settings.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{
			Time:  *settings.DeletedAt,
			Valid: true,
		}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
