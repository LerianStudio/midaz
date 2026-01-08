package mmodel

import (
	"time"

	"github.com/google/uuid"
)

// OperationRoute is a struct designed to store Operation Route object data.
//
// swagger:model OperationRoute
// @Description OperationRoute object
type OperationRoute struct {
	// The unique identifier of the Operation Route.
	ID uuid.UUID `json:"id,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Organization.
	OrganizationID uuid.UUID `json:"organizationId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// The unique identifier of the Ledger.
	LedgerID uuid.UUID `json:"ledgerId,omitempty" example:"01965ed9-7fa4-75b2-8872-fc9e8509ab0a"`
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" example:"This operation route handles cash-in transactions from service charge collections"`
	// External reference of the operation route.
	Code string `json:"code,omitempty" example:"EXT-001"`
	// The type of the operation route.
	OperationType string `json:"operationType,omitempty" example:"source" enum:"source,destination"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
	// The timestamp when the operation route was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name OperationRoute

// CreateOperationRouteInput is a struct designed to store Operation Route input data.
//
// @Description CreateOperationRouteInput payload for creating a new Operation Route with title, description, operation type, and optional account rules.
type CreateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"required,max=50" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections"`
	// External reference of the operation route.
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-001"`
	// The type of the operation route.
	OperationType string `json:"operationType,omitempty" validate:"required" example:"source" enum:"source,destination"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
} // @name CreateOperationRouteInput

// UpdateOperationRouteInput is a struct designed to store Operation Route input data.
//
// swagger:model UpdateOperationRouteInput
// @Description UpdateOperationRouteInput payload
type UpdateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"max=50" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections"`
	// External reference of the operation route.
	Code string `json:"code,omitempty" validate:"max=100" example:"EXT-001"`
	// Additional metadata stored as JSON
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`
	// The account selection rule configuration.
	Account *AccountRule `json:"account,omitempty"`
} // @name UpdateOperationRouteInput

// AccountRule represents the account selection rule configuration.
//
// @Description AccountRule object containing the rule type and condition for account selection in operation routes.
type AccountRule struct {
	// The rule type for account selection.
	RuleType string `json:"ruleType,omitempty" example:"alias" enum:"alias,account_type"`
	// The rule condition for account selection. String for alias type (e.g. "@cash_account"), array for account_type.
	ValidIf any `json:"validIf,omitempty"`
} // @name AccountRule
