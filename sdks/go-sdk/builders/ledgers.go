// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// LedgerClientInterface defines the minimal client interface required by the ledger builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for ledger operations.
type LedgerClientInterface interface {
	// CreateLedger sends a request to create a new ledger with the specified parameters.
	// It requires a context for the API request and an organization ID.
	// Returns the created ledger or an error if the API request fails.
	CreateLedger(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error)

	// UpdateLedger sends a request to update an existing ledger with the specified parameters.
	// It requires a context for the API request, organization ID, and ledger ID.
	// Returns the updated ledger or an error if the API request fails.
	UpdateLedger(ctx context.Context, organizationID, ledgerID string, input *models.UpdateLedgerInput) (*models.Ledger, error)
}

// LedgerBuilder defines the builder interface for creating ledgers.
// A ledger is a core financial record-keeping structure that contains accounts and transactions.
// This builder provides a fluent API for configuring and creating ledger resources.
type LedgerBuilder interface {
	// WithOrganization sets the organization ID for the ledger.
	// This is a required field for ledger creation.
	WithOrganization(orgID string) LedgerBuilder

	// WithName sets the name for the ledger.
	// This is a required field for ledger creation.
	WithName(name string) LedgerBuilder

	// WithStatus sets the status for the ledger.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "ARCHIVED".
	// If not specified, the default status is "ACTIVE".
	// TODO: Create a GET endpoint for ledger statuses, aligning with the API spec.
	WithStatus(status string) LedgerBuilder

	// WithMetadata adds metadata to the ledger.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) LedgerBuilder

	// WithTag adds a single tag to the ledger's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) LedgerBuilder

	// WithTags adds multiple tags to the ledger's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) LedgerBuilder

	// Create executes the ledger creation and returns the created ledger.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Ledger, error)
}

// LedgerUpdateBuilder defines the builder interface for updating ledgers.
// This builder provides a fluent API for configuring and updating existing ledger resources.
type LedgerUpdateBuilder interface {
	// WithOrganization sets the organization ID for the ledger.
	// This method is typically not needed as the organization ID is already set when creating the builder.
	WithOrganization(orgID string) LedgerUpdateBuilder

	// WithName sets the name for the ledger.
	// This is an optional field for ledger updates.
	WithName(name string) LedgerUpdateBuilder

	// WithStatus sets the status for the ledger.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "ARCHIVED".
	// This is an optional field for ledger updates.
	// TODO: Create a GET endpoint for ledger statuses, aligning with the API spec.
	WithStatus(status string) LedgerUpdateBuilder

	// WithMetadata adds metadata to the ledger.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) LedgerUpdateBuilder

	// WithTag adds a single tag to the ledger's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) LedgerUpdateBuilder

	// WithTags adds multiple tags to the ledger's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) LedgerUpdateBuilder

	// Update executes the ledger update operation and returns the updated ledger.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Ledger, error)
}

// baseLedgerBuilder implements common functionality for ledger builders.
type baseLedgerBuilder struct {
	client         LedgerClientInterface
	organizationID string
	name           string
	status         string
	metadata       map[string]any
	tags           []string
}

// ledgerBuilder implements the LedgerBuilder interface.
type ledgerBuilder struct {
	baseLedgerBuilder
}

