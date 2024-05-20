package instrument

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// InstrumentPostgreSQLModel represents the entity Instrument into SQL context in Database
type InstrumentPostgreSQLModel struct {
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

// CreateInstrumentInput is a struct design to encapsulate request create payload data.
type CreateInstrumentInput struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Code     string         `json:"code"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// UpdateInstrumentInput is a struct design to encapsulate request update payload data.
type UpdateInstrumentInput struct {
	Name     string         `json:"name"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// Instrument is a struct designed to encapsulate payload data.
type Instrument struct {
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

// ToEntity converts an InstrumentPostgreSQLModel to entity response Instrument
func (t *InstrumentPostgreSQLModel) ToEntity() *Instrument {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	instrument := &Instrument{
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
		instrument.DeletedAt = &deletedAtCopy
	}

	return instrument
}

// FromEntity converts a request entity Instrument to InstrumentPostgreSQLModel
func (t *InstrumentPostgreSQLModel) FromEntity(instrument *Instrument) {
	*t = InstrumentPostgreSQLModel{
		ID:                uuid.New().String(),
		Name:              instrument.Name,
		Type:              instrument.Type,
		Code:              instrument.Code,
		Status:            instrument.Status.Code,
		StatusDescription: instrument.Status.Description,
		LedgerID:          instrument.LedgerID,
		OrganizationID:    instrument.OrganizationID,
		CreatedAt:         instrument.CreatedAt,
		UpdatedAt:         instrument.UpdatedAt,
	}

	if instrument.DeletedAt != nil {
		deletedAtCopy := *instrument.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
