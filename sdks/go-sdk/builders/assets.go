// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AssetClientInterface defines the minimal client interface required by the asset builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services.
type AssetClientInterface interface {
	// CreateAsset sends a request to create a new asset with the specified parameters.
	// It requires a context for the API request.
	// Returns an error if the API request fails.
	CreateAsset(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error)

	// UpdateAsset sends a request to update an existing asset with the specified parameters.
	// It requires a context for the API request.
	// Returns an error if the API request fails.
	UpdateAsset(ctx context.Context, organizationID, ledgerID, assetID string, input *models.UpdateAssetInput) (*models.Asset, error)
}

// AssetBuilder defines the builder interface for creating assets.
// It provides a fluent API for configuring and creating new asset resources.
type AssetBuilder interface {
	// WithOrganization sets the organization ID for the asset.
	// This is a required field for asset creation.
	WithOrganization(orgID string) AssetBuilder

	// WithLedger sets the ledger ID for the asset.
	// This is a required field for asset creation.
	WithLedger(ledgerID string) AssetBuilder

	// WithName sets the name for the asset.
	// This is a required field for asset creation.
	WithName(name string) AssetBuilder

	// WithCode sets the code for the asset.
	// This is a required field for asset creation.
	// The code is a unique identifier for the asset within the ledger.
	WithCode(code string) AssetBuilder

	// WithType sets the type for the asset.
	// This is an optional field for asset creation.
	// Valid types include: "CURRENCY", "SECURITY", "COMMODITY", etc.
	// TODO: Create a GET endpoint for asset types, aligning with the API spec.
	WithType(assetType string) AssetBuilder

	// WithStatus sets the status for the asset.
	// This is an optional field for asset creation.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "ARCHIVED".
	// If not specified, the default status is "ACTIVE".
	// TODO: Create a GET endpoint for asset statuses, aligning with the API spec.
	WithStatus(status string) AssetBuilder

	// WithMetadata adds metadata to the asset.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) AssetBuilder

	// WithTag adds a single tag to the asset's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTag(tag string) AssetBuilder

	// WithTags adds multiple tags to the asset's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTags(tags []string) AssetBuilder

	// Create executes the asset creation and returns the created asset.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	Create(ctx context.Context) (*models.Asset, error)
}

// AssetUpdateBuilder defines the builder interface for updating assets.
// It provides a fluent API for configuring and updating existing asset resources.
type AssetUpdateBuilder interface {
	// WithName sets the name for the asset.
	// This is an optional field for asset updates.
	WithName(name string) AssetUpdateBuilder

	// WithStatus sets the status for the asset.
	// Valid statuses include: "ACTIVE", "INACTIVE", "PENDING", "ARCHIVED".
	// TODO: Create a GET endpoint for asset statuses, aligning with the API spec.
	WithStatus(status string) AssetUpdateBuilder

	// WithMetadata adds metadata to the asset.
	// This is an optional field that allows storing additional information as key-value pairs.
	// The provided map will be merged with any existing metadata.
	WithMetadata(metadata map[string]any) AssetUpdateBuilder

	// WithTag adds a single tag to the asset's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTag(tag string) AssetUpdateBuilder

	// WithTags adds multiple tags to the asset's metadata.
	// Tags are stored in the metadata under the "tags" key as an array of strings.
	WithTags(tags []string) AssetUpdateBuilder

	// Update executes the asset update operation and returns the updated asset.
	// It requires a context for the API request.
	// Returns an error if no fields are specified for update or if the API request fails.
	Update(ctx context.Context) (*models.Asset, error)
}

// baseAssetBuilder implements common functionality for asset builders.
type baseAssetBuilder struct {
	client         AssetClientInterface
	organizationID string
	ledgerID       string
	name           string
	code           string
	assetType      string
	status         string
	metadata       map[string]any
	tags           []string
}

// assetBuilder implements the AssetBuilder interface.
type assetBuilder struct {
	baseAssetBuilder
}

// NewAsset creates a new builder for creating assets.
//
// Assets represent the units of value that can be tracked within a ledger, such as currencies,
// securities, or commodities. This function returns a builder that allows for fluent configuration
// of the asset before creation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the AssetClientInterface with the CreateAsset method.
//
// Returns:
//   - AssetBuilder: A builder interface for configuring and creating asset resources.
//     Use the builder's methods to set required and optional parameters, then call Create()
//     to perform the asset creation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an asset builder
//	assetBuilder := builders.NewAsset(client)
//
//	// Configure and create the asset
//	asset, err := assetBuilder.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("US Dollar").
//	    WithCode("USD").
//	    WithType("CURRENCY").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "country": "United States",
//	        "symbol": "$",
//	    }).
//	    WithTags([]string{"fiat", "currency"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create asset: %v", err)
//	}
//
//	fmt.Printf("Asset created: %s (%s)\n", asset.ID, asset.Code)
func NewAsset(client AssetClientInterface) AssetBuilder {
	return &assetBuilder{
		baseAssetBuilder: baseAssetBuilder{
			client:   client,
			status:   "ACTIVE", // Default status
			metadata: make(map[string]any),
		},
	}
}

