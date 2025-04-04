package models

import (
	"fmt"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
)

// Transaction represents a transaction in the Midaz Ledger.
// A transaction is a financial event that affects one or more accounts
// through a series of operations (debits and credits).
//
// Transactions are the core financial records in the Midaz system, representing
// the movement of assets between accounts. Each transaction consists of one or more
// operations (debits and credits) that must balance (sum to zero) for each asset type.
//
// Transactions can be in different states as indicated by their Status field:
//   - PENDING: The transaction is created but not yet committed
//   - COMPLETED: The transaction is committed and has affected account balances
//   - FAILED: The transaction processing failed
//   - CANCELED: The transaction was canceled before being committed
//
// Example usage:
//
//	// Accessing transaction details
//	fmt.Printf("Transaction ID: %s\n", transaction.ID)
//	fmt.Printf("Amount: %d (scale: %d)\n", transaction.Amount, transaction.Scale)
//	fmt.Printf("Asset: %s\n", transaction.AssetCode)
//	fmt.Printf("Status: %s\n", transaction.Status)
//	fmt.Printf("Created: %s\n", transaction.CreatedAt.Format(time.RFC3339))
//
//	// Iterating through operations
//	for i, op := range transaction.Operations {
//	    fmt.Printf("Operation %d: %s %s %d (scale: %d) on account %s\n",
//	        i+1, op.Type, op.AssetCode, op.Amount.Value, op.Amount.Scale, op.AccountID)
//	}
//
//	// Accessing metadata
//	if reference, ok := transaction.Metadata["reference"].(string); ok {
//	    fmt.Printf("Reference: %s\n", reference)
//	}
type Transaction struct {
	// ID is the unique identifier for the transaction
	// This is a system-generated UUID that uniquely identifies the transaction
	ID string `json:"id"`

	// Template is an optional identifier for the transaction template used
	// Templates can be used to create standardized transactions with predefined
	// structures and validation rules
	Template string `json:"template,omitempty"`

	// Amount is the numeric value of the transaction
	// This represents the total value of the transaction as a fixed-point integer
	// The actual amount is calculated as Amount / 10^Scale
	Amount int64 `json:"amount"`

	// Scale represents the decimal precision for the amount
	// For example, a scale of 2 means the amount is in cents (100 = $1.00)
	Scale int64 `json:"scale"`

	// AssetCode identifies the currency or asset type for this transaction
	// Common examples include "USD", "EUR", "BTC", etc.
	AssetCode string `json:"assetCode"`

	// Status indicates the current processing status of the transaction
	// See the Status enum for possible values (PENDING, COMPLETED, FAILED, CANCELED)
	Status Status `json:"status"`

	// LedgerID identifies the ledger this transaction belongs to
	// A ledger is a collection of accounts and transactions within an organization
	LedgerID string `json:"ledgerId"`

	// OrganizationID identifies the organization this transaction belongs to
	// An organization is the top-level entity that owns ledgers and accounts
	OrganizationID string `json:"organizationId"`

	// Operations contains the individual debit and credit operations
	// Each operation represents a single accounting entry (debit or credit)
	// The sum of all operations for each asset must balance to zero
	Operations []Operation `json:"operations,omitempty"`

	// Metadata contains additional custom data for the transaction
	// This can be used to store application-specific information
	// such as references to external systems, tags, or other contextual data
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the transaction was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the transaction was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the transaction was deleted, if applicable
	// This field is only set if the transaction has been soft-deleted
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// ExternalID is an optional identifier for linking to external systems
	// This can be used to correlate transactions with records in other systems
	ExternalID string `json:"externalId,omitempty"`

	// Description is a human-readable description of the transaction
	// This should provide context about the purpose or nature of the transaction
	Description string `json:"description,omitempty"`

	// Internal field to store the lib-commons transaction
	libTransaction *libTransaction.Transaction
}

// DSLAmount represents an amount with a value, scale, and asset code for DSL transactions.
// This is aligned with the lib-commons Amount structure.
type DSLAmount struct {
	// Value is the numeric value of the amount
	Value int64 `json:"value"`

	// Scale represents the decimal precision for the amount
	Scale int64 `json:"scale"`

	// Asset is the asset code for the amount
	Asset string `json:"asset,omitempty"`
}

