// Package builders provides fluent builder interfaces for the Midaz SDK.
//
// This package defines builder patterns for creating various Midaz resources
// and operations with a chainable API. Builders help make the SDK more
// intuitive and easier to use by providing method chaining and progressive
// disclosure of available options.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// ClientInterface defines the minimal client interface required by the builders.
// This allows the builders to be used with both the real client and mocks.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services.
type ClientInterface interface {
	// CreateTransaction sends a request to create a new transaction with the specified parameters.
	// It requires a context for the API request and returns the created transaction or an error.
	CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error)
}

// DepositBuilder defines the builder interface for deposit transactions.
// A deposit transaction increases the balance of an account by adding funds to it.
// This builder provides a fluent API for configuring and executing deposit operations.
type DepositBuilder interface {
	// WithOrganization sets the organization ID for the transaction.
	// This is a required field for creating a deposit transaction.
	WithOrganization(orgID string) DepositBuilder

	// WithLedger sets the ledger ID for the transaction.
	// This is a required field for creating a deposit transaction.
	WithLedger(ledgerID string) DepositBuilder

	// WithAmount sets the amount and scale for the transaction.
	// The amount must be greater than zero.
	// Scale represents the number of decimal places (e.g., scale 2 for cents).
	// This is a required field for creating a deposit transaction.
	WithAmount(amount int64, scale int) DepositBuilder

	// WithAssetCode sets the asset code for the transaction.
	// This is a required field for creating a deposit transaction.
	WithAssetCode(assetCode string) DepositBuilder

	// WithDescription sets a human-readable description for the transaction.
	// This is an optional field that provides context about the transaction.
	WithDescription(description string) DepositBuilder

	// WithMetadata adds metadata to the transaction.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) DepositBuilder

	// WithTag adds a single tag to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) DepositBuilder

	// WithTags adds multiple tags to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) DepositBuilder

	// WithExternalID sets an external ID for the transaction.
	// This is an optional field that allows linking the transaction to an external system.
	WithExternalID(externalID string) DepositBuilder

	// WithIdempotencyKey sets an idempotency key for the transaction.
	// This is an optional field that ensures the same transaction is not created multiple times.
	// If a transaction with the same idempotency key already exists, that transaction will be returned.
	WithIdempotencyKey(key string) DepositBuilder

	// ToAccount sets the destination account for the deposit.
	// This is a required field that specifies which account receives the funds.
	ToAccount(accountAlias string) DepositBuilder

	// Execute performs the deposit operation and returns the created transaction.
	//
	// This method validates all required parameters, constructs the transaction DSL,
	// and sends the request to the Midaz API to create the deposit transaction.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//     This context is passed to the underlying API client.
	//
	// Returns:
	//   - *models.Transaction: The created transaction object if successful.
	//     This contains all details about the transaction, including its ID, status,
	//     and other properties.
	//   - error: An error if the operation fails. Possible error types include:
	//     - errors.ErrValidation: If required parameters are missing or invalid
	//     - errors.ErrAuthentication: If authentication fails
	//     - errors.ErrPermission: If the client lacks permission
	//     - errors.ErrNotFound: If the organization, ledger, or account is not found
	//     - errors.ErrInternal: For other internal errors
	//
	// Example:
	//
	//	// Create a deposit transaction
	//	tx, err := builders.NewDeposit(client).
	//	    WithOrganization("org-123").
	//	    WithLedger("ledger-456").
	//	    WithAmount(1000, 2). // $10.00
	//	    WithAssetCode("USD").
	//	    WithDescription("Customer deposit").
	//	    WithMetadata(map[string]any{
	//	        "reference": "invoice-789",
	//	        "channel": "web",
	//	    }).
	//	    WithTag("customer-deposit").
	//	    ToAccount("customer-account").
	//	    Execute(context.Background())
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the transaction
	//	fmt.Printf("Created deposit transaction: %s (status: %s)\n", tx.ID, tx.Status)
	Execute(ctx context.Context) (*models.Transaction, error)
}

