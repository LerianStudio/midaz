package mmodel

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/google/uuid"
)

// Balance is a struct designed to encapsulate response payload data.
//
// swagger:model Balance
// @Description Complete balance entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for balance operations. Balances represent the amount of a specific asset held in an account, including available and on-hold amounts.
//
//	@example {
//	  "id": "a1b2c3d4-e5f6-7890-abcd-1234567890ab",
//	  "organizationId": "b2c3d4e5-f6a1-7890-bcde-2345678901cd",
//	  "ledgerId": "c3d4e5f6-a1b2-7890-cdef-3456789012de",
//	  "accountId": "d4e5f6a1-b2c3-7890-defg-4567890123ef",
//	  "alias": "@treasury_main",
//	  "key": "default",
//	  "assetCode": "USD",
//	  "available": "15000.00",
//	  "onHold": "500.00",
//	  "version": 1,
//	  "accountType": "deposit",
//	  "allowSending": true,
//	  "allowReceiving": true,
//	  "createdAt": "2022-04-15T09:30:00Z",
//	  "updatedAt": "2022-04-15T09:30:00Z",
//	  "metadata": {
//	    "purpose": "Main treasury balance",
//	    "category": "Operations"
//	  }
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

	// ID of the ledger containing the account this balance belongs to (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// ID of the account that holds this balance (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Alias for the account, used for easy identification or tagging
	// example: @person1
	// maxLength: 256
	Alias string `json:"alias" example:"@person1" maxLength:"256"`

	// Unique key for the balance within the account (e.g., default, asset-freeze)
	// example: default
	// maxLength: 100
	Key string `json:"key" example:"default" maxLength:"100"`

	// Asset code identifying the currency or asset type of this balance
	// example: USD
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"USD" minLength:"2" maxLength:"10"`

	// Amount available for transactions (decimal string representation)
	// example: 1500.00
	// minimum: 0
	Available decimal.Decimal `json:"available" swaggertype:"string" example:"1500.00" minimum:"0"`

	// Amount currently on hold and unavailable for transactions (decimal string representation)
	// example: 500.00
	// minimum: 0
	OnHold decimal.Decimal `json:"onHold" swaggertype:"string" example:"500.00" minimum:"0"`

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

	// Timestamp when the balance was soft deleted, null if not deleted (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the balance information
	// example: {"purpose": "Main savings", "category": "Personal"}
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateAdditionalBalance is a struct designed to encapsulate balance create request payload data.
//
// swagger:model CreateAdditionalBalance
// @Description Request payload for creating a new additional balance with specified permissions and custom key. Additional balances allow accounts to hold multiple balances with different purposes (e.g., frozen funds, reserved amounts).
//
//	@example {
//	  "key": "asset-freeze",
//	  "allowSending": false,
//	  "allowReceiving": true
//	}
type CreateAdditionalBalance struct {
	// Unique key identifier for the balance within the account
	// required: true
	// example: asset-freeze
	// maxLength: 100
	Key string `json:"key" validate:"required,nowhitespaces,max=100" example:"asset-freeze" maxLength:"100"`

	// Whether the account should be allowed to send funds from this balance
	// required: false
	// example: true
	AllowSending *bool `json:"allowSending" example:"true"`

	// Whether the account should be allowed to receive funds to this balance
	// required: false
	// example: true
	AllowReceiving *bool `json:"allowReceiving" example:"true"`
} // @name CreateAdditionalBalance

// UpdateBalance is a struct designed to encapsulate balance update request payload data.
//
// swagger:model UpdateBalance
// @Description Request payload for updating an existing balance's permissions. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
//
//	@example {
//	  "allowSending": true,
//	  "allowReceiving": false
//	}
type UpdateBalance struct {
	// Whether the account should be allowed to send funds from this balance
	// required: false
	// example: true
	AllowSending *bool `json:"allowSending" example:"true"`

	// Whether the account should be allowed to receive funds to this balance
	// required: false
	// example: true
	AllowReceiving *bool `json:"allowReceiving" example:"true"`
} // @name UpdateBalance

// CreateBalanceInput is the input model used by services to create a balance synchronously.
//
// It centralizes all properties required to perform validations and persist the new balance,
// keeping call sites simple and reducing the chance of inconsistent argument ordering.
type CreateBalanceInput struct {
	// Organization that owns this balance
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID uuid.UUID

	// Ledger containing the account this balance belongs to
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID uuid.UUID

	// Account that holds this balance
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID uuid.UUID

	// Alias for the account, used for easy identification or tagging
	// example: @person1
	// maxLength: 256
	Alias string

	// Unique key for the balance
	// example: asset-freeze
	// maxLength: 100
	Key string

	// Asset code identifying the currency or asset type of this balance
	// example: USD
	// minLength: 2
	// maxLength: 10
	AssetCode string

	// Type of account holding this balance
	// example: creditCard
	// maxLength: 50
	AccountType string

	// Whether the account should be allowed to send funds from this balance
	// example: true
	AllowSending bool

	// Whether the account should be allowed to receive funds to this balance
	// example: true
	AllowReceiving bool
}

// IDtoUUID is a func that convert UUID string to uuid.UUID
func (b *Balance) IDtoUUID() uuid.UUID {
	return uuid.MustParse(b.ID)
}

// Balances struct to return paginated list of balances.
//
// swagger:model Balances
// @Description Paginated list of balances with metadata about the current page, limit, and the balance items themselves. Used for list operations.
type Balances struct {
	// Array of balance records returned in this page
	// example: [{"id":"00000000-0000-0000-0000-000000000000","accountId":"00000000-0000-0000-0000-000000000000","assetCode":"USD","available":1500}]
	Items []Balance `json:"items"`

	// Current page number in the pagination
	// example: 1
	// minimum: 1
	Page int `json:"page" example:"1" minimum:"1"`

	// Maximum number of items per page
	// example: 10
	// minimum: 1
	// maximum: 100
	Limit int `json:"limit" example:"10" minimum:"1" maximum:"100"`
} // @name Balances

// BalanceRedis is an internal struct for Redis cache representation of balance data.
//
// This is an internal model not exposed via API.
type BalanceRedis struct {
	// Unique identifier for the balance (UUID format)
	ID string `json:"id"`

	// Alias for the account, used for easy identification or tagging
	// example: @person1
	// maxLength: 256
	Alias string `json:"alias" example:"@person1" maxLength:"256"`

	// Account that holds this balance
	AccountID string `json:"accountId"`

	// Asset code identifying the currency or asset type of this balance
	AssetCode string `json:"assetCode"`

	// Amount available for transactions
	Available decimal.Decimal `json:"available" swaggertype:"string" example:"1500.00"`

	// Amount currently on hold
	OnHold decimal.Decimal `json:"onHold" swaggertype:"string" example:"500.00"`

	// Optimistic concurrency control version
	Version int64 `json:"version"`

	// Type of account holding this balance
	AccountType string `json:"accountType"`

	// Whether the account can send funds (1=true, 0=false)
	AllowSending int `json:"allowSending"`

	// Whether the account can receive funds (1=true, 0=false)
	AllowReceiving int `json:"allowReceiving"`

	// Unique key for the balance
	Key string `json:"key"`
}

// ConvertBalancesToLibBalances is a func that convert []*Balance to []*libTransaction.Balance
func ConvertBalancesToLibBalances(balances []*Balance) []*libTransaction.Balance {
	out := make([]*libTransaction.Balance, 0, len(balances))

	for _, b := range balances {
		if b != nil {
			out = append(out, b.ConvertToLibBalance())
		}
	}

	return out
}

// ConvertBalanceOperationsToLibBalances is a func that convert []*BalanceOperation to []*libTransaction.Balance
func ConvertBalanceOperationsToLibBalances(operations []BalanceOperation) []*libTransaction.Balance {
	out := make([]*libTransaction.Balance, 0, len(operations))
	for _, op := range operations {
		out = append(out, op.Balance.ConvertToLibBalance())
	}

	return out
}

// ConvertToLibBalance is a func that convert Balance to libTransaction.Balance
func (b *Balance) ConvertToLibBalance() *libTransaction.Balance {
	return &libTransaction.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		Key:            b.Key,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
		DeletedAt:      b.DeletedAt,
		Metadata:       b.Metadata,
	}
}

// UnmarshalJSON is a custom unmarshal function for BalanceRedis
func (b *BalanceRedis) UnmarshalJSON(data []byte) error {
	type Alias BalanceRedis

	aux := struct {
		Available any `json:"available"`
		OnHold    any `json:"onHold"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.Available.(type) {
	case float64:
		b.Available = decimal.NewFromFloat(v)
	case string:
		decimalValue, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("err to converter available field from string to decimal: %v", err)
		}

		b.Available = decimalValue
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			f, err := v.Float64()
			if err != nil {
				return fmt.Errorf("err to converter available field from json.Number: %v", err)
			}

			b.Available = decimal.NewFromFloat(f)
		} else {
			b.Available = decimal.NewFromInt(i)
		}
	default:
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("type unsuported to available: %T", v)
		}

		b.Available = decimal.NewFromFloat(f)
	}

	switch v := aux.OnHold.(type) {
	case float64:
		b.OnHold = decimal.NewFromFloat(v)
	case string:
		decimalValue, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("err to converter onHold field from string to decimal: %v", err)
		}

		b.OnHold = decimalValue
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			f, err := v.Float64()
			if err != nil {
				return fmt.Errorf("err to converter onHold field from json.Number: %v", err)
			}

			b.OnHold = decimal.NewFromFloat(f)
		} else {
			b.OnHold = decimal.NewFromInt(i)
		}
	default:
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("type unsuported to  onHold: %T", v)
		}

		b.OnHold = decimal.NewFromFloat(f)
	}

	return nil
}

