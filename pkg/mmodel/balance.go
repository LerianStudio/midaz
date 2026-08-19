// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Balance is a struct designed to encapsulate response payload data.
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
}

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
}

// UpdateBalance is a struct designed to encapsulate balance update request payload data.
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
}

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

	// DefaultDirection is the account type's configured default balance
	// direction ("credit" or "debit"), resolved by the caller. Empty means
	// the type has no configured default; the direction then falls back to
	// the account-type-implied default (external -> debit, others -> credit).
	DefaultDirection string

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
}

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

// ToRedis returns the complete economic snapshot consumed and emitted by the
// atomic balance Lua script. Defaults deliberately match the script's ARGV
// plan so a Rabbit redelivery can compare its decoded model with the
// authoritative Redis envelope without treating omitted legacy settings as a
// different economic fact.
func (b *Balance) ToRedis() BalanceRedis {
	allowSending := 0
	if b.AllowSending {
		allowSending = 1
	}
	allowReceiving := 0
	if b.AllowReceiving {
		allowReceiving = 1
	}
	allowOverdraft := 0
	overdraftLimitEnabled := 0
	overdraftLimit := "0"
	balanceScope := BalanceScopeTransactional
	if b.Settings != nil {
		if b.Settings.AllowOverdraft {
			allowOverdraft = 1
		}
		if b.Settings.OverdraftLimitEnabled {
			overdraftLimitEnabled = 1
		}
		if b.Settings.OverdraftLimit != nil {
			overdraftLimit = *b.Settings.OverdraftLimit
		}
		if b.Settings.BalanceScope != "" {
			balanceScope = b.Settings.BalanceScope
		}
	}

	return BalanceRedis{
		ID: b.ID, Alias: b.Alias, Key: b.Key, AccountID: b.AccountID, AssetCode: b.AssetCode,
		Available: b.Available, OnHold: b.OnHold, Version: b.Version, AccountType: b.AccountType,
		AllowSending: allowSending, AllowReceiving: allowReceiving, Direction: b.Direction,
		OverdraftUsed: b.OverdraftUsed.String(), AllowOverdraft: allowOverdraft,
		OverdraftLimitEnabled: overdraftLimitEnabled, OverdraftLimit: overdraftLimit,
		BalanceScope: balanceScope,
	}
}

// BalancesToRedis converts a complete model snapshot to the Lua/cache wire
// shape. A nil member is invalid economic evidence and therefore returns nil;
// callers' non-empty proof checks fail closed.
func BalancesToRedis(balances []*Balance) []BalanceRedis {
	result := make([]BalanceRedis, 0, len(balances))
	for _, balance := range balances {
		if balance == nil {
			return nil
		}
		result = append(result, balance.ToRedis())
	}

	return result
}

// RedisBalanceSetEconomicEqual compares the complete, order-independent
// balance fact copied into the transaction backup and immutable outcome by the
// same Lua command. One transaction may touch the same balance more than once
// (for example principal and fee settlement), so this is an exact multiset of
// snapshots rather than a map keyed by balance identity.
func RedisBalanceSetEconomicEqual(left, right []BalanceRedis) bool {
	if len(left) != len(right) {
		return false
	}

	used := make([]bool, len(right))
	for _, candidate := range left {
		matched := false
		for index, canonical := range right {
			if used[index] || !redisBalanceEconomicEqual(candidate, canonical) {
				continue
			}
			used[index] = true
			matched = true
			break
		}
		if !matched {
			return false
		}
	}

	return true
}

// RedisBalanceSetEconomicComplete rejects a terminal snapshot that cannot
// carry every money-path discriminator needed for replay/reconciliation. Zero
// amounts and versions are valid; missing identities, policy fields, or
// non-boolean wire flags are not.
func RedisBalanceSetEconomicComplete(balances []BalanceRedis) bool {
	if len(balances) == 0 {
		return false
	}
	for _, balance := range balances {
		if balance.ID == "" || balance.Key == "" || balance.AccountID == "" || balance.AssetCode == "" ||
			balance.AccountType == "" || balance.Direction == "" || balance.OverdraftUsed == "" ||
			balance.OverdraftLimit == "" || balance.BalanceScope == "" ||
			(balance.AllowSending != 0 && balance.AllowSending != 1) ||
			(balance.AllowReceiving != 0 && balance.AllowReceiving != 1) ||
			(balance.AllowOverdraft != 0 && balance.AllowOverdraft != 1) ||
			(balance.OverdraftLimitEnabled != 0 && balance.OverdraftLimitEnabled != 1) {
			return false
		}
	}

	return true
}

