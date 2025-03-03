package mmodel

import (
	"time"
)

// OperationStatus structure for marshaling/unmarshalling JSON.
type OperationStatus struct {
	Code        string  `json:"code" validate:"max=100" example:"ACTIVE"`
	Description *string `json:"description" validate:"omitempty,max=256" example:"Active status"`
}

// Amount structure for marshaling/unmarshalling JSON.
type Amount struct {
	Amount *int64 `json:"amount" example:"1500"`
	Scale  *int64 `json:"scale" example:"2"`
}

// Balance structure for marshaling/unmarshalling JSON.
type BalanceOperation struct {
	Available *int64 `json:"available" example:"1500"`
	OnHold    *int64 `json:"onHold" example:"500"`
	Scale     *int64 `json:"scale" example:"2"`
}

// Operation is a struct designed to encapsulate response payload data.
type Operation struct {
	ID              string            `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	TransactionID   string            `json:"transactionId" example:"00000000-0000-0000-0000-000000000000"`
	Description     string            `json:"description" example:"Credit card operation"`
	Type            string            `json:"type" example:"creditCard"`
	AssetCode       string            `json:"assetCode" example:"BRL"`
	ChartOfAccounts string            `json:"chartOfAccounts" example:"1000"`
	Amount          Amount            `json:"amount"`
	Balance         BalanceOperation  `json:"balance"`
	BalanceAfter    BalanceOperation  `json:"balanceAfter"`
	Status          OperationStatus   `json:"status"`
	AccountID       string            `json:"accountId" example:"00000000-0000-0000-0000-000000000000"`
	AccountAlias    string            `json:"accountAlias" example:"@person1"`
	BalanceID       string            `json:"balanceId" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID  string            `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID        string            `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	CreatedAt       time.Time         `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt       time.Time         `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt       *time.Time        `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata        map[string]any    `json:"metadata"`
}

// Operations struct to return get all.
type Operations struct {
	Items []Operation `json:"items"`
	Page  int         `json:"page" example:"1"`
	Limit int         `json:"limit" example:"10"`
}

// UpdateOperationInput is a struct design to encapsulate payload data.
type UpdateOperationInput struct {
	Description string         `json:"description" validate:"max=256" example:"Credit card operation"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}