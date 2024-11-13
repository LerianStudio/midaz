package mmodel

import (
	proto "github.com/LerianStudio/midaz/common/mgrpc/account"
	"time"
)

// CreateAccountInput is a struct design to encapsulate request create payload data.
type CreateAccountInput struct {
	AssetCode       string         `json:"assetCode" validate:"required,max=100"`
	Name            string         `json:"name" validate:"max=256"`
	Alias           *string        `json:"alias" validate:"max=100"`
	Type            string         `json:"type" validate:"required"`
	ParentAccountID *string        `json:"parentAccountId" validate:"omitempty,uuid"`
	ProductID       *string        `json:"productId" validate:"omitempty,uuid"`
	PortfolioID     *string        `json:"portfolioId" validate:"omitempty,uuid"`
	EntityID        *string        `json:"entityId" validate:"omitempty,max=256"`
	Status          Status         `json:"status"`
	AllowSending    *bool          `json:"allowSending"`
	AllowReceiving  *bool          `json:"allowReceiving"`
	Metadata        map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// UpdateAccountInput is a struct design to encapsulate request update payload data.
type UpdateAccountInput struct {
	Name           string         `json:"name" validate:"max=256"`
	Status         Status         `json:"status"`
	AllowSending   *bool          `json:"allowSending"`
	AllowReceiving *bool          `json:"allowReceiving"`
	Alias          *string        `json:"alias" validate:"max=100"`
	ProductID      *string        `json:"productId" validate:"uuid"`
	Metadata       map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}

// SearchAccountsInput is a struct design to encapsulate request search payload data.
type SearchAccountsInput struct {
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid"`
}

// Account is a struct designed to encapsulate response payload data.
type Account struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	ParentAccountID *string        `json:"parentAccountId"`
	EntityID        *string        `json:"entityId"`
	AssetCode       string         `json:"assetCode"`
	OrganizationID  string         `json:"organizationId"`
	LedgerID        string         `json:"ledgerId"`
	PortfolioID     *string        `json:"portfolioId"`
	ProductID       *string        `json:"productId"`
	Balance         Balance        `json:"balance"`
	Status          Status         `json:"status"`
	AllowSending    *bool          `json:"allowSending"`
	AllowReceiving  *bool          `json:"allowReceiving"`
	Alias           *string        `json:"alias"`
	Type            string         `json:"type"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       *time.Time     `json:"deletedAt"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Balance structure for marshaling/unmarshalling JSON.
type Balance struct {
	Available *float64 `json:"available"`
	OnHold    *float64 `json:"onHold"`
	Scale     *float64 `json:"scale"`
}

// IsEmpty method that set empty or nil in fields
func (b Balance) IsEmpty() bool {
	return b.Available == nil && b.OnHold == nil && b.Scale == nil
}

// Accounts struct to return get all.
type Accounts struct {
	Items []Account `json:"items"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
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