func redisBalanceEconomicEqual(left, right BalanceRedis) bool {
	return left.ID == right.ID && left.Alias == right.Alias && left.Key == right.Key &&
		left.AccountID == right.AccountID && left.AssetCode == right.AssetCode &&
		left.Available.Equal(right.Available) && left.OnHold.Equal(right.OnHold) &&
		left.Version == right.Version && left.AccountType == right.AccountType &&
		left.AllowSending == right.AllowSending && left.AllowReceiving == right.AllowReceiving &&
		left.Direction == right.Direction && redisEconomicDecimalEqual(left.OverdraftUsed, right.OverdraftUsed) &&
		left.AllowOverdraft == right.AllowOverdraft && left.OverdraftLimitEnabled == right.OverdraftLimitEnabled &&
		redisEconomicDecimalEqual(left.OverdraftLimit, right.OverdraftLimit) && left.BalanceScope == right.BalanceScope
}

func redisEconomicDecimalEqual(left, right string) bool {
	if left == "" || right == "" {
		return left == right
	}
	leftDecimal, leftErr := decimal.NewFromString(left)
	rightDecimal, rightErr := decimal.NewFromString(right)

	return leftErr == nil && rightErr == nil && leftDecimal.Equal(rightDecimal)
}

// UnmarshalJSON is a custom unmarshal function for BalanceRedis
func (b *BalanceRedis) UnmarshalJSON(data []byte) error {
	type Alias BalanceRedis

	aux := struct {
		Available     json.RawMessage `json:"available"`
		OnHold        json.RawMessage `json:"onHold"`
		OverdraftUsed json.RawMessage `json:"overdraftUsed"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	available, err := parseExactJSONDecimal(aux.Available, "available", false)
	if err != nil {
		return err
	}
	b.Available = available

	onHold, err := parseExactJSONDecimal(aux.OnHold, "onHold", false)
	if err != nil {
		return err
	}
	b.OnHold = onHold

	overdraftUsed, err := exactJSONDecimalText(aux.OverdraftUsed, "overdraftUsed", true)
	if err != nil {
		return err
	}
	b.OverdraftUsed = overdraftUsed

	if b.OverdraftLimit == "" {
		b.OverdraftLimit = "0"
	}

	// Set default value for Key if not provided (backwards compatibility)
	if b.Key == "" {
		b.Key = constant.DefaultBalanceKey
	}

	return nil
}

func exactJSONDecimalText(raw json.RawMessage, field string, defaultZero bool) (string, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		if defaultZero {
			return "0", nil
		}

		return "", fmt.Errorf("%s field is required", field)
	}
	if value[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("decode %s decimal string: %w", field, err)
		}
	}
	if value == "" && defaultZero {
		return "0", nil
	}
	if _, err := decimal.NewFromString(value); err != nil {
		return "", fmt.Errorf("convert %s field to decimal: %w", field, err)
	}

	return value, nil
}

func parseExactJSONDecimal(raw json.RawMessage, field string, defaultZero bool) (decimal.Decimal, error) {
	value, err := exactJSONDecimalText(raw, field, defaultZero)
	if err != nil {
		return decimal.Zero, err
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("convert %s field to decimal: %w", field, err)
	}

	return parsed, nil
}

// BalanceErrorResponse represents an error response for balance operations.
type BalanceErrorResponse struct {
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
	Balance              *Balance
	Alias                string
	Amount               mtransaction.Amount
	InternalKey          string
	EconomicSide         string
	EconomicRole         string
	ExpectedEconomicPlan *ExpectedEconomicPlan
}

// BalanceAtomicResult holds the before and after states returned by the
// Lua atomic balance operation script. Before contains pre-mutation snapshots
// (used by BuildOperations for operation records). After contains post-mutation
// states (used by UpdateBalances for PostgreSQL persistence).
type BalanceAtomicResult struct {
	Before []*Balance
	After  []*Balance
}

const (
	TransactionOutcomeCommitted = "COMMITTED"
	TransactionOutcomeAborted   = "ABORTED"
)

// BalanceExecutionAttempt identifies one owner authorized to ask the balance
// Lua for an immutable economic outcome. The execution and outcome keys share
// the transaction hash slot with balances and the backup queue.
type BalanceExecutionAttempt struct {
	ExecutionKey    string
	OutcomeKey      string
	Owner           string
	Outcome         string
	Identity        uuid.UUID
	RedisGeneration string
}

// BalanceExecutionOutcome is the immutable fact written by the balance Lua in
// the same atomic command as the economic mutation. Before and After are the
// exact response replayed after a lost Redis response.
type BalanceExecutionOutcome struct {
	Identity            uuid.UUID      `json:"identity"`
	Outcome             string         `json:"outcome"`
	Owner               string         `json:"owner"`
	EconomicPlanVersion string         `json:"economic_plan_version"`
	EconomicPlanDigest  string         `json:"economic_plan_digest"`
	Before              []BalanceRedis `json:"before"`
	After               []BalanceRedis `json:"after"`
}

// TransactionEconomicContext is the caller's immutable view of the terminal
// transaction identity. Redis compares it with the Lua-authored envelope so a
// candidate cannot relabel the parent, lifecycle status, or action while
// binding operation IDs.
type TransactionEconomicContext struct {
	ParentTransactionID  *uuid.UUID
	TransactionStatus    string
	Action               string
	TransactionAmount    string
	TransactionAssetCode string
	Operations           []OperationRedis
	BalancesAfter        []BalanceRedis
}

// TransactionPersistenceTombstone is the append-only terminal receipt written
// in the same Redis command that removes a persisted transaction's backup and
// economic outcome. It lets lost-ack redelivery prove the exact generation,
// outcome, canonical operations, and after-balances without treating missing
// hot-store evidence as a successful replay.
type TransactionPersistenceTombstone struct {
	Identity              uuid.UUID             `json:"identity"`
	ParentTransactionID   string                `json:"parent_transaction_id"`
	Outcome               string                `json:"outcome"`
	Owner                 string                `json:"owner"`
	RedisGeneration       string                `json:"redis_generation"`
	TransactionStatus     string                `json:"transaction_status"`
	Action                string                `json:"action"`
	TransactionAmount     string                `json:"transaction_amount"`
	TransactionAssetCode  string                `json:"transaction_asset_code"`
	Operations            []OperationRedis      `json:"operations"`
	BalancesAfter         []BalanceRedis        `json:"balancesAfter"`
	EconomicEffectDigest  string                `json:"economic_effect_digest"`
	ExpectedEconomicPlan  *ExpectedEconomicPlan `json:"expected_economic_plan,omitempty"`
	OperationTypeOverride string                `json:"operation_type_override,omitempty"`
}

// TransactionRedisQueue represents a transaction queue for cache-aside
type TransactionRedisQueue struct {
	HeaderID              string                   `json:"header_id"`
	TransactionID         uuid.UUID                `json:"transaction_id"`
	ParentTransactionID   *uuid.UUID               `json:"parent_transaction_id,omitempty"`
	OrganizationID        uuid.UUID                `json:"organization_id"`
	LedgerID              uuid.UUID                `json:"ledger_id"`
	Balances              []BalanceRedis           `json:"balances"`
	BalancesAfter         []BalanceRedis           `json:"balancesAfter,omitempty"`
	TransactionInput      mtransaction.Transaction `json:"parserDSL"`
	TTL                   time.Time                `json:"ttl"`
	Validate              *mtransaction.Responses  `json:"validate"`
	TransactionStatus     string                   `json:"transaction_status"`
	Action                string                   `json:"action,omitempty"`
	EffectModeVersion     int                      `json:"effect_mode_version,omitempty"`
	EffectMode            TransactionEffectMode    `json:"effect_mode,omitempty"`
	OperationTypeOverride string                   `json:"operation_type_override,omitempty"`
	ExpectedEconomicPlan  *ExpectedEconomicPlan    `json:"expected_economic_plan,omitempty"`
	AttemptOwner          string                   `json:"attempt_owner,omitempty"`
	ExpectedOutcome       string                   `json:"expected_outcome,omitempty"`
	RevertRolloutMode     string                   `json:"revert_rollout_mode,omitempty"`
	RevertRolloutToken    string                   `json:"revert_rollout_token,omitempty"`
	RevertLegacyFenceKey  string                   `json:"revert_legacy_fence_key,omitempty"`
	RedisGeneration       string                   `json:"redis_generation,omitempty"`
	TransactionDate       time.Time                `json:"transaction_date"`
	Operations            []OperationRedis         `json:"operations,omitempty"`
	EconomicEffectDigest  string                   `json:"economic_effect_digest,omitempty"`
}
