package models

import (
	"fmt"
)

// Amount represents a monetary amount in the Midaz Ledger.
// It includes the value, scale for precision, and the asset code.
type Amount struct {
	// Value is the numeric value of the amount
	Value int64 `json:"value"`

	// Scale represents the decimal precision (e.g., 2 for cents)
	Scale int `json:"scale"`

	// AssetCode identifies the currency or asset type
	AssetCode string `json:"assetCode"`
}

// Operation represents an operation in a transaction.
// Operations are the individual accounting entries that make up a transaction,
// typically representing debits and credits to accounts.
type Operation struct {
	// ID is the unique identifier for the operation
	ID string `json:"id,omitempty"`

	// Type indicates whether this is a debit or credit operation
	Type string `json:"type"`

	// AccountID is the identifier of the account affected by this operation
	AccountID string `json:"accountId,omitempty"`

	// Amount is the numeric value of the operation
	Amount int64 `json:"amount"`

	// Source contains information about the source account if applicable
	Source *Source `json:"source,omitempty"`

	// Destination contains information about the destination account if applicable
	Destination *Destination `json:"destination,omitempty"`

	// AssetCode identifies the currency or asset type for this operation
	AssetCode string `json:"assetCode"`

	// Scale represents the decimal precision for the amount
	Scale int `json:"scale"`

	// AccountAlias is an optional human-readable name for the account
	AccountAlias *string `json:"accountAlias"`
}

// OperationType represents the type of an operation.
// This is typically either a debit or credit in double-entry accounting.
type OperationType string

const (
	// OperationTypeDebit represents a debit operation.
	// In accounting, a debit typically increases asset and expense accounts,
	// and decreases liability, equity, and revenue accounts.
	OperationTypeDebit OperationType = "DEBIT"

	// OperationTypeCredit represents a credit operation.
	// In accounting, a credit typically increases liability, equity, and revenue accounts,
	// and decreases asset and expense accounts.
	OperationTypeCredit OperationType = "CREDIT"
)

// Source represents the source of an operation.
// This identifies where funds or assets are coming from in a transaction.
type Source struct {
	// ID is the unique identifier for the source account
	ID string `json:"id"`

	// Alias is an optional human-readable name for the source account
	Alias *string `json:"alias,omitempty"`

	// Destination indicates if this source is also a destination
	Destination bool `json:"destination"`
}

// Destination represents the destination of an operation.
// This identifies where funds or assets are going to in a transaction.
type Destination struct {
	// ID is the unique identifier for the destination account
	ID string `json:"id"`

	// Alias is an optional human-readable name for the destination account
	Alias *string `json:"alias,omitempty"`

	// Source indicates if this destination is also a source
	Source bool `json:"source"`
}

// CreateOperationInput is the input for creating an operation.
// This structure contains all the fields needed to create a new operation
// as part of a transaction.
type CreateOperationInput struct {
	// Type indicates whether this is a debit or credit operation
	Type string `json:"type"`

	// AccountID is the identifier of the account to be affected
	AccountID string `json:"accountId"`

	// Amount is the numeric value of the operation
	Amount int64 `json:"amount"`

	// AssetCode identifies the currency or asset type for this operation
	AssetCode string `json:"assetCode,omitempty"`

	// Scale represents the decimal precision for the amount
	Scale int `json:"scale,omitempty"`

	// AccountAlias is an optional human-readable name for the account
	AccountAlias *string `json:"accountAlias,omitempty"`
}

// Validate checks that the CreateOperationInput meets all validation requirements.
// It ensures that required fields are present and that all fields meet their
// validation constraints as defined in the API specification.
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (input *CreateOperationInput) Validate() error {
	// Validate required fields
	if input.Type == "" {
		return fmt.Errorf("type is required")
	}

	// Validate type is a valid operation type
	if input.Type != string(OperationTypeDebit) && input.Type != string(OperationTypeCredit) {
		return fmt.Errorf("type must be either %s or %s, got %s", OperationTypeDebit, OperationTypeCredit, input.Type)
	}

	if input.AccountID == "" {
		return fmt.Errorf("accountId is required")
	}

	// Validate amount
	if input.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0, got %d", input.Amount)
	}

	// Validate scale if provided
	if input.Scale < 0 {
		return fmt.Errorf("scale must be non-negative, got %d", input.Scale)
	}

	// Validate asset code if provided
	if input.AssetCode == "" {
		return fmt.Errorf("assetCode is required")
	}

	return nil
}

// FromMmodelOperation converts an mmodel Operation (if it exists) to an SDK Operation.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - operation: The mmodel operation to convert, as a generic interface
//
// Returns:
//   - An Operation instance with values extracted from the input
func FromMmodelOperation(operation any) Operation {
	// Since we don't have access to the actual mmodel Operation struct,
	// we'll create a basic conversion based on what we know about the operation structure
	op, ok := operation.(map[string]any)

	if !ok {
		// Return empty operation if conversion fails
		return Operation{}
	}

	var result Operation

	// Convert fields we know should be present
	if id, ok := op["id"].(string); ok {
		result.ID = id
	}

	if typ, ok := op["type"].(string); ok {
		result.Type = typ
	}

	if accountID, ok := op["accountId"].(string); ok {
		result.AccountID = accountID
	}

	if amount, ok := op["amount"].(float64); ok {
		result.Amount = int64(amount)
	}

	if assetCode, ok := op["assetCode"].(string); ok {
		result.AssetCode = assetCode
	}

	if scale, ok := op["scale"].(float64); ok {
		result.Scale = int(scale)
	}

	if alias, ok := op["accountAlias"].(string); ok {
		result.AccountAlias = &alias
	}

	return result
}