// func (b *assetBuilder) WithOrganization performs an operation
// WithOrganization sets the organization ID for the asset.
func (b *assetBuilder) WithOrganization(orgID string) AssetBuilder {
	b.organizationID = orgID
	return b
}

// func (b *assetBuilder) WithLedger performs an operation
// WithLedger sets the ledger ID for the asset.
func (b *assetBuilder) WithLedger(ledgerID string) AssetBuilder {
	b.ledgerID = ledgerID
	return b
}

// func (b *assetBuilder) WithName performs an operation
// WithName sets the name for the asset.
func (b *assetBuilder) WithName(name string) AssetBuilder {
	b.name = name
	return b
}

// func (b *assetBuilder) WithCode performs an operation
// WithCode sets the code for the asset.
func (b *assetBuilder) WithCode(code string) AssetBuilder {
	b.code = code
	return b
}

// func (b *assetBuilder) WithType performs an operation
// WithType sets the type for the asset.
func (b *assetBuilder) WithType(assetType string) AssetBuilder {
	b.assetType = assetType
	return b
}

// func (b *assetBuilder) WithStatus performs an operation
// WithStatus sets the status for the asset.
func (b *assetBuilder) WithStatus(status string) AssetBuilder {
	b.status = status
	return b
}

// func (b *assetBuilder) WithMetadata performs an operation
// WithMetadata adds metadata to the asset.
func (b *assetBuilder) WithMetadata(metadata map[string]any) AssetBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	return b
}

// func (b *assetBuilder) WithTag performs an operation
// WithTag adds a single tag to the asset's metadata.
func (b *assetBuilder) WithTag(tag string) AssetBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// func (b *assetBuilder) WithTags performs an operation
// WithTags adds multiple tags to the asset's metadata.
func (b *assetBuilder) WithTags(tags []string) AssetBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
	}

	return b
}

// Create executes the asset creation and returns the created asset.
//
// This method validates all required parameters, constructs the asset creation input,
// and sends the request to the Midaz API to create the asset.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Asset: The created asset object if successful.
//     This contains all details about the asset, including its ID, code, name,
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
//	// Create a basic currency asset
//	asset, err := builders.NewAsset(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Euro").
//	    WithCode("EUR").
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset
//	fmt.Printf("Created asset: %s (code: %s)\n", asset.ID, asset.Code)
//
//	// Create a more detailed asset with additional properties
//	asset, err = builders.NewAsset(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithName("Bitcoin").
//	    WithCode("BTC").
//	    WithType("CRYPTOCURRENCY").
//	    WithStatus("ACTIVE").
//	    WithMetadata(map[string]any{
//	        "decimals": 8,
//	        "network": "mainnet",
//	        "description": "Digital cryptocurrency",
//	    }).
//	    WithTags([]string{"crypto", "digital"}).
//	    Create(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset
//	fmt.Printf("Created asset with metadata: %s\n", asset.ID)
func (b *assetBuilder) Create(ctx context.Context) (*models.Asset, error) {
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

	if b.code == "" {
		return nil, fmt.Errorf("code is required")
	}

	// Create asset input
	input := &models.CreateAssetInput{
		Name:     b.name,
		Code:     b.code,
		Type:     b.assetType,
		Status:   models.NewStatus(b.status),
		Metadata: b.metadata,
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset input: %v", err)
	}

	// Execute asset creation
	return b.client.CreateAsset(ctx, b.organizationID, b.ledgerID, input)
}

// assetUpdateBuilder implements the AssetUpdateBuilder interface.
type assetUpdateBuilder struct {
	baseAssetBuilder
	assetID        string
	fieldsToUpdate map[string]bool
}