// DSLFromTo represents a source or destination in a DSL transaction.
// This is aligned with the lib-commons FromTo structure.
type DSLFromTo struct {
	// Account is the identifier of the account
	Account string `json:"account"`

	// Amount specifies the amount details if applicable
	Amount *DSLAmount `json:"amount,omitempty"`

	// Share is the sharing configuration
	Share *Share `json:"share,omitempty"`

	// Remaining is an optional remaining account
	Remaining string `json:"remaining,omitempty"`

	// Rate is the exchange rate configuration
	Rate *Rate `json:"rate,omitempty"`

	// Description is a human-readable description
	Description string `json:"description,omitempty"`

	// ChartOfAccounts is the chart of accounts code
	ChartOfAccounts string `json:"chartOfAccounts,omitempty"`

	// Metadata contains additional custom data
	Metadata map[string]any `json:"metadata,omitempty"`
}

// DSLSource represents the source of a DSL transaction.
// This is aligned with the lib-commons Source structure.
type DSLSource struct {
	// Remaining is an optional remaining account
	Remaining string `json:"remaining,omitempty"`

	// From is a collection of source accounts and amounts
	From []DSLFromTo `json:"from"`
}

// DSLDistribute represents the distribution of a DSL transaction.
// This is aligned with the lib-commons Distribute structure.
type DSLDistribute struct {
	// Remaining is an optional remaining account
	Remaining string `json:"remaining,omitempty"`

	// To is a collection of destination accounts and amounts
	To []DSLFromTo `json:"to"`
}

// DSLSend represents the send operation in a DSL transaction.
// This is aligned with the lib-commons Send structure.
type DSLSend struct {
	// Asset identifies the currency or asset type for this transaction
	Asset string `json:"asset"`

	// Value is the numeric value of the transaction
	Value int64 `json:"value"`

	// Scale represents the decimal precision for the amount
	Scale int64 `json:"scale"`

	// Source specifies where the funds come from
	Source *DSLSource `json:"source,omitempty"`

	// Distribute specifies where the funds go to
	Distribute *DSLDistribute `json:"distribute,omitempty"`
}

// TransactionDSLInput represents the input for creating a transaction using DSL.
// This is aligned with the lib-commons Transaction structure.
type TransactionDSLInput struct {
	// ChartOfAccountsGroupName specifies the chart of accounts group to use
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Description provides a human-readable description of the transaction
	Description string `json:"description,omitempty"`

	// Send contains the sending configuration
	Send *DSLSend `json:"send,omitempty"`

	// Metadata contains additional custom data for the transaction
	Metadata map[string]any `json:"metadata,omitempty"`

	// Code is a custom transaction code for categorization
	Code string `json:"code,omitempty"`

	// Pending indicates whether the transaction requires explicit commitment
	Pending bool `json:"pending,omitempty"`
}

// Share represents the sharing configuration for a transaction.
type Share struct {
	Percentage             int64 `json:"percentage"`
	PercentageOfPercentage int64 `json:"percentageOfPercentage,omitempty"`
}

// Rate represents an exchange rate configuration.
type Rate struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Value      int64  `json:"value"`
	Scale      int64  `json:"scale"`
	ExternalID string `json:"externalId"`
}

// Validate checks that the DSLSend meets all validation requirements.
func (send *DSLSend) Validate() error {
	// Validate required fields
	if send.Asset == "" {
		return fmt.Errorf("asset is required")
	}

	if send.Value <= 0 {
		return fmt.Errorf("value must be greater than 0, got %d", send.Value)
	}

	if send.Scale <= 0 {
		return fmt.Errorf("scale must be greater than 0, got %d", send.Scale)
	}

	// Validate source
	if send.Source == nil || len(send.Source.From) == 0 {
		return fmt.Errorf("source.from must contain at least one entry")
	}

	for i, from := range send.Source.From {
		if from.Account == "" {
			return fmt.Errorf("source.from[%d].account is required", i)
		}
	}

	// Validate distribute
	if send.Distribute == nil || len(send.Distribute.To) == 0 {
		return fmt.Errorf("distribute.to must contain at least one entry")
	}

	for i, to := range send.Distribute.To {
		if to.Account == "" {
			return fmt.Errorf("distribute.to[%d].account is required", i)
		}
	}

	return nil
}