// WithdrawalBuilder defines the builder interface for withdrawal transactions.
// A withdrawal transaction decreases the balance of an account by removing funds from it.
// This builder provides a fluent API for configuring and executing withdrawal operations.
type WithdrawalBuilder interface {
	// WithOrganization sets the organization ID for the transaction.
	// This is a required field for creating a withdrawal transaction.
	WithOrganization(orgID string) WithdrawalBuilder

	// WithLedger sets the ledger ID for the transaction.
	// This is a required field for creating a withdrawal transaction.
	WithLedger(ledgerID string) WithdrawalBuilder

	// WithAmount sets the amount and scale for the transaction.
	// The amount must be greater than zero.
	// Scale represents the number of decimal places (e.g., scale 2 for cents).
	// This is a required field for creating a withdrawal transaction.
	WithAmount(amount int64, scale int) WithdrawalBuilder

	// WithAssetCode sets the asset code for the transaction.
	// This is a required field for creating a withdrawal transaction.
	WithAssetCode(assetCode string) WithdrawalBuilder

	// WithDescription sets a human-readable description for the transaction.
	// This is an optional field that provides context about the transaction.
	WithDescription(description string) WithdrawalBuilder

	// WithMetadata adds metadata to the transaction.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) WithdrawalBuilder

	// WithTag adds a single tag to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) WithdrawalBuilder

	// WithTags adds multiple tags to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) WithdrawalBuilder

	// WithExternalID sets an external ID for the transaction.
	// This is an optional field that allows linking the transaction to an external system.
	WithExternalID(externalID string) WithdrawalBuilder

	// WithIdempotencyKey sets an idempotency key for the transaction.
	// This is an optional field that ensures the same transaction is not created multiple times.
	// If a transaction with the same idempotency key already exists, that transaction will be returned.
	WithIdempotencyKey(key string) WithdrawalBuilder

	// FromAccount sets the source account for the withdrawal.
	// This is a required field that specifies which account the funds are withdrawn from.
	FromAccount(accountAlias string) WithdrawalBuilder

	// Execute performs the withdrawal operation and returns the created transaction.
	//
	// This method validates all required parameters, constructs the transaction DSL,
	// and sends the request to the Midaz API to create the withdrawal transaction.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//     This context is passed to the underlying API client.
	//
	// Returns:
	//   - *models.Transaction: The created transaction object if successful.
	//     This contains all details about the transaction, including its ID, status,
	//     and other properties.
	//   - error: An error if the operation fails. Possible error types include:
	//     - errors.ErrValidation: If required parameters are missing or invalid
	//     - errors.ErrAuthentication: If authentication fails
	//     - errors.ErrPermission: If the client lacks permission
	//     - errors.ErrNotFound: If the organization, ledger, or account is not found
	//     - errors.ErrInternal: For other internal errors
	//
	// Example:
	//
	//	// Create a withdrawal transaction
	//	tx, err := builders.NewWithdrawal(client).
	//	    WithOrganization("org-123").
	//	    WithLedger("ledger-456").
	//	    WithAmount(5000, 2). // $50.00
	//	    WithAssetCode("USD").
	//	    WithDescription("Customer withdrawal").
	//	    WithMetadata(map[string]any{
	//	        "reference": "withdrawal-789",
	//	        "channel": "mobile",
	//	    }).
	//	    WithExternalID("ext-withdrawal-123").
	//	    FromAccount("customer-account").
	//	    Execute(context.Background())
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the transaction
	//	fmt.Printf("Created withdrawal transaction: %s (status: %s)\n", tx.ID, tx.Status)
	Execute(ctx context.Context) (*models.Transaction, error)
}

