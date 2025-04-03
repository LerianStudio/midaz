// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AccountClientInterface defines the minimal client interface required by the account builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services.
type AccountClientInterface interface {
	// CreateAccount sends a request to create a new account with the specified parameters.
	// It requires a context for the API request.
	// Returns an error if the API request fails.
	CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error)

	// UpdateAccount sends a request to update an existing account with the specified parameters.
	// It requires a context for the API request.
	// Returns an error if the API request fails.
	UpdateAccount(ctx context.Context, organizationID, ledgerID, accountID string, input *models.UpdateAccountInput) (*models.Account, error)
}

// AccountBuilder defines the builder interface for creating accounts.
// It provides a fluent API for configuring and creating new account resources.
type AccountBuilder interface {
	// WithOrganization sets the organization ID for the account.
	// This is a required field for account creation.
	WithOrganization(orgID string) AccountBuilder

	// WithLedger sets the ledger ID for the account.
	// This is a required field for account creation.
	WithLedger(ledgerID string) AccountBuilder

	// WithName sets the name for the account.
	// This is a required field for account creation.
	WithName(name string) AccountBuilder

	// WithAssetCode sets the asset code for the account.
	// This is a required field for account creation.
	WithAssetCode(assetCode string) AccountBuilder

	// WithType sets the type for the account.
	// This is a required field for account creation.
	// Valid types include: "ASSET", "LIABILITY", "EQUITY", "REVENUE", "EXPENSE".
	// TODO: Create a GET endpoint for account types, aligning with the API spec.
	WithType(accountType string) AccountBuilder

	// WithParentAccount sets the parent account ID for the account.
	// This is an optional field that establishes a hierarchical relationship.
	WithParentAccount(parentAccountID string) AccountBuilder

	// WithEntityID sets the entity ID for the account.
	// This is an optional field that associates the account with a specific entity.
	WithEntityID(entityID string) AccountBuilder

	// WithPortfolio sets the portfolio ID for the account.
	// This is an optional field that associates the account with a specific portfolio.
	WithPortfolio(portfolioID string) AccountBuilder

	// WithSegment sets the segment ID for the account.
	// This is an optional field that associates the account with a specific segment.
	WithSegment(segmentID string) AccountBuilder

	// WithAlias sets the alias for the account.
	// This is an optional field that provides an alternative identifier for the account.
	WithAlias(alias string) AccountBuilder

	// WithStatus sets the status for the account.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// If not specified, the default status is "ACTIVE".
	// TODO: Create a GET endpoint for account statuses, aligning with the API spec.
	WithStatus(status string) AccountBuilder

	// WithMetadata adds metadata to the account.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) AccountBuilder

	// WithTag adds a single tag to the account's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTag(tag string) AccountBuilder

	// WithTags adds multiple tags to the account's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTags(tags []string) AccountBuilder

	// Create executes the account creation and returns the created account.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Account, error)
}

