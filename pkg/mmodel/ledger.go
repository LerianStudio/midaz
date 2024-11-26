package mmodel

import "time"

// CreateLedgerInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateLedgerInput
// @Description CreateLedgerInput is the input payload to create a ledger.
type CreateLedgerInput struct {
	Name     string         `json:"name" validate:"required,max=256" example:"Lerian Studio"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateLedgerInput

// UpdateLedgerInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateLedgerInput
// @Description UpdateLedgerInput is the input payload to update a ledger.
type UpdateLedgerInput struct {
	Name     string         `json:"name" validate:"max=256" example:"Lerian Studio Updated"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateLedgerInput

// Ledger is a struct designed to encapsulate payload data.
//
// swagger:model Ledger
// @Description Ledger is a struct designed to store ledger data.
type Ledger struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name           string         `json:"name" example:"Lerian Studio"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt      *time.Time     `json:"deletedAt" sql:"index" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata,omitempty"`
} // @name Ledger

// Ledgers struct to return get all.
//
// swagger:model Ledgers
// @Description Ledgers is the struct designed to return a list of ledgers with pagination.
type Ledgers struct {
	Items []Ledger `json:"items"`
	Page  int      `json:"page" example:"1"`
	Limit int      `json:"limit" example:"10"`
} // @name Ledgers