// TransferBuilder defines the builder interface for transfer transactions.
// A transfer transaction moves funds from one account to another within the same ledger.
// This builder provides a fluent API for configuring and executing transfer operations.
type TransferBuilder interface {
	// WithOrganization sets the organization ID for the transaction.
	// This is a required field for creating a transfer transaction.
	WithOrganization(orgID string) TransferBuilder

	// WithLedger sets the ledger ID for the transaction.
	// This is a required field for creating a transfer transaction.
	WithLedger(ledgerID string) TransferBuilder

	// WithAmount sets the amount and scale for the transaction.
	// The amount must be greater than zero.
	// Scale represents the number of decimal places (e.g., scale 2 for cents).
	// This is a required field for creating a transfer transaction.
	WithAmount(amount int64, scale int) TransferBuilder

	// WithAssetCode sets the asset code for the transaction.
	// This is a required field for creating a transfer transaction.
	WithAssetCode(assetCode string) TransferBuilder

	// WithDescription sets a human-readable description for the transaction.
	// This is an optional field that provides context about the transaction.
	WithDescription(description string) TransferBuilder

	// WithMetadata adds metadata to the transaction.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) TransferBuilder

	// WithTag adds a single tag to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) TransferBuilder

	// WithTags adds multiple tags to the transaction.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) TransferBuilder

	// WithExternalID sets an external ID for the transaction.
	// This is an optional field that allows linking the transaction to an external system.
	WithExternalID(externalID string) TransferBuilder

	// WithIdempotencyKey sets an idempotency key for the transaction.
	// This is an optional field that ensures the same transaction is not created multiple times.
	// If a transaction with the same idempotency key already exists, that transaction will be returned.
	WithIdempotencyKey(key string) TransferBuilder

	// FromAccount sets the source account for the transfer.
	// This is a required field that specifies which account the funds are transferred from.
	FromAccount(accountAlias string) TransferBuilder

	// ToAccount sets the destination account for the transfer.
	// This is a required field that specifies which account the funds are transferred to.
	ToAccount(accountAlias string) TransferBuilder

	// Execute performs the transfer operation and returns the created transaction.
	//
	// This method validates all required parameters, constructs the transaction DSL,
	// and sends the request to the Midaz API to create the transfer transaction.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//     This context is passed to the underlying API client.
	//
	// Returns:
	//   - *models.Transaction: The created transaction object if successful.
	//     This contains all details about the transaction, including its ID, status,
	//     and other properties.
	//   - error: An error if the operation fails. Possible error types include:
	//     - errors.ErrValidation: If required parameters are missing or invalid
	//     - errors.ErrAuthentication: If authentication fails
	//     - errors.ErrPermission: If the client lacks permission
	//     - errors.ErrNotFound: If the organization, ledger, or account is not found
	//     - errors.ErrInternal: For other internal errors
	//
	// Example:
	//
	//	// Create a transfer transaction
	//	tx, err := builders.NewTransfer(client).
	//	    WithOrganization("org-123").
	//	    WithLedger("ledger-456").
	//	    WithAmount(2500, 2). // $25.00
	//	    WithAssetCode("USD").
	//	    WithDescription("Transfer between accounts").
	//	    WithMetadata(map[string]any{
	//	        "reference": "transfer-789",
	//	        "purpose": "monthly-allocation",
	//	    }).
	//	    WithIdempotencyKey("transfer-idempotency-key-123").
	//	    FromAccount("source-account").
	//	    ToAccount("destination-account").
	//	    Execute(context.Background())
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the transaction
	//	fmt.Printf("Created transfer transaction: %s (status: %s)\n", tx.ID, tx.Status)
	Execute(ctx context.Context) (*models.Transaction, error)
}

// baseBuilder implements common functionality for all transaction builders.
type baseBuilder struct {
	client         ClientInterface
	orgID          string
	ledgerID       string
	amount         int64
	scale          int
	assetCode      string
	description    string
	metadata       map[string]any
	tags           []string
	externalID     string
	idempotencyKey string
}