// AccountUpdateBuilder defines the builder interface for updating accounts.
// It provides a fluent API for configuring and updating existing account resources.
type AccountUpdateBuilder interface {
	// WithName sets the name for the account.
	// This is an optional field for account updates.
	WithName(name string) AccountUpdateBuilder

	// WithPortfolio sets the portfolio ID for the account.
	// This is an optional field for account updates.
	WithPortfolio(portfolioID string) AccountUpdateBuilder

	// WithSegment sets the segment ID for the account.
	// This is an optional field for account updates.
	WithSegment(segmentID string) AccountUpdateBuilder

	// WithStatus sets the status for the account.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// TODO: Create a GET endpoint for account statuses, aligning with the API spec.
	WithStatus(status string) AccountUpdateBuilder

	// WithMetadata adds metadata to the account.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) AccountUpdateBuilder

	// WithTag adds a single tag to the account's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTag(tag string) AccountUpdateBuilder

	// WithTags adds multiple tags to the account's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTags(tags []string) AccountUpdateBuilder

	// Update executes the account update operation and returns the updated account.
	//
	// This method validates that at least one field is set for update, constructs the update input,
	// and sends the request to the Midaz API to update the account. Only fields that have been
	// explicitly set using the With* methods will be included in the update.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//     This context is passed to the underlying API client.
	//
	// Returns:
	//   - *models.Account: The updated account object if successful.
	//     This contains all details about the account, including its ID, name, status,
	//     and other properties with the updated values.
	//   - error: An error if the operation fails. Possible error types include:
	//     - errors.ErrValidation: If no fields are specified for update
	//     - errors.ErrAuthentication: If authentication fails
	//     - errors.ErrPermission: If the client lacks permission
	//     - errors.ErrNotFound: If the organization, ledger, or account is not found
	//     - errors.ErrInternal: For other internal errors
	//
	// Example:
	//
	//	// Update only the name of an account
	//	updatedAccount, err := builders.NewAccountUpdate(client, "org-123", "ledger-456", "account-789").
	//	    WithName("New Account Name").
	//	    Update(context.Background())
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	fmt.Printf("Updated account name: %s\n", updatedAccount.Name)
	//
	//	// Update multiple fields of an account
	//	updatedAccount, err = builders.NewAccountUpdate(client, "org-123", "ledger-456", "account-789").
	//	    WithStatus("INACTIVE").
	//	    WithMetadata(map[string]any{
	//	        "archived": true,
	//	        "archivedDate": time.Now().Format(time.RFC3339),
	//	    }).
	//	    WithTag("archived").
	//	    Update(context.Background())
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	fmt.Printf("Account status updated to: %s\n", updatedAccount.Status.Code)
	Update(ctx context.Context) (*models.Account, error)
}

// baseAccountBuilder implements common functionality for account builders.
type baseAccountBuilder struct {
	client          AccountClientInterface
	organizationID  string
	ledgerID        string
	name            string
	assetCode       string
	accountType     string
	parentAccountID *string
	entityID        *string
	portfolioID     *string
	segmentID       *string
	alias           *string
	status          string
	metadata        map[string]any
	tags            []string
}

// accountBuilder implements the AccountBuilder interface.
type accountBuilder struct {
	baseAccountBuilder
}

// NewAccount creates a new builder for creating accounts.
//
// An account is a financial record used to track assets, liabilities, equity, revenue, or expenses.
// This function returns a builder that allows for fluent configuration of the account
// before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the AccountClientInterface with the CreateAccount method.
//
// Returns:
//   - AccountBuilder: A builder interface for configuring and creating account resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the account creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an account builder
//	accountBuilder := builders.NewAccount(client)
//
//	// Configure and create the account
//	account, err := accountBuilder.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Checking Account").
//	    WithAssetCode("USD").
//	    WithType("ASSET").
//	    WithStatus("ACTIVE").
//	    WithAlias("checking-001").
//	    WithMetadata(map[string]any{
//	        "department": "finance",
//	        "purpose": "operational",
//	    }).
//	    WithTag("primary").
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create account: %v", err)
//	}
//
//	fmt.Printf("Account created: %s\n", account.ID)
func NewAccount(client AccountClientInterface) AccountBuilder {
	return &accountBuilder{
		baseAccountBuilder: baseAccountBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *accountBuilder) WithOrganization sets the organization ID for the account.
func (b *accountBuilder) WithOrganization(orgID string) AccountBuilder {
	b.organizationID = orgID
	return b
}

// func (b *accountBuilder) WithLedger sets the ledger ID for the account.
func (b *accountBuilder) WithLedger(ledgerID string) AccountBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *accountBuilder) WithName sets the name for the account.
func (b *accountBuilder) WithName(name string) AccountBuilder {
	b.name = name
	return b
}

// func (b *accountBuilder) WithAssetCode sets the asset code for the account.
func (b *accountBuilder) WithAssetCode(assetCode string) AccountBuilder {
	b.assetCode = assetCode
	return b
}

// func (b *accountBuilder) WithType sets the type for the account.
func (b *accountBuilder) WithType(accountType string) AccountBuilder {
	b.accountType = accountType
	return b
}

// func (b *accountBuilder) WithParentAccount sets the parent account ID for the account.
func (b *accountBuilder) WithParentAccount(parentAccountID string) AccountBuilder {
	b.parentAccountID = &parentAccountID
	return b
}

// func (b *accountBuilder) WithEntityID sets the entity ID for the account.
func (b *accountBuilder) WithEntityID(entityID string) AccountBuilder {
	b.entityID = &entityID
	return b
}

// func (b *accountBuilder) WithPortfolio sets the portfolio ID for the account.
func (b *accountBuilder) WithPortfolio(portfolioID string) AccountBuilder {
	b.portfolioID = &portfolioID
	return b
}

// func (b *accountBuilder) WithSegment sets the segment ID for the account.
func (b *accountBuilder) WithSegment(segmentID string) AccountBuilder {
	b.segmentID = &segmentID
	return b
}

// func (b *accountBuilder) WithAlias sets the alias for the account.
func (b *accountBuilder) WithAlias(alias string) AccountBuilder {
	b.alias = &alias
	return b
}

// func (b *accountBuilder) WithStatus sets the status for the account.
func (b *accountBuilder) WithStatus(status string) AccountBuilder {
	b.status = status
	return b
}

// func (b *accountBuilder) WithMetadata adds metadata to the account.
func (b *accountBuilder) WithMetadata(metadata map[string]any) AccountBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *accountBuilder) WithTag adds a single tag to the account's metadata.
func (b *accountBuilder) WithTag(tag string) AccountBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *accountBuilder) WithTags adds multiple tags to the account's metadata.
func (b *accountBuilder) WithTags(tags []string) AccountBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the account creation and returns the created account.
//
// This method validates all required parameters, constructs the account creation input,
// and sends the request to the Midaz API to create the account.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Account: The created account object if successful.
//     This contains all details about the account, including its ID, name, type,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization or ledger is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a basic asset account
//	account, err := builders.NewAccount(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Cash Account").
//	    WithAssetCode("USD").
//	    WithType("ASSET").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the account
//	fmt.Printf("Created account: %s (type: %s)\n", account.ID, account.Type)
//
//	// Create a liability account with additional properties
//	account, err = builders.NewAccount(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Credit Card").
//	    WithAssetCode("USD").
//	    WithType("LIABILITY").
//	    WithStatus("ACTIVE").
//	    WithAlias("cc-main").
//	    WithMetadata(map[string]any{
//	        "cardType": "visa",
//	        "lastFour": "1234",
//	    }).
//	    WithTags([]string{"credit", "revolving"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the account
//	fmt.Printf("Created account with metadata: %s\n", account.ID)
func (b *accountBuilder) Create(ctx context.Context) (*models.Account, error) {
	// Validate required fields
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if b.name == "" {
		return nil, fmt.Errorf("name is required")
	}

	if b.assetCode == "" {
		return nil, fmt.Errorf("asset code is required")
	}

	if b.accountType == "" {
		return nil, fmt.Errorf("account type is required")
	}

	// Create account input
	input := &models.CreateAccountInput{
		Name:      b.name,
		AssetCode: b.assetCode,
		Type:      b.accountType,
		Status:    models.NewStatus(b.status),
		Metadata:  b.metadata,
	}

	// Set optional fields if provided
	if b.parentAccountID != nil {
		input.ParentAccountID = b.parentAccountID
	}

	if b.entityID != nil {
		input.EntityID = b.entityID
	}

	if b.portfolioID != nil {
		input.PortfolioID = b.portfolioID
	}

	if b.segmentID != nil {
		input.SegmentID = b.segmentID
	}

	if b.alias != nil {
		input.Alias = b.alias
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account input: %v", err)
	}

	// Execute account creation
	return b.client.CreateAccount(ctx, b.organizationID, b.ledgerID, input)
}

// accountUpdateBuilder implements the AccountUpdateBuilder interface.
type accountUpdateBuilder struct {
	baseAccountBuilder
	accountID      string
	fieldsToUpdate map[string]bool
}

