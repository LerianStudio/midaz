package transaction

import (
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Balance structure for marshaling/unmarshalling JSON.
//
// swagger:model Balance
// @Description Balance represents the current state of an account's balance including available and on-hold amounts. Used for transaction validation and processing.
//
//	@example {
//	  "id": "00000000-0000-0000-0000-000000000000",
//	  "accountId": "00000000-0000-0000-0000-000000000000",
//	  "alias": "@treasury_main",
//	  "assetCode": "USD",
//	  "available": "15000.00",
//	  "onHold": "500.00",
//	  "version": 1,
//	  "accountType": "deposit",
//	  "allowSending": true,
//	  "allowReceiving": true
//	}
type Balance struct {
	// Unique identifier for the balance (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the organization that owns this balance (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the ledger containing the account (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the account that holds this balance (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Alias for the account, used for easy identification
	// example: @person1
	// maxLength: 256
	Alias string `json:"alias" example:"@person1" maxLength:"256"`

	// Unique key for the balance within the account
	// example: default
	// maxLength: 100
	Key string `json:"key" example:"default" maxLength:"100"`

	// Asset code identifying the currency or asset type
	// example: BRL
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"BRL" maxLength:"10"`

	// Amount available for transactions (decimal string representation)
	// example: 1500.00
	Available decimal.Decimal `json:"available" swaggertype:"string" example:"1500.00"`

	// Amount currently on hold (decimal string representation)
	// example: 500.00
	OnHold decimal.Decimal `json:"onHold" swaggertype:"string" example:"500.00"`

	// Optimistic concurrency control version number
	// example: 1
	// minimum: 1
	Version int64 `json:"version" example:"1" minimum:"1"`

	// Type classification of the account holding this balance
	// example: deposit
	// maxLength: 50
	AccountType string `json:"accountType" example:"deposit" maxLength:"50"`

	// Whether the account can send funds from this balance
	// example: true
	AllowSending bool `json:"allowSending" example:"true"`

	// Whether the account can receive funds to this balance
	// example: true
	AllowReceiving bool `json:"allowReceiving" example:"true"`

	// Timestamp when the balance was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the balance was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the balance was soft deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the balance information
	// example: {"purpose": "Main savings", "category": "Personal"}
	Metadata map[string]any `json:"metadata,omitempty"`
} // @name Balance

// Responses is an internal struct for transaction validation results.
// This is not exposed via API.
type Responses struct {
	Total               decimal.Decimal
	Asset               string
	From                map[string]Amount
	To                  map[string]Amount
	Sources             []string
	Destinations        []string
	Aliases             []string
	Pending             bool
	TransactionRoute    string
	OperationRoutesFrom map[string]string
	OperationRoutesTo   map[string]string
}

// Metadata structure for marshaling/unmarshalling JSON.
//
// swagger:model Metadata
// @Description Generic key-value metadata container used for storing custom attributes.
type Metadata struct {
	// Key identifier for the metadata entry
	// example: department
	Key string `json:"key,omitempty" example:"department"`

	// Value associated with the key (can be any JSON-compatible type)
	// example: Treasury
	Value any `json:"value,omitempty" example:"Treasury"`
} // @name Metadata

// Amount structure for marshaling/unmarshalling JSON.
//
// swagger:model Amount
// @Description Amount represents a monetary value with its associated asset code for transaction operations. Used to specify the value being transferred in a transaction.
//
//	@example {
//	  "asset": "BRL",
//	  "value": "1000.00"
//	}
type Amount struct {
	// Asset code for the amount (e.g., USD, BRL, BTC)
	// required: true
	// example: BRL
	// maxLength: 10
	Asset string `json:"asset,omitempty" validate:"required" example:"BRL" maxLength:"10"`

	// Numeric value of the amount (decimal string representation)
	// required: true
	// example: 1000.00
	Value decimal.Decimal `json:"value,omitempty" validate:"required" swaggertype:"string" example:"1000.00"`

	// Operation type (internal use)
	Operation string `json:"operation,omitempty"`

	// Transaction type classification (internal use)
	TransactionType string `json:"transactionType,omitempty"`
} // @name Amount

// Share structure for marshaling/unmarshalling JSON.
//
// swagger:model Share
// @Description Share represents a percentage-based distribution for splitting transaction amounts. Used when multiple destinations should receive proportional amounts.
//
//	@example {
//	  "percentage": 50,
//	  "percentageOfPercentage": 0
//	}
type Share struct {
	// Percentage of the total amount (0-100)
	// required: true
	// example: 50
	// minimum: 0
	// maximum: 100
	Percentage int64 `json:"percentage,omitempty" validate:"required" example:"50" minimum:"0" maximum:"100"`

	// Secondary percentage applied to the calculated percentage amount (0-100)
	// example: 0
	// minimum: 0
	// maximum: 100
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty" example:"0" minimum:"0" maximum:"100"`
} // @name Share

// Send structure for marshaling/unmarshalling JSON.
//
// swagger:model Send
// @Description Send defines the complete sending specification for a transaction, including the asset, value, source accounts, and distribution to destination accounts.
//
//	@example {
//	  "asset": "BRL",
//	  "value": "1000.00",
//	  "source": {
//	    "from": [{"accountAlias": "@treasury"}]
//	  },
//	  "distribute": {
//	    "to": [{"accountAlias": "@customer", "amount": {"asset": "BRL", "value": "1000.00"}}]
//	  }
//	}
type Send struct {
	// Asset code for the transaction
	// required: true
	// example: BRL
	// maxLength: 10
	Asset string `json:"asset,omitempty" validate:"required" example:"BRL" maxLength:"10"`

	// Total value to be sent (decimal string representation)
	// required: true
	// example: 1000.00
	Value decimal.Decimal `json:"value,omitempty" validate:"required" swaggertype:"string" example:"1000.00"`

	// Source configuration specifying which accounts to debit
	// required: true
	Source Source `json:"source,omitempty" validate:"required"`

	// Distribution configuration specifying which accounts to credit
	// required: true
	Distribute Distribute `json:"distribute,omitempty" validate:"required"`
} // @name Send

// Source structure for marshaling/unmarshalling JSON.
//
// swagger:model Source
// @Description Source defines the debit side of a transaction, specifying which accounts funds will be drawn from.
//
//	@example {
//	  "remaining": "remaining",
//	  "from": [{"accountAlias": "@treasury", "amount": {"asset": "BRL", "value": "1000.00"}}]
//	}
type Source struct {
	// Identifier for the account that receives any remaining balance after distribution
	// example: remaining
	Remaining string `json:"remaining,omitempty" example:"remaining"`

	// Array of source accounts to debit funds from
	// required: true
	From []FromTo `json:"from,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Source

// Rate structure for marshaling/unmarshalling JSON.
//
// swagger:model Rate
// @Description Rate defines currency conversion parameters for cross-currency transactions. Used when the source and destination assets differ.
//
//	@example {
//	  "from": "BRL",
//	  "to": "USD",
//	  "value": "5.25",
//	  "externalId": "00000000-0000-0000-0000-000000000000"
//	}
type Rate struct {
	// Source asset code for the conversion
	// required: true
	// example: BRL
	// maxLength: 10
	From string `json:"from" validate:"required" example:"BRL" maxLength:"10"`

	// Destination asset code for the conversion
	// required: true
	// example: USD
	// maxLength: 10
	To string `json:"to" validate:"required" example:"USD" maxLength:"10"`

	// Exchange rate value (decimal string representation)
	// required: true
	// example: 5.25
	Value decimal.Decimal `json:"value" validate:"required" swaggertype:"string" example:"5.25"`

	// External reference ID for the exchange rate (UUID format)
	// required: true
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ExternalID string `json:"externalId" validate:"uuid,required" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
} // @name Rate

// IsEmpty method that set empty or nil in fields
func (r Rate) IsEmpty() bool {
	return r.ExternalID == "" && r.From == "" && r.To == "" && r.Value.IsZero()
}

// FromTo structure for marshaling/unmarshalling JSON.
//
// swagger:model FromTo
// @Description FromTo represents a single source or destination entry in a transaction. It specifies the account, amount or share, and optional metadata for the operation.
//
//	@example {
//	  "accountAlias": "@treasury",
//	  "balanceKey": "default",
//	  "amount": {"asset": "BRL", "value": "1000.00"},
//	  "description": "Payment for services",
//	  "chartOfAccounts": "1000",
//	  "metadata": {"reference": "INV-001"}
//	}
type FromTo struct {
	// Account alias identifying the source or destination account
	// example: @treasury
	// maxLength: 256
	AccountAlias string `json:"accountAlias,omitempty" example:"@treasury" maxLength:"256"`

	// Balance key within the account (defaults to "default" if not specified)
	// example: default
	// maxLength: 100
	BalanceKey string `json:"balanceKey,omitempty" example:"default" maxLength:"100"`

	// Fixed amount specification (mutually exclusive with Share)
	Amount *Amount `json:"amount,omitempty"`

	// Percentage-based amount specification (mutually exclusive with Amount)
	Share *Share `json:"share,omitempty"`

	// Identifier for the account that receives any remaining balance
	// example: remaining
	Remaining string `json:"remaining,omitempty" example:"remaining"`

	// Exchange rate configuration for cross-currency transactions
	Rate *Rate `json:"rate,omitempty"`

	// Description for this specific operation entry
	// example: Payment for services
	// maxLength: 256
	Description string `json:"description,omitempty" example:"Payment for services" maxLength:"256"`

	// Chart of accounts code for accounting classification
	// example: 1000
	// maxLength: 50
	ChartOfAccounts string `json:"chartOfAccounts" example:"1000" maxLength:"50"`

	// Custom key-value pairs for this operation entry
	// example: {"reference": "INV-001"}
	Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`

	// Indicates if this entry is a source (true) or destination (false)
	// example: true
	IsFrom bool `json:"isFrom,omitempty" example:"true"`

	// Operation route ID for routing validation (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000" format:"uuid" maxLength:"250"`
} // @name FromTo

// SplitAlias function to split alias with index.
func (ft FromTo) SplitAlias() string {
	if strings.Contains(ft.AccountAlias, "#") {
		return strings.Split(ft.AccountAlias, "#")[1]
	}

	return ft.AccountAlias
}

// SplitAliasWithKey extracts the substring after the '#' character from the provided alias or returns the alias if '#' is not present.
func SplitAliasWithKey(alias string) string {
	if idx := strings.Index(alias, "#"); idx != -1 {
		return alias[idx+1:]
	}

	return alias
}

// ConcatAlias function to concat alias with index.
func (ft FromTo) ConcatAlias(i int) string {
	return strconv.Itoa(i) + "#" + ft.AccountAlias + "#" + ft.BalanceKey
}

// Distribute structure for marshaling/unmarshalling JSON.
//
// swagger:model Distribute
// @Description Distribute defines the credit side of a transaction, specifying which accounts will receive funds.
//
//	@example {
//	  "remaining": "remaining",
//	  "to": [{"accountAlias": "@customer", "amount": {"asset": "BRL", "value": "1000.00"}}]
//	}
type Distribute struct {
	// Identifier for the account that receives any remaining balance after distribution
	// example: remaining
	Remaining string `json:"remaining,omitempty" example:"remaining"`

	// Array of destination accounts to credit funds to
	// required: true
	To []FromTo `json:"to,omitempty" validate:"singletransactiontype,required,dive"`
} // @name Distribute

// Transaction structure for marshaling/unmarshalling JSON.
//
// swagger:model Transaction
// @Description Transaction defines a complete transaction using the Midaz DSL (Domain Specific Language). It specifies what to send, from where, and to where, along with metadata and routing information.
//
//	@example {
//	  "chartOfAccountsGroupName": "1000",
//	  "description": "Service payment",
//	  "code": "PAY-001",
//	  "pending": false,
//	  "transactionDate": "2024-01-15T10:30:00Z",
//	  "send": {
//	    "asset": "BRL",
//	    "value": "1000.00",
//	    "source": {
//	      "from": [{"accountAlias": "@treasury"}]
//	    },
//	    "distribute": {
//	      "to": [{"accountAlias": "@customer"}]
//	    }
//	  },
//	  "metadata": {
//	    "invoiceId": "INV-001",
//	    "department": "Sales"
//	  }
//	}
type Transaction struct {
	// Chart of accounts group name for accounting classification
	// example: 1000
	// maxLength: 50
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty" example:"1000" maxLength:"50"`

	// Human-readable description of the transaction
	// example: Service payment
	// maxLength: 256
	Description string `json:"description,omitempty" example:"Service payment" maxLength:"256"`

	// External reference code for the transaction
	// example: PAY-001
	// maxLength: 100
	Code string `json:"code,omitempty" example:"PAY-001" maxLength:"100"`

	// Whether the transaction should be created in pending state
	// example: false
	Pending bool `json:"pending,omitempty" example:"false"`

	// Custom key-value pairs for extending the transaction information
	// example: {"invoiceId": "INV-001", "department": "Sales"}
	Metadata map[string]any `json:"metadata,omitempty" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`

	// Transaction route ID for routing validation (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	// maxLength: 250
	Route string `json:"route,omitempty" validate:"omitempty,max=250" example:"00000000-0000-0000-0000-000000000000" format:"uuid" maxLength:"250"`

	// Date when the transaction occurred (RFC3339 format)
	// example: 2024-01-15T10:30:00Z
	// format: date-time
	TransactionDate time.Time `json:"transactionDate,omitempty" example:"2024-01-15T10:30:00Z" format:"date-time"`

	// Send specification defining the complete transaction flow
	// required: true
	Send Send `json:"send" validate:"required"`
} // @name Transaction

// IsEmpty is a func that validate if transaction is Empty.
func (t Transaction) IsEmpty() bool {
	return t.Send.Asset == "" && t.Send.Value.IsZero()
}