// Validate checks that the TransactionDSLInput meets all validation requirements.
func (input *TransactionDSLInput) Validate() error {
	// Validate send
	if input.Send == nil {
		return fmt.Errorf("send is required")
	}

	// Validate send operation
	if err := input.Send.Validate(); err != nil {
		return fmt.Errorf("invalid send operation: %w", err)
	}

	// Validate string length constraints
	if len(input.ChartOfAccountsGroupName) > 256 {
		return fmt.Errorf("chartOfAccountsGroupName must be at most 256 characters, got %d", len(input.ChartOfAccountsGroupName))
	}

	if len(input.Description) > 256 {
		return fmt.Errorf("description must be at most 256 characters, got %d", len(input.Description))
	}

	if len(input.Code) > 100 {
		return fmt.Errorf("code must be at most 100 characters, got %d", len(input.Code))
	}

	return nil
}

// ToLibTransaction converts a TransactionDSLInput to a lib-commons Transaction.
func (input *TransactionDSLInput) ToLibTransaction() *libTransaction.Transaction {
	// Create a new lib-commons Transaction
	transaction := &libTransaction.Transaction{
		ChartOfAccountsGroupName: input.ChartOfAccountsGroupName,
		Description:              input.Description,
		Code:                     input.Code,
		Pending:                  input.Pending,
		Metadata:                 input.Metadata,
	}

	// Convert Send
	if input.Send != nil {
		transaction.Send = libTransaction.Send{
			Asset: input.Send.Asset,
			Value: input.Send.Value,
			Scale: input.Send.Scale,
		}

		// Convert Source
		if input.Send.Source != nil {
			transaction.Send.Source = libTransaction.Source{
				Remaining: input.Send.Source.Remaining,
			}

			// Convert From entries
			for _, from := range input.Send.Source.From {
				libFrom := libTransaction.FromTo{
					Account:         from.Account,
					Remaining:       from.Remaining,
					Description:     from.Description,
					ChartOfAccounts: from.ChartOfAccounts,
					Metadata:        from.Metadata,
				}

				// Convert Amount
				if from.Amount != nil {
					libFrom.Amount = &libTransaction.Amount{
						Asset: from.Amount.Asset,
						Value: from.Amount.Value,
						Scale: from.Amount.Scale,
					}
				}

				// Convert Share
				if from.Share != nil {
					libFrom.Share = &libTransaction.Share{
						Percentage:             from.Share.Percentage,
						PercentageOfPercentage: from.Share.PercentageOfPercentage,
					}
				}

				// Convert Rate
				if from.Rate != nil {
					libFrom.Rate = &libTransaction.Rate{
						From:       from.Rate.From,
						To:         from.Rate.To,
						Value:      from.Rate.Value,
						Scale:      from.Rate.Scale,
						ExternalID: from.Rate.ExternalID,
					}
				}

				transaction.Send.Source.From = append(transaction.Send.Source.From, libFrom)
			}
		}

		// Convert Distribute
		if input.Send.Distribute != nil {
			transaction.Send.Distribute = libTransaction.Distribute{
				Remaining: input.Send.Distribute.Remaining,
			}

			// Convert To entries
			for _, to := range input.Send.Distribute.To {
				libTo := libTransaction.FromTo{
					Account:         to.Account,
					Remaining:       to.Remaining,
					Description:     to.Description,
					ChartOfAccounts: to.ChartOfAccounts,
					Metadata:        to.Metadata,
				}

				// Convert Amount
				if to.Amount != nil {
					libTo.Amount = &libTransaction.Amount{
						Asset: to.Amount.Asset,
						Value: to.Amount.Value,
						Scale: to.Amount.Scale,
					}
				}

				// Convert Share
				if to.Share != nil {
					libTo.Share = &libTransaction.Share{
						Percentage:             to.Share.Percentage,
						PercentageOfPercentage: to.Share.PercentageOfPercentage,
					}
				}

				// Convert Rate
				if to.Rate != nil {
					libTo.Rate = &libTransaction.Rate{
						From:       to.Rate.From,
						To:         to.Rate.To,
						Value:      to.Rate.Value,
						Scale:      to.Rate.Scale,
						ExternalID: to.Rate.ExternalID,
					}
				}

				transaction.Send.Distribute.To = append(transaction.Send.Distribute.To, libTo)
			}
		}
	}

	return transaction
}

