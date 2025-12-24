package mmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
)

var (
	// ErrConvertAvailableToDecimal is returned when converting available field to decimal fails
	ErrConvertAvailableToDecimal = errors.New("failed to convert available field to decimal")
	// ErrUnsupportedAvailableType is returned when available field type is unsupported
	ErrUnsupportedAvailableType = errors.New("unsupported type for available field")
	// ErrConvertOnHoldToDecimal is returned when converting onHold field to decimal fails
	ErrConvertOnHoldToDecimal = errors.New("failed to convert onHold field to decimal")
	// ErrUnsupportedOnHoldType is returned when onHold field type is unsupported
	ErrUnsupportedOnHoldType = errors.New("unsupported type for onHold field")
	// ErrUnmarshalBalanceRedis is returned when unmarshaling balance redis data fails
	ErrUnmarshalBalanceRedis = errors.New("failed to unmarshal balance redis")
)

// BalanceError wraps a sentinel error with an underlying cause
type BalanceError struct {
	Sentinel error
	Cause    error
	Context  string
}

// Error returns the error message
func (e BalanceError) Error() string {
	if e.Context != "" && e.Cause != nil {
		return fmt.Sprintf("%s %s: %v", e.Sentinel.Error(), e.Context, e.Cause)
	}

	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Sentinel.Error(), e.Cause)
	}

	return e.Sentinel.Error()
}

// Unwrap returns the underlying errors for errors.Is/As
func (e BalanceError) Unwrap() []error {
	if e.Cause != nil {
		return []error{e.Sentinel, e.Cause}
	}

	return []error{e.Sentinel}
}

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
	assert.That(assert.ValidUUID(b.ID),
		"balance ID must be valid UUID",
		"value", b.ID)

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

	// Unique key for the balance
	Key string `json:"key"`
}

// ConvertBalancesToTransactionBalances converts []*Balance to []*pkgTransaction.Balance
func ConvertBalancesToTransactionBalances(balances []*Balance) []*pkgTransaction.Balance {
	out := make([]*pkgTransaction.Balance, 0, len(balances))

	for _, b := range balances {
		if b != nil {
			out = append(out, b.ToTransactionBalance())
		}
	}

	return out
}

// ConvertBalanceOperationsToTransactionBalances converts []*BalanceOperation to []*pkgTransaction.Balance
func ConvertBalanceOperationsToTransactionBalances(operations []BalanceOperation) []*pkgTransaction.Balance {
	out := make([]*pkgTransaction.Balance, 0, len(operations))
	for _, op := range operations {
		out = append(out, op.Balance.ToTransactionBalance())
	}

	return out
}

// ToTransactionBalance converts mmodel.Balance to pkgTransaction.Balance
func (b *Balance) ToTransactionBalance() *pkgTransaction.Balance {
	return &pkgTransaction.Balance{
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
		return BalanceError{Sentinel: ErrUnmarshalBalanceRedis, Cause: err}
	}

	var err error

	b.Available, err = convertToDecimal(aux.Available, ErrConvertAvailableToDecimal, ErrUnsupportedAvailableType)
	if err != nil {
		return err
	}

	b.OnHold, err = convertToDecimal(aux.OnHold, ErrConvertOnHoldToDecimal, ErrUnsupportedOnHoldType)
	if err != nil {
		return err
	}

	return nil
}

// convertToDecimal converts various types to decimal.Decimal
func convertToDecimal(value any, conversionErr, unsupportedErr error) (decimal.Decimal, error) {
	switch v := value.(type) {
	case float64:
		return decimal.NewFromFloat(v), nil
	case string:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return decimal.Decimal{}, BalanceError{Sentinel: conversionErr, Cause: err, Context: "from string"}
		}

		return d, nil
	case json.Number:
		return convertJSONNumberToDecimal(v, conversionErr)
	default:
		f, ok := v.(float64)
		if !ok {
			return decimal.Decimal{}, BalanceError{Sentinel: unsupportedErr, Context: fmt.Sprintf("%T", v)}
		}

		return decimal.NewFromFloat(f), nil
	}
}

// convertJSONNumberToDecimal converts json.Number to decimal.Decimal
func convertJSONNumberToDecimal(num json.Number, conversionErr error) (decimal.Decimal, error) {
	i, err := num.Int64()
	if err != nil {
		f, err := num.Float64()
		if err != nil {
			return decimal.Decimal{}, BalanceError{Sentinel: conversionErr, Cause: err, Context: "from json.Number"}
		}

		return decimal.NewFromFloat(f), nil
	}

	return decimal.NewFromInt(i), nil
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
	Amount      pkgTransaction.Amount
	InternalKey string
}

// TransactionRedisQueue represents a transaction queue for cache-aside
type TransactionRedisQueue struct {
	HeaderID          string                     `json:"header_id"`
	TransactionID     uuid.UUID                  `json:"transaction_id"`
	OrganizationID    uuid.UUID                  `json:"organization_id"`
	LedgerID          uuid.UUID                  `json:"ledger_id"`
	Balances          []BalanceRedis             `json:"balances"`
	ParserDSL         pkgTransaction.Transaction `json:"parserDSL"`
	TTL               time.Time                  `json:"ttl"`
	Validate          *pkgTransaction.Responses  `json:"validate"`
	TransactionStatus string                     `json:"transaction_status"`
	TransactionDate   time.Time                  `json:"transaction_date"`
}
