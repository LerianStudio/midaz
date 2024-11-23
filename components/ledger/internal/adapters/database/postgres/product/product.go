package product

import (
	"database/sql"
	"time"
	
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
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

// ToEntity converts an ProductPostgreSQLModel to entity.Product
func (t *ProductPostgreSQLModel) ToEntity() *mmodel.Product {
	status := mmodel.Status{
		Code:        t.Status,
		Description: t.StatusDescription,
	}

	product := &mmodel.Product{
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
func (t *ProductPostgreSQLModel) FromEntity(product *mmodel.Product) {
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
