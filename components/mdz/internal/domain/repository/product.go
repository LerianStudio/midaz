package repository

import "github.com/LerianStudio/midaz/pkg/mmodel"

type Product interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateProductInput) (*mmodel.Product, error)
	Get(organizationID, ledgerID string, limit, page int) (*mmodel.Products, error)
	GetByID(organizationID, ledgerID, productID string) (*mmodel.Product, error)
	Update(organizationID, ledgerID, productID string, inp mmodel.UpdateProductInput) (*mmodel.Product, error)
	Delete(organizationID, ledgerID, productID string) error
}
