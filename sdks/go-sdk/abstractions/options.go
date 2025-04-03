// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import "github.com/LerianStudio/midaz/sdks/go-sdk/models"

// Option is a function that configures transaction creation.
//
// Options are used with the CreateDeposit, CreateWithdrawal, and CreateTransfer
// methods to customize the transaction's properties beyond the basic required parameters.
// These options allow for adding metadata, specifying idempotency keys, setting transaction
// status, and other advanced configurations.
//
// The Option type follows the functional options pattern, which provides a clean and
// extensible way to configure objects with many optional parameters. Each option is
// a function that modifies the transaction input in some way.
//
// Multiple options can be combined to fully customize a transaction:
//
//	tx, err := client.CreateTransfer(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "account-source", "account-target",
//	    1000, 2, "USD", "Payment",
//	    transactions.WithIdempotencyKey("unique-key-123"),
//	    transactions.WithMetadata(map[string]any{
//	        "reference": "INV-456",
//	        "department": "Sales",
//	    }),
//	    transactions.WithPending(true),
//	)
//
// Available options include:
//   - WithMetadata: Add custom key-value data to the transaction
//   - WithIdempotencyKey: Ensure transaction uniqueness with a client-generated key
//   - WithExternalID: Link the transaction to an external system identifier
//   - WithPending: Create the transaction in a pending state requiring explicit commitment
//   - WithChartOfAccountsGroupName: Categorize the transaction for accounting purposes
//   - WithCode: Add a custom transaction code for categorization
//   - WithNotes: Add detailed notes about the transaction
//   - WithRequestID: Track the transaction with a request identifier
type Option func(*models.TransactionDSLInput)

