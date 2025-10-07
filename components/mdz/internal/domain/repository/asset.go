// Package repository defines repository interfaces for the MDZ CLI domain layer.
// This file contains the Asset repository interface.
package repository

import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

// Asset defines the interface for asset data operations.
//
// This interface abstracts asset CRUD operations, allowing CLI commands
// to work with assets without knowing the underlying HTTP implementation.
type Asset interface {
	Create(organizationID, ledgerID string, inp mmodel.CreateAssetInput) (*mmodel.Asset, error)
	Get(organizationID, ledgerID string, limit, page int, SortOrder, StartDate, EndDate string) (*mmodel.Assets, error)
	GetByID(organizationID, ledgerID, assetID string) (*mmodel.Asset, error)
	Update(organizationID, ledgerID, assetID string, inp mmodel.UpdateAssetInput) (*mmodel.Asset, error)
	Delete(organizationID, ledgerID, assetID string) error
}
