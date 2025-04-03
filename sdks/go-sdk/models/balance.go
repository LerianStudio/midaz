package models

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Balance represents an account balance in the Midaz system.
// A balance tracks the available and on-hold amounts for a specific account and asset.
// Balances are used to determine the current state of funds in an account and
// to enforce constraints on transactions.
type Balance struct {
	// ID is the unique identifier for the balance
	ID string `json:"id"`

	// OrganizationID is the ID of the organization that owns this balance
	OrganizationID string `json:"organizationId"`

	// LedgerID is the ID of the ledger that contains this balance
	LedgerID string `json:"ledgerId"`

	// AccountID is the ID of the account this balance belongs to
	AccountID string `json:"accountId"`

	// Alias is a human-friendly identifier for the account
	Alias string `json:"alias"`

	// AssetCode identifies the type of asset for this balance
	AssetCode string `json:"assetCode"`

	// Available is the amount available for use in the account
	// The actual value is Available/Scale (e.g., 1000/100 = 10.00)
	Available int64 `json:"available"`

	// OnHold is the amount that is reserved but not yet settled
	// The actual value is OnHold/Scale (e.g., 500/100 = 5.00)
	OnHold int64 `json:"onHold"`

	// Scale is the divisor to convert the integer amounts to decimal values
	// For example, a scale of 100 means the values are stored as cents
	Scale int64 `json:"scale"`

	// Version is the optimistic concurrency control version number
	Version int64 `json:"version"`

	// AccountType defines the type of the account (e.g., "ASSET", "LIABILITY")
	AccountType string `json:"accountType"`

	// AllowSending indicates whether the account can send funds
	AllowSending bool `json:"allowSending"`

	// AllowReceiving indicates whether the account can receive funds
	AllowReceiving bool `json:"allowReceiving"`

	// CreatedAt is the timestamp when the balance was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the balance was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the balance was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// Metadata contains additional custom data associated with the balance
	Metadata map[string]any `json:"metadata,omitempty"`
}

// UpdateBalanceInput represents input for updating a balance.
// This structure contains the fields that can be modified when updating an existing balance.
type UpdateBalanceInput struct {
	// AllowSending indicates whether to allow sending funds from the account
	// If nil, the current value is preserved
	AllowSending *bool `json:"allowSending,omitempty"`

	// AllowReceiving indicates whether to allow receiving funds to the account
	// If nil, the current value is preserved
	AllowReceiving *bool `json:"allowReceiving,omitempty"`
}

// Validate checks that the UpdateBalanceInput meets all validation requirements.
// For UpdateBalanceInput, we simply need to ensure that at least one field is set.
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (input *UpdateBalanceInput) Validate() error {
	// Check if at least one field is set
	if input.AllowSending == nil && input.AllowReceiving == nil {
		return fmt.Errorf("at least one field must be set (allowSending or allowReceiving)")
	}

	return nil
}

// NewUpdateBalanceInput creates a new empty UpdateBalanceInput.
// This constructor initializes an empty update input that can be customized
// using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdateBalanceInput
func NewUpdateBalanceInput() *UpdateBalanceInput {
	return &UpdateBalanceInput{}
}

// WithAllowSending sets whether sending is allowed.
// This controls whether funds can be sent from the account.
//
// Parameters:
//   - allowed: Whether to allow sending funds
//
// Returns:
//   - A pointer to the modified UpdateBalanceInput for method chaining
func (u *UpdateBalanceInput) WithAllowSending(allowed bool) *UpdateBalanceInput {
	u.AllowSending = &allowed
	return u
}

// WithAllowReceiving sets whether receiving is allowed.
// This controls whether funds can be received into the account.
//
// Parameters:
//   - allowed: Whether to allow receiving funds
//
// Returns:
//   - A pointer to the modified UpdateBalanceInput for method chaining
func (u *UpdateBalanceInput) WithAllowReceiving(allowed bool) *UpdateBalanceInput {
	u.AllowReceiving = &allowed
	return u
}

// ToMmodelUpdateBalance converts an SDK UpdateBalanceInput to a backend UpdateBalance.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdateBalance instance with the same values
func (u *UpdateBalanceInput) ToMmodelUpdateBalance() mmodel.UpdateBalance {
	return mmodel.UpdateBalance{
		AllowSending:   u.AllowSending,
		AllowReceiving: u.AllowReceiving,
	}
}

// Balances represents a collection of balances with pagination.
// This structure is used for paginated responses when listing balances.
type Balances struct {
	// Items is the collection of balances in the current page
	Items []Balance `json:"items"`

	// Page is the current page number
	Page int `json:"page"`

	// Limit is the maximum number of items per page
	Limit int `json:"limit"`
}

// FromMmodelBalance converts a backend Balance to an SDK Balance.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - balance: The mmodel.Balance to convert
//
// Returns:
//   - A models.Balance instance with the same values
func FromMmodelBalance(balance mmodel.Balance) Balance {
	result := Balance{
		ID:             balance.ID,
		OrganizationID: balance.OrganizationID,
		LedgerID:       balance.LedgerID,
		AccountID:      balance.AccountID,
		Alias:          balance.Alias,
		AssetCode:      balance.AssetCode,
		Available:      balance.Available,
		OnHold:         balance.OnHold,
		Scale:          balance.Scale,
		Version:        balance.Version,
		AccountType:    balance.AccountType,
		AllowSending:   balance.AllowSending,
		AllowReceiving: balance.AllowReceiving,
		CreatedAt:      balance.CreatedAt,
		UpdatedAt:      balance.UpdatedAt,
		Metadata:       balance.Metadata,
	}

	if balance.DeletedAt != nil {
		deletedAt := *balance.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// ToMmodelBalance converts an SDK Balance to a backend Balance.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Balance instance with the same values
func (b *Balance) ToMmodelBalance() mmodel.Balance {
	result := mmodel.Balance{
		ID:             b.ID,
		OrganizationID: b.OrganizationID,
		LedgerID:       b.LedgerID,
		AccountID:      b.AccountID,
		Alias:          b.Alias,
		AssetCode:      b.AssetCode,
		Available:      b.Available,
		OnHold:         b.OnHold,
		Scale:          b.Scale,
		Version:        b.Version,
		AccountType:    b.AccountType,
		AllowSending:   b.AllowSending,
		AllowReceiving: b.AllowReceiving,
		CreatedAt:      b.CreatedAt,
		UpdatedAt:      b.UpdatedAt,
		Metadata:       b.Metadata,
	}

	if b.DeletedAt != nil {
		deletedAt := *b.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// FromMmodelBalances converts a backend Balances to an SDK Balances.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - balances: The mmodel.Balances to convert
//
// Returns:
//   - A models.Balances instance with the same values
func FromMmodelBalances(balances mmodel.Balances) Balances {
	result := Balances{
		Page:  balances.Page,
		Limit: balances.Limit,
		Items: make([]Balance, 0, len(balances.Items)),
	}

	for _, balance := range balances.Items {
		result.Items = append(result.Items, FromMmodelBalance(balance))
	}

	return result
}
