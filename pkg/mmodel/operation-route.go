package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// OperationRoute is a struct designed to store Operation Route object data.
//
// swagger:model OperationRoute
// @Description Complete operation route entity containing all fields including system-generated fields. Operation routes define the rules for routing transactions to specific accounts based on account type or alias matching. They are building blocks for transaction routes.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "title": "Cash-in from service charge",
//	  "description": "This operation route handles cash-in transactions from service charge collections",
//	  "code": "EXT-001",
//	  "operationType": "source",
//	  "account": {
//	    "ruleType": "alias",
//	    "validIf": "@cash_account"
//	  },
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "department": "Treasury",
//	    "category": "Cash Management"
//	  }
//	}
type OperationRoute struct {
	// Unique identifier for the operation route (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID uuid.UUID `json:"id,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the organization that owns this operation route (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID uuid.UUID `json:"organizationId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// ID of the ledger this operation route belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID uuid.UUID `json:"ledgerId,omitempty" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`

	// Short text summarizing the purpose of the operation route
	// example: Cash-in from service charge
	// maxLength: 50
	Title string `json:"title,omitempty" example:"Cash-in from service charge" maxLength:"50"`

	// Detailed description of the operation route purpose and usage
	// example: This operation route handles cash-in transactions from service charge collections
	// maxLength: 250
	Description string `json:"description,omitempty" example:"This operation route handles cash-in transactions from service charge collections" maxLength:"250"`

	// External reference code for the operation route
	// example: EXT-001
	// maxLength: 100
	Code string `json:"code,omitempty" example:"EXT-001" maxLength:"100"`

	// The type of the operation route (source for debits, destination for credits)
	// example: source
	// enum: source,destination
	OperationType string `json:"operationType,omitempty" example:"source" enum:"source,destination"`

	// Custom key-value pairs for extending the operation route information
	// example: {"department": "Treasury", "category": "Cash Management"}
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Account selection rule configuration that determines which accounts are valid for this route
	Account *AccountRule `json:"account,omitempty"`

	// Timestamp when the operation route was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the operation route was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the operation route was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name OperationRoute

// CreateOperationRouteInput is a struct designed to store Operation Route input data.
//
// swagger:model CreateOperationRouteInput
// @Description Request payload for creating a new operation route. Operation routes define how transactions are routed to accounts based on rules. They specify whether an account acts as source (debit) or destination (credit) in a transaction.
//
//	@example {
//	  "title": "Cash-in from service charge",
//	  "description": "This operation route handles cash-in transactions from service charge collections",
//	  "code": "EXT-001",
//	  "operationType": "source",
//	  "account": {
//	    "ruleType": "alias",
//	    "validIf": "@cash_account"
//	  },
//	  "metadata": {
//	    "department": "Treasury",
//	    "category": "Cash Management"
//	  }
//	}
type CreateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation route
	// required: true
	// example: Cash-in from service charge
	// maxLength: 50
	Title string `json:"title,omitempty" validate:"required,max=50" example:"Cash-in from service charge" maxLength:"50"`

	// Detailed description of the operation route purpose and usage
	// required: false
	// example: This operation route handles cash-in transactions from service charge collections
	// maxLength: 250
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections" maxLength:"250"`

	// External reference code for the operation route
	// required: false
	// example: EXT-001
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-001" maxLength:"100"`

	// The type of the operation route (source for debits, destination for credits)
	// required: true
	// example: source
	// enum: source,destination
	OperationType string `json:"operationType,omitempty" validate:"required" example:"source" enum:"source,destination"`

	// Custom key-value pairs for extending the operation route information
	// required: false
	// example: {"department": "Treasury", "category": "Cash Management"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Account selection rule configuration that determines which accounts are valid for this route
	// required: false
	Account *AccountRule `json:"account,omitempty"`
} // @name CreateOperationRouteInput

// UpdateOperationRouteInput is a struct designed to store Operation Route input data.
//
// swagger:model UpdateOperationRouteInput
// @Description Request payload for updating an existing operation route. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "title": "Updated cash-in route",
//	  "description": "Updated description for the operation route",
//	  "code": "EXT-002",
//	  "metadata": {
//	    "department": "Global Treasury",
//	    "category": "Cash Management",
//	    "updated": true
//	  }
//	}
type UpdateOperationRouteInput struct {
	// Updated short text summarizing the purpose of the operation route
	// required: false
	// example: Updated cash-in route
	// maxLength: 50
	Title string `json:"title,omitempty" validate:"max=50" example:"Updated cash-in route" maxLength:"50"`

	// Updated detailed description of the operation route purpose and usage
	// required: false
	// example: Updated description for the operation route
	// maxLength: 250
	Description string `json:"description,omitempty" validate:"max=250" example:"Updated description for the operation route" maxLength:"250"`

	// Updated external reference code for the operation route
	// required: false
	// example: EXT-002
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-002" maxLength:"100"`

	// Updated custom key-value pairs for extending the operation route information
	// required: false
	// example: {"department": "Global Treasury", "category": "Cash Management", "updated": true}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Updated account selection rule configuration
	// required: false
	Account *AccountRule `json:"account,omitempty"`
} // @name UpdateOperationRouteInput

// AccountRule represents the account selection rule configuration.
//
// swagger:model AccountRule
// @Description Account selection rule that determines which accounts are valid for an operation route. Rules can match by alias (specific account) or by account_type (category of accounts).
//
//	@example {
//	  "ruleType": "alias",
//	  "validIf": "@cash_account"
//	}
type AccountRule struct {
	// The rule type for account selection (alias for specific accounts, account_type for account categories)
	// example: alias
	// enum: alias,account_type
	RuleType string `json:"ruleType,omitempty" example:"alias" enum:"alias,account_type"`

	// The rule condition for account selection. String value for alias type (e.g., "@cash_account"), array of strings for account_type (e.g., ["deposit", "savings"])
	// example: @cash_account
	ValidIf any `json:"validIf,omitempty" example:"@cash_account"`
} // @name AccountRule