// WithMetadata adds structured metadata to a transaction.
//
// Metadata is a flexible key-value store that can be used to add business-specific
// information to transactions. This is useful for storing references to external
// systems, categorizing transactions, or adding any other contextual information.
//
// The metadata will be stored with the transaction and can be retrieved later
// when querying the transaction. It can also be used for searching and filtering
// transactions in reporting operations.
//
// Common use cases for metadata include:
//   - Storing external reference IDs (invoice numbers, order IDs)
//   - Adding business context (department, cost center, project)
//   - Including customer information (customer ID, email)
//   - Tracking source information (channel, device, location)
//   - Adding operational details (batch number, processing flags)
//
// Parameters:
//   - metadata: A map of key-value pairs to store as transaction metadata
//
// Returns:
//   - An Option function that can be passed to transaction creation methods
//
// Example - Basic metadata:
//
//	transactions.WithMetadata(map[string]any{
//	    "invoice_id": "INV-123",
//	    "customer_ref": "CUST-456",
//	    "channel": "web",
//	    "payment_method": "credit_card",
//	})
//
// Example - Nested metadata structures:
//
//	transactions.WithMetadata(map[string]any{
//	    "order": map[string]any{
//	        "id": "ORD-789",
//	        "items": 3,
//	        "shipping": "express",
//	    },
//	    "customer": map[string]any{
//	        "id": "CUST-456",
//	        "tier": "premium",
//	    },
//	})
func WithMetadata(metadata map[string]any) Option {
	return func(input *models.TransactionDSLInput) {
		input.Metadata = metadata
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
//   - An Option function that can be passed to transaction creation methods
//
// Example:
//
//	transactions.WithChartOfAccountsGroupName("revenue:subscription")
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
//   - An Option function that can be passed to transaction creation methods
//
// Example:
//
//	transactions.WithCode("SUBS-RENEW")
func WithCode(code string) Option {
	return func(input *models.TransactionDSLInput) {
		input.Code = code
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
//   - An Option function that can be passed to transaction creation methods
//
// Example - Creating a pending transaction:
//
//	// Create a pending transaction
//	tx, err := client.CreateTransfer(
//	    ctx, orgID, ledgerID,
//	    sourceAccount, targetAccount,
//	    amount, scale, asset, description,
//	    transactions.WithPending(true),
//	)
//
// Example - Committing a pending transaction:
//
//	// Create a pending transaction
//	tx, err := client.CreateTransfer(
//	    ctx, orgID, ledgerID,
//	    sourceAccount, targetAccount,
//	    amount, scale, asset, description,
//	    transactions.WithPending(true),
//	)
//
//	// Later, after verification or approval:
//	err = client.Transactions.CommitTransaction(ctx, orgID, ledgerID, tx.ID)
//
// Example - Implementing an approval workflow:
//
//	// Create a pending high-value transfer
//	tx, err := client.CreateTransfer(
//	    ctx, orgID, ledgerID,
//	    "account:treasury", "account:investments",
//	    1000000, 2, "USD", // $10,000.00
//	    "Investment allocation",
//	    transactions.WithPending(true),
//	    transactions.WithMetadata(map[string]any{
//	        "requires_approval": true,
//	        "approval_level": "director",
//	        "requested_by": "jane.doe",
//	    }),
//	)
//
//	// Store the transaction ID for the approval process
//	pendingTransactionID := tx.ID
//
//	// Later, in the approval handler:
//	if approved {
//	    err = client.Transactions.CommitTransaction(ctx, orgID, ledgerID, pendingTransactionID)
//	} else {
//	    err = client.Transactions.DeleteTransaction(ctx, orgID, ledgerID, pendingTransactionID)
//	}
func WithPending(pending bool) Option {
	return func(input *models.TransactionDSLInput) {
		input.Pending = pending
	}
}

// WithIdempotencyKey adds an idempotency key to ensure transaction uniqueness.
//
// Idempotency keys protect against duplicate transaction creation in case of
// network errors or retries. If a request with the same idempotency key is
// received multiple times, only one transaction will be created, and subsequent
// requests will return the same result.
//
// The key must be unique for each distinct transaction. Reusing an idempotency key
// for different transactions will return the result of the first transaction created
// with that key.
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
//   - An Option function that can be passed to transaction creation methods
//
// Example - Basic idempotency key:
//
//	transactions.WithIdempotencyKey("payment-2023-03-15-12345")
//
// Example - Using a UUID as an idempotency key:
//
//	import "github.com/google/uuid"
//
//	// Generate a UUID for the idempotency key
//	idempotencyKey := uuid.New().String()
//
//	// Use the UUID as the idempotency key
//	tx, err := txAbstraction.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithIdempotencyKey(idempotencyKey),
//	)
//
// Example - Structured idempotency key with business meaning:
//
//	import (
//	    "fmt"
//	    "time"
//	)
//
//	// Create a structured key with business meaning
//	idempotencyKey := fmt.Sprintf(
//	    "customer-%s-invoice-%s-%s",
//	    customerID,
//	    invoiceNumber,
//	    time.Now().Format("20060102"),
//	)
//
//	// Use the structured key
//	abstractions.WithIdempotencyKey(idempotencyKey)
func WithIdempotencyKey(key string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}

		input.Metadata["idempotencyKey"] = key
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
//   - An Option function that can be passed to transaction creation methods
//
// Example:
//
//	transactions.WithExternalID("PO-12345")
func WithExternalID(externalID string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}

		input.Metadata["externalID"] = externalID
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
//   - An Option function that can be passed to transaction creation methods
//
// Example:
//
//	transactions.WithNotes("Customer requested refund due to damaged product.")
func WithNotes(notes string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}

		input.Metadata["notes"] = notes
	}
}

// WithRequestID attaches a unique request ID to the transaction.
//
// Request IDs are primarily used for tracking API requests across systems for
// debugging and auditing purposes. They can help correlate logs and trace request flow.
//
// Parameters:
//   - id: The request identifier to associate with the transaction
//
// Returns:
//   - An Option function that can be passed to transaction creation methods
//
// Example:
//
//	transactions.WithRequestID("req-abc-123-xyz")
func WithRequestID(id string) Option {
	return func(input *models.TransactionDSLInput) {
		if input.Metadata == nil {
			input.Metadata = make(map[string]any)
		}

		input.Metadata["requestID"] = id
	}
}

// WithSendingOptions configures sending options for a transaction.
func WithSendingOptions(asset string, value int64, scale int64, source *models.DSLSource, distribute *models.DSLDistribute) Option {
	return func(input *models.TransactionDSLInput) {
		input.Send = &models.DSLSend{
			Asset:      asset,
			Value:      value,
			Scale:      scale,
			Source:     source,
			Distribute: distribute,
		}
	}
}

// WithFromTo adds a from/to entry to a transaction.
func WithFromTo(account string, amount *models.DSLAmount, isSource bool, description string, chartOfAccounts string, metadata map[string]any) Option {
	return func(input *models.TransactionDSLInput) {
		fromTo := models.DSLFromTo{
			Account:         account,
			Amount:          amount,
			Description:     description,
			ChartOfAccounts: chartOfAccounts,
			Metadata:        metadata,
		}

		if input.Send == nil {
			input.Send = &models.DSLSend{}
		}

		if isSource {
			if input.Send.Source == nil {
				input.Send.Source = &models.DSLSource{}
			}
			input.Send.Source.From = append(input.Send.Source.From, fromTo)
		} else {
			if input.Send.Distribute == nil {
				input.Send.Distribute = &models.DSLDistribute{}
			}
			input.Send.Distribute.To = append(input.Send.Distribute.To, fromTo)
		}
	}
}

// WithShare adds a share configuration to a from/to entry.
func WithShare(percentage int64, percentageOfPercentage int64) Option {
	return func(input *models.TransactionDSLInput) {
		share := &models.Share{
			Percentage:             percentage,
			PercentageOfPercentage: percentageOfPercentage,
		}

		// Apply the share to the last from/to entry
		if input.Send != nil && input.Send.Source != nil && len(input.Send.Source.From) > 0 {
			lastIdx := len(input.Send.Source.From) - 1
			input.Send.Source.From[lastIdx].Share = share
		}
		if input.Send != nil && input.Send.Distribute != nil && len(input.Send.Distribute.To) > 0 {
			lastIdx := len(input.Send.Distribute.To) - 1
			input.Send.Distribute.To[lastIdx].Share = share
		}
	}
}

// WithRate adds an exchange rate to a from/to entry.
func WithRate(from, to string, value, scale int64, externalID string) Option {
	return func(input *models.TransactionDSLInput) {
		rate := &models.Rate{
			From:       from,
			To:         to,
			Value:      value,
			Scale:      scale,
			ExternalID: externalID,
		}

		// Apply the rate to the last from/to entry
		if input.Send != nil && input.Send.Source != nil && len(input.Send.Source.From) > 0 {
			lastIdx := len(input.Send.Source.From) - 1
			input.Send.Source.From[lastIdx].Rate = rate
		}
		if input.Send != nil && input.Send.Distribute != nil && len(input.Send.Distribute.To) > 0 {
			lastIdx := len(input.Send.Distribute.To) - 1
			input.Send.Distribute.To[lastIdx].Rate = rate
		}
	}
}
