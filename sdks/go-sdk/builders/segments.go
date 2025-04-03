// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// SegmentClientInterface defines the minimal client interface required by the segment builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for segment operations.
type SegmentClientInterface interface {
	// CreateSegment sends a request to create a new segment with the specified parameters.
	// It requires a context for the API request, organization ID, ledger ID, and portfolio ID.
	// Returns the created segment or an error if the API request fails.
	CreateSegment(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error)

	// UpdateSegment sends a request to update an existing segment with the specified parameters.
	// It requires a context for the API request, organization ID, ledger ID, portfolio ID, and segment ID.
	// Returns the updated segment or an error if the API request fails.
	UpdateSegment(ctx context.Context, organizationID, ledgerID, portfolioID, segmentID string, input *models.UpdateSegmentInput) (*models.Segment, error)
}

// SegmentBuilder defines the builder interface for creating segments.
// A segment is a subdivision of a portfolio, typically representing a specific business area,
// project, or classification within a portfolio.
// This builder provides a fluent API for configuring and creating segment resources.
type SegmentBuilder interface {
	// WithOrganization sets the organization ID for the segment.
	// This is a required field for segment creation.
	WithOrganization(orgID string) SegmentBuilder

	// WithLedger sets the ledger ID for the segment.
	// This is a required field for segment creation.
	WithLedger(ledgerID string) SegmentBuilder

	// WithPortfolio sets the portfolio ID for the segment.
	// This is a required field for segment creation.
	// A segment must belong to a portfolio.
	WithPortfolio(portfolioID string) SegmentBuilder

	// WithName sets the name for the segment.
	// This is a required field for segment creation.
	WithName(name string) SegmentBuilder

	// WithStatus sets the status for the segment.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// If not specified, the default status is "ACTIVE".
	WithStatus(status string) SegmentBuilder

	// WithMetadata adds metadata to the segment.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) SegmentBuilder

	// WithTag adds a single tag to the segment's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) SegmentBuilder

	// WithTags adds multiple tags to the segment's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) SegmentBuilder

	// Create executes the segment creation and returns the created segment.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Segment, error)
}

// SegmentUpdateBuilder defines the builder interface for updating segments.
// This builder provides a fluent API for configuring and updating existing segment resources.
type SegmentUpdateBuilder interface {
	// WithName sets the name for the segment.
	// This is an optional field for segment updates.
	WithName(name string) SegmentUpdateBuilder

	// WithStatus sets the status for the segment.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "CLOSED".
	// This is an optional field for segment updates.
	WithStatus(status string) SegmentUpdateBuilder

	// WithMetadata adds metadata to the segment.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) SegmentUpdateBuilder

	// WithTag adds a single tag to the segment's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTag(tag string) SegmentUpdateBuilder

	// WithTags adds multiple tags to the segment's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	// This is an optional field that helps with categorization and searching.
	WithTags(tags []string) SegmentUpdateBuilder

	// Update executes the segment update operation and returns the updated segment.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Segment, error)
}

// baseSegmentBuilder implements common functionality for segment builders.
type baseSegmentBuilder struct {
	client         SegmentClientInterface
	organizationID string
	ledgerID       string
	portfolioID    string
	name           string
	status         string
	metadata       map[string]any
	tags           []string
}

// segmentBuilder implements the SegmentBuilder interface.
type segmentBuilder struct {
	baseSegmentBuilder
}