// BalanceErrorResponse represents an error response for balance operations.
//
// swagger:response BalanceErrorResponse
// @Description Error response for balance operations with error code and message.
type BalanceErrorResponse struct {
	// in: body
	Body struct {
		// Error code identifying the specific error
		// example: 400001
		Code int `json:"code"`

		// Human-readable error message
		// example: Invalid input: field 'assetCode' is required
		Message string `json:"message"`

		// Additional error details if available
		// example: {"field": "assetCode", "violation": "required"}
		Details map[string]any `json:"details,omitempty"`
	}
}

// BalanceOperation represents a balance operation with associated metadata for transaction processing on redis by cache-aside
type BalanceOperation struct {
	Balance     *Balance
	Alias       string
	Amount      libTransaction.Amount
	InternalKey string
}

// TransactionRedisQueue represents a transaction queue for cache-aside
type TransactionRedisQueue struct {
	HeaderID          string                     `json:"header_id"`
	TransactionID     uuid.UUID                  `json:"transaction_id" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	OrganizationID    uuid.UUID                  `json:"organization_id" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID          uuid.UUID                  `json:"ledger_id" swaggertype:"string" format:"uuid" example:"00000000-0000-0000-0000-000000000000"`
	Balances          []BalanceRedis             `json:"balances"`
	ParserDSL         libTransaction.Transaction `json:"parserDSL"`
	TTL               time.Time                  `json:"ttl"`
	Validate          *libTransaction.Responses  `json:"validate"`
	TransactionStatus string                     `json:"transaction_status"`
	TransactionDate   time.Time                  `json:"transaction_date"`
}