// Validate common parameters for all transaction types.
func (b *baseBuilder) validate() error {
	if b.orgID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if b.amount <= 0 {
		return fmt.Errorf("amount must be greater than 0, got %d", b.amount)
	}

	if b.scale <= 0 {
		return fmt.Errorf("scale must be greater than 0, got %d", b.scale)
	}

	if b.assetCode == "" {
		return fmt.Errorf("asset code is required")
	}

	// Validate description length if provided
	if len(b.description) > 256 {
		return fmt.Errorf("description must be at most 256 characters, got %d", len(b.description))
	}

	// Validate external ID length if provided
	if len(b.externalID) > 100 {
		return fmt.Errorf("external ID must be at most 100 characters, got %d", len(b.externalID))
	}

	// Validate idempotency key length if provided
	if len(b.idempotencyKey) > 100 {
		return fmt.Errorf("idempotency key must be at most 100 characters, got %d", len(b.idempotencyKey))
	}

	return nil
}

// depositBuilder implements DepositBuilder interface.
type depositBuilder struct {
	baseBuilder
	targetAccount string
}

// NewDeposit creates a new builder for deposit transactions.
//
// A deposit transaction increases the balance of an account by adding funds to it.
// This function returns a builder that allows for fluent configuration of the deposit
// transaction before execution.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the ClientInterface with the CreateTransaction method.
//
// Returns:
//   - DepositBuilder: A builder interface for configuring and executing deposit transactions.
//     Use the builder's methods to set required and optional parameters, then call Execute()
//     to perform the deposit operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a deposit builder
//	deposit := builders.NewDeposit(client)
//
//	// Configure and execute the deposit
//	tx, err := deposit.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(1000, 2). // $10.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer deposit").
//	    ToAccount("customer-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to execute deposit: %v", err)
//	}
//
//	fmt.Printf("Deposit transaction created: %s\n", tx.ID)
func NewDeposit(client ClientInterface) DepositBuilder {
	return &depositBuilder{
		baseBuilder: baseBuilder{
			client:   client,
			metadata: make(map[string]any),
		},
	}
}

// func (b *depositBuilder) WithOrganization(orgID string) DepositBuilder { performs an operation
func (b *depositBuilder) WithOrganization(orgID string) DepositBuilder {
	b.orgID = orgID
	return b
}

// func (b *depositBuilder) WithLedger(ledgerID string) DepositBuilder { performs an operation
func (b *depositBuilder) WithLedger(ledgerID string) DepositBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *depositBuilder) WithAmount(amount int64, scale int) DepositBuilder { performs an operation
func (b *depositBuilder) WithAmount(amount int64, scale int) DepositBuilder {
	b.amount = amount

	b.scale = scale

	return b
}

// func (b *depositBuilder) WithAssetCode(assetCode string) DepositBuilder { performs an operation
func (b *depositBuilder) WithAssetCode(assetCode string) DepositBuilder {
	b.assetCode = assetCode
	return b
}

// func (b *depositBuilder) WithDescription(description string) DepositBuilder { performs an operation
func (b *depositBuilder) WithDescription(description string) DepositBuilder {
	b.description = description
	return b
}

// func (b *depositBuilder) WithMetadata(metadata map[string]any) DepositBuilder { performs an operation
func (b *depositBuilder) WithMetadata(metadata map[string]any) DepositBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *depositBuilder) WithTag(tag string) DepositBuilder { performs an operation
func (b *depositBuilder) WithTag(tag string) DepositBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		// Update the metadata tags field
		if len(b.tags) > 0 {
			var tagsStr string

			for i, t := range b.tags {
				if i > 0 {
					tagsStr += ","
				}

				tagsStr += t
			}

			b.metadata["tags"] = tagsStr
		}
	}

	return b
}

