package mmodel

import (
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"time"
)

// CreateAccountInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAccountInput
// @Description CreateAccountInput is a struct design to encapsulate request create payload data.
type CreateAccountInput struct {
	AssetCode       string         `json:"assetCode" validate:"required,max=100" example:"BRL"`
	Name            string         `json:"name" validate:"max=256" example:"My Account"`
	Alias           *string        `json:"alias" validate:"max=100" example:"@person1"`
	Type            string         `json:"type" validate:"required" example:"creditCard"`
	ParentAccountID *string        `json:"parentAccountId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	ProductID       *string        `json:"productId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	PortfolioID     *string        `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000"`
	EntityID        *string        `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000"`
	Status          Status         `json:"status"`
	AllowSending    *bool          `json:"allowSending" example:"true"`
	AllowReceiving  *bool          `json:"allowReceiving" example:"true"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateAccountInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAccountInput
// @Description UpdateAccountInput is a struct design to encapsulate request update payload data.
type UpdateAccountInput struct {
	Name           string         `json:"name" validate:"max=256" example:"My Account Updated"`
	Status         Status         `json:"status"`
	AllowSending   *bool          `json:"allowSending" example:"true"`
	AllowReceiving *bool          `json:"allowReceiving" example:"true"`
	Alias          *string        `json:"alias" validate:"max=100" example:"@person1"`
	ProductID      *string        `json:"productId" validate:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Metadata       map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// Account is a struct designed to encapsulate response payload data.
//
// swagger:model Account
// @Description Account is a struct designed to encapsulate response payload data.
type Account struct {
	ID              string         `json:"id" example:"00000000-0000-0000-0000-000000000000"`
	Name            string         `json:"name" example:"My Account"`
	ParentAccountID *string        `json:"parentAccountId" example:"00000000-0000-0000-0000-000000000000"`
	EntityID        *string        `json:"entityId" example:"00000000-0000-0000-0000-000000000000"`
	AssetCode       string         `json:"assetCode" example:"BRL"`
	OrganizationID  string         `json:"organizationId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID        string         `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000"`
	PortfolioID     *string        `json:"portfolioId" example:"00000000-0000-0000-0000-000000000000"`
	ProductID       *string        `json:"productId" example:"00000000-0000-0000-0000-000000000000"`
	Balance         Balance        `json:"balance"`
	Status          Status         `json:"status"`
	AllowSending    *bool          `json:"allowSending" example:"true"`
	AllowReceiving  *bool          `json:"allowReceiving" example:"true"`
	Alias           *string        `json:"alias" example:"@person1"`
	Type            string         `json:"type" example:"creditCard"`
	CreatedAt       time.Time      `json:"createdAt" example:"2021-01-01T00:00:00Z"`
	UpdatedAt       time.Time      `json:"updatedAt" example:"2021-01-01T00:00:00Z"`
	DeletedAt       *time.Time     `json:"deletedAt" example:"2021-01-01T00:00:00Z"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance structure for marshaling/unmarshalling JSON.
type Balance struct {
	Available *float64 `json:"available" example:"1500"`
	OnHold    *float64 `json:"onHold" example:"500"`
	Scale     *float64 `json:"scale" example:"2"`
}

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// Accounts struct to return get all.
//
// swagger:model Accounts
// @Description Accounts struct to return get all.
type Accounts struct {
	Items []Account `json:"items"`
	Page  int       `json:"page" example:"1"`
	Limit int       `json:"limit" example:"10"`
}

// ToProto converts entity Account to a response protobuf proto
func (e *Account) ToProto() *proto.Account {
	status := proto.Status{
		Code: e.Status.Code,
	}

	if e.Status.Description != nil {
		status.Description = *e.Status.Description
	}

	balance := proto.Balance{
		Available: *e.Balance.Available,
		OnHold:    *e.Balance.OnHold,
		Scale:     *e.Balance.Scale,
	}

	account := &proto.Account{
		Id:             e.ID,
		Name:           e.Name,
		AssetCode:      e.AssetCode,
		OrganizationId: e.OrganizationID,
		LedgerId:       e.LedgerID,
		Balance:        &balance,
		Status:         &status,
		AllowSending:   *e.AllowSending,
		AllowReceiving: *e.AllowReceiving,
		Type:           e.Type,
	}

	if e.ParentAccountID != nil {
		account.ParentAccountId = *e.ParentAccountID
	}

	if e.DeletedAt != nil {
		account.DeletedAt = e.DeletedAt.String()
	}

	if !e.UpdatedAt.IsZero() {
		account.UpdatedAt = e.UpdatedAt.String()
	}

	if !e.CreatedAt.IsZero() {
		account.CreatedAt = e.CreatedAt.String()
	}

	if e.EntityID != nil {
		account.EntityId = *e.EntityID
	}

	if e.PortfolioID != nil {
		account.PortfolioId = *e.PortfolioID
	}

	if e.ProductID != nil {
		account.ProductId = *e.ProductID
	}

	if e.Alias != nil {
		account.Alias = *e.Alias
	}

	return account
}
