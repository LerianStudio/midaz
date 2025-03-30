package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// CreateTransactionInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateTransactionInput
// @Description CreateTransactionInput is the input payload to create a transaction.
type CreateTransactionInput struct {
	Description             string         `json:"description" validate:"required,max=256" example:"Transaction description"`
	Entries                 []EntryInput   `json:"entries" validate:"required,min=1,dive"`
	Metadata                map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
	Template                string         `json:"template,omitempty" example:"transfer"`
	Amount                  *int64         `json:"amount,omitempty" example:"100"`
	AmountScale             *int64         `json:"amountScale,omitempty" example:"2"`
	AssetCode               string         `json:"assetCode,omitempty" example:"USD"`
	ChartOfAccountsGroupName string        `json:"chartOfAccountsGroupName,omitempty" example:"group1"`
	Source                  []string       `json:"source,omitempty" example:"[\"00000000-0000-0000-0000-000000000000\"]"`
	Destination             []string       `json:"destination,omitempty" example:"[\"00000000-0000-0000-0000-000000000000\"]"`
	ParentTransactionID     *string        `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Status                  *Status        `json:"status,omitempty"`
} // @name CreateTransactionInput

// EntryInput is a struct design to encapsulate entry data for transaction creation.
//
// swagger:model EntryInput
// @Description EntryInput is the input payload for a transaction entry.
type EntryInput struct {
	AccountID string `json:"accountId" validate:"required,uuid" example:"00000000-0000-0000-0000-000000000000"`
	Amount    int64  `json:"amount" validate:"required" example:"100"`
} // @name EntryInput

// Transaction is a struct designed to encapsulate response payload data.
//
// swagger:model Transaction
// @Description Transaction is a struct designed to store transaction data.
type Transaction struct {
	ID                      string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Description             string         `json:"description" example:"Transaction description"`
	Status                  *Status        `json:"status"`
	Entries                 []Entry        `json:"entries"`
	Metadata                map[string]any `json:"metadata"`
	Template                string         `json:"template,omitempty" example:"transfer"`
	Amount                  *int64         `json:"amount,omitempty" example:"100"`
	AmountScale             *int64         `json:"amountScale,omitempty" example:"2"`
	AssetCode               string         `json:"assetCode,omitempty" example:"USD"`
	ChartOfAccountsGroupName string        `json:"chartOfAccountsGroupName,omitempty" example:"group1"`
	Source                  []string       `json:"source,omitempty" example:"[\"00000000-0000-0000-0000-000000000000\"]"`
	Destination             []string       `json:"destination,omitempty" example:"[\"00000000-0000-0000-0000-000000000000\"]"`
	ParentTransactionID     *string        `json:"parentTransactionId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Operations              []Operation    `json:"operations,omitempty"`
	CreatedAt               time.Time      `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt               time.Time      `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
	DeletedAt               *time.Time     `json:"deletedAt,omitempty" example:"2023-01-01T00:00:00Z"`
} // @name Transaction

// Entry is a struct designed to encapsulate transaction entry data.
//
// swagger:model Entry
// @Description Entry is a struct designed to store transaction entry data.
type Entry struct {
	ID        string    `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	AccountID string    `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	Amount    int64     `json:"amount" example:"100"`
	CreatedAt time.Time `json:"createdAt" example:"2023-01-01T00:00:00Z"`
	UpdatedAt time.Time `json:"updatedAt" example:"2023-01-01T00:00:00Z"`
} // @name Entry

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (t *Transaction) IDtoUUID() uuid.UUID {
	id, _ := uuid.Parse(t.ID)
	return id
}

// Transactions struct to return get all.
//
// swagger:model Transactions
// @Description Transactions is the struct designed to return a list of transactions with pagination.
type Transactions struct {
	Items      []Transaction `json:"items"`
	Pagination *Pagination   `json:"pagination"`
} // @name Transactions