// func (b *depositBuilder) WithTags(tags []string) DepositBuilder { performs an operation
func (b *depositBuilder) WithTags(tags []string) DepositBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		// Update the metadata tags field
		var tagsStr string

		for i, tag := range b.tags {
			if i > 0 {
				tagsStr += ","
			}

			tagsStr += tag
		}

		b.metadata["tags"] = tagsStr
	}

	return b
}

// func (b *depositBuilder) WithExternalID(externalID string) DepositBuilder { performs an operation
func (b *depositBuilder) WithExternalID(externalID string) DepositBuilder {
	b.externalID = externalID
	if externalID != "" {
		b.metadata["externalId"] = externalID
	}

	return b
}

// func (b *depositBuilder) WithIdempotencyKey(key string) DepositBuilder { performs an operation
func (b *depositBuilder) WithIdempotencyKey(key string) DepositBuilder {
	b.idempotencyKey = key
	if key != "" {
		b.metadata["idempotencyKey"] = key
	}

	return b
}

// func (b *depositBuilder) ToAccount(accountAlias string) DepositBuilder { performs an operation
func (b *depositBuilder) ToAccount(accountAlias string) DepositBuilder {
	b.targetAccount = accountAlias
	return b
}

// Execute performs the deposit operation and returns the created transaction.
//
// This method validates all required parameters, constructs the transaction DSL,
// and sends the request to the Midaz API to create the deposit transaction.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Transaction: The created transaction object if successful.
//     This contains all details about the transaction, including its ID, status,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or account is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a deposit transaction
//	tx, err := builders.NewDeposit(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(1000, 2). // $10.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer deposit").
//	    WithMetadata(map[string]any{
//	        "reference": "invoice-789",
//	        "channel": "web",
//	    }).
//	    WithTag("customer-deposit").
//	    ToAccount("customer-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the transaction
//	fmt.Printf("Created deposit transaction: %s (status: %s)\n", tx.ID, tx.Status)
func (b *depositBuilder) Execute(ctx context.Context) (*models.Transaction, error) {
	// Validate builder state
	if err := b.validate(); err != nil {
		return nil, err
	}

	if b.targetAccount == "" {
		return nil, fmt.Errorf("target account is required for a deposit")
	}

	// Create the external account reference (source account for deposit)
	externalAccount := "@external/" + b.assetCode

	// Build the transaction DSL input
	input := &models.TransactionDSLInput{
		Description: b.description,
		Metadata:    b.metadata,
		Send: &models.DSLSend{
			Asset: b.assetCode,
			Value: b.amount,
			Scale: int64(b.scale),
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: externalAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: b.targetAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
		},
	}

	// Add external ID if present (through metadata)
	if b.externalID != "" {
		// External ID is already added to metadata in WithExternalID method
		// No need to set it directly on the input as it's not a field in TransactionDSLInput
	}

	// Add idempotency key if present (through metadata)
	if b.idempotencyKey != "" {
		// Idempotency key is already added to metadata in WithIdempotencyKey method
		// No need to set it directly on the input as it's not a field in TransactionDSLInput
	}

	// Validate the TransactionDSLInput
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction input: %v", err)
	}

	// Execute the transaction
	return b.client.CreateTransaction(ctx, b.orgID, b.ledgerID, input)
}

// withdrawalBuilder implements WithdrawalBuilder interface.
type withdrawalBuilder struct {
	baseBuilder
	sourceAccount string
}

// NewWithdrawal creates a new builder for withdrawal transactions.
//
// A withdrawal transaction decreases the balance of an account by removing funds from it.
// This function returns a builder that allows for fluent configuration of the withdrawal
// transaction before execution.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the ClientInterface with the CreateTransaction method.
//
// Returns:
//   - WithdrawalBuilder: A builder interface for configuring and executing withdrawal transactions.
//     Use the builder's methods to set required and optional parameters, then call Execute()
//     to perform the withdrawal operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a withdrawal builder
//	withdrawal := builders.NewWithdrawal(client)
//
//	// Configure and execute the withdrawal
//	tx, err := withdrawal.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(5000, 2). // $50.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer withdrawal").
//	    WithExternalID("ext-withdrawal-123").
//	    FromAccount("customer-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to execute withdrawal: %v", err)
//	}
//
//	fmt.Printf("Withdrawal transaction created: %s\n", tx.ID)
func NewWithdrawal(client ClientInterface) WithdrawalBuilder {
	return &withdrawalBuilder{
		baseBuilder: baseBuilder{
			client:   client,
			metadata: make(map[string]any),
		},
	}
}

