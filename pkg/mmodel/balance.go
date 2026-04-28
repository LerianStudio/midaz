// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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

	// Unique key for the balance
	// example: asset-freeze
	// maxLength: 100
	Key string `json:"key" example:"asset-freeze" maxLength:"100"`

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

	// Direction is the accounting direction of the balance. One of
	// "credit" or "debit". Empty string denotes legacy rows predating the
	// overdraft feature and is treated as "credit" by the engine.
	// example: credit
	Direction string `json:"direction,omitempty" example:"credit"`

	// OverdraftUsed is the amount of overdraft currently consumed by this
	// balance. Always non-negative; zero when the balance is in the black.
	// example: 0
	OverdraftUsed decimal.Decimal `json:"overdraftUsed" example:"0"`

	// Settings carries optional per-balance configuration (overdraft,
	// balance scope). Nil for legacy balances without custom settings.
	Settings *BalanceSettings `json:"settings,omitempty"`

	// Timestamp when the balance was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the balance was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the balance was softly deleted, null if not deleted (RFC3339 format)
	// example: null
	// format: date-time
	DeletedAt *time.Time `json:"deletedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Custom key-value pairs for extending the balance information
	// example: {"purpose": "Main savings", "category": "Personal"}
	Metadata map[string]any `json:"metadata,omitempty"`
}

// BalanceHistory represents a historical balance snapshot without permission flags.
// Permission flags (AllowSending/AllowReceiving) are not tracked historically.
//
// swagger:model BalanceHistory
// @Description Historical balance snapshot at a specific point in time. Does not include permission flags (allowSending/allowReceiving) as these are not tracked historically.
type BalanceHistory struct {
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

	// Unique key for the balance
	// example: asset-freeze
	// maxLength: 100
	Key string `json:"key" example:"asset-freeze" maxLength:"100"`

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

	// Direction is the accounting direction of the balance at the time of
	// the snapshot. One of "credit" or "debit". Empty string denotes
	// legacy rows predating the overdraft feature.
	// example: credit
	Direction string `json:"direction,omitempty" example:"credit"`

	// OverdraftUsed is the amount of overdraft consumed at the time of
	// the snapshot. Always non-negative.
	// example: 0
	OverdraftUsed decimal.Decimal `json:"overdraftUsed" example:"0"`

	// Settings is the per-balance configuration snapshot at the time the
	// history row was recorded. Nil for legacy balances.
	Settings *BalanceSettings `json:"settings,omitempty"`

	// Timestamp when the balance was created (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	CreatedAt time.Time `json:"createdAt" example:"2021-01-01T00:00:00Z" format:"date-time"`

	// Timestamp when the balance was last updated (RFC3339 format)
	// example: 2021-01-01T00:00:00Z
	// format: date-time
	UpdatedAt time.Time `json:"updatedAt" example:"2021-01-01T00:00:00Z" format:"date-time"`
} // @name BalanceHistory

// ToHistoryResponse converts a Balance to BalanceHistory (without permission flags).
// Settings are deep-copied so the history snapshot is fully independent of the
// live balance — mutations on either side cannot affect the other.
func (b *Balance) ToHistoryResponse() *BalanceHistory {
	return &BalanceHistory{
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
		Direction:      b.Direction,
		OverdraftUsed:  b.OverdraftUsed,
		Settings:       deepCopySettings(b.Settings),
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
	}
}

// deepCopySettings returns an independent copy of the given BalanceSettings,
// including the inner OverdraftLimit pointer. Returns nil when src is nil.
func deepCopySettings(src *BalanceSettings) *BalanceSettings {
	if src == nil {
		return nil
	}

	cp := *src

	if src.OverdraftLimit != nil {
		v := *src.OverdraftLimit
		cp.OverdraftLimit = &v
	}

	return &cp
}

// ToTransactionBalance converts mmodel.Balance to mtransaction.Balance,
// flattening the optional Settings into individual fields.
//
// Returns an error when Settings.OverdraftLimit is non-nil but cannot be
// parsed as a decimal. Callers must surface this error rather than continue
// with a silently-zeroed limit, because a corrupted limit combined with
// OverdraftLimitEnabled=true would otherwise admit an unbounded overdraft
// authorization at the validation/Lua boundary. Validate() prevents creation
// of invalid limits, so this only triggers on data corruption (manual DB
// edits, migration bugs) — fail closed.
func (b *Balance) ToTransactionBalance() (*mtransaction.Balance, error) {
	result := &mtransaction.Balance{
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
		Direction:      b.Direction,
		OverdraftUsed:  b.OverdraftUsed,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
		DeletedAt:      b.DeletedAt,
		Metadata:       b.Metadata,
	}

	if b.Settings != nil {
		result.AllowOverdraft = b.Settings.AllowOverdraft
		result.OverdraftLimitEnabled = b.Settings.OverdraftLimitEnabled
		result.BalanceScope = b.Settings.BalanceScope

		if b.Settings.OverdraftLimit != nil {
			lim, err := decimal.NewFromString(*b.Settings.OverdraftLimit)
			if err != nil {
				return nil, fmt.Errorf("invalid OverdraftLimit %q on balance %s: %w", *b.Settings.OverdraftLimit, b.ID, err)
			}

			result.OverdraftLimit = lim
		}
	}

	return result, nil
}

// CreateAdditionalBalance is a struct designed to encapsulate balance create request payload data.
//
// swagger:model CreateAdditionalBalance
// @Description Request payload for creating a new balance with specified permissions and custom key.
type CreateAdditionalBalance struct {
	// Unique key for the balance
	// required: true
	// maxLength: 100
	// example: asset-freeze
	Key string `json:"key" validate:"required,nowhitespaces,max=100" example:"asset-freeze"`
	// Whether the account should be allowed to send funds from this balance
	// required: false
	// example: true
	AllowSending *bool `json:"allowSending" example:"true"`

	// Whether the account should be allowed to receive funds to this balance
	// required: false
	// example: true
	AllowReceiving *bool `json:"allowReceiving" example:"true"`

	// Direction is the accounting direction of the balance ("credit" or
	// "debit"). Optional at creation; when omitted, defaults to "credit".
	// required: false
	// example: credit
	Direction *string `json:"direction,omitempty" example:"credit"`

	// Settings is the optional per-balance configuration (overdraft,
	// balance scope). When omitted, platform defaults are applied.
	// required: false
	Settings *BalanceSettings `json:"settings,omitempty"`
} // @name CreateAdditionalBalance

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

	// Settings is the per-balance configuration (overdraft, balance
	// scope). When provided, replaces the existing settings in full.
	// Direction is intentionally absent: it is immutable after creation.
	// required: false
	Settings *BalanceSettings `json:"settings,omitempty"`
} // @name UpdateBalance

// CreateBalanceInput is the input model used by services to create a balance synchronously.
//
// It centralizes all properties required to perform validations and persist the new balance,
// keeping call sites simple and reducing the chance of inconsistent argument ordering.
type CreateBalanceInput struct {
	// Request ID for tracing
	// example: 123e4567-e89b-12d3-a456-426614174000
	// format: string uuid
	RequestID string

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
//
// CACHE JSON CASING CONTRACT: the Redis balance entry is a JSON string whose
// keys are CamelCase (e.g. "Available", "Direction", "AllowOverdraft") because
// the original writer is the Lua atomic script (cjson.encode on a table with
// CamelCase keys) and Lua table access is case-sensitive. Every Go writer to
// the same key MUST emit CamelCase. If a Go writer uses the default BalanceRedis
// struct tags (which are camelCase: "available", "direction", etc.), the next
// Lua cjson.decode will see balance.Available == nil and arithmetic helpers
// will fail with "attempt to compare nil with number".
//
// The canonical Go writer that respects this contract is
// UpdateBalanceCacheSettings in adapters/redis/transaction/consumer.redis.go,
// which operates on map[string]any with explicit CamelCase keys and uses the
// luaBalanceSettingKey helper to purge legacy camelCase aliases. Any new Go
// writer to the balance cache MUST follow the same pattern — do NOT marshal
// BalanceRedis directly; use map[string]any with CamelCase keys.
type BalanceRedis struct {
	// Unique identifier for the balance (UUID format)
	ID string `json:"id"`

	// Alias for the account, used for easy identification or tagging
	// example: @person1
	// maxLength: 256
	Alias string `json:"alias" example:"@person1" maxLength:"256"`

	// Unique key for the balance (defaults to "default" if not provided)
	// example: default
	// maxLength: 100
	Key string `json:"key" example:"default" maxLength:"100"`

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

	// Accounting direction of the balance ("credit" or "debit")
	Direction string `json:"direction"`

	// Amount of overdraft currently consumed (decimal string for Lua)
	OverdraftUsed string `json:"overdraftUsed"`

	// Whether overdraft is allowed (1=true, 0=false for Lua)
	AllowOverdraft int `json:"allowOverdraft"`

	// Whether the overdraft limit is enabled (1=true, 0=false for Lua)
	OverdraftLimitEnabled int `json:"overdraftLimitEnabled"`

	// Maximum overdraft amount (decimal string for Lua)
	OverdraftLimit string `json:"overdraftLimit"`

	// Balance scope ("transactional" or "internal")
	BalanceScope string `json:"balanceScope"`
}

// UnmarshalJSON is a custom unmarshal function for BalanceRedis
func (b *BalanceRedis) UnmarshalJSON(data []byte) error {
	type Alias BalanceRedis

	aux := struct {
		Available     any `json:"available"`
		OnHold        any `json:"onHold"`
		OverdraftUsed any `json:"overdraftUsed"`
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

	b.OverdraftUsed = utils.ParseDecimalString(aux.OverdraftUsed, "0")

	if b.OverdraftLimit == "" {
		b.OverdraftLimit = "0"
	}

	// Set default value for Key if not provided (backwards compatibility)
	if b.Key == "" {
		b.Key = constant.DefaultBalanceKey
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
	Amount      mtransaction.Amount
	InternalKey string
}

// BalanceAtomicResult holds the before and after states returned by the
// Lua atomic balance operation script. Before contains pre-mutation snapshots
// (used by BuildOperations for operation records). After contains post-mutation
// states (used by UpdateBalances for PostgreSQL persistence).
type BalanceAtomicResult struct {
	Before []*Balance
	After  []*Balance
}

// TransactionRedisQueue represents a transaction queue for cache-aside
type TransactionRedisQueue struct {
	HeaderID          string                   `json:"header_id"`
	TransactionID     uuid.UUID                `json:"transaction_id"`
	OrganizationID    uuid.UUID                `json:"organization_id"`
	LedgerID          uuid.UUID                `json:"ledger_id"`
	Balances          []BalanceRedis           `json:"balances"`
	BalancesAfter     []BalanceRedis           `json:"balancesAfter,omitempty"`
	TransactionInput  mtransaction.Transaction `json:"parserDSL"`
	TTL               time.Time                `json:"ttl"`
	Validate          *mtransaction.Responses  `json:"validate"`
	TransactionStatus string                   `json:"transaction_status"`
	Action            string                   `json:"action,omitempty"`
	TransactionDate   time.Time                `json:"transaction_date"`
	Operations        []OperationRedis         `json:"operations,omitempty"`
}
