package portfolio

import (
	"database/sql"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"time"
)

// PortfolioPostgreSQLModel represents the entity Portfolio into SQL context in Database
type PortfolioPostgreSQLModel struct {
	ID                string
	Name              string
	EntityID          string
	LedgerID          string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// ToEntity converts an PortfolioPostgreSQLModel to entity.Portfolio
func (t *PortfolioPostgreSQLModel) ToEntity() *mmodel.Portfolio {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	portfolio := &mmodel.Portfolio{
		ID:             t.ID,
		Name:           t.Name,
		EntityID:       t.EntityID,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		portfolio.DeletedAt = &deletedAtCopy
	}

	return portfolio
}

// FromEntity converts an entity.Portfolio to PortfolioPostgreSQLModel
func (t *PortfolioPostgreSQLModel) FromEntity(portfolio *mmodel.Portfolio) {
	*t = PortfolioPostgreSQLModel{
		ID:                libCommons.GenerateUUIDv7().String(),
		Name:              portfolio.Name,
		EntityID:          portfolio.EntityID,
		LedgerID:          portfolio.LedgerID,
		OrganizationID:    portfolio.OrganizationID,
		Status:            portfolio.Status.Code,
		StatusDescription: portfolio.Status.Description,
		CreatedAt:         portfolio.CreatedAt,
		UpdatedAt:         portfolio.UpdatedAt,
	}

	if portfolio.DeletedAt != nil {
		deletedAtCopy := *portfolio.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