// func (b *withdrawalBuilder) WithOrganization(orgID string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithOrganization(orgID string) WithdrawalBuilder {
	b.orgID = orgID
	return b
}

// func (b *withdrawalBuilder) WithLedger(ledgerID string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithLedger(ledgerID string) WithdrawalBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *withdrawalBuilder) WithAmount(amount int64, scale int) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithAmount(amount int64, scale int) WithdrawalBuilder {
	b.amount = amount

	b.scale = scale

	return b
}

// func (b *withdrawalBuilder) WithAssetCode(assetCode string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithAssetCode(assetCode string) WithdrawalBuilder {
	b.assetCode = assetCode
	return b
}

// func (b *withdrawalBuilder) WithDescription(description string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithDescription(description string) WithdrawalBuilder {
	b.description = description
	return b
}

// func (b *withdrawalBuilder) WithMetadata(metadata map[string]any) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithMetadata(metadata map[string]any) WithdrawalBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *withdrawalBuilder) WithTag(tag string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithTag(tag string) WithdrawalBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		// Update the metadata tags field
		if len(b.tags) > 0 {
			var tagsStr string

			for i, t := range b.tags {
				if i > 0 {
					tagsStr += ","
				}

				tagsStr += t
			}

			b.metadata["tags"] = tagsStr
		}
	}

	return b
}

// func (b *withdrawalBuilder) WithTags(tags []string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithTags(tags []string) WithdrawalBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		// Update the metadata tags field
		var tagsStr string

		for i, tag := range b.tags {
			if i > 0 {
				tagsStr += ","
			}

			tagsStr += tag
		}

		b.metadata["tags"] = tagsStr
	}

	return b
}

// func (b *withdrawalBuilder) WithExternalID(externalID string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithExternalID(externalID string) WithdrawalBuilder {
	b.externalID = externalID
	if externalID != "" {
		b.metadata["externalId"] = externalID
	}

	return b
}

// func (b *withdrawalBuilder) WithIdempotencyKey(key string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) WithIdempotencyKey(key string) WithdrawalBuilder {
	b.idempotencyKey = key
	if key != "" {
		b.metadata["idempotencyKey"] = key
	}

	return b
}

// func (b *withdrawalBuilder) FromAccount(accountAlias string) WithdrawalBuilder { performs an operation
func (b *withdrawalBuilder) FromAccount(accountAlias string) WithdrawalBuilder {
	b.sourceAccount = accountAlias
	return b
}

