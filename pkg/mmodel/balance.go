package mmodel

import (
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
	"github.com/google/uuid"
)

// Balance is a struct designed to encapsulate response payload data.
//
// swagger:model Balance
// @Description Complete balance entity containing all fields including system-generated fields like ID, creation timestamps, and metadata. This is the response format for balance operations. Balances represent the amount of a specific asset held in an account, including available and on-hold amounts.
type Balance struct {
	// Unique identifier for the balance (UUID format)
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Organization that owns this balance
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	OrganizationID string `json:"organizationId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Ledger containing the account this balance belongs to
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	LedgerID string `json:"ledgerId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Account that holds this balance
	// example: 00000000-0000-0000-0000-000000000000
	// format: uuid
	AccountID string `json:"accountId" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`

	// Alias for the account, used for easy identification or tagging
	// example: @person1
	// maxLength: 256
	Alias string `json:"alias" example:"@person1" maxLength:"256"`

	// Asset code identifying the currency or asset type of this balance
	// example: USD
	// minLength: 2
	// maxLength: 10
	AssetCode string `json:"assetCode" example:"USD" minLength:"2" maxLength:"10"`

	// Amount available for transactions (in the smallest unit of the asset, e.g. cents)
	// example: 1500
	// minimum: 0
	Available decimal.Decimal `json:"available" example:"1500" minimum:"0"`

	// Amount currently on hold and unavailable for transactions
	// example: 500
	// minimum: 0
	OnHold decimal.Decimal `json:"onHold" example:"500" minimum:"0"`

	// Optimistic concurrency control version
	// example: 1
	// minimum: 1
	Version int64 `json:"version" example:"1" minimum:"1"`

	// Type of account holding this balance
	// example: creditCard
	// maxLength: 50
	AccountType string `json:"accountType" example:"creditCard" maxLength:"50"`

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
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the balance information
	// example: {"purpose": "Main savings", "category": "Personal"}
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateBalance is a struct designed to encapsulate balance update request payload data.
//
// swagger:model UpdateBalance
// @Description Request payload for updating an existing balance's permissions. All fields are optional - only specified fields will be updated. Omitted fields will remain unchanged.
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

	// Account that holds this balance
	AccountID string `json:"accountId"`

	// Asset code identifying the currency or asset type of this balance
	AssetCode string `json:"assetCode"`

	// Amount available for transactions
	Available decimal.Decimal `json:"available"`

	// Amount currently on hold
	OnHold decimal.Decimal `json:"onHold"`

	// Optimistic concurrency control version
	Version int64 `json:"version"`

	// Type of account holding this balance
	AccountType string `json:"accountType"`

	// Whether the account can send funds (1=true, 0=false)
	AllowSending int `json:"allowSending"`

	// Whether the account can receive funds (1=true, 0=false)
	AllowReceiving int `json:"allowReceiving"`
}

// ConvertBalancesToLibBalances is a func that convert []*Balance to []*libTransaction.Balance
func ConvertBalancesToLibBalances(balances []*Balance) []*libTransaction.Balance {
	result := make([]*libTransaction.Balance, 0)
	for _, balance := range balances {
		result = append(result, &libTransaction.Balance{
			ID:             balance.ID,
			OrganizationID: balance.OrganizationID,
			LedgerID:       balance.LedgerID,
			AccountID:      balance.AccountID,
			Alias:          balance.Alias,
			AssetCode:      balance.AssetCode,
			Available:      balance.Available,
			OnHold:         balance.OnHold,
			Version:        balance.Version,
			AccountType:    balance.AccountType,
			AllowSending:   balance.AllowSending,
			AllowReceiving: balance.AllowReceiving,
			CreatedAt:      balance.CreatedAt,
			UpdatedAt:      balance.UpdatedAt,
			DeletedAt:      balance.DeletedAt,
			Metadata:       balance.Metadata,
		})
	}

	return result
}

// ConvertToLibBalance is a func that convert Balance to libTransaction.Balance
func (b *Balance) ConvertToLibBalance() *libTransaction.Balance {
	return &libTransaction.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
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
