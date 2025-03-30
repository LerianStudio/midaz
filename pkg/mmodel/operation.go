package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// Operation is a struct designed to encapsulate response payload data.
//
// swagger:model Operation
// @Description Operation is a struct designed to store operation data.
type Operation struct {
	ID            string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	AccountID     string         `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	Amount        int64          `json:"amount" example:"100"`
	Description   string         `json:"description" example:"Operation description"`
	Type          string         `json:"type" example:"credit"`
	Status        *Status        `json:"status"`
	Metadata      map[string]any `json:"metadata"`
	TransactionID string         `json:"transactionId" example:"00000000-0000-0000-0000-000000000000"`
	AssetCode     string         `json:"assetCode" example:"USD"`
	CreatedAt     time.Time      `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt     time.Time      `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
} // @name Operation

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (o *Operation) IDtoUUID() uuid.UUID {
	id, _ := uuid.Parse(o.ID)
	return id
}

// Operations struct to return get all.
//
// swagger:model Operations
// @Description Operations is the struct designed to return a list of operations with pagination.
type Operations struct {
	Items      []Operation `json:"items"`
	Pagination *Pagination `json:"pagination"`
} // @name Operations
