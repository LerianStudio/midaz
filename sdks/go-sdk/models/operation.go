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
//
// In double-entry accounting, each transaction consists of at least two operations:
// one or more debits and one or more credits. The sum of all debits must equal the
// sum of all credits for the transaction to be balanced.
//
// Operations have the following characteristics:
//   - Type: Either "debit" or "credit"
//   - Account: The account affected by the operation
//   - Amount: The value of the operation
//   - Asset: The currency or asset type involved
//
// Common Use Cases:
//   - Recording financial transactions (payments, transfers, etc.)
//   - Tracking account activity and history
//   - Generating financial reports and statements
//   - Auditing and reconciliation
//
// Double-Entry Accounting Rules:
//   - Asset accounts: Debits increase, credits decrease
//   - Liability accounts: Debits decrease, credits increase
//   - Equity accounts: Debits decrease, credits increase
//   - Revenue accounts: Debits decrease, credits increase
//   - Expense accounts: Debits increase, credits decrease
//
// Example - Payment Transaction:
//
//	// A payment transaction typically involves:
//	// 1. Debit to an expense account (increase expense)
//	// 2. Credit to a cash/bank account (decrease asset)
//
//	// Debit operation (expense)
//	debitOp := Operation{
//	    Type:         "debit",
//	    AccountID:    "acc-expense-123",
//	    AccountAlias: stringPtr("expenses:office"),
//	    Amount:       5000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
//	// Credit operation (bank account)
//	creditOp := Operation{
//	    Type:         "credit",
//	    AccountID:    "acc-bank-456",
//	    AccountAlias: stringPtr("assets:bank"),
//	    Amount:       5000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
// Example - Transfer Transaction:
//
//	// A transfer between accounts typically involves:
//	// 1. Debit to the source account (decrease asset)
//	// 2. Credit to the destination account (increase asset)
//
//	// Debit operation (source account)
//	debitOp := Operation{
//	    Type:         "debit",
//	    AccountID:    "acc-savings-123",
//	    AccountAlias: stringPtr("assets:savings"),
//	    Amount:       10000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
//	// Credit operation (destination account)
//	creditOp := Operation{
//	    Type:         "credit",
//	    AccountID:    "acc-checking-456",
//	    AccountAlias: stringPtr("assets:checking"),
//	    Amount:       10000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
// Example - Revenue Transaction:
//
//	// A revenue transaction typically involves:
//	// 1. Debit to a cash/bank account (increase asset)
//	// 2. Credit to a revenue account (increase revenue)
//
//	// Debit operation (bank account)
//	debitOp := Operation{
//	    Type:         "debit",
//	    AccountID:    "acc-bank-123",
//	    AccountAlias: stringPtr("assets:bank"),
//	    Amount:       15000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
//	// Credit operation (revenue account)
//	creditOp := Operation{
//	    Type:         "credit",
//	    AccountID:    "acc-revenue-456",
//	    AccountAlias: stringPtr("revenue:sales"),
//	    Amount:       15000,
//	    Scale:        2,
//	    AssetCode:    "USD",
//	}
//
// Example usage:
//
//	// Accessing operation details
//	fmt.Printf("Operation Type: %s\n", operation.Type)
//	fmt.Printf("Account: %s\n", operation.AccountID)
//	if operation.AccountAlias != nil {
//	    fmt.Printf("Account Alias: %s\n", *operation.AccountAlias)
//	}
//	fmt.Printf("Amount: %d (scale: %d)\n", operation.Amount, operation.Scale)
//	fmt.Printf("Asset: %s\n", operation.AssetCode)
type Operation struct {
	// ID is the unique identifier for the operation
	// This is a system-generated UUID that uniquely identifies the operation
	ID string `json:"id,omitempty"`

	// Type indicates whether this is a debit or credit operation
	// Valid values are "debit" and "credit"
	Type string `json:"type"`

	// AccountID is the unique identifier of the account affected by this operation
	// This is the system-generated ID of the account
	AccountID string `json:"accountId,omitempty"`

	// Amount is the numeric value of the operation
	// This represents the value as a fixed-point integer
	// The actual amount is calculated as Amount / 10^Scale
	Amount int64 `json:"amount"`

	// Source contains information about the source account if this is a transfer
	// This is only used for certain transaction types
	Source *Source `json:"source,omitempty"`

	// Destination contains information about the destination account if this is a transfer
	// This is only used for certain transaction types
	Destination *Destination `json:"destination,omitempty"`

	// AssetCode identifies the currency or asset type for this operation
	// Common examples include "USD", "EUR", "BTC", etc.
	AssetCode string `json:"assetCode"`

	// Scale represents the decimal precision for the amount
	// For example, a scale of 2 means the amount is in cents (100 = $1.00)
	Scale int `json:"scale"`

	// AccountAlias is an optional human-readable name for the account
	// This can be used to reference accounts by their alias instead of ID
	// Format is typically "<type>:<identifier>[:subtype]", e.g., "customer:john.doe"
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
//
// CreateOperationInput is used within the CreateTransactionInput structure to define
// the individual debit and credit operations that make up a transaction. Each transaction
// must have at least one operation, and the sum of all debits must equal the sum of all
// credits for each asset type.
//
// The Type field must be either "debit" or "credit":
//   - Debit: Increases asset and expense accounts, decreases liability, equity, and revenue accounts
//   - Credit: Decreases asset and expense accounts, increases liability, equity, and revenue accounts
//
// Example - Creating a debit operation:
//
//	// Debit a customer account (decrease balance)
//	debitOp := models.CreateOperationInput{
//	    Type:         "debit",
//	    AccountID:    "acc-123",                    // Account ID
//	    AccountAlias: stringPtr("customer:john.doe"), // Optional alias
//	    Amount:       10000,                        // $100.00
//	    AssetCode:    "USD",
//	    Scale:        2,
//	}
//
// Example - Creating a credit operation:
//
//	// Credit a revenue account (increase balance)
//	creditOp := models.CreateOperationInput{
//	    Type:         "credit",
//	    AccountID:    "acc-456",                  // Account ID
//	    AccountAlias: stringPtr("revenue:payments"), // Optional alias
//	    Amount:       10000,                      // $100.00
//	    AssetCode:    "USD",
//	    Scale:        2,
//	}
//
// Example - Using operations in a transaction:
//
//	// Create a balanced transaction with a debit and credit
//	tx := &models.CreateTransactionInput{
//	    Description: "Payment for invoice #123",
//	    AssetCode:   "USD",
//	    Amount:      10000,
//	    Scale:       2,
//	    Operations: []models.CreateOperationInput{
//	        // Debit customer account
//	        {
//	            Type:         "debit",
//	            AccountID:    "acc-123",
//	            AccountAlias: stringPtr("customer:john.doe"),
//	            Amount:       10000,
//	            AssetCode:    "USD",
//	            Scale:        2,
//	        },
//	        // Credit revenue account
//	        {
//	            Type:         "credit",
//	            AccountID:    "acc-456",
//	            AccountAlias: stringPtr("revenue:payments"),
//	            Amount:       10000,
//	            AssetCode:    "USD",
//	            Scale:        2,
//	        },
//	    },
//	}
//
// Helper function for creating string pointers:
//
//	func stringPtr(s string) *string {
//	    return &s
//	}
type CreateOperationInput struct {
	// Type indicates whether this is a debit or credit operation
	// Must be either "debit" or "credit"
	Type string `json:"type"`

	// AccountID is the identifier of the account to be affected
	// This must be a valid account ID in the ledger
	AccountID string `json:"accountId"`

	// Amount is the numeric value of the operation
	// This represents the value as a fixed-point integer
	// The actual amount is calculated as Amount / 10^Scale
	Amount int64 `json:"amount"`

	// AssetCode identifies the currency or asset type for this operation
	// Common examples include "USD", "EUR", "BTC", etc.
	AssetCode string `json:"assetCode,omitempty"`

	// Scale represents the decimal precision for the amount
	// For example, a scale of 2 means the amount is in cents (100 = $1.00)
	Scale int `json:"scale,omitempty"`

	// AccountAlias is an optional human-readable name for the account
	// This can be used to reference accounts by their alias instead of ID
	// Format is typically "<type>:<identifier>[:subtype]", e.g., "customer:john.doe"
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
