package ledger

import (
	"database/sql"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
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

// ToEntity converts an LedgerPostgreSQLModel to entity.Ledger
func (t *LedgerPostgreSQLModel) ToEntity() *mmodel.Ledger {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	ledger := &mmodel.Ledger{
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
func (t *LedgerPostgreSQLModel) FromEntity(ledger *mmodel.Ledger) {
	*t = LedgerPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
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
