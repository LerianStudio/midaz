package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// AccountType is a struct designed to store Account Type object data.
//
// swagger:model AccountType
// @Description AccountType object
type AccountType struct {
	// The unique identifier of the Account Type.
	ID uuid.UUID `json:"id,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	// The unique identifier of the Organization.
	OrganizationID uuid.UUID `json:"organizationId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	// The unique identifier of the Ledger.
	LedgerID uuid.UUID `json:"ledgerId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	// The name of the account type.
	Name string `json:"name,omitempty" example:"Current Assets"`
	// Detailed description of the account type.
	Description string `json:"description,omitempty" example:"Assets that are expected to be converted to cash within one year"`
	// A unique key value identifier for the account type.
	KeyValue string `json:"keyValue,omitempty" example:"current_assets"`
	// The timestamp when the account type was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the account type was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the account type was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// Custom key-value pairs for extending the account type information
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name AccountType

// CreateAccountTypeInput is a struct designed to store Account Type input data.
//
// swagger:model CreateAccountTypeInput
// @Description CreateAccountTypeInput payload
type CreateAccountTypeInput struct {
	// The name of the account type.
	Name string `json:"name" validate:"required,max=100" example:"Current Assets"`
	// Detailed description of the account type.
	Description string `json:"description,omitempty" validate:"max=500" example:"Assets that are expected to be converted to cash within one year"`
	// A unique key value identifier for the account type.
	KeyValue string `json:"keyValue" validate:"required,max=50,invalidaccounttype" example:"current_assets"`
	// Custom key-value pairs for extending the account type information
	// required: false
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAccountTypeInput

// UpdateAccountTypeInput is a struct designed to store Account Type input data.
//
// swagger:model UpdateAccountTypeInput
// @Description UpdateAccountTypeInput payload
type UpdateAccountTypeInput struct {
	// The name of the account type.
	Name string `json:"name,omitempty" validate:"max=100" example:"Current Assets"`
	// Detailed description of the account type.
	Description string `json:"description,omitempty" validate:"max=500" example:"Assets that are expected to be converted to cash within one year"`
	// Custom key-value pairs for extending the account type information
	// required: false
	// example: {"department": "Treasury", "purpose": "Operating Expenses", "region": "Global"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateAccountTypeInput
