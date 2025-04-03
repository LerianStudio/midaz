// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// OrganizationClientInterface defines the minimal client interface required by the organization builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for organization operations.
type OrganizationClientInterface interface {
	// CreateOrganization sends a request to create a new organization with the specified parameters.
	// It requires a context for the API request.
	// Returns the created organization or an error if the API request fails.
	CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error)

	// UpdateOrganization sends a request to update an existing organization with the specified parameters.
	// It requires a context for the API request and an organization ID.
	// Returns the updated organization or an error if the API request fails.
	UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error)
}

// OrganizationBuilder defines the builder interface for creating organizations.
// An organization is the top-level entity in the Midaz hierarchy that contains ledgers and accounts.
// This builder provides a fluent API for configuring and creating organization resources.
type OrganizationBuilder interface {
	// WithLegalName sets the legal name for the organization.
	// This is a required field for organization creation.
	WithLegalName(name string) OrganizationBuilder

	// WithLegalDocument sets the legal document (e.g., tax ID) for the organization.
	// This is a required field for organization creation.
	WithLegalDocument(document string) OrganizationBuilder

	// WithStatus sets the status for the organization.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "SUSPENDED".
	// If not specified, the default status is "ACTIVE".
	// TODO: Create a GET endpoint for organization statuses, aligning with the API spec.
	WithStatus(status string) OrganizationBuilder

	// WithAddress sets the address for the organization.
	// This is an optional field that provides location information for the organization.
	// All address components are optional, but it's recommended to provide as much information as possible.
	WithAddress(street, postalCode, city, state, country string) OrganizationBuilder

	// WithMetadata adds metadata to the organization.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) OrganizationBuilder

	// WithTag adds a single tag to the organization's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) OrganizationBuilder

	// WithTags adds multiple tags to the organization's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) OrganizationBuilder

	// Create executes the organization creation and returns the created organization.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Organization, error)
}

// OrganizationUpdateBuilder defines the builder interface for updating organizations.
// This builder provides a fluent API for configuring and updating existing organization resources.
type OrganizationUpdateBuilder interface {
	// WithLegalName sets the legal name for the organization.
	// This is an optional field for organization updates.
	WithLegalName(name string) OrganizationUpdateBuilder

	// WithLegalDocument sets the legal document (e.g., tax ID) for the organization.
	// This is an optional field for organization updates.
	WithLegalDocument(document string) OrganizationUpdateBuilder

	// WithStatus sets the status for the organization.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "SUSPENDED".
	// This is an optional field for organization updates.
	// TODO: Create a GET endpoint for organization statuses, aligning with the API spec.
	WithStatus(status string) OrganizationUpdateBuilder

	// WithAddress sets the address for the organization.
	// This is an optional field that provides location information for the organization.
	// All address components are optional, but it's recommended to provide as much information as possible.
	WithAddress(street, postalCode, city, state, country string) OrganizationUpdateBuilder

	// WithMetadata adds metadata to the organization.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) OrganizationUpdateBuilder

	// WithTag adds a single tag to the organization's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) OrganizationUpdateBuilder

	// WithTags adds multiple tags to the organization's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) OrganizationUpdateBuilder

	// Update executes the organization update operation and returns the updated organization.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Organization, error)
}

// baseOrganizationBuilder implements common functionality for organization builders.
type baseOrganizationBuilder struct {
	client        OrganizationClientInterface
	legalName     string
	legalDocument string
	status        string
	street        string
	postalCode    string
	city          string
	state         string
	country       string
	metadata      map[string]any
	tags          []string
}

// organizationBuilder implements the OrganizationBuilder interface.
type organizationBuilder struct {
	baseOrganizationBuilder
}

