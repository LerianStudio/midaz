package repository

import "github.com/LerianStudio/midaz/common/mmodel"

type Product interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateProductInput) (*mmodel.Product, error)
	Get(organizationID, ledgerID string, limit, page int) (*mmodel.Products, error)
}