// NewLedger creates a new builder for creating ledgers.
//
// A ledger is a core financial record-keeping structure that contains accounts and transactions.
// This function returns a builder that allows for fluent configuration of the ledger
// before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the LedgerClientInterface with the CreateLedger method.
//
// Returns:
//   - LedgerBuilder: A builder interface for configuring and creating ledger resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the ledger creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a ledger builder
//	ledgerBuilder := builders.NewLedger(client)
//
//	// Configure and create the ledger
//	ledger, err := ledgerBuilder.
//	    WithOrganization("org-123").
//	    WithName("Main Ledger").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "department": "finance",
//	        "purpose": "general-accounting",
//	    }).
//	    WithTag("production").
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create ledger: %v", err)
//	}
//
//	fmt.Printf("Ledger created: %s\n", ledger.ID)
func NewLedger(client LedgerClientInterface) LedgerBuilder {
	return &ledgerBuilder{
		baseLedgerBuilder: baseLedgerBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *ledgerBuilder) WithOrganization(orgID string) LedgerBuilder { performs an operation
// WithOrganization sets the organization ID for the ledger.
func (b *ledgerBuilder) WithOrganization(orgID string) LedgerBuilder {
	b.organizationID = orgID
	return b
}

// func (b *ledgerBuilder) WithName(name string) LedgerBuilder { performs an operation
// WithName sets the name for the ledger.
func (b *ledgerBuilder) WithName(name string) LedgerBuilder {
	b.name = name
	return b
}

// func (b *ledgerBuilder) WithStatus(status string) LedgerBuilder { performs an operation
// WithStatus sets the status for the ledger.
func (b *ledgerBuilder) WithStatus(status string) LedgerBuilder {
	b.status = status
	return b
}

// func (b *ledgerBuilder) WithMetadata(metadata map[string]any) LedgerBuilder { performs an operation
// WithMetadata adds metadata to the ledger.
func (b *ledgerBuilder) WithMetadata(metadata map[string]any) LedgerBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *ledgerBuilder) WithTag(tag string) LedgerBuilder { performs an operation
// WithTag adds a single tag to the ledger's metadata.
func (b *ledgerBuilder) WithTag(tag string) LedgerBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *ledgerBuilder) WithTags(tags []string) LedgerBuilder { performs an operation
// WithTags adds multiple tags to the ledger's metadata.
func (b *ledgerBuilder) WithTags(tags []string) LedgerBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the ledger creation and returns the created ledger.
//
// This method validates all required parameters, constructs the ledger creation input,
// and sends the request to the Midaz API to create the ledger.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Ledger: The created ledger object if successful.
//     This contains all details about the ledger, including its ID, name, status,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a ledger with minimal required fields
//	ledger, err := builders.NewLedger(client).
//	    WithOrganization("org-123").
//	    WithName("Main Ledger").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the ledger
//	fmt.Printf("Created ledger: %s (status: %s)\n", ledger.ID, ledger.Status)
//
//	// Create a ledger with additional metadata and tags
//	ledger, err = builders.NewLedger(client).
//	    WithOrganization("org-123").
//	    WithName("Secondary Ledger").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "department": "finance",
//	        "purpose": "subsidiary-accounting",
//	    }).
//	    WithTags([]string{"subsidiary", "finance", "reporting"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the ledger
//	fmt.Printf("Created ledger with metadata: %s\n", ledger.ID)
func (b *ledgerBuilder) Create(ctx context.Context) (*models.Ledger, error) {
	// Validate builder state
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Build the ledger creation input
	input := &models.CreateLedgerInput{
		Name:     b.name,
		Status:   models.NewStatus(b.status),
		Metadata: b.metadata,
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ledger input: %v", err)
	}

	// Execute the ledger creation
	return b.client.CreateLedger(ctx, b.organizationID, input)
}

// ledgerUpdateBuilder implements the LedgerUpdateBuilder interface.
type ledgerUpdateBuilder struct {
	baseLedgerBuilder
	ledgerID       string
	fieldsToUpdate map[string]bool
}

// NewLedgerUpdate creates a new builder for updating ledgers.
//
// This function returns a builder that allows for fluent configuration of ledger updates
// before applying them to an existing ledger. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the LedgerClientInterface with the UpdateLedger method.
//   - orgID: The organization ID that the ledger belongs to.
//   - ledgerID: The ID of the ledger to update.
//
// Returns:
//   - LedgerUpdateBuilder: A builder interface for configuring and executing ledger updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the ledger update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a ledger update builder
//	updateBuilder := builders.NewLedgerUpdate(client, "org-123", "ledger-456")
//
//	// Configure and execute the update
//	updatedLedger, err := updateBuilder.
//	    WithName("Updated Ledger Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "department": "finance",
//	        "updated": true,
//	        "reason": "reorganization",
//	    }).
//	    WithTag("archived").
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update ledger: %v", err)
//	}
//
//	fmt.Printf("Ledger updated: %s (new status: %s)\n", updatedLedger.ID, updatedLedger.Status.Code)
func NewLedgerUpdate(client LedgerClientInterface, orgID, ledgerID string) LedgerUpdateBuilder {
	return &ledgerUpdateBuilder{
		baseLedgerBuilder: baseLedgerBuilder{
			client:         client,
			organizationID: orgID,
			metadata:       make(map[string]any),
		},
		ledgerID:       ledgerID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *ledgerUpdateBuilder) WithOrganization(orgID string) LedgerUpdateBuilder { performs an operation
// WithOrganization sets the organization ID for the ledger.
func (b *ledgerUpdateBuilder) WithOrganization(orgID string) LedgerUpdateBuilder {
	b.organizationID = orgID
	return b
}

// func (b *ledgerUpdateBuilder) WithName(name string) LedgerUpdateBuilder { performs an operation
// WithName sets the name for the ledger.
func (b *ledgerUpdateBuilder) WithName(name string) LedgerUpdateBuilder {
	b.name = name
	b.fieldsToUpdate["name"] = true

	return b
}

// func (b *ledgerUpdateBuilder) WithStatus(status string) LedgerUpdateBuilder { performs an operation
// WithStatus sets the status for the ledger.
func (b *ledgerUpdateBuilder) WithStatus(status string) LedgerUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *ledgerUpdateBuilder) WithMetadata(metadata map[string]any) LedgerUpdateBuilder { performs an operation
// WithMetadata adds metadata to the ledger.
func (b *ledgerUpdateBuilder) WithMetadata(metadata map[string]any) LedgerUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *ledgerUpdateBuilder) WithTag(tag string) LedgerUpdateBuilder { performs an operation
// WithTag adds a single tag to the ledger's metadata.
func (b *ledgerUpdateBuilder) WithTag(tag string) LedgerUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *ledgerUpdateBuilder) WithTags(tags []string) LedgerUpdateBuilder { performs an operation
// WithTags adds multiple tags to the ledger's metadata.
func (b *ledgerUpdateBuilder) WithTags(tags []string) LedgerUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the ledger update operation and returns the updated ledger.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the ledger. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Ledger: The updated ledger object if successful.
//     This contains all details about the ledger, including its ID, name, status,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization or ledger is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the name of a ledger
//	updatedLedger, err := builders.NewLedgerUpdate(client, "org-123", "ledger-456").
//	    WithName("New Ledger Name").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated ledger name: %s\n", updatedLedger.Name)
//
//	// Update multiple fields of a ledger
//	updatedLedger, err = builders.NewLedgerUpdate(client, "org-123", "ledger-456").
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
//	fmt.Printf("Ledger status updated to: %s\n", updatedLedger.Status.Code)
func (b *ledgerUpdateBuilder) Update(ctx context.Context) (*models.Ledger, error) {
	// Check if any fields are set for update
	if len(b.fieldsToUpdate) == 0 {
		return nil, fmt.Errorf("no fields specified for update")
	}

	// Create update input
	input := models.NewUpdateLedgerInput()

	// Add fields that are set for update
	if b.fieldsToUpdate["name"] {
		input.WithName(b.name)
	}

	if b.fieldsToUpdate["status"] {
		input.WithStatus(models.NewStatus(b.status))
	}

	if b.fieldsToUpdate["metadata"] {
		input.WithMetadata(b.metadata)
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ledger update input: %v", err)
	}

	// Execute ledger update
	return b.client.UpdateLedger(ctx, b.organizationID, b.ledgerID, input)
}
