// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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

	// Blocked mirrors the owning account's blocked flag (accounts.blocked).
	// Account-level cache-only state used by the transaction flow; excluded
	// from JSON so balance API responses keep their contract — the account
	// resource is the public surface for the blocked flag.
	Blocked bool `json:"-"`

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
		Blocked:        b.Blocked,
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
	// Setting allowOverdraft=true provisions the system-managed "overdraft"
	// companion balance for this account.
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
// CACHE JSON CASING CONTRACT: the shared Go balance-cache codec owns lossless
// dual encoding and decoding. CamelCase keys (e.g. "Available", "Direction",
// "AllowOverdraft") remain the legacy representation consumed by the active
// Lua atomic writer; lowerCamel keys are its dual representation. Do not
// marshal BalanceRedis directly into the cache: its JSON tags are the new
// lowerCamel representation, not the complete dual wire format.
//
// Settings writes use scripts/update_balance_settings.lua through
// UpdateBalanceCacheSettings in adapters/redis/transaction/consumer.redis.go.
// They preserve live monetary state and emit the legacy fields required by the
// accounting script. The shared codec owns the dual wire format.
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

	// Whether the owning account is blocked (1=true, 0=false for Lua).
	// Legacy blobs lack the field and decode as 0 (not blocked).
	Blocked int `json:"blocked"`

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
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	if fields == nil {
		return fmt.Errorf("type unsuported to available: <nil>")
	}

	schemaVersion, err := decodeBalanceRedisSchemaVersion(fields)
	if err != nil {
		return err
	}

	if err := decodeBalanceRedisTextFields(fields, b); err != nil {
		return err
	}

	if err := decodeBalanceRedisDecimals(fields, b); err != nil {
		return err
	}

	if err := decodeBalanceRedisVersion(fields, schemaVersion, b); err != nil {
		return err
	}

	if err := decodeBalanceRedisFlags(fields, schemaVersion, b); err != nil {
		return err
	}

	if b.OverdraftLimit == "" {
		b.OverdraftLimit = "0"
	}

	// Set default value for Key if not provided (backwards compatibility)
	if b.Key == "" {
		b.Key = constant.DefaultBalanceKey
	}

	return nil
}

func decodeBalanceRedisSchemaVersion(fields map[string]json.RawMessage) (int64, error) {
	raw, _ := selectBalanceRedisField(fields, "SchemaVersion", "schemaVersion")
	if raw == nil {
		return 0, nil
	}

	var schemaVersion int64
	if err := json.Unmarshal(raw, &schemaVersion); err != nil {
		return 0, fmt.Errorf("invalid balance schema version: %w", err)
	}

	return schemaVersion, nil
}

func decodeBalanceRedisTextFields(fields map[string]json.RawMessage, balance *BalanceRedis) error {
	for _, field := range []struct {
		upper string
		lower string
		dst   any
	}{
		{upper: "ID", lower: "id", dst: &balance.ID},
		{upper: "Alias", lower: "alias", dst: &balance.Alias},
		{upper: "Key", lower: "key", dst: &balance.Key},
		{upper: "AccountID", lower: "accountId", dst: &balance.AccountID},
		{upper: "AssetCode", lower: "assetCode", dst: &balance.AssetCode},
		{upper: "AccountType", lower: "accountType", dst: &balance.AccountType},
		{upper: "Direction", lower: "direction", dst: &balance.Direction},
		{upper: "OverdraftLimit", lower: "overdraftLimit", dst: &balance.OverdraftLimit},
		{upper: "BalanceScope", lower: "balanceScope", dst: &balance.BalanceScope},
	} {
		raw, _ := selectBalanceRedisField(fields, field.upper, field.lower)
		if raw == nil {
			continue
		}

		if err := json.Unmarshal(raw, field.dst); err != nil {
			return fmt.Errorf("invalid %s field: %w", field.lower, err)
		}
	}

	return nil
}

func decodeBalanceRedisDecimals(fields map[string]json.RawMessage, balance *BalanceRedis) error {
	available, err := decodeRequiredBalanceRedisDecimal(fields, "Available", "available")
	if err != nil {
		return err
	}

	onHold, err := decodeRequiredBalanceRedisDecimal(fields, "OnHold", "onHold")
	if err != nil {
		return err
	}

	overdraftRaw, _ := selectBalanceRedisField(fields, "OverdraftUsed", "overdraftUsed")

	overdraftUsed, err := parseBalanceRedisDecimalString(overdraftRaw, "overdraftUsed")
	if err != nil {
		return err
	}

	balance.Available = available
	balance.OnHold = onHold
	balance.OverdraftUsed = overdraftUsed

	return nil
}