// NewOrganization creates a new builder for creating organizations.
//
// An organization is the top-level entity in the Midaz hierarchy that contains ledgers and accounts.
// This function returns a builder that allows for fluent configuration of the organization
// before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the OrganizationClientInterface with the CreateOrganization method.
//
// Returns:
//   - OrganizationBuilder: A builder interface for configuring and creating organization resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the organization creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an organization builder
//	orgBuilder := builders.NewOrganization(client)
//
//	// Configure and create the organization
//	org, err := orgBuilder.
//	    WithLegalName("Acme Corporation").
//	    WithLegalDocument("123456789").
//	    WithStatus("ACTIVE").
//	    WithAddress(
//	        "123 Main St",
//	        "94105",
//	        "San Francisco",
//	        "CA",
//	        "US",
//	    ).
//	    WithMetadata(map[string]any{
//	        "industry": "technology",
//	        "size": "enterprise",
//	    }).
//	    WithTag("customer").
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create organization: %v", err)
//	}
//
//	fmt.Printf("Organization created: %s\n", org.ID)
func NewOrganization(client OrganizationClientInterface) OrganizationBuilder {
	return &organizationBuilder{
		baseOrganizationBuilder: baseOrganizationBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *organizationBuilder) WithLegalName(name string) OrganizationBuilder { performs an operation
// WithLegalName sets the legal name for the organization.
func (b *organizationBuilder) WithLegalName(name string) OrganizationBuilder {
	b.legalName = name
	return b
}

// func (b *organizationBuilder) WithLegalDocument(document string) OrganizationBuilder { performs an operation
// WithLegalDocument sets the legal document (e.g., tax ID) for the organization.
func (b *organizationBuilder) WithLegalDocument(document string) OrganizationBuilder {
	b.legalDocument = document
	return b
}

// func (b *organizationBuilder) WithStatus(status string) OrganizationBuilder { performs an operation
// WithStatus sets the status for the organization.
func (b *organizationBuilder) WithStatus(status string) OrganizationBuilder {
	b.status = status
	return b
}

// func (b *organizationBuilder) WithAddress(street, postalCode, city, state, country string) OrganizationBuilder { performs an operation
// WithAddress sets the address for the organization.
func (b *organizationBuilder) WithAddress(street, postalCode, city, state, country string) OrganizationBuilder {
	b.street = street

	b.postalCode = postalCode

	b.city = city

	b.state = state

	b.country = country

	return b
}

// func (b *organizationBuilder) WithMetadata(metadata map[string]any) OrganizationBuilder { performs an operation
// WithMetadata adds metadata to the organization.
func (b *organizationBuilder) WithMetadata(metadata map[string]any) OrganizationBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *organizationBuilder) WithTag(tag string) OrganizationBuilder { performs an operation
// WithTag adds a single tag to the organization's metadata.
func (b *organizationBuilder) WithTag(tag string) OrganizationBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *organizationBuilder) WithTags(tags []string) OrganizationBuilder { performs an operation
// WithTags adds multiple tags to the organization's metadata.
func (b *organizationBuilder) WithTags(tags []string) OrganizationBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the organization creation and returns the created organization.
//
// This method validates all required parameters, constructs the organization creation input,
// and sends the request to the Midaz API to create the organization.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Organization: The created organization object if successful.
//     This contains all details about the organization, including its ID, legal name,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a basic organization with required fields
//	org, err := builders.NewOrganization(client).
//	    WithLegalName("Example Inc.").
//	    WithLegalDocument("987654321").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the organization
//	fmt.Printf("Created organization: %s (status: %s)\n", org.ID, org.Status.Code)
//
//	// Create an organization with complete details
//	org, err = builders.NewOrganization(client).
//	    WithLegalName("Advanced Corp").
//	    WithLegalDocument("123-45-6789").
//	    WithStatus("ACTIVE").
//	    WithAddress(
//	        "456 Business Ave",
//	        "10001",
//	        "New York",
//	        "NY",
//	        "US",
//	    ).
//	    WithMetadata(map[string]any{
//	        "sector": "finance",
//	        "established": 2020,
//	        "public": false,
//	    }).
//	    WithTags([]string{"client", "finance", "enterprise"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the organization
//	fmt.Printf("Created organization with metadata: %s\n", org.ID)
func (b *organizationBuilder) Create(ctx context.Context) (*models.Organization, error) {
	// Validate required fields
	if b.legalName == "" {
		return nil, fmt.Errorf("legal name is required")
	}

	if b.legalDocument == "" {
		return nil, fmt.Errorf("legal document is required")
	}

	// Create organization input
	input := &models.CreateOrganizationInput{
		LegalName:     b.legalName,
		LegalDocument: b.legalDocument,
		Status:        models.NewStatus(b.status),
		Metadata:      b.metadata,
	}

	// Add address if provided
	if b.street != "" || b.postalCode != "" || b.city != "" || b.state != "" || b.country != "" {
		input.Address = models.NewAddress(b.street, b.postalCode, b.city, b.state, b.country)
	}

	// Execute organization creation
	return b.client.CreateOrganization(ctx, input)
}

// organizationUpdateBuilder implements the OrganizationUpdateBuilder interface.
type organizationUpdateBuilder struct {
	baseOrganizationBuilder
	organizationID string
	fieldsToUpdate map[string]bool
}

// NewOrganizationUpdate creates a new builder for updating organizations.
// It initializes a builder with default values and the provided client interface
// for API communication.
//
// This function returns a builder that allows for fluent configuration of organization updates
// before applying them to an existing organization. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the OrganizationClientInterface with the UpdateOrganization method.
//   - id: The ID of the organization to update.
//
// Returns:
//   - OrganizationUpdateBuilder: A builder interface for configuring and executing organization updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the organization update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an organization update builder
//	updateBuilder := builders.NewOrganizationUpdate(client, "org-123")
//
//	// Configure and execute the update
//	updatedOrg, err := updateBuilder.
//	    WithLegalName("Updated Company Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "reason": "restructuring",
//	        "updated": true,
//	        "effectiveDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTags([]string{"archived", "inactive"}).
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update organization: %v", err)
//	}
//
//	fmt.Printf("Organization updated: %s (new status: %s)\n", updatedOrg.ID, updatedOrg.Status.Code)
func NewOrganizationUpdate(client OrganizationClientInterface, id string) OrganizationUpdateBuilder {
	return &organizationUpdateBuilder{
		baseOrganizationBuilder: baseOrganizationBuilder{
			client:   client,
			metadata: make(map[string]any),
		},
		organizationID: id,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *organizationUpdateBuilder) WithLegalName(name string) OrganizationUpdateBuilder { performs an operation
// WithLegalName sets the legal name for the organization.
func (b *organizationUpdateBuilder) WithLegalName(name string) OrganizationUpdateBuilder {
	b.legalName = name
	b.fieldsToUpdate["legalName"] = true

	return b
}

// func (b *organizationUpdateBuilder) WithLegalDocument(document string) OrganizationUpdateBuilder { performs an operation
// WithLegalDocument sets the legal document (e.g., tax ID) for the organization.
func (b *organizationUpdateBuilder) WithLegalDocument(document string) OrganizationUpdateBuilder {
	b.legalDocument = document
	b.fieldsToUpdate["legalDocument"] = true

	return b
}

// func (b *organizationUpdateBuilder) WithStatus(status string) OrganizationUpdateBuilder { performs an operation
// WithStatus sets the status for the organization.
func (b *organizationUpdateBuilder) WithStatus(status string) OrganizationUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *organizationUpdateBuilder) WithAddress(street, postalCode, city, state, country string) OrganizationUpdateBuilder { performs an operation
// WithAddress sets the address for the organization.
func (b *organizationUpdateBuilder) WithAddress(street, postalCode, city, state, country string) OrganizationUpdateBuilder {
	b.street = street

	b.postalCode = postalCode

	b.city = city

	b.state = state

	b.country = country
	b.fieldsToUpdate["address"] = true

	return b
}

// func (b *organizationUpdateBuilder) WithMetadata(metadata map[string]any) OrganizationUpdateBuilder { performs an operation
// WithMetadata adds metadata to the organization.
func (b *organizationUpdateBuilder) WithMetadata(metadata map[string]any) OrganizationUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *organizationUpdateBuilder) WithTag(tag string) OrganizationUpdateBuilder { performs an operation
// WithTag adds a single tag to the organization's metadata.
func (b *organizationUpdateBuilder) WithTag(tag string) OrganizationUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *organizationUpdateBuilder) WithTags(tags []string) OrganizationUpdateBuilder { performs an operation
// WithTags adds multiple tags to the organization's metadata.
func (b *organizationUpdateBuilder) WithTags(tags []string) OrganizationUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the organization update operation and returns the updated organization.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the organization. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Organization: The updated organization object if successful.
//     This contains all details about the organization, including its ID, legal name,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the legal name of an organization
//	updatedOrg, err := builders.NewOrganizationUpdate(client, "org-123").
//	    WithLegalName("New Legal Name Inc.").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated organization name: %s\n", updatedOrg.LegalName)
//
//	// Update multiple fields of an organization
//	updatedOrg, err = builders.NewOrganizationUpdate(client, "org-123").
//	    WithStatus("INACTIVE").
//	    WithAddress(
//	        "789 New Address St",
//	        "60601",
//	        "Chicago",
//	        "IL",
//	        "US",
//	    ).
//	    WithMetadata(map[string]any{
//	        "archived": true,
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTags([]string{"archived", "inactive"}).
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Organization status updated to: %s\n", updatedOrg.Status.Code)
func (b *organizationUpdateBuilder) Update(ctx context.Context) (*models.Organization, error) {
	// Check if any fields are set for update
	if len(b.fieldsToUpdate) == 0 {
		return nil, fmt.Errorf("no fields specified for update")
	}

	// Create update input
	input := &models.UpdateOrganizationInput{}

	// Add fields that are set for update
	if b.fieldsToUpdate["legalName"] {
		input.LegalName = b.legalName
	}

	if b.fieldsToUpdate["status"] {
		status := models.NewStatus(b.status)
		input.Status = status
	}

	if b.fieldsToUpdate["address"] {
		address := models.NewAddress(b.street, b.postalCode, b.city, b.state, b.country)
		input.Address = address
	}

	if b.fieldsToUpdate["metadata"] {
		input.Metadata = b.metadata
	}

	// Execute organization update
	return b.client.UpdateOrganization(ctx, b.organizationID, input)
}

// Helper function to update tags in metadata
func updateTagsInMetadata(metadata map[string]any, tags []string) {
	// Join tags with comma
	if len(tags) > 0 {
		var tagsStr string

		for i, tag := range tags {
			if i > 0 {
				tagsStr += ","
			}

			tagsStr += tag
		}

		metadata["tags"] = tagsStr
	}
}
