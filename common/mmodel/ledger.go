package mmodel

import "time"

// CreateLedgerInput is a struct design to encapsulate request create payload data.
type CreateLedgerInput struct {
	Name     string         `json:"name" validate:"required,max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateLedgerInput is a struct design to encapsulate request update payload data.
type UpdateLedgerInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Ledger is a struct designed to encapsulate payload data.
type Ledger struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	OrganizationID string         `json:"organizationId"`
	Status         Status         `json:"status"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt" sql:"index"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Ledgers struct {
	Items []Ledger `json:"items"`
	Page  int      `json:"page"`
	Limit int      `json:"limit"`
}
