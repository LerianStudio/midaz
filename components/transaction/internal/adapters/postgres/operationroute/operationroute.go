package operationroute

import (
	"database/sql"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// OperationRoutePostgreSQLModel represents the database model for operation routes
type OperationRoutePostgreSQLModel struct {
	ID             uuid.UUID    `db:"id"`
	OrganizationID uuid.UUID    `db:"organization_id"`
	LedgerID       uuid.UUID    `db:"ledger_id"`
	Title          string       `db:"title"`
	Description    string       `db:"description"`
	Type           string       `db:"type"`
	AccountTypes   string       `db:"account_types"`
	AccountAlias   string       `db:"account_alias"`
	CreatedAt      time.Time    `db:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at"`
	DeletedAt      sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *OperationRoutePostgreSQLModel) ToEntity() *mmodel.OperationRoute {
	if m == nil {
		return nil
	}

	var accountTypes []string
	if m.AccountTypes != "" {
		accountTypes = strings.Split(m.AccountTypes, ";")
	}

	e := &mmodel.OperationRoute{
		ID:             m.ID,
		OrganizationID: m.OrganizationID,
		LedgerID:       m.LedgerID,
		Title:          m.Title,
		Description:    m.Description,
		Type:           m.Type,
		AccountTypes:   accountTypes,
		AccountAlias:   m.AccountAlias,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}

	if m.DeletedAt.Valid {
		e.DeletedAt = &m.DeletedAt.Time
	}

	return e
}

// FromEntity converts a domain model to the database model
func (m *OperationRoutePostgreSQLModel) FromEntity(e *mmodel.OperationRoute) {
	if e == nil {
		return
	}

	m.ID = e.ID
	m.OrganizationID = e.OrganizationID
	m.LedgerID = e.LedgerID
	m.Title = e.Title
	m.Description = e.Description
	m.Type = strings.ToLower(e.Type)

	if e.AccountTypes != nil {
		accountTypes := make([]string, len(e.AccountTypes))
		for i, accountType := range e.AccountTypes {
			accountTypes[i] = strings.ToLower(accountType)
		}

		m.AccountTypes = strings.Join(accountTypes, ";")
	}

	m.AccountAlias = e.AccountAlias
	m.CreatedAt = e.CreatedAt
	m.UpdatedAt = e.UpdatedAt

	if e.DeletedAt != nil {
		m.DeletedAt = sql.NullTime{
			Time:  *e.DeletedAt,
			Valid: true,
		}
	} else {
		m.DeletedAt = sql.NullTime{}
	}
}