// NewSegment creates a new builder for creating segments.
//
// Segments are subdivisions of a portfolio, typically representing specific business areas,
// projects, or classifications within a portfolio. This function returns a builder that allows
// for fluent configuration of the segment before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the SegmentClientInterface with the CreateSegment method.
//
// Returns:
//   - SegmentBuilder: A builder interface for configuring and creating segment resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the segment creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a segment builder
//	segmentBuilder := builders.NewSegment(client)
//
//	// Configure and create the segment
//	segment, err := segmentBuilder.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithPortfolio("portfolio-789").
//	    WithName("Project Alpha").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "projectType": "internal",
//	        "department": "engineering",
//	    }).
//	    WithTags([]string{"project", "engineering"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create segment: %v", err)
//	}
//
//	fmt.Printf("Segment created: %s\n", segment.ID)
func NewSegment(client SegmentClientInterface) SegmentBuilder {
	return &segmentBuilder{
		baseSegmentBuilder: baseSegmentBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *segmentBuilder) WithOrganization(orgID string) SegmentBuilder { performs an operation
// WithOrganization sets the organization ID for the segment.
func (b *segmentBuilder) WithOrganization(orgID string) SegmentBuilder {
	b.organizationID = orgID
	return b
}

// func (b *segmentBuilder) WithLedger(ledgerID string) SegmentBuilder { performs an operation
// WithLedger sets the ledger ID for the segment.
func (b *segmentBuilder) WithLedger(ledgerID string) SegmentBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *segmentBuilder) WithPortfolio(portfolioID string) SegmentBuilder { performs an operation
// WithPortfolio sets the portfolio ID for the segment.
func (b *segmentBuilder) WithPortfolio(portfolioID string) SegmentBuilder {
	b.portfolioID = portfolioID
	return b
}

// func (b *segmentBuilder) WithName(name string) SegmentBuilder { performs an operation
// WithName sets the name for the segment.
func (b *segmentBuilder) WithName(name string) SegmentBuilder {
	b.name = name
	return b
}

// func (b *segmentBuilder) WithStatus(status string) SegmentBuilder { performs an operation
// WithStatus sets the status for the segment.
func (b *segmentBuilder) WithStatus(status string) SegmentBuilder {
	b.status = status
	return b
}

// func (b *segmentBuilder) WithMetadata(metadata map[string]any) SegmentBuilder { performs an operation
// WithMetadata adds metadata to the segment.
func (b *segmentBuilder) WithMetadata(metadata map[string]any) SegmentBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *segmentBuilder) WithTag(tag string) SegmentBuilder { performs an operation
// WithTag adds a single tag to the segment's metadata.
func (b *segmentBuilder) WithTag(tag string) SegmentBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *segmentBuilder) WithTags(tags []string) SegmentBuilder { performs an operation
// WithTags adds multiple tags to the segment's metadata.
func (b *segmentBuilder) WithTags(tags []string) SegmentBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the segment creation and returns the created segment.
//
// This method validates all required parameters, constructs the segment creation input,
// and sends the request to the Midaz API to create the segment.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Segment: The created segment object if successful.
//     This contains all details about the segment, including its ID, name,
//     and other properties.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or portfolio is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a basic segment with required fields
//	segment, err := builders.NewSegment(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithPortfolio("portfolio-789").
//	    WithName("Marketing Campaign").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the segment
//	fmt.Printf("Created segment: %s (portfolio: %s)\n", segment.ID, segment.PortfolioID)
//
//	// Create a segment with additional properties
//	segment, err = builders.NewSegment(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithPortfolio("portfolio-789").
//	    WithName("Q3 Projects").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "quarter": "Q3",
//	        "year": 2023,
//	        "budget": 150000,
//	    }).
//	    WithTags([]string{"quarterly", "projects", "2023"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the segment
//	fmt.Printf("Created segment with metadata: %s\n", segment.ID)
func (b *segmentBuilder) Create(ctx context.Context) (*models.Segment, error) {
	// Validate required fields
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if b.portfolioID == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	if b.name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Create segment input
	input := &models.CreateSegmentInput{
		Name:     b.name,
		Status:   models.NewStatus(b.status),
		Metadata: b.metadata,
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid segment input: %v", err)
	}

	// Execute segment creation
	return b.client.CreateSegment(ctx, b.organizationID, b.ledgerID, b.portfolioID, input)
}

// segmentUpdateBuilder implements the SegmentUpdateBuilder interface.
type segmentUpdateBuilder struct {
	baseSegmentBuilder
	segmentID      string
	fieldsToUpdate map[string]bool
}

// NewSegmentUpdate creates a new builder for updating segments.
//
// This function returns a builder that allows for fluent configuration of segment updates
// before applying them to an existing segment. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the SegmentClientInterface with the UpdateSegment method.
//   - orgID: The organization ID that the segment belongs to.
//   - ledgerID: The ledger ID that the segment belongs to.
//   - portfolioID: The portfolio ID that the segment belongs to.
//   - segmentID: The ID of the segment to update.
//
// Returns:
//   - SegmentUpdateBuilder: A builder interface for configuring and executing segment updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the segment update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create a segment update builder
//	updateBuilder := builders.NewSegmentUpdate(client, "org-123", "ledger-456", "portfolio-789", "segment-abc")
//
//	// Configure and execute the update
//	updatedSegment, err := updateBuilder.
//	    WithName("Updated Segment Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "updated": true,
//	        "reason": "project completion",
//	        "completionDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTags([]string{"completed", "archived"}).
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update segment: %v", err)
//	}
//
//	fmt.Printf("Segment updated: %s (new status: %s)\n", updatedSegment.ID, updatedSegment.Status.Code)
func NewSegmentUpdate(client SegmentClientInterface, orgID, ledgerID, portfolioID, segmentID string) SegmentUpdateBuilder {
	return &segmentUpdateBuilder{
		baseSegmentBuilder: baseSegmentBuilder{
			client:         client,
			organizationID: orgID,
			ledgerID:       ledgerID,
			portfolioID:    portfolioID,
			metadata:       make(map[string]any),
		},
		segmentID:      segmentID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *segmentUpdateBuilder) WithName(name string) SegmentUpdateBuilder { performs an operation
// WithName sets the name for the segment.
func (b *segmentUpdateBuilder) WithName(name string) SegmentUpdateBuilder {
	b.name = name
	b.fieldsToUpdate["name"] = true

	return b
}

// func (b *segmentUpdateBuilder) WithStatus(status string) SegmentUpdateBuilder { performs an operation
// WithStatus sets the status for the segment.
func (b *segmentUpdateBuilder) WithStatus(status string) SegmentUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *segmentUpdateBuilder) WithMetadata(metadata map[string]any) SegmentUpdateBuilder { performs an operation
// WithMetadata adds metadata to the segment.
func (b *segmentUpdateBuilder) WithMetadata(metadata map[string]any) SegmentUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *segmentUpdateBuilder) WithTag(tag string) SegmentUpdateBuilder { performs an operation
// WithTag adds a single tag to the segment's metadata.
func (b *segmentUpdateBuilder) WithTag(tag string) SegmentUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *segmentUpdateBuilder) WithTags(tags []string) SegmentUpdateBuilder { performs an operation
// WithTags adds multiple tags to the segment's metadata.
func (b *segmentUpdateBuilder) WithTags(tags []string) SegmentUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the segment update operation and returns the updated segment.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the segment. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Segment: The updated segment object if successful.
//     This contains all details about the segment, including its ID, name, status,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, portfolio, or segment is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the name of a segment
//	updatedSegment, err := builders.NewSegmentUpdate(client, "org-123", "ledger-456", "portfolio-789", "segment-abc").
//	    WithName("New Segment Name").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated segment name: %s\n", updatedSegment.Name)
//
//	// Update multiple fields of a segment
//	updatedSegment, err = builders.NewSegmentUpdate(client, "org-123", "ledger-456", "portfolio-789", "segment-abc").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "archived": true,
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	        "reason": "project completed",
//	    }).
//	    WithTag("archived").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Segment status updated to: %s\n", updatedSegment.Status.Code)
func (b *segmentUpdateBuilder) Update(ctx context.Context) (*models.Segment, error) {
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

	if b.segmentID == "" {
		return nil, fmt.Errorf("segment ID is required")
	}

	// Create update input
	input := &models.UpdateSegmentInput{}

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
		return nil, fmt.Errorf("invalid segment update input: %v", err)
	}

	// Execute segment update
	return b.client.UpdateSegment(ctx, b.organizationID, b.ledgerID, b.portfolioID, b.segmentID, input)
}
