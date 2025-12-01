package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// AccountType is a struct designed to store Account Type object data.
//
// swagger:model AccountType
// @Description Complete account type entity containing all fields including system-generated fields. Account types define the classification categories for accounts within a ledger, such as assets, liabilities, equity, revenue, or expenses.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "name": "Current Assets",
//	  "description": "Assets that are expected to be converted to cash within one year",
//	  "keyValue": "current_assets",
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "category": "Assets",
//	    "subcategory": "Current"
//	  }
//	}
type AccountType struct {
	// Unique identifier for the account type (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID uuid.UUID `json:"id,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the organization that owns this account type (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID uuid.UUID `json:"organizationId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the ledger this account type belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID uuid.UUID `json:"ledgerId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Human-readable name of the account type
	// example: Current Assets
	// maxLength: 100
	Name string `json:"name,omitempty" example:"Current Assets" maxLength:"100"`

	// Detailed description of the account type purpose and usage
	// example: Assets that are expected to be converted to cash within one year
	// maxLength: 500
	Description string `json:"description,omitempty" example:"Assets that are expected to be converted to cash within one year" maxLength:"500"`

	// Unique key identifier for the account type (used in code references)
	// example: current_assets
	// maxLength: 50
	KeyValue string `json:"keyValue,omitempty" example:"current_assets" maxLength:"50"`

	// Timestamp when the account type was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the account type was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the account type was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the account type information
	// example: {"category": "Assets", "subcategory": "Current"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name AccountType

// CreateAccountTypeInput is a struct designed to store Account Type input data.
//
// swagger:model CreateAccountTypeInput
// @Description Request payload for creating a new account type. Account types define the classification categories for accounts within a ledger.
//
//	@example {
//	  "name": "Current Assets",
//	  "description": "Assets that are expected to be converted to cash within one year",
//	  "keyValue": "current_assets",
//	  "metadata": {
//	    "category": "Assets",
//	    "subcategory": "Current"
//	  }
//	}
type CreateAccountTypeInput struct {
	// Human-readable name of the account type
	// required: true
	// example: Current Assets
	// maxLength: 100
	Name string `json:"name" validate:"required,max=100" example:"Current Assets" maxLength:"100"`

	// Detailed description of the account type purpose and usage
	// required: false
	// example: Assets that are expected to be converted to cash within one year
	// maxLength: 500
	Description string `json:"description,omitempty" validate:"max=500" example:"Assets that are expected to be converted to cash within one year" maxLength:"500"`

	// Unique key identifier for the account type (used in code references, no spaces allowed)
	// required: true
	// example: current_assets
	// maxLength: 50
	KeyValue string `json:"keyValue" validate:"required,max=50,invalidaccounttype" example:"current_assets" maxLength:"50"`

	// Custom key-value pairs for extending the account type information
	// required: false
	// example: {"category": "Assets", "subcategory": "Current"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
} // @name CreateAccountTypeInput

// UpdateAccountTypeInput is a struct designed to store Account Type input data.
//
// swagger:model UpdateAccountTypeInput
// @Description Request payload for updating an existing account type. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "name": "Updated Current Assets",
//	  "description": "Updated description for current assets",
//	  "metadata": {
//	    "category": "Assets",
//	    "subcategory": "Current",
//	    "updated": true
//	  }
//	}
type UpdateAccountTypeInput struct {
	// Updated human-readable name of the account type
	// required: false
	// example: Updated Current Assets
	// maxLength: 100
	Name string `json:"name,omitempty" validate:"max=100" example:"Updated Current Assets" maxLength:"100"`

	// Updated detailed description of the account type
	// required: false
	// example: Updated description for current assets
	// maxLength: 500
	Description string `json:"description,omitempty" validate:"max=500" example:"Updated description for current assets" maxLength:"500"`

	// Updated custom key-value pairs for extending the account type information
	// required: false
	// example: {"category": "Assets", "subcategory": "Current", "updated": true}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
} // @name UpdateAccountTypeInput
