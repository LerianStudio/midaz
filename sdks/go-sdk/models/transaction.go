package models

import (
	"fmt"
	"time"

	libTransaction "github.com/LerianStudio/lib-commons/commons/transaction"
)

// Transaction represents a transaction in the Midaz Ledger.
// A transaction is a financial event that affects one or more accounts
// through a series of operations (debits and credits).
type Transaction struct {
	// ID is the unique identifier for the transaction
	ID string `json:"id"`

	// Template is an optional identifier for the transaction template used
	Template string `json:"template,omitempty"`

	// Amount is the numeric value of the transaction
	Amount int64 `json:"amount"`

	// Scale represents the decimal precision for the amount
	Scale int64 `json:"scale"`

	// AssetCode identifies the currency or asset type for this transaction
	AssetCode string `json:"assetCode"`

	// Status indicates the current processing status of the transaction
	Status Status `json:"status"`

	// LedgerID identifies the ledger this transaction belongs to
	LedgerID string `json:"ledgerId"`

	// OrganizationID identifies the organization this transaction belongs to
	OrganizationID string `json:"organizationId"`

	// Operations contains the individual debit and credit operations
	Operations []Operation `json:"operations,omitempty"`

	// Metadata contains additional custom data for the transaction
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the transaction was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the transaction was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the transaction was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

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
type CreateTransactionInput struct {
	// Template is an optional identifier for the transaction template to use
	Template string `json:"template,omitempty"`

	// Amount is the numeric value of the transaction
	Amount int64 `json:"amount"`

	// Scale represents the decimal precision for the amount
	Scale int64 `json:"scale"`

	// AssetCode identifies the currency or asset type for this transaction
	AssetCode string `json:"assetCode"`

	// Operations contains the individual debit and credit operations
	Operations []CreateOperationInput `json:"operations,omitempty"`

	// Metadata contains additional custom data for the transaction
	Metadata map[string]any `json:"metadata,omitempty"`

	// ChartOfAccountsGroupName specifies the chart of accounts group to use
	ChartOfAccountsGroupName string `json:"chartOfAccountsGroupName,omitempty"`

	// Description provides a human-readable description of the transaction
	Description string `json:"description,omitempty"`
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
