package mmodel

import (
	"time"
)

// CreateAssetRateInput is a struct design to encapsulate payload data.
type CreateAssetRateInput struct {
	From       string         `json:"from" validate:"required" example:"USD"`
	To         string         `json:"to" validate:"required" example:"BRL"`
	Rate       int            `json:"rate" validate:"required" example:"100"`
	Scale      int            `json:"scale,omitempty" validate:"gte=0" example:"2"`
	Source     *string        `json:"source,omitempty" example:"External System"`
	TTL        *int           `json:"ttl,omitempty" example:"3600"`
	ExternalID *string        `json:"externalId,omitempty" example:"00000000-0000-0000-0000-000000000000"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// AssetRate is a struct designed to encapsulate response payload data.
type AssetRate struct {
	ID             string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID       string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	ExternalID     string         `json:"externalId" example:"00000000-0000-0000-0000-000000000000"`
	From           string         `json:"from" example:"USD"`
	To             string         `json:"to" example:"BRL"`
	Rate           float64        `json:"rate" example:"100"`
	Scale          *float64       `json:"scale" example:"2"`
	Source         *string        `json:"source" example:"External System"`
	TTL            int            `json:"ttl" example:"3600"`
	CreatedAt      time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt      time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	Metadata       map[string]any `json:"metadata"`
}

// AssetRates struct to return get all.
type AssetRates struct {
	Items []AssetRate `json:"items"`
	Page  int         `json:"page" example:"1"`
	Limit int         `json:"limit" example:"10"`
}