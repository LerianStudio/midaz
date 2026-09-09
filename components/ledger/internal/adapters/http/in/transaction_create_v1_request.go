// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/shopspring/decimal"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// CreateTransactionRequest is the transport payload for a v1 transaction create.
// It is published as CreateTransactionInput to preserve the existing OpenAPI contract.
type CreateTransactionRequest struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"FUNDING"`

	// Human-readable description of the transaction
	// example: New Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Whether the transaction should be created in pending state
	// example: true
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom key-value attributes. Values must be flat (string, number, boolean) — no nested objects.
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Deprecated: legacy route identifier, use routeId instead. Contains the transaction route UUID as a free-form string for backwards compatibility.
	// example: "00000000-0000-0000-0000-000000000000"
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000"`

	// UUID of the transaction route. Used instead of route for proper UUID validation and referential integrity.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// format: date-time
	TransactionDate *mtransaction.TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including source and distribution
	// required: true
	Send mtransaction.Send `json:"send" validate:"required,dive"`
}

// BuildTransaction converts a CreateTransactionRequest to the canonical transaction.
func (cti *CreateTransactionRequest) BuildTransaction() *mtransaction.Transaction {
	fromClone := make([]mtransaction.FromTo, len(cti.Send.Source.From))
	copy(fromClone, cti.Send.Source.From)

	for i := range fromClone {
		fromClone[i].IsFrom = true
	}

	send := cti.Send
	send.Source.From = fromClone

	return &mtransaction.Transaction{
		ChartOfAccountsGroupName: cti.ChartOfAccountsGroupName,
		Description:              cti.Description,
		Code:                     cti.Code,
		Pending:                  cti.Pending,
		Metadata:                 cti.Metadata,
		TransactionDate:          cti.TransactionDate,
		Route:                    cti.Route,
		RouteID:                  cti.RouteID,
		Send:                     send,
	}
}

// TransactionInflowSendRequest is the transport send block for inflow transactions.
// It is published as SendInflow to preserve the existing OpenAPI contract.
type TransactionInflowSendRequest struct {
	Asset      string                  `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      decimal.Decimal         `json:"value,omitempty" validate:"required" example:"1000"`
	Distribute mtransaction.Distribute `json:"distribute,omitempty" validate:"required"`
}

// CreateTransactionInflowRequestBody is the transport payload for a v1 inflow create.
// It is published as CreateTransactionInflowInput to preserve the existing OpenAPI contract.
type CreateTransactionInflowRequestBody struct {
	// Chart of accounts group name for accounting purposes
	// example: FUNDING
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"FUNDING"`

	// Human-readable description of the transaction
	// example: New Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Additional custom key-value attributes. Values must be flat (string, number, boolean) — no nested objects.
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Deprecated: legacy route identifier, use routeId instead. Contains the transaction route UUID as a free-form string for backwards compatibility.
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000"`

	// UUID of the transaction route. Used instead of route for proper UUID validation and referential integrity.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// format: date-time
	TransactionDate *mtransaction.TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including distribution only (no source)
	// required: true
	Send TransactionInflowSendRequest `json:"send" validate:"required,dive"`
}

// BuildInflowEntry converts a CreateTransactionInflowRequestBody to the canonical transaction.
func (c *CreateTransactionInflowRequestBody) BuildInflowEntry() *mtransaction.Transaction {
	from := mtransaction.FromTo{
		IsFrom:       true,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &mtransaction.Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	return &mtransaction.Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Metadata:                 c.Metadata,
		TransactionDate:          c.TransactionDate,
		Route:                    c.Route,
		RouteID:                  c.RouteID,
		Send: mtransaction.Send{
			Asset:      c.Send.Asset,
			Value:      c.Send.Value,
			Distribute: c.Send.Distribute,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{from},
			},
		},
	}
}

// TransactionOutflowSendRequest is the transport send block for outflow transactions.
// It is published as SendOutflow to preserve the existing OpenAPI contract.
type TransactionOutflowSendRequest struct {
	Asset  string              `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value  decimal.Decimal     `json:"value,omitempty" validate:"required" example:"1000"`
	Source mtransaction.Source `json:"source,omitempty" validate:"required"`
}

// CreateTransactionOutflowRequestBody is the transport payload for a v1 outflow create.
// It is published as CreateTransactionOutflowInput to preserve the existing OpenAPI contract.
type CreateTransactionOutflowRequestBody struct {
	// Chart of accounts group name for accounting purposes
	// example: WITHDRAWAL
	// maxLength: 256
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" validate:"max=256" maxLength:"256" example:"WITHDRAWAL"`

	// Human-readable description of the transaction
	// example: New Outflow Transaction
	// maxLength: 256
	Description string `json:"description,omitempty" validate:"max=256" example:"New Outflow Transaction" maxLength:"256"`

	// Transaction code for reference
	// example: TR12345
	// maxLength: 100
	Code string `json:"code,omitempty" validate:"max=100" example:"TR12345" maxLength:"100"`

	// Whether the transaction should be created in pending state
	// example: true
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom key-value attributes. Values must be flat (string, number, boolean) — no nested objects.
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,omitempty,nonested,valuemax=2000"`

	// Deprecated: legacy route identifier, use routeId instead. Contains the transaction route UUID as a free-form string for backwards compatibility.
	// example: 00000000-0000-0000-0000-000000000000
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000"`

	// UUID of the transaction route. Used instead of route for proper UUID validation and referential integrity.
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	RouteID *string `json:"routeId,omitempty" validate:"omitempty,uuid" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// TransactionDate Period from transaction creation date until now
	// Example "2021-01-01T00:00:00Z"
	// format: date-time
	TransactionDate *mtransaction.TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including source only (no distribution)
	// required: true
	Send TransactionOutflowSendRequest `json:"send" validate:"required,dive"`
}

// BuildOutflowEntry converts a CreateTransactionOutflowRequestBody to the canonical transaction.
func (c *CreateTransactionOutflowRequestBody) BuildOutflowEntry() *mtransaction.Transaction {
	to := mtransaction.FromTo{
		IsFrom:       false,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &mtransaction.Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	fromClone := make([]mtransaction.FromTo, len(c.Send.Source.From))
	copy(fromClone, c.Send.Source.From)

	for i := range fromClone {
		fromClone[i].IsFrom = true
	}

	return &mtransaction.Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Pending:                  c.Pending,
		Metadata:                 c.Metadata,
		TransactionDate:          c.TransactionDate,
		Route:                    c.Route,
		RouteID:                  c.RouteID,
		Send: mtransaction.Send{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
			Source: mtransaction.Source{
				From: fromClone,
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{to},
			},
		},
	}
}
