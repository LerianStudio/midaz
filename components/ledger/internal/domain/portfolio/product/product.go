package product

import (
	"database/sql"
	"github.com/LerianStudio/midaz/common"
	"time"
)

// ProductPostgreSQLModel represents the entity Product into SQL context in Database
type ProductPostgreSQLModel struct {
	ID                string
	Name              string
	LedgerID          string
	OrganizationID    string
	Status            string
	StatusDescription *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         sql.NullTime
	Metadata          map[string]any
}

// CreateProductInput is a struct design to encapsulate request create payload data.
type CreateProductInput struct {
	Name     string         `json:"name" validate:"required,max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateProductInput is a struct design to encapsulate request update payload data.
type UpdateProductInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Product is a struct designed to encapsulate payload data.
type Product struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Status structure for marshaling/unmarshalling JSON.
type Status struct {
	Code        string  `json:"code" validate:"max=100"`
	Description *string `json:"description" validate:"max=256"`
}

// IsEmpty method that set empty or nil in fields
func (s Status) IsEmpty() bool {
	return s.Code == "" && s.Description == nil
}

// ToEntity converts an ProductPostgreSQLModel to entity.Product
func (t *ProductPostgreSQLModel) ToEntity() *Product {
	status := Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	product := &Product{
		ID:             t.ID,
		Name:           t.Name,
		LedgerID:       t.LedgerID,
		OrganizationID: t.OrganizationID,
		Status:         status,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		DeletedAt:      nil,
	}

	if !t.DeletedAt.Time.IsZero() {
		deletedAtCopy := t.DeletedAt.Time
		product.DeletedAt = &deletedAtCopy
	}

	return product
}

// FromEntity converts an entity.Product to ProductPostgreSQLModel
func (t *ProductPostgreSQLModel) FromEntity(product *Product) {
	*t = ProductPostgreSQLModel{
		ID:                common.GenerateUUIDv7().String(),
		Name:              product.Name,
		LedgerID:          product.LedgerID,
		OrganizationID:    product.OrganizationID,
		Status:            product.Status.Code,
		StatusDescription: product.Status.Description,
		CreatedAt:         product.CreatedAt,
		UpdatedAt:         product.UpdatedAt,
	}

	if product.DeletedAt != nil {
		deletedAtCopy := *product.DeletedAt
		t.DeletedAt = sql.NullTime{Time: deletedAtCopy, Valid: true}
	}
}