// Execute performs the withdrawal operation and returns the created transaction.
//
// This method validates all required parameters, constructs the transaction DSL,
// and sends the request to the Midaz API to create the withdrawal transaction.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Transaction: The created transaction object if successful.
//     This contains all details about the transaction, including its ID, status,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or account is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a withdrawal transaction
//	tx, err := builders.NewWithdrawal(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(5000, 2). // $50.00
//	    WithAssetCode("USD").
//	    WithDescription("Customer withdrawal").
//	    WithMetadata(map[string]any{
//	        "reference": "withdrawal-789",
//	        "channel": "mobile",
//	    }).
//	    WithExternalID("ext-withdrawal-123").
//	    FromAccount("customer-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the transaction
//	fmt.Printf("Created withdrawal transaction: %s (status: %s)\n", tx.ID, tx.Status)
func (b *withdrawalBuilder) Execute(ctx context.Context) (*models.Transaction, error) {
	// Validate builder state
	if err := b.validate(); err != nil {
		return nil, err
	}

	if b.sourceAccount == "" {
		return nil, fmt.Errorf("source account is required for a withdrawal")
	}

	// Create the external account reference (target account for withdrawal)
	externalAccount := "@external/" + b.assetCode

	// Build the transaction DSL input
	input := &models.TransactionDSLInput{
		Description: b.description,
		Metadata:    b.metadata,
		Send: &models.DSLSend{
			Asset: b.assetCode,
			Value: b.amount,
			Scale: int64(b.scale),
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: b.sourceAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: externalAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
		},
	}

	// Add external ID if present (through metadata)
	if b.externalID != "" {
		// External ID is already added to metadata in WithExternalID method
		// No need to set it directly on the input as it's not a field in TransactionDSLInput
	}

	// Add idempotency key if present (through metadata)
	if b.idempotencyKey != "" {
		// Idempotency key is already added to metadata in WithIdempotencyKey method
		// No need to set it directly on the input as it's not a field in TransactionDSLInput
	}

	// Validate the TransactionDSLInput
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction input: %v", err)
	}

	// Execute the transaction
	return b.client.CreateTransaction(ctx, b.orgID, b.ledgerID, input)
}

// transferBuilder implements TransferBuilder interface.
type transferBuilder struct {
	baseBuilder
	sourceAccount string
	targetAccount string
}

// NewTransfer creates a new builder for transfer transactions.
//
// A transfer transaction moves funds from one account to another within the same ledger.
// This function returns a builder that allows for fluent configuration of the transfer
// transaction before execution.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the ClientInterface with the CreateTransaction method.
//
// Returns:
//   - TransferBuilder: A builder interface for configuring and executing transfer transactions.
//     Use the builder's methods to set required and optional parameters, then call Execute()
//     to perform the transfer operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a transfer builder
//	transfer := builders.NewTransfer(client)
//
//	// Configure and execute the transfer
//	tx, err := transfer.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(2500, 2). // $25.00
//	    WithAssetCode("USD").
//	    WithDescription("Transfer between accounts").
//	    WithIdempotencyKey("transfer-idempotency-key-123").
//	    FromAccount("source-account").
//	    ToAccount("destination-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to execute transfer: %v", err)
//	}
//
//	fmt.Printf("Transfer transaction created: %s\n", tx.ID)
func NewTransfer(client ClientInterface) TransferBuilder {
	return &transferBuilder{
		baseBuilder: baseBuilder{
			client:   client,
			metadata: make(map[string]any),
		},
	}
}

// func (b *transferBuilder) WithOrganization(orgID string) TransferBuilder { performs an operation
func (b *transferBuilder) WithOrganization(orgID string) TransferBuilder {
	b.orgID = orgID
	return b
}

// func (b *transferBuilder) WithLedger(ledgerID string) TransferBuilder { performs an operation
func (b *transferBuilder) WithLedger(ledgerID string) TransferBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *transferBuilder) WithAmount(amount int64, scale int) TransferBuilder { performs an operation
func (b *transferBuilder) WithAmount(amount int64, scale int) TransferBuilder {
	b.amount = amount

	b.scale = scale

	return b
}

// func (b *transferBuilder) WithAssetCode(assetCode string) TransferBuilder { performs an operation
func (b *transferBuilder) WithAssetCode(assetCode string) TransferBuilder {
	b.assetCode = assetCode
	return b
}

// func (b *transferBuilder) WithDescription(description string) TransferBuilder { performs an operation
func (b *transferBuilder) WithDescription(description string) TransferBuilder {
	b.description = description
	return b
}

// func (b *transferBuilder) WithMetadata(metadata map[string]any) TransferBuilder { performs an operation
func (b *transferBuilder) WithMetadata(metadata map[string]any) TransferBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *transferBuilder) WithTag(tag string) TransferBuilder { performs an operation
func (b *transferBuilder) WithTag(tag string) TransferBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		// Update the metadata tags field
		if len(b.tags) > 0 {
			var tagsStr string

			for i, t := range b.tags {
				if i > 0 {
					tagsStr += ","
				}

				tagsStr += t
			}

			b.metadata["tags"] = tagsStr
		}
	}

	return b
}