// FromLibTransaction converts a lib-commons Transaction to a TransactionDSLInput.
func FromLibTransaction(t *libTransaction.Transaction) *TransactionDSLInput {
	if t == nil {
		return nil
	}

	// Create a new TransactionDSLInput
	input := &TransactionDSLInput{
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Description:              t.Description,
		Code:                     t.Code,
		Pending:                  t.Pending,
		Metadata:                 t.Metadata,
	}

	// Convert Send
	input.Send = &DSLSend{
		Asset: t.Send.Asset,
		Value: t.Send.Value,
		Scale: t.Send.Scale,
	}

	// Convert Source
	if len(t.Send.Source.From) > 0 {
		input.Send.Source = &DSLSource{
			Remaining: t.Send.Source.Remaining,
		}

		// Convert From entries
		for _, from := range t.Send.Source.From {
			dslFrom := DSLFromTo{
				Account:         from.Account,
				Remaining:       from.Remaining,
				Description:     from.Description,
				ChartOfAccounts: from.ChartOfAccounts,
				Metadata:        from.Metadata,
			}

			// Convert Amount
			if from.Amount != nil {
				dslFrom.Amount = &DSLAmount{
					Asset: from.Amount.Asset,
					Value: from.Amount.Value,
					Scale: from.Amount.Scale,
				}
			}

			// Convert Share
			if from.Share != nil {
				dslFrom.Share = &Share{
					Percentage:             from.Share.Percentage,
					PercentageOfPercentage: from.Share.PercentageOfPercentage,
				}
			}

			// Convert Rate
			if from.Rate != nil {
				dslFrom.Rate = &Rate{
					From:       from.Rate.From,
					To:         from.Rate.To,
					Value:      from.Rate.Value,
					Scale:      from.Rate.Scale,
					ExternalID: from.Rate.ExternalID,
				}
			}

			input.Send.Source.From = append(input.Send.Source.From, dslFrom)
		}
	}

	// Convert Distribute
	if len(t.Send.Distribute.To) > 0 {
		input.Send.Distribute = &DSLDistribute{
			Remaining: t.Send.Distribute.Remaining,
		}

		// Convert To entries
		for _, to := range t.Send.Distribute.To {
			dslTo := DSLFromTo{
				Account:         to.Account,
				Remaining:       to.Remaining,
				Description:     to.Description,
				ChartOfAccounts: to.ChartOfAccounts,
				Metadata:        to.Metadata,
			}

			// Convert Amount
			if to.Amount != nil {
				dslTo.Amount = &DSLAmount{
					Asset: to.Amount.Asset,
					Value: to.Amount.Value,
					Scale: to.Amount.Scale,
				}
			}

			// Convert Share
			if to.Share != nil {
				dslTo.Share = &Share{
					Percentage:             to.Share.Percentage,
					PercentageOfPercentage: to.Share.PercentageOfPercentage,
				}
			}

			// Convert Rate
			if to.Rate != nil {
				dslTo.Rate = &Rate{
					From:       to.Rate.From,
					To:         to.Rate.To,
					Value:      to.Rate.Value,
					Scale:      to.Rate.Scale,
					ExternalID: to.Rate.ExternalID,
				}
			}

			input.Send.Distribute.To = append(input.Send.Distribute.To, dslTo)
		}
	}

	return input
}

