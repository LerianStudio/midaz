// Package builders provides fluent builder interfaces for the Midaz SDK.
//
// This package contains builders and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers using a fluent API.
package builders

import "github.com/LerianStudio/midaz/sdks/go-sdk/models"

// Option is a function that configures a transaction builder.
//
// Options are used with transaction builders to customize the transaction's properties
// beyond the basic required parameters. These options allow for adding metadata,
// specifying idempotency keys, setting transaction status, and other configurations.
//
// The Option type follows the functional options pattern, which provides a clean and
// extensible way to configure objects with many optional parameters. Each option is
// a function that modifies the transaction input in some way.
//
// Multiple options can be combined to fully customize a transaction:
//
//	tx, err := builder.NewTransfer().
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(1000, 2).
//	    WithAssetCode("USD").
//	    WithDescription("Payment").
//	    FromAccount("account-source").
//	    ToAccount("account-target").
//	    WithMetadata(map[string]any{
//	        "reference": "INV-456",
//	        "department": "Sales",
//	    }).
//	    WithPending(true).
//	    Execute(ctx)
//
// Available options include:
//   - WithMetadata: Add custom key-value data to the transaction
//   - WithIdempotencyKey: Ensure transaction uniqueness with a client-generated key
//   - WithExternalID: Link the transaction to an external system identifier
//   - WithPending: Create the transaction in a pending state requiring explicit commitment
//   - WithChartOfAccountsGroupName: Categorize the transaction for accounting purposes
//   - WithCode: Add a custom transaction code for categorization
//   - WithNotes: Add detailed notes about the transaction
type Option func(*models.TransactionDSLInput)

// WithMetadata adds structured metadata to a transaction.
//
// Metadata is a flexible key-value store that can be used to add business-specific
// information to transactions. This is useful for storing references to external
// systems, categorizing transactions, or adding any other contextual information.
//
// Parameters:
//   - metadata: A map of key-value pairs to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewDeposit().
//	    WithMetadata(map[string]any{
//	        "reference": "DEP-123",
//	        "customer_id": "CUST-456",
//	    })
func WithMetadata(metadata map[string]any) Option {
	return func(input *models.TransactionDSLInput) {
		input.Metadata = metadata
	}
}

// WithIdempotencyKey adds an idempotency key to ensure transaction uniqueness.
//
// Idempotency keys protect against duplicate transaction creation in case of
// network errors or retries. If a request with the same idempotency key is
// received multiple times, only one transaction will be created, and subsequent
// requests will return the same result.
//
// Best practice is to use a UUID or another globally unique identifier as the key.
//
// Important considerations for idempotency keys:
//   - Keys should be unique per transaction, not per request
//   - Keys should be stored on the client side before making the request
//   - Keys should have a reasonable TTL (Time To Live) - typically 24 hours
//   - Keys should be URL-safe (alphanumeric characters, hyphens, underscores)
//   - Keys should be reasonably sized (typically 36-128 characters)
//
// Parameters:
//   - key: The unique idempotency key to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewTransfer().
//	    WithIdempotencyKey("tx-123-abc")
func WithIdempotencyKey(key string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}
		input.Metadata["idempotency_key"] = key
	}
}

// WithExternalID sets an external ID for the transaction.
//
// External IDs are used to link transactions in the Midaz system with identifiers
// from external systems. This is useful for reconciliation and tracking purposes.
//
// Parameters:
//   - externalID: The external identifier to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewWithdrawal().
//	    WithExternalID("WD-123")
func WithExternalID(externalID string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}
		input.Metadata["external_id"] = externalID
	}
}

// WithPending marks a transaction as pending, requiring explicit commitment later.
//
// When a transaction is created with pending=true, it must be explicitly committed
// using CommitTransaction before its effects are applied to account balances.
// This two-phase approach allows for validation and verification before the
// transaction is finalized.
//
// Default is pending=false, which means the transaction is immediately committed.
//
// Use cases for pending transactions include:
//   - High-value transactions that require manual approval
//   - Transactions that depend on external verification or conditions
//   - Implementing custom approval workflows
//   - Batching related transactions for atomic commitment
//   - Simulating transactions for testing or forecasting
//
// Pending transactions have the following characteristics:
//   - They are visible in transaction listings with a PENDING status
//   - They do not affect account balances until committed
//   - They can be committed or canceled (deleted)
//   - They have a default TTL (Time To Live) - typically 24 hours
//
// Parameters:
//   - pending: Whether the transaction should be created in a pending state
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewTransfer().
//	    WithPending(true)
func WithPending(pending bool) Option {
	return func(input *models.TransactionDSLInput) {
		input.Pending = pending
	}
}

// WithChartOfAccountsGroupName sets the chart of accounts group for a transaction.
//
// This option is used when integrating with traditional accounting systems that
// use a chart of accounts. The group name helps categorize the transaction for
// accounting purposes, making it easier to generate financial reports like
// balance sheets and income statements.
//
// Parameters:
//   - name: The chart of accounts group name to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewDeposit().
//	    WithChartOfAccountsGroupName("revenue:subscription")
func WithChartOfAccountsGroupName(name string) Option {
	return func(input *models.TransactionDSLInput) {
		input.ChartOfAccountsGroupName = name
	}
}

// WithCode sets a custom transaction code for a transaction.
//
// Transaction codes are short alphanumeric identifiers that can be used to
// categorize transactions according to your business logic. They're useful
// for filtering and searching transactions in reports.
//
// Parameters:
//   - code: The custom code to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewTransfer().
//	    WithCode("SUBS-RENEW")
func WithCode(code string) Option {
	return func(input *models.TransactionDSLInput) {
		input.Code = code
	}
}

// WithNotes adds detailed notes to a transaction.
//
// Notes provide additional context or explanation about a transaction
// beyond the basic description. They can be used for internal documentation,
// explaining unusual circumstances, or providing additional context for auditing.
//
// Parameters:
//   - notes: The detailed notes to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction builders
//
// Example:
//
//	builder.NewWithdrawal().
//	    WithNotes("Customer requested refund due to damaged product.")
func WithNotes(notes string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}
		input.Metadata["notes"] = notes
	}
}
