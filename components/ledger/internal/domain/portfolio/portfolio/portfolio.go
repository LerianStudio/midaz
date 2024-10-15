package portfolio

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
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
	AllowSending      bool
	AllowReceiving    bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// CreatePortfolioInput is a struct design to encapsulate request create payload data.
type CreatePortfolioInput struct {
	EntityID string         `json:"entityId"`
	Name     string         `json:"name" validate:"max=100"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// UpdatePortfolioInput is a struct design to encapsulate payload data.
type UpdatePortfolioInput struct {
	Name     string         `json:"name" validate:"max=100"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata"`
}

// Portfolio is a struct designed to encapsulate request update payload data.
type Portfolio struct {
	ID             string         `json:"id"`
	Name           string         `json:"name" validate:"max=100"`
	EntityID       string         `json:"entityId"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code           string  `json:"code" validate:"max=100"`
	Description    *string `json:"description" validate:"max=100"`
	AllowSending   bool    `json:"allowSending"`
	AllowReceiving bool    `json:"allowReceiving"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// ToEntity converts an PortfolioPostgreSQLModel to entity.Portfolio
func (t *PortfolioPostgreSQLModel) ToEntity() *Portfolio {
	status := Status{
		Code:           t.Status,
		Description:    t.StatusDescription,
		AllowSending:   t.AllowSending,
		AllowReceiving: t.AllowReceiving,
	}

	portfolio := &Portfolio{
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
func (t *PortfolioPostgreSQLModel) FromEntity(portfolio *Portfolio) {
	*t = PortfolioPostgreSQLModel{
		ID:                uuid.New().String(),
		Name:              portfolio.Name,
		EntityID:          portfolio.EntityID,
		LedgerID:          portfolio.LedgerID,
		OrganizationID:    portfolio.OrganizationID,
		Status:            portfolio.Status.Code,
		StatusDescription: portfolio.Status.Description,
		AllowSending:      portfolio.Status.AllowSending,
		AllowReceiving:    portfolio.Status.AllowReceiving,
		CreatedAt:         portfolio.CreatedAt,
		UpdatedAt:         portfolio.UpdatedAt,
	}

	if portfolio.DeletedAt != nil {
		deletedAtCopy := *portfolio.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
