// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// PortfolioClientInterface defines the minimal client interface required by the portfolio builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for portfolio operations.
type PortfolioClientInterface interface {
	// CreatePortfolio sends a request to create a new portfolio with the specified parameters.
	// It requires a context for the API request, organization ID, and ledger ID.
	// Returns the created portfolio or an error if the API request fails.
	CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error)

	// UpdatePortfolio sends a request to update an existing portfolio with the specified parameters.
	// It requires a context for the API request, organization ID, ledger ID, and portfolio ID.
	// Returns the updated portfolio or an error if the API request fails.
	UpdatePortfolio(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.UpdatePortfolioInput) (*models.Portfolio, error)
}

// PortfolioBuilder defines the builder interface for creating portfolios.
// A portfolio is a grouping mechanism for accounts, typically representing a customer or business unit.
// This builder provides a fluent API for configuring and creating portfolio resources.
type PortfolioBuilder interface {
	// WithOrganization sets the organization ID for the portfolio.
	// This is a required field for portfolio creation.
	WithOrganization(orgID string) PortfolioBuilder

	// WithLedger sets the ledger ID for the portfolio.
	// This is a required field for portfolio creation.
	WithLedger(ledgerID string) PortfolioBuilder

	// WithName sets the name for the portfolio.
	// This is a required field for portfolio creation.
	WithName(name string) PortfolioBuilder

	// WithEntityID sets the entity ID for the portfolio.
	// This is a required field for portfolio creation that links the portfolio to an entity.
	// An entity typically represents a customer, user, or business unit.
	WithEntityID(entityID string) PortfolioBuilder

	// WithStatus sets the status for the portfolio.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// If not specified, the default status is "ACTIVE".
	WithStatus(status string) PortfolioBuilder

	// WithMetadata adds metadata to the portfolio.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) PortfolioBuilder

	// WithTag adds a single tag to the portfolio's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) PortfolioBuilder

	// WithTags adds multiple tags to the portfolio's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) PortfolioBuilder

	// Create executes the portfolio creation and returns the created portfolio.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Portfolio, error)
}

// PortfolioUpdateBuilder defines the builder interface for updating portfolios.
// This builder provides a fluent API for configuring and updating existing portfolio resources.
type PortfolioUpdateBuilder interface {
	// WithName sets the name for the portfolio.
	// This is an optional field for portfolio updates.
	WithName(name string) PortfolioUpdateBuilder

	// WithStatus sets the status for the portfolio.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// This is an optional field for portfolio updates.
	WithStatus(status string) PortfolioUpdateBuilder

	// WithMetadata adds metadata to the portfolio.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) PortfolioUpdateBuilder

	// WithTag adds a single tag to the portfolio's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) PortfolioUpdateBuilder

	// WithTags adds multiple tags to the portfolio's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) PortfolioUpdateBuilder

	// Update executes the portfolio update operation and returns the updated portfolio.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Portfolio, error)
}

// basePortfolioBuilder implements common functionality for portfolio builders.
type basePortfolioBuilder struct {
	client         PortfolioClientInterface
	organizationID string
	ledgerID       string
	name           string
	entityID       string
	status         string
	metadata       map[string]any
	tags           []string
}

// portfolioBuilder implements the PortfolioBuilder interface.
type portfolioBuilder struct {
	basePortfolioBuilder
}

