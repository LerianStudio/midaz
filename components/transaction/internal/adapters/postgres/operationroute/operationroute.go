package operationroute

import (
	"database/sql"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

// OperationRoutePostgreSQLModel represents the database model for operation routes
type OperationRoutePostgreSQLModel struct {
	ID          uuid.UUID    `db:"id"`
	Title       string       `db:"title"`
	Description string       `db:"description"`
	Type        string       `db:"type"`
	CreatedAt   time.Time    `db:"created_at"`
	UpdatedAt   time.Time    `db:"updated_at"`
	DeletedAt   sql.NullTime `db:"deleted_at"`
}

// ToEntity converts the database model to a domain model
func (m *OperationRoutePostgreSQLModel) ToEntity() *mmodel.OperationRoute {
	if m == nil {
		return nil
	}

	e := &mmodel.OperationRoute{
		ID:          m.ID,
		Title:       m.Title,
		Description: m.Description,
		Type:        m.Type,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
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
	m.Title = e.Title
	m.Description = e.Description
	m.Type = e.Type
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
