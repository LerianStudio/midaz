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
	// The type of the operation route.
	Type string `json:"type,omitempty" example:"debit" enum:"debit,credit"`
	// Array of allowed account types for this operation route.
	AccountTypes []string `json:"accountTypes,omitempty" example:"[\"asset\",\"liability\"]"`
	// Specific account alias for this operation route.
	AccountAlias string `json:"accountAlias,omitempty" example:"@cash_account"`
	// The timestamp when the operation route was created.
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was last updated.
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
	// The timestamp when the operation route was deleted.
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name OperationRoute

// CreateOperationRouteInput is a struct designed to store Operation Route input data.
//
// swagger:model CreateOperationRouteInput
// @Description CreateOperationRouteInput payload
type CreateOperationRouteInput struct {
	// Short text summarizing the purpose of the operation. Used as an entry note for identification.
	Title string `json:"title,omitempty" validate:"required,max=50" example:"Cashin from service charge"`
	// Detailed description of the operation route purpose and usage.
	Description string `json:"description,omitempty" validate:"max=250" example:"This operation route handles cash-in transactions from service charge collections"`
	// The type of the operation route.
	Type string `json:"type,omitempty" validate:"required" example:"debit" enum:"debit,credit"`
	// Array of allowed account types for this operation route.
	AccountTypes []string `json:"accountTypes,omitempty" example:"[\"asset\",\"liability\"]"`
	// Specific account alias for this operation route.
	AccountAlias string `json:"accountAlias,omitempty" example:"@cash_account"`
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
	// Array of allowed account types for this operation route.
	AccountTypes []string `json:"accountTypes,omitempty" example:"[\"asset\",\"liability\"]"`
	// Specific account alias for this operation route.
	AccountAlias string `json:"accountAlias,omitempty" example:"@cash_account"`
} // @name UpdateOperationRouteInput