// CreateTransactionInput is the input for creating a transaction.
// This structure contains all the fields needed to create a new transaction.
//
// CreateTransactionInput is used with the TransactionsService.CreateTransaction method
// to create new transactions in the standard format (as opposed to the DSL format).
// It allows for specifying the transaction details including operations, metadata,
// and other properties.
//
// When creating a transaction, the following rules apply:
//   - The transaction must be balanced (total debits must equal total credits for each asset)
//   - Each operation must specify an account, type (debit or credit), amount, and asset code
//   - The transaction can be created as pending (requiring explicit commitment later)
//   - External IDs and idempotency keys can be used to prevent duplicate transactions
//
// Example - Creating a simple payment transaction:
//
//	// Create a payment transaction with two operations (debit and credit)
//	input := &models.CreateTransactionInput{
//	    Description: "Payment for invoice #123",
//	    AssetCode:   "USD",
//	    Amount:      10000,
//	    Scale:       2, // $100.00
//	    Operations: []models.CreateOperationInput{
//	        {
//	            // Debit the customer's account (decrease balance)
//	            Type:        "debit",
//	            AccountID:   "acc-123", // Customer account ID
//	            AccountAlias: stringPtr("customer:john.doe"), // Optional alias
//	            Amount:      10000,
//	            AssetCode:   "USD",
//	            Scale:       2,
//	        },
//	        {
//	            // Credit the revenue account (increase balance)
//	            Type:        "credit",
//	            AccountID:   "acc-456", // Revenue account ID
//	            AccountAlias: stringPtr("revenue:payments"), // Optional alias
//	            Amount:      10000,
//	            AssetCode:   "USD",
//	            Scale:       2,
//	        },
//	    },
//	    Metadata: map[string]interface{}{
//	        "invoice_id": "inv-123",
//	        "customer_id": "cust-456",
//	    },
//	    ExternalID: "payment-inv123-20230401",
//	}
//
// Example - Creating a pending transaction:
//
//	// Create a pending transaction that requires explicit commitment
//	input := &models.CreateTransactionInput{
//	    Description: "Large transfer pending approval",
//	    AssetCode:   "USD",
//	    Amount:      100000,
//	    Scale:       2, // $1,000.00
//	    Operations: []models.CreateOperationInput{
//	        // Debit operation
//	        {
//	            Type:        "debit",
//	            AccountID:   "acc-789", // Source account ID
//	            Amount:      100000,
//	            AssetCode:   "USD",
//	            Scale:       2,
//	        },
//	        // Credit operation
//	        {
//	            Type:        "credit",
//	            AccountID:   "acc-012", // Target account ID
//	            Amount:      100000,
//	            AssetCode:   "USD",
//	            Scale:       2,
//	        },
//	    },
//	    Pending: true, // Create as pending, requiring explicit commitment
//	    Metadata: map[string]interface{}{
//	        "requires_approval": true,
//	        "approval_level": "manager",
//	    },
//	}
//
//	// Later, after approval:
//	// client.Transactions.CommitTransaction(ctx, orgID, ledgerID, tx.ID)
//
// Helper function for creating string pointers:
//
//	func stringPtr(s string) *string {
//	    return &s
//	}
type CreateTransactionInput struct {
	// Template is an optional identifier for the transaction template to use
	// Templates can be used to create standardized transactions with predefined
	// structures and validation rules
	Template string `json:"template,omitempty"`

	// Amount is the numeric value of the transaction
	// This represents the total value of the transaction as a fixed-point integer
	// The actual amount is calculated as Amount / 10^Scale
	Amount int64 `json:"amount"`

	// Scale represents the decimal precision for the amount
	// For example, a scale of 2 means the amount is in cents (100 = $1.00)
	Scale int64 `json:"scale"`

	// AssetCode identifies the currency or asset type for this transaction
	// Common examples include "USD", "EUR", "BTC", etc.
	AssetCode string `json:"assetCode"`

	// Operations contains the individual debit and credit operations
	// Each operation represents a single accounting entry (debit or credit)
	// The sum of all operations for each asset must balance to zero
	Operations []CreateOperationInput `json:"operations,omitempty"`

	// Metadata contains additional custom data for the transaction
	// This can be used to store application-specific information
	// such as references to external systems, tags, or other contextual data
	Metadata map[string]any `json:"metadata,omitempty"`

	// ChartOfAccountsGroupName specifies the chart of accounts group to use
	// This is used when integrating with traditional accounting systems
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Description is a human-readable description of the transaction
	// This should provide context about the purpose or nature of the transaction
	Description string `json:"description,omitempty"`

	// ExternalID is an optional identifier for linking to external systems
	// This can be used to correlate transactions with records in other systems
	// and to prevent duplicate transactions
	ExternalID string `json:"externalId,omitempty"`

	// Pending indicates whether the transaction should be created in a pending state
	// Pending transactions require explicit commitment before they affect account balances
	Pending bool `json:"pending,omitempty"`

	// IdempotencyKey is a client-generated key to ensure transaction uniqueness
	// If a transaction with the same idempotency key already exists, that transaction
	// will be returned instead of creating a new one
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// Validate checks that the CreateTransactionInput meets all validation requirements.
func (input *CreateTransactionInput) Validate() error {
	if input.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0, got %d", input.Amount)
	}

	if input.Scale <= 0 {
		return fmt.Errorf("scale must be greater than 0, got %d", input.Scale)
	}

	if input.AssetCode == "" {
		return fmt.Errorf("assetCode is required")
	}

	if len(input.Operations) == 0 {
		return fmt.Errorf("at least one operation is required")
	}

	for i, op := range input.Operations {
		if err := op.Validate(); err != nil {
			return fmt.Errorf("invalid operation at index %d: %w", i, err)
		}
	}

	// Validate string length constraints
	if len(input.ChartOfAccountsGroupName) > 256 {
		return fmt.Errorf("chartOfAccountsGroupName must be at most 256 characters, got %d", len(input.ChartOfAccountsGroupName))
	}

	if len(input.Description) > 256 {
		return fmt.Errorf("description must be at most 256 characters, got %d", len(input.Description))
	}

	return nil
}

