package mmodel

import "time"

// CreateAssetInput is a struct design to encapsulate request create payload data.
type CreateAssetInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Type     string         `json:"type"`
	Code     string         `json:"code" validate:"required,max=100"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateAssetInput is a struct design to encapsulate request update payload data.
type UpdateAssetInput struct {
	Name     string         `json:"name" validate:"max=256"`
	Status   Status         `json:"status"`
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Asset is a struct designed to encapsulate payload data.
type Asset struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Code           string         `json:"code"`
	Status         Status         `json:"status"`
	LedgerID       string         `json:"ledgerId"`
	OrganizationID string         `json:"organizationId"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      *time.Time     `json:"deletedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// Assets struct to return get all.
type Assets struct {
	Items []Asset `json:"items"`
	Page  int     `json:"page"`
	Limit int     `json:"limit"`
}
