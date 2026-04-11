// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mtransaction

import (
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"

	"github.com/shopspring/decimal"
)

// CreateTransactionInput is a struct design to encapsulate payload data.
//
// swagger:model CreateTransactionInput
// @Description CreateTransactionInput is the input payload to create a transaction. Contains all necessary fields to create a financial transaction, including source and destination information.
type CreateTransactionInput struct {
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
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
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
	TransactionDate *TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including source and distribution
	// required: true
	// swagger:type object
	Send Send `json:"send" validate:"required,dive"`
} // @name CreateTransactionInput

// BuildTransaction converts a CreateTransactionInput to a Transaction.
func (cti *CreateTransactionInput) BuildTransaction() *Transaction {
	fromClone := make([]FromTo, len(cti.Send.Source.From))
	copy(fromClone, cti.Send.Source.From)

	for i := range fromClone {
		fromClone[i].IsFrom = true
	}

	send := cti.Send
	send.Source.From = fromClone

	return &Transaction{
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

// SendInflow structure for marshaling/unmarshalling JSON for inflow transactions.
//
// swagger:model SendInflow
// @Description SendInflow is the struct designed to represent the sending fields of an inflow operation without source information.
type SendInflow struct {
	Asset      string          `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value      decimal.Decimal `json:"value,omitempty" validate:"required" example:"1000"`
	Distribute Distribute      `json:"distribute,omitempty" validate:"required"`
} // @name SendInflow

// CreateTransactionInflowInput is a struct designed to encapsulate payload data for inflow transactions.
//
// swagger:model CreateTransactionInflowInput
// @Description CreateTransactionInflowInput is the input payload to create an inflow transaction. Contains all necessary fields to create a financial transaction without source information, only destination.
type CreateTransactionInflowInput struct {
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

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
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
	TransactionDate *TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including distribution only (no source)
	// required: true
	// swagger:type object
	Send SendInflow `json:"send" validate:"required,dive"`
} // @name CreateTransactionInflowInput

// BuildInflowEntry converts a CreateTransactionInflowInput to a Transaction.
func (c *CreateTransactionInflowInput) BuildInflowEntry() *Transaction {
	from := FromTo{
		IsFrom:       true,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	return &Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Metadata:                 c.Metadata,
		TransactionDate:          c.TransactionDate,
		Route:                    c.Route,
		RouteID:                  c.RouteID,
		Send: Send{
			Asset:      c.Send.Asset,
			Value:      c.Send.Value,
			Distribute: c.Send.Distribute,
			Source: Source{
				From: []FromTo{from},
			},
		},
	}
}

// SendOutflow structure for marshaling/unmarshalling JSON for outflow transactions.
//
// swagger:model SendOutflow
// @Description SendOutflow is the struct designed to represent the sending fields of an outflow operation without distribution information.
type SendOutflow struct {
	Asset  string          `json:"asset,omitempty" validate:"required" example:"BRL"`
	Value  decimal.Decimal `json:"value,omitempty" validate:"required" example:"1000"`
	Source Source          `json:"source,omitempty" validate:"required"`
} // @name SendOutflow

// CreateTransactionOutflowInput is a struct design to encapsulate payload data for outflow transactions.
//
// swagger:model CreateTransactionOutflowInput
// @Description CreateTransactionOutflowInput is the input payload to create an outflow transaction. Contains all necessary fields to create a financial transaction with source information only, without destination.
type CreateTransactionOutflowInput struct {
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
	// swagger: type boolean
	Pending bool `json:"pending" example:"true" default:"false"`

	// Additional custom attributes
	// example: {"reference": "TRANSACTION-001", "source": "api"}
	// swagger:type object
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
	TransactionDate *TransactionDate `json:"transactionDate,omitempty" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Send operation details including source only (no distribution)
	// required: true
	// swagger:type object
	Send SendOutflow `json:"send" validate:"required,dive"`
} // @name CreateTransactionOutflowInput

// BuildOutflowEntry converts a CreateTransactionOutflowInput to a Transaction.
func (c *CreateTransactionOutflowInput) BuildOutflowEntry() *Transaction {
	to := FromTo{
		IsFrom:       false,
		AccountAlias: cn.DefaultExternalAccountAliasPrefix + c.Send.Asset,
		Amount: &Amount{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
		},
	}

	fromClone := make([]FromTo, len(c.Send.Source.From))
	copy(fromClone, c.Send.Source.From)

	for i := range fromClone {
		fromClone[i].IsFrom = true
	}

	return &Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Code:                     c.Code,
		Pending:                  c.Pending,
		Metadata:                 c.Metadata,
		TransactionDate:          c.TransactionDate,
		Route:                    c.Route,
		RouteID:                  c.RouteID,
		Send: Send{
			Asset: c.Send.Asset,
			Value: c.Send.Value,
			Source: Source{
				From: fromClone,
			},
			Distribute: Distribute{
				To: []FromTo{to},
			},
		},
	}
}
