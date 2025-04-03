package mmodel

import (
	"github.com/google/uuid"
	"time"
)

// CreateAccountInput is a struct design to encapsulate request create payload data.
//
// swagger:model CreateAccountInput
//
// @Description CreateAccountInput is the input payload to create an account within a ledger, representing an individual financial entity like a bank account, credit card, or expense category.
type CreateAccountInput struct {
	// Name of the account (optional, max length 256 characters)
	Name string `json:"name" validate:"max=256" example:"My Account"`
	
	// ID of the parent account if this is a sub-account (optional, UUID format)
	ParentAccountID *string `json:"parentAccountId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Optional external entity identifier (max length 256 characters)
	EntityID *string `json:"entityId" validate:"omitempty,max=256" example:"00000000-0000-0000-0000-000000000000" maxLength:"256"`
	
	// Asset code that this account will use (required, max length 100 characters)
	AssetCode string `json:"assetCode" validate:"required,max=100" example:"BRL" maxLength:"100"`
	
	// ID of the portfolio this account belongs to (optional, UUID format)
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the segment this account belongs to (optional, UUID format)
	SegmentID *string `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Status of the account (active, inactive, pending)
	Status Status `json:"status"`
	
	// Unique alias for the account (optional, max length 100 characters, must follow alias format rules)
	Alias *string `json:"alias" validate:"omitempty,max=100,prohibitedexternalaccountprefix" example:"@person1" maxLength:"100"`
	
	// Type of the account (e.g., checking, savings, creditCard, expense)
	Type string `json:"type" validate:"required" example:"creditCard"`
	
	// Additional custom attributes for the account
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAccountInput

// UpdateAccountInput is a struct design to encapsulate request update payload data.
//
// swagger:model UpdateAccountInput
//
// @Description UpdateAccountInput is the input payload to update an existing account's properties such as name, status, portfolio, segment, and metadata.
type UpdateAccountInput struct {
	// Updated name of the account (optional, max length 256 characters)
	Name string `json:"name" validate:"max=256" example:"My Account Updated" maxLength:"256"`
	
	// Updated segment ID for the account (optional, UUID format)
	SegmentID *string `json:"segmentId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Updated portfolio ID for the account (optional, UUID format)
	PortfolioID *string `json:"portfolioId" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Updated status of the account (active, inactive, pending)
	Status Status `json:"status"`
	
	// Updated or additional custom attributes for the account
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateAccountInput

// Account is a struct designed to encapsulate response payload data.
//
// swagger:model Account
//
// @Description Account represents an individual financial entity within a ledger, such as a bank account, credit card, or expense category.
type Account struct {
	// Unique identifier for the account (UUID format)
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Name of the account (max length 256 characters)
	Name string `json:"name" example:"My Account" maxLength:"256"`
	
	// ID of the parent account if this is a sub-account (UUID format)
	ParentAccountID *string `json:"parentAccountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Optional external entity identifier (max length 256 characters)
	EntityID *string `json:"entityId" example:"00000000-0000-0000-0000-000000000000" maxLength:"256"`
	
	// Asset code associated with this account (max length 100 characters)
	AssetCode string `json:"assetCode" example:"BRL" maxLength:"100"`
	
	// ID of the organization that owns this account (UUID format)
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the ledger this account belongs to (UUID format)
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the portfolio this account belongs to (UUID format)
	PortfolioID *string `json:"portfolioId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// ID of the segment this account belongs to (UUID format)
	SegmentID *string `json:"segmentId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
	
	// Status of the account (active, inactive, pending)
	Status Status `json:"status"`
	
	// Unique alias for the account (max length 100 characters)
	Alias *string `json:"alias" example:"@person1" maxLength:"100"`
	
	// Type of the account (e.g., checking, savings, creditCard, expense)
	Type string `json:"type" example:"creditCard"`
	
	// Timestamp when the account was created
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the account was last updated
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Timestamp when the account was deleted (null if not deleted)
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	
	// Additional custom attributes for the account
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Account

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (a *Account) IDtoUUID() uuid.UUID {
	return uuid.MustParse(a.ID)
}

// Accounts struct to return get all.
//
// swagger:model Accounts
//
// @Description Accounts represents a paginated collection of account records returned by list operations.
type Accounts struct {
	// Array of account records
	Items []Account `json:"items"`
	
	// Current page number
	Page int `json:"page" example:"1" minimum:"1"`
	
	// Maximum number of items per page
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Accounts
