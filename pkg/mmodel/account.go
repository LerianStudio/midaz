package mmodel

import (
	"github.com/google/uuid"
	"time"

	proto "github.com/LerianStudio/midaz/pkg/mgrpc/account"
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

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance is the struct designed to represent the account balance.
type Balance struct {
	Available *float64 `json:"available" example:"1500"`
	OnHold    *float64 `json:"onHold" example:"500"`
	Scale     *float64 `json:"scale" example:"2"`
} // @name Balance

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
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

// ToProto converts entity Account to a response protobuf proto
func (a *Account) ToProto() *proto.Account {
	status := proto.Status{
		Code: a.Status.Code,
	}

	if a.Status.Description != nil {
		status.Description = *a.Status.Description
	}

	account := &proto.Account{
		Id:             a.ID,
		Name:           a.Name,
		AssetCode:      a.AssetCode,
		OrganizationId: a.OrganizationID,
		LedgerId:       a.LedgerID,
		Status:         &status,
		Type:           a.Type,
	}

	if a.ParentAccountID != nil {
		account.ParentAccountId = *a.ParentAccountID
	}

	if a.DeletedAt != nil {
		account.DeletedAt = a.DeletedAt.String()
	}

	if !a.UpdatedAt.IsZero() {
		account.UpdatedAt = a.UpdatedAt.String()
	}

	if !a.CreatedAt.IsZero() {
		account.CreatedAt = a.CreatedAt.String()
	}

	if a.EntityID != nil {
		account.EntityId = *a.EntityID
	}

	if a.PortfolioID != nil {
		account.PortfolioId = *a.PortfolioID
	}

	if a.SegmentID != nil {
		account.SegmentId = *a.SegmentID
	}

	if a.Alias != nil {
		account.Alias = *a.Alias
	}

	return account
}