// ToLibTransaction converts a CreateTransactionInput to a lib-commons Transaction.
func (c *CreateTransactionInput) ToLibTransaction() *libTransaction.Transaction {
	if c == nil {
		return nil
	}

	lt := &libTransaction.Transaction{
		ChartOfAccountsGroupName: c.ChartOfAccountsGroupName,
		Description:              c.Description,
		Metadata:                 c.Metadata,
		Send: libTransaction.Send{
			Asset: c.AssetCode,
			Value: c.Amount,
			Scale: c.Scale,
			Source: libTransaction.Source{
				From: make([]libTransaction.FromTo, 0),
			},
			Distribute: libTransaction.Distribute{
				To: make([]libTransaction.FromTo, 0),
			},
		},
	}

	// Convert operations to FromTo entries
	for _, op := range c.Operations {
		fromTo := libTransaction.FromTo{
			Account: op.AccountID,
			Amount: &libTransaction.Amount{
				Value: op.Amount,
				Scale: int64(op.Scale), // Explicitly convert int to int64
			},
		}
		if op.AccountAlias != nil {
			fromTo.Description = *op.AccountAlias
		}

		if op.Type == string(OperationTypeDebit) {
			lt.Send.Source.From = append(lt.Send.Source.From, fromTo)
		} else {
			lt.Send.Distribute.To = append(lt.Send.Distribute.To, fromTo)
		}
	}

	return lt
}

// FromLibTransaction converts a lib-commons Transaction to an SDK Transaction.
// This function is used internally to convert between backend and SDK models.
func (t *Transaction) FromLibTransaction(lib *libTransaction.Transaction) *Transaction {
	if lib == nil {
		return t
	}

	var operations []Operation
	var amount int64
	var scale int64
	var assetCode string

	// Extract asset, amount, scale from Send
	amount = lib.Send.Value
	scale = lib.Send.Scale
	assetCode = lib.Send.Asset

	// Convert Source (debits)
	for _, from := range lib.Send.Source.From {
		if from.Amount != nil {
			op := Operation{
				Type:      string(OperationTypeDebit),
				AccountID: from.Account,
				Amount:    from.Amount.Value,
				Scale:     int(from.Amount.Scale), // Convert int64 to int
				AssetCode: assetCode,
			}
			if from.Account != "" {
				op.AccountAlias = &from.Account
			}
			operations = append(operations, op)
		}
	}

	// Convert Distribute (credits)
	for _, to := range lib.Send.Distribute.To {
		if to.Amount != nil {
			op := Operation{
				Type:      string(OperationTypeCredit),
				AccountID: to.Account,
				Amount:    to.Amount.Value,
				Scale:     int(to.Amount.Scale), // Convert int64 to int
				AssetCode: assetCode,
			}
			if to.Account != "" {
				op.AccountAlias = &to.Account
			}
			operations = append(operations, op)
		}
	}

	t.Amount = amount
	t.Scale = scale
	t.AssetCode = assetCode
	t.Operations = operations
	t.Metadata = lib.Metadata
	t.libTransaction = lib

	return t
}

