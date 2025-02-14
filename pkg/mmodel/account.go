package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// CreateAccountInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAccountInput
// @Description CreateAccountInput is the input payload to create an account.
type CreateAccountInput struct {
	Name            string         `json:"name" validate:"max=256" example:"My Account"`
	ParentAccountID *string        `json:"parentAccountId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	EntityID        *string        `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	AssetCode       string         `json:"assetCode" validate:"required,max=100" example:"BRL"`
	PortfolioID     *string        `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	SegmentID       *string        `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	Status          Status         `json:"status"`
	Alias           *string        `json:"alias" validate:"required,max=100,prohibitedexternalaccountprefix" example:"@person1"`
	Type            string         `json:"type" validate:"required" example:"creditCard"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAccountInput

// UpdateAccountInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAccountInput
// @Description UpdateAccountInput is the input payload to update an account.
type UpdateAccountInput struct {
	Name        string         `json:"name" validate:"max=256" example:"My Account Updated"`
	SegmentID   *string        `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	PortfolioID *string        `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	Status      Status         `json:"status"`
	Metadata    map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name UpdateAccountInput

// Account is a struct designed to encapsulate response payload data.
//
// swagger:model Account
// @Description Account is a struct designed to store account data.
type Account struct {
	ID              string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name            string         `json:"name" example:"My Account"`
	ParentAccountID *string        `json:"parentAccountId" example:"00000000-0000-0000-0000-000000000000"`
	EntityID        *string        `json:"entityId" example:"00000000-0000-0000-0000-000000000000"`
	AssetCode       string         `json:"assetCode" example:"BRL"`
	OrganizationID  string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID        string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	PortfolioID     *string        `json:"portfolioId" example:"00000000-0000-0000-0000-000000000000"`
	SegmentID       *string        `json:"segmentId" example:"00000000-0000-0000-0000-000000000000"`
	Status          Status         `json:"status"`
	Alias           *string        `json:"alias" example:"@person1"`
	Type            string         `json:"type" example:"creditCard"`
	CreatedAt       time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt       time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt       *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata        map[string]any `json:"metadata,omitempty"`
} // @name Account

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (a *Account) IDtoUUID() uuid.UUID {
	return uuid.MustParse(a.ID)
}

// Accounts struct to return get all.
//
// swagger:model Accounts
// @Description Accounts is the struct designed to return a list of accounts with pagination.
type Accounts struct {
	Items []Account `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
} // @name Accounts