// NewAccountUpdate creates a new builder for updating accounts.
//
// This function returns a builder that allows for fluent configuration of account updates
// before applying them to an existing account. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the AccountClientInterface with the UpdateAccount method.
//   - orgID: The organization ID that the account belongs to.
//   - ledgerID: The ledger ID that the account belongs to.
//   - accountID: The ID of the account to update.
//
// Returns:
//   - AccountUpdateBuilder: A builder interface for configuring and executing account updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the account update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an account update builder
//	updateBuilder := builders.NewAccountUpdate(client, "org-123", "ledger-456", "account-789")
//
//	// Configure and execute the update
//	updatedAccount, err := updateBuilder.
//	    WithName("Updated Account Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "department": "finance",
//	        "updated": true,
//	        "reason": "consolidation",
//	    }).
//	    WithTag("primary").
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update account: %v", err)
//	}
//
//	fmt.Printf("Account updated: %s (new status: %s)\n", updatedAccount.ID, updatedAccount.Status.Code)
func NewAccountUpdate(client AccountClientInterface, orgID, ledgerID, accountID string) AccountUpdateBuilder {
	return &accountUpdateBuilder{
		baseAccountBuilder: baseAccountBuilder{
			client:         client,
			organizationID: orgID,
			ledgerID:       ledgerID,
			metadata:       make(map[string]any),
		},
		accountID:      accountID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *accountUpdateBuilder) WithName sets the name for the account.
func (b *accountUpdateBuilder) WithName(name string) AccountUpdateBuilder {
	b.name = name
	b.fieldsToUpdate["name"] = true

	return b
}

// func (b *accountUpdateBuilder) WithPortfolio sets the portfolio ID for the account.
func (b *accountUpdateBuilder) WithPortfolio(portfolioID string) AccountUpdateBuilder {
	b.portfolioID = &portfolioID
	b.fieldsToUpdate["portfolioID"] = true

	return b
}

// func (b *accountUpdateBuilder) WithSegment sets the segment ID for the account.
func (b *accountUpdateBuilder) WithSegment(segmentID string) AccountUpdateBuilder {
	b.segmentID = &segmentID
	b.fieldsToUpdate["segmentID"] = true

	return b
}

// func (b *accountUpdateBuilder) WithStatus sets the status for the account.
func (b *accountUpdateBuilder) WithStatus(status string) AccountUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *accountUpdateBuilder) WithMetadata adds metadata to the account.
func (b *accountUpdateBuilder) WithMetadata(metadata map[string]any) AccountUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *accountUpdateBuilder) WithTag adds a single tag to the account's metadata.
func (b *accountUpdateBuilder) WithTag(tag string) AccountUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *accountUpdateBuilder) WithTags adds multiple tags to the account's metadata.
func (b *accountUpdateBuilder) WithTags(tags []string) AccountUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the account update operation and returns the updated account.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the account. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Account: The updated account object if successful.
//     This contains all details about the account, including its ID, name, status,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or account is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the name of an account
//	updatedAccount, err := builders.NewAccountUpdate(client, "org-123", "ledger-456", "account-789").
//	    WithName("New Account Name").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated account name: %s\n", updatedAccount.Name)
//
//	// Update multiple fields of an account
//	updatedAccount, err = builders.NewAccountUpdate(client, "org-123", "ledger-456", "account-789").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "archived": true,
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTag("archived").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Account status updated to: %s\n", updatedAccount.Status.Code)
func (b *accountUpdateBuilder) Update(ctx context.Context) (*models.Account, error) {
	// Check if any fields are set for update
	if len(b.fieldsToUpdate) == 0 {
		return nil, fmt.Errorf("no fields specified for update")
	}

	// Validate required IDs
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if b.accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	// Create update input
	input := models.NewUpdateAccountInput()

	// Add fields that are set for update
	if b.fieldsToUpdate["name"] {
		input.WithName(b.name)
	}

	if b.fieldsToUpdate["status"] {
		input.WithStatus(models.NewStatus(b.status))
	}

	if b.fieldsToUpdate["portfolioID"] && b.portfolioID != nil {
		input.WithPortfolioID(*b.portfolioID)
	}

	if b.fieldsToUpdate["segmentID"] && b.segmentID != nil {
		input.WithSegmentID(*b.segmentID)
	}

	if b.fieldsToUpdate["metadata"] {
		input.WithMetadata(b.metadata)
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account update input: %v", err)
	}

	// Execute account update
	return b.client.UpdateAccount(ctx, b.organizationID, b.ledgerID, b.accountID, input)
}