// ToLibTransaction converts an SDK Transaction to a lib-commons Transaction.
// This method is used internally to convert between SDK and backend models.
func (t *Transaction) ToLibTransaction() *libTransaction.Transaction {
	if t == nil {
		return nil
	}

	// If we already have a libTransaction stored, return it
	if t.libTransaction != nil {
		return t.libTransaction
	}

	lt := &libTransaction.Transaction{
		Send: libTransaction.Send{
			Asset: t.AssetCode,
			Value: t.Amount,
			Scale: t.Scale,
			Source: libTransaction.Source{
				From: make([]libTransaction.FromTo, 0),
			},
			Distribute: libTransaction.Distribute{
				To: make([]libTransaction.FromTo, 0),
			},
		},
		Metadata: t.Metadata,
	}

	// Convert operations to FromTo entries
	for _, op := range t.Operations {
		fromTo := libTransaction.FromTo{
			Account: op.AccountID,
			Amount: &libTransaction.Amount{
				Value: op.Amount,
				Scale: int64(op.Scale), // Explicitly convert int to int64
			},
		}
		if op.AccountAlias != nil {
			fromTo.Description = *op.AccountAlias
		}

		if op.Type == string(OperationTypeDebit) {
			lt.Send.Source.From = append(lt.Send.Source.From, fromTo)
		} else {
			lt.Send.Distribute.To = append(lt.Send.Distribute.To, fromTo)
		}
	}

	// Store the created libTransaction
	t.libTransaction = lt

	return lt
}

// UpdateTransactionInput represents the input for updating a transaction.
// This structure contains the fields that can be updated on an existing transaction.
//
// UpdateTransactionInput is used with the TransactionsService.UpdateTransaction method
// to update existing transactions. It allows for updating metadata and other mutable
// properties of a transaction.
//
// Note that not all fields of a transaction can be updated after creation, especially
// for transactions that have already been committed. Typically, only metadata and
// certain status-related fields can be modified.
//
// Example - Updating transaction metadata:
//
//	// Update a transaction's metadata
//	input := &models.UpdateTransactionInput{
//	    Metadata: map[string]interface{}{
//	        "updated_by": "admin",
//	        "approval_status": "approved",
//	        "notes": "Verified and approved by finance team",
//	    },
//	}
//
//	updatedTx, err := client.Transactions.UpdateTransaction(
//	    ctx, orgID, ledgerID, transactionID, input,
//	)
type UpdateTransactionInput struct {
	// Metadata contains additional custom data for the transaction
	// This can be used to store application-specific information
	// such as references to external systems, tags, or other contextual data
	Metadata map[string]any `json:"metadata,omitempty"`

	// Description is a human-readable description of the transaction
	// This should provide context about the purpose or nature of the transaction
	Description string `json:"description,omitempty"`

	// ExternalID is an optional identifier for linking to external systems
	// This can be used to correlate transactions with records in other systems
	ExternalID string `json:"externalId,omitempty"`
}

// Validate checks if the UpdateTransactionInput meets the validation requirements.
// It returns an error if any of the validation checks fail.
//
// Returns:
//   - error: An error if the input is invalid, nil otherwise
func (input *UpdateTransactionInput) Validate() error {
	// Validate description length if provided
	if input.Description != "" && len(input.Description) > 256 {
		return fmt.Errorf("description must not exceed 256 characters")
	}

	// Validate external ID if provided
	if input.ExternalID != "" && len(input.ExternalID) > 64 {
		return fmt.Errorf("externalId must not exceed 64 characters")
	}

	// Validate metadata if provided
	if input.Metadata != nil {
		for key, value := range input.Metadata {
			if len(key) > 64 {
				return fmt.Errorf("metadata key '%s' exceeds 64 characters", key)
			}
			if len(fmt.Sprintf("%v", value)) > 256 {
				return fmt.Errorf("metadata value for key '%s' exceeds 256 characters", key)
			}
		}
	}

	return nil
}