// NewAssetUpdate creates a new builder for updating assets.
//
// This function returns a builder that allows for fluent configuration of asset updates
// before applying them to an existing asset. Only the fields that are explicitly set
// on the builder will be included in the update operation.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the AssetClientInterface with the UpdateAsset method.
//   - orgID: The organization ID that the asset belongs to.
//   - ledgerID: The ledger ID that the asset belongs to.
//   - assetID: The ID of the asset to update.
//
// Returns:
//   - AssetUpdateBuilder: A builder interface for configuring and executing asset updates.
//     Use the builder's methods to set the fields to update, then call Update()
//     to perform the asset update operation.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an asset update builder
//	updateBuilder := builders.NewAssetUpdate(client, "org-123", "ledger-456", "asset-789")
//
//	// Configure and execute the update
//	updatedAsset, err := updateBuilder.
//	    WithName("Updated Asset Name").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "deprecated": true,
//	        "replacedBy": "asset-abc",
//	        "deprecationDate": time.Now().Format(time.RFC3339),
//	    }).
//	    WithTags([]string{"deprecated", "legacy"}).
//	    Update(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to update asset: %v", err)
//	}
//
//	fmt.Printf("Asset updated: %s (new status: %s)\n", updatedAsset.ID, updatedAsset.Status.Code)
func NewAssetUpdate(client AssetClientInterface, orgID, ledgerID, assetID string) AssetUpdateBuilder {
	return &assetUpdateBuilder{
		baseAssetBuilder: baseAssetBuilder{
			client:         client,
			organizationID: orgID,
			ledgerID:       ledgerID,
			metadata:       make(map[string]any),
		},
		assetID:        assetID,
		fieldsToUpdate: make(map[string]bool),
	}
}

// func (b *assetUpdateBuilder) WithName performs an operation
// WithName sets the name for the asset.
func (b *assetUpdateBuilder) WithName(name string) AssetUpdateBuilder {
	b.name = name
	b.fieldsToUpdate["name"] = true

	return b
}

// func (b *assetUpdateBuilder) WithStatus performs an operation
// WithStatus sets the status for the asset.
func (b *assetUpdateBuilder) WithStatus(status string) AssetUpdateBuilder {
	b.status = status
	b.fieldsToUpdate["status"] = true

	return b
}

// func (b *assetUpdateBuilder) WithMetadata performs an operation
// WithMetadata adds metadata to the asset.
func (b *assetUpdateBuilder) WithMetadata(metadata map[string]any) AssetUpdateBuilder {
	for k, v := range metadata {
		b.metadata[k] = v
	}

	b.fieldsToUpdate["metadata"] = true

	return b
}

// func (b *assetUpdateBuilder) WithTag performs an operation
// WithTag adds a single tag to the asset's metadata.
func (b *assetUpdateBuilder) WithTag(tag string) AssetUpdateBuilder {
	if tag != "" {
		b.tags = append(b.tags, tag)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// func (b *assetUpdateBuilder) WithTags performs an operation
// WithTags adds multiple tags to the asset's metadata.
func (b *assetUpdateBuilder) WithTags(tags []string) AssetUpdateBuilder {
	if len(tags) > 0 {
		b.tags = append(b.tags, tags...)
		updateTagsInMetadata(b.metadata, b.tags)
		b.fieldsToUpdate["metadata"] = true
	}

	return b
}

// Update executes the asset update operation and returns the updated asset.
//
// This method validates that at least one field is set for update, constructs the update input,
// and sends the request to the Midaz API to update the asset. Only fields that have been
// explicitly set using the With* methods will be included in the update.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.Asset: The updated asset object if successful.
//     This contains all details about the asset, including its ID, name, status,
//     and other properties with the updated values.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If no fields are specified for update
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or asset is not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Update only the name of an asset
//	updatedAsset, err := builders.NewAssetUpdate(client, "org-123", "ledger-456", "asset-789").
//	    WithName("New Asset Name").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Updated asset name: %s\n", updatedAsset.Name)
//
//	// Update multiple fields of an asset
//	updatedAsset, err = builders.NewAssetUpdate(client, "org-123", "ledger-456", "asset-789").
//	    WithStatus("INACTIVE").
//	    WithMetadata(map[string]any{
//	        "archived": true,
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	        "reason": "obsolete",
//	    }).
//	    WithTag("archived").
//	    Update(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	fmt.Printf("Asset status updated to: %s\n", updatedAsset.Status.Code)
func (b *assetUpdateBuilder) Update(ctx context.Context) (*models.Asset, error) {
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

	if b.assetID == "" {
		return nil, fmt.Errorf("asset ID is required")
	}

	// Create update input
	input := &models.UpdateAssetInput{}

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
		return nil, fmt.Errorf("invalid asset update input: %v", err)
	}

	// Execute asset update
	return b.client.UpdateAsset(ctx, b.organizationID, b.ledgerID, b.assetID, input)
}