func decodeRequiredBalanceRedisDecimal(fields map[string]json.RawMessage, upper, lower string) (decimal.Decimal, error) {
	raw, _ := selectBalanceRedisField(fields, upper, lower)
	if raw == nil {
		return decimal.Zero, fmt.Errorf("type unsuported to %s: <nil>", lower)
	}

	return parseBalanceRedisDecimal(raw, lower)
}

func decodeBalanceRedisVersion(fields map[string]json.RawMessage, schemaVersion int64, balance *BalanceRedis) error {
	raw, isUpper := selectBalanceRedisField(fields, "Version", "version")
	if raw == nil {
		return nil
	}

	if schemaVersion != 2 || isUpper {
		if err := json.Unmarshal(raw, &balance.Version); err != nil {
			return fmt.Errorf("invalid version field: %w", err)
		}

		return nil
	}

	var version string
	if err := json.Unmarshal(raw, &version); err != nil {
		return fmt.Errorf("invalid version field: %w", err)
	}

	parsed, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid version field: %w", err)
	}

	balance.Version = parsed

	return nil
}

func decodeBalanceRedisFlags(fields map[string]json.RawMessage, schemaVersion int64, balance *BalanceRedis) error {
	for _, flag := range []struct {
		upper string
		lower string
		dst   *int
	}{
		{upper: "AllowSending", lower: "allowSending", dst: &balance.AllowSending},
		{upper: "AllowReceiving", lower: "allowReceiving", dst: &balance.AllowReceiving},
		{upper: "Blocked", lower: "blocked", dst: &balance.Blocked},
		{upper: "AllowOverdraft", lower: "allowOverdraft", dst: &balance.AllowOverdraft},
		{upper: "OverdraftLimitEnabled", lower: "overdraftLimitEnabled", dst: &balance.OverdraftLimitEnabled},
	} {
		if err := decodeBalanceRedisFlag(fields, schemaVersion, flag.upper, flag.lower, flag.dst); err != nil {
			return err
		}
	}

	return nil
}

func decodeBalanceRedisFlag(fields map[string]json.RawMessage, schemaVersion int64, upper, lower string, dst *int) error {
	raw, isUpper := selectBalanceRedisField(fields, upper, lower)
	if raw == nil {
		return nil
	}

	if schemaVersion != 2 || isUpper {
		if err := json.Unmarshal(raw, dst); err != nil {
			return fmt.Errorf("invalid %s field: %w", lower, err)
		}

		return nil
	}

	var value *bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("invalid %s field: %w", lower, err)
	}

	if value == nil {
		return fmt.Errorf("invalid %s field: null", lower)
	}

	*dst = 0
	if *value {
		*dst = 1
	}

	return nil
}

func selectBalanceRedisField(fields map[string]json.RawMessage, upper, lower string) (json.RawMessage, bool) {
	if raw, ok := fields[upper]; ok {
		return raw, true
	}

	return fields[lower], false
}

func parseBalanceRedisDecimal(raw json.RawMessage, field string) (decimal.Decimal, error) {
	var value string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return decimal.Zero, fmt.Errorf("err to converter %s field from string to decimal: %w", field, err)
		}
	} else {
		value = string(raw)
	}

	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero, fmt.Errorf("err to converter %s field to decimal: %w", field, err)
	}

	return parsed, nil
}

func parseBalanceRedisDecimalString(raw json.RawMessage, field string) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "0", nil
	}

	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return "", fmt.Errorf("err to converter %s field from string to decimal: %w", field, err)
		}

		if value == "" {
			return "0", nil
		}

		if _, err := decimal.NewFromString(value); err != nil {
			return "", fmt.Errorf("err to converter %s field to decimal: %w", field, err)
		}

		return value, nil
	}

	value, err := decimal.NewFromString(string(trimmed))
	if err != nil {
		return "", fmt.Errorf("err to converter %s field to decimal: %w", field, err)
	}

	return value.String(), nil
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