// NewPortfolio creates a new builder for creating portfolios.
//
// Portfolios are grouping mechanisms for accounts, typically representing a customer or business unit.
// This function returns a builder that allows for fluent configuration of the portfolio
// before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the PortfolioClientInterface with the CreatePortfolio method.
//
// Returns:
//   - PortfolioBuilder: A builder interface for configuring and creating portfolio resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the portfolio creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a portfolio builder
//	portfolioBuilder := builders.NewPortfolio(client)
//
//	// Configure and create the portfolio
//	portfolio, err := portfolioBuilder.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Customer Portfolio").
//	    WithEntityID("entity-789").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "customerType": "enterprise",
//	        "region": "north-america",
//	    }).
//	    WithTags([]string{"customer", "enterprise"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create portfolio: %v", err)
//	}
//
//	fmt.Printf("Portfolio created: %s\n", portfolio.ID)
func NewPortfolio(client PortfolioClientInterface) PortfolioBuilder {
	return &portfolioBuilder{
		basePortfolioBuilder: basePortfolioBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *portfolioBuilder) WithOrganization(orgID string) PortfolioBuilder { performs an operation
// WithOrganization sets the organization ID for the portfolio.
func (b *portfolioBuilder) WithOrganization(orgID string) PortfolioBuilder {
	b.organizationID = orgID
	return b
}

// func (b *portfolioBuilder) WithLedger(ledgerID string) PortfolioBuilder { performs an operation
// WithLedger sets the ledger ID for the portfolio.
func (b *portfolioBuilder) WithLedger(ledgerID string) PortfolioBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *portfolioBuilder) WithName(name string) PortfolioBuilder { performs an operation
// WithName sets the name for the portfolio.
func (b *portfolioBuilder) WithName(name string) PortfolioBuilder {
	b.name = name
	return b
}

// func (b *portfolioBuilder) WithEntityID(entityID string) PortfolioBuilder { performs an operation
// WithEntityID sets the entity ID for the portfolio.
func (b *portfolioBuilder) WithEntityID(entityID string) PortfolioBuilder {
	b.entityID = entityID
	return b
}

// func (b *portfolioBuilder) WithStatus(status string) PortfolioBuilder { performs an operation
// WithStatus sets the status for the portfolio.
func (b *portfolioBuilder) WithStatus(status string) PortfolioBuilder {
	b.status = status
	return b
}

// func (b *portfolioBuilder) WithMetadata(metadata map[string]any) PortfolioBuilder { performs an operation
// WithMetadata adds metadata to the portfolio.
func (b *portfolioBuilder) WithMetadata(metadata map[string]any) PortfolioBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *portfolioBuilder) WithTag(tag string) PortfolioBuilder { performs an operation
// WithTag adds a single tag to the portfolio's metadata.
func (b *portfolioBuilder) WithTag(tag string) PortfolioBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *portfolioBuilder) WithTags(tags []string) PortfolioBuilder { performs an operation
// WithTags adds multiple tags to the portfolio's metadata.
func (b *portfolioBuilder) WithTags(tags []string) PortfolioBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the portfolio creation and returns the created portfolio.
//
// This method validates all required parameters, constructs the portfolio creation input,
// and sends the request to the Midaz API to create the portfolio.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Portfolio: The created portfolio object if successful.
//     This contains all details about the portfolio, including its ID, name, entity ID,
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
//	// Create a basic portfolio with required fields
//	portfolio, err := builders.NewPortfolio(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Personal Portfolio").
//	    WithEntityID("entity-789").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the portfolio
//	fmt.Printf("Created portfolio: %s (entity: %s)\n", portfolio.ID, portfolio.EntityID)
//
//	// Create a portfolio with additional properties
//	portfolio, err = builders.NewPortfolio(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Business Portfolio").
//	    WithEntityID("entity-abc").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "businessType": "LLC",
//	        "industry": "technology",
//	        "established": 2020,
//	    }).
//	    WithTags([]string{"business", "technology"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the portfolio
//	fmt.Printf("Created portfolio with metadata: %s\n", portfolio.ID)
func (b *portfolioBuilder) Create(ctx context.Context) (*models.Portfolio, error) {
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

	if b.entityID == "" {
		return nil, fmt.Errorf("entity ID is required")
	}

	// Create portfolio input
	input := &models.CreatePortfolioInput{
		Name:     b.name,
		EntityID: b.entityID,
		Status:   models.NewStatus(b.status),
		Metadata: b.metadata,
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid portfolio input: %v", err)
	}

	// Execute portfolio creation
	return b.client.CreatePortfolio(ctx, b.organizationID, b.ledgerID, input)
}

// portfolioUpdateBuilder implements the PortfolioUpdateBuilder interface.
type portfolioUpdateBuilder struct {
	basePortfolioBuilder
	portfolioID    string
	fieldsToUpdate map[string]bool
}

// NewPortfolioUpdate creates a new builder for updating portfolios.
//
// This function returns a builder that allows for fluent configuration of portfolio updates
// before applying them to an existing portfolio. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the PortfolioClientInterface with the UpdatePortfolio method.
//   - orgID: The organization ID that the portfolio belongs to.
//   - ledgerID: The ledger ID that the portfolio belongs to.
//   - portfolioID: The ID of the portfolio to update.
//
// Returns:
//   - PortfolioUpdateBuilder: A builder interface for configuring and executing portfolio updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the portfolio update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a portfolio update builder
//	updateBuilder := builders.NewPortfolioUpdate(client, "org-123", "ledger-456", "portfolio-789")
//
//	// Configure and execute the update
//	updatedPortfolio, err := updateBuilder.
//	    WithName("Updated Portfolio Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "updated": true,
//	        "reason": "rebranding",
//	        "updatedDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTags([]string{"inactive", "archived"}).
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update portfolio: %v", err)
//	}
//
//	fmt.Printf("Portfolio updated: %s (new status: %s)\n", updatedPortfolio.ID, updatedPortfolio.Status.Code)
func NewPortfolioUpdate(client PortfolioClientInterface, orgID, ledgerID, portfolioID string) PortfolioUpdateBuilder {
	return &portfolioUpdateBuilder{
		basePortfolioBuilder: basePortfolioBuilder{
			client:         client,
			organizationID: orgID,
			ledgerID:       ledgerID,
			metadata:       make(map[string]any),
		},
		portfolioID:    portfolioID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *portfolioUpdateBuilder) WithName(name string) PortfolioUpdateBuilder { performs an operation
// WithName sets the name for the portfolio.
func (b *portfolioUpdateBuilder) WithName(name string) PortfolioUpdateBuilder {
	b.name = name
	b.fieldsToUpdate["name"] = true

	return b
}

// func (b *portfolioUpdateBuilder) WithStatus(status string) PortfolioUpdateBuilder { performs an operation
// WithStatus sets the status for the portfolio.
func (b *portfolioUpdateBuilder) WithStatus(status string) PortfolioUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *portfolioUpdateBuilder) WithMetadata(metadata map[string]any) PortfolioUpdateBuilder { performs an operation
// WithMetadata adds metadata to the portfolio.
func (b *portfolioUpdateBuilder) WithMetadata(metadata map[string]any) PortfolioUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *portfolioUpdateBuilder) WithTag(tag string) PortfolioUpdateBuilder { performs an operation
// WithTag adds a single tag to the portfolio's metadata.
func (b *portfolioUpdateBuilder) WithTag(tag string) PortfolioUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *portfolioUpdateBuilder) WithTags(tags []string) PortfolioUpdateBuilder { performs an operation
// WithTags adds multiple tags to the portfolio's metadata.
func (b *portfolioUpdateBuilder) WithTags(tags []string) PortfolioUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the portfolio update operation and returns the updated portfolio.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the portfolio. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Portfolio: The updated portfolio object if successful.
//     This contains all details about the portfolio, including its ID, name, status,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or portfolio is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the name of a portfolio
//	updatedPortfolio, err := builders.NewPortfolioUpdate(client, "org-123", "ledger-456", "portfolio-789").
//	    WithName("New Portfolio Name").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated portfolio name: %s\n", updatedPortfolio.Name)
//
//	// Update multiple fields of a portfolio
//	updatedPortfolio, err = builders.NewPortfolioUpdate(client, "org-123", "ledger-456", "portfolio-789").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "archived": true,
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	        "reason": "customer request",
//	    }).
//	    WithTag("archived").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Portfolio status updated to: %s\n", updatedPortfolio.Status.Code)
func (b *portfolioUpdateBuilder) Update(ctx context.Context) (*models.Portfolio, error) {
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

	if b.portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	// Create update input
	input := &models.UpdatePortfolioInput{}

	// Add fields that are set for update
	if b.fieldsToUpdate["name"] {
		input.Name = b.name
	}

	if b.fieldsToUpdate["status"] {
		input.Status = models.NewStatus(b.status)
	}

	if b.fieldsToUpdate["metadata"] {
		input.Metadata = b.metadata
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid portfolio update input: %v", err)
	}

	// Execute portfolio update
	return b.client.UpdatePortfolio(ctx, b.organizationID, b.ledgerID, b.portfolioID, input)
}