// func (b *transferBuilder) WithTags(tags []string) TransferBuilder { performs an operation
func (b *transferBuilder) WithTags(tags []string) TransferBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		// Update the metadata tags field
		var tagsStr string

		for i, tag := range b.tags {
			if i > 0 {
				tagsStr += ","
			}

			tagsStr += tag
		}

		b.metadata["tags"] = tagsStr
	}

	return b
}

// func (b *transferBuilder) WithExternalID(externalID string) TransferBuilder { performs an operation
func (b *transferBuilder) WithExternalID(externalID string) TransferBuilder {
	b.externalID = externalID
	if externalID != "" {
		b.metadata["externalId"] = externalID
	}

	return b
}

// func (b *transferBuilder) WithIdempotencyKey(key string) TransferBuilder { performs an operation
func (b *transferBuilder) WithIdempotencyKey(key string) TransferBuilder {
	b.idempotencyKey = key
	if key != "" {
		b.metadata["idempotencyKey"] = key
	}

	return b
}

// func (b *transferBuilder) FromAccount(accountAlias string) TransferBuilder { performs an operation
func (b *transferBuilder) FromAccount(accountAlias string) TransferBuilder {
	b.sourceAccount = accountAlias
	return b
}

// func (b *transferBuilder) ToAccount(accountAlias string) TransferBuilder { performs an operation
func (b *transferBuilder) ToAccount(accountAlias string) TransferBuilder {
	b.targetAccount = accountAlias
	return b
}

// Execute performs the transfer operation and returns the created transaction.
//
// This method validates all required parameters, constructs the transaction DSL,
// and sends the request to the Midaz API to create the transfer transaction.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Transaction: The created transaction object if successful.
//     This contains all details about the transaction, including its ID, status,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or account is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a transfer transaction
//	tx, err := builders.NewTransfer(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithAmount(2500, 2). // $25.00
//	    WithAssetCode("USD").
//	    WithDescription("Transfer between accounts").
//	    WithMetadata(map[string]any{
//	        "reference": "transfer-789",
//	        "purpose": "monthly-allocation",
//	    }).
//	    WithIdempotencyKey("transfer-idempotency-key-123").
//	    FromAccount("source-account").
//	    ToAccount("destination-account").
//	    Execute(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the transaction
//	fmt.Printf("Created transfer transaction: %s (status: %s)\n", tx.ID, tx.Status)
func (b *transferBuilder) Execute(ctx context.Context) (*models.Transaction, error) {
	// Validate builder state
	if err := b.validate(); err != nil {
		return nil, err
	}

	if b.sourceAccount == "" {
		return nil, fmt.Errorf("source account is required for a transfer")
	}

	if b.targetAccount == "" {
		return nil, fmt.Errorf("target account is required for a transfer")
	}

	if b.sourceAccount == b.targetAccount {
		return nil, fmt.Errorf("source and target accounts must be different")
	}

	// Build the transaction DSL input
	input := &models.TransactionDSLInput{
		Description: b.description,
		Metadata:    b.metadata,
		Send: &models.DSLSend{
			Asset: b.assetCode,
			Value: b.amount,
			Scale: int64(b.scale),
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: b.sourceAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: b.targetAccount,
						Amount: &models.DSLAmount{
							Value: b.amount,
							Scale: int64(b.scale),
							Asset: b.assetCode,
						},
					},
				},
			},
		},
	}

	// Validate the TransactionDSLInput
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction input: %v", err)
	}

	// Execute the transaction
	return b.client.CreateTransaction(ctx, b.orgID, b.ledgerID, input)
}
