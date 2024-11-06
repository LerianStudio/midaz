package ledger

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/common"
)

// LedgerPostgreSQLModel represents the entity.Ledger into SQL context in Database
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

// CreateLedgerInput is a struct design to encapsulate request create payload data.
type CreateLedgerInput struct {
	Name     string         `json:"name" validate:"required,max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateLedgerInput is a struct design to encapsulate request update payload data.
type UpdateLedgerInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Ledger is a struct designed to encapsulate payload data.
type Ledger struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt" sql:"index"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"omitempty,max=256"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// ToEntity converts an LedgerPostgreSQLModel to entity.Ledger
func (t *LedgerPostgreSQLModel) ToEntity() *Ledger {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	ledger := &Ledger{
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

// FromEntity converts an entity.Ledger to LedgerPostgreSQLModel
func (t *LedgerPostgreSQLModel) FromEntity(ledger *Ledger) {
	*t = LedgerPostgreSQLModel{
		ID:                common.GenerateUUIDv7().String(),
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
