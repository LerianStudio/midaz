package models

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Asset represents an asset in the Midaz Ledger.
// Assets are the fundamental units of value that can be tracked and transferred
// within the ledger system. Each asset has a unique code and belongs to a specific
// organization and ledger.
type Asset struct {
	// ID is the unique identifier for the asset
	ID string `json:"id"`

	// Name is the human-readable name of the asset
	Name string `json:"name"`

	// Type defines the asset type (e.g., "CURRENCY", "SECURITY", "COMMODITY")
	Type string `json:"type"`

	// Code is a unique identifier for the asset type (e.g., "USD", "BTC", "AAPL")
	Code string `json:"code"`

	// Status represents the current status of the asset (e.g., "ACTIVE", "INACTIVE")
	Status Status `json:"status"`

	// LedgerID is the ID of the ledger that contains this asset
	LedgerID string `json:"ledgerId"`

	// OrganizationID is the ID of the organization that owns this asset
	OrganizationID string `json:"organizationId"`

	// CreatedAt is the timestamp when the asset was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the asset was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the asset was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// Metadata contains additional custom data associated with the asset
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewAsset creates a new Asset with required fields.
// This constructor ensures that all mandatory fields are provided when creating an asset.
//
// Parameters:
//   - id: Unique identifier for the asset
//   - name: Human-readable name for the asset
//   - code: Unique code identifying the asset type
//   - organizationID: ID of the organization that owns this asset
//   - ledgerID: ID of the ledger that contains this asset
//   - status: Current status of the asset
//
// Returns:
//   - A pointer to the newly created Asset
func NewAsset(id, name, code, organizationID, ledgerID string, status Status) *Asset {
	return &Asset{
		ID:             id,
		Name:           name,
		Code:           code,
		Status:         status,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// WithType adds a type to the asset.
// The asset type categorizes the asset (e.g., "CURRENCY", "SECURITY", "COMMODITY").
//
// Parameters:
//   - assetType: The type to set for the asset
//
// Returns:
//   - A pointer to the modified Asset for method chaining
func (a *Asset) WithType(assetType string) *Asset {
	a.Type = assetType
	return a
}

// WithMetadata adds metadata to the asset.
// Metadata can store additional custom information about the asset.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified Asset for method chaining
func (a *Asset) WithMetadata(metadata map[string]any) *Asset {
	a.Metadata = metadata
	return a
}

// FromMmodelAsset converts an mmodel Asset to an SDK Asset.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - asset: The mmodel.Asset to convert
//
// Returns:
//   - A models.Asset instance with the same values
func FromMmodelAsset(asset mmodel.Asset) Asset {
	result := Asset{
		ID:             asset.ID,
		Name:           asset.Name,
		Type:           asset.Type,
		Code:           asset.Code,
		Status:         FromMmodelStatus(asset.Status),
		LedgerID:       asset.LedgerID,
		OrganizationID: asset.OrganizationID,
		CreatedAt:      asset.CreatedAt,
		UpdatedAt:      asset.UpdatedAt,
		Metadata:       asset.Metadata,
	}

	if asset.DeletedAt != nil {
		deletedAt := *asset.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// ToMmodelAsset converts an SDK Asset to an mmodel Asset.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Asset instance with the same values
func (a *Asset) ToMmodelAsset() mmodel.Asset {
	result := mmodel.Asset{
		ID:             a.ID,
		Name:           a.Name,
		Type:           a.Type,
		Code:           a.Code,
		Status:         a.Status.ToMmodelStatus(),
		LedgerID:       a.LedgerID,
		OrganizationID: a.OrganizationID,
		CreatedAt:      a.CreatedAt,
		UpdatedAt:      a.UpdatedAt,
		Metadata:       a.Metadata,
	}

	if a.DeletedAt != nil {
		deletedAt := *a.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// CreateAssetInput is the input for creating an asset.
// This structure contains all the fields that can be specified when creating a new asset.
type CreateAssetInput struct {
	// Name is the human-readable name for the asset
	Name string `json:"name"`

	// Type defines the asset type (e.g., "CURRENCY", "SECURITY", "COMMODITY")
	Type string `json:"type,omitempty"`

	// Code is a unique identifier for the asset type (e.g., "USD", "BTC", "AAPL")
	Code string `json:"code"`

	// Status represents the initial status of the asset
	Status Status `json:"status,omitempty"`

	// Metadata contains additional custom data for the asset
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewCreateAssetInput creates a new CreateAssetInput with required fields.
// This constructor ensures that all mandatory fields are provided when creating an asset input.
//
// Parameters:
//   - name: Human-readable name for the asset
//   - code: Unique code identifying the asset type
//
// Returns:
//   - A pointer to the newly created CreateAssetInput
func NewCreateAssetInput(name, code string) *CreateAssetInput {
	return &CreateAssetInput{
		Name: name,
		Code: code,
	}
}

// WithType adds a type to the create asset input.
// The asset type categorizes the asset (e.g., "CURRENCY", "SECURITY", "COMMODITY").
//
// Parameters:
//   - assetType: The type to set for the asset
//
// Returns:
//   - A pointer to the modified CreateAssetInput for method chaining
func (c *CreateAssetInput) WithType(assetType string) *CreateAssetInput {
	c.Type = assetType
	return c
}

// WithStatus adds a status to the create asset input.
// This sets the initial status of the asset.
//
// Parameters:
//   - status: The status to set for the asset
//
// Returns:
//   - A pointer to the modified CreateAssetInput for method chaining
func (c *CreateAssetInput) WithStatus(status Status) *CreateAssetInput {
	c.Status = status
	return c
}

// WithMetadata adds metadata to the create asset input.
// Metadata can store additional custom information about the asset.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified CreateAssetInput for method chaining
func (c *CreateAssetInput) WithMetadata(metadata map[string]any) *CreateAssetInput {
	c.Metadata = metadata
	return c
}

// Validate validates the CreateAssetInput and returns an error if it's invalid.
// This method checks that all required fields are present and meet the validation constraints
// defined by the backend.
//
// Returns:
//   - error: An error if the input is invalid, nil otherwise
func (input *CreateAssetInput) Validate() error {
	// Check required fields
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}

	if len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	if input.Code == "" {
		return fmt.Errorf("code is required")
	}

	if len(input.Code) > 100 {
		return fmt.Errorf("code must be at most 100 characters, got %d", len(input.Code))
	}

	// Validate metadata keys and values if present
	if input.Metadata != nil {
		if err := validateMetadata(input.Metadata); err != nil {
			return err
		}
	}

	return nil
}

// ToMmodelCreateAssetInput converts an SDK CreateAssetInput to an mmodel CreateAssetInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.CreateAssetInput instance with the same values
func (c *CreateAssetInput) ToMmodelCreateAssetInput() mmodel.CreateAssetInput {
	result := mmodel.CreateAssetInput{
		Name:     c.Name,
		Type:     c.Type,
		Code:     c.Code,
		Metadata: c.Metadata,
	}

	if !c.Status.IsEmpty() {
		result.Status = c.Status.ToMmodelStatus()
	}

	return result
}

// UpdateAssetInput is the input for updating an asset.
// This structure contains the fields that can be modified when updating an existing asset.
type UpdateAssetInput struct {
	// Name is the updated human-readable name for the asset
	Name string `json:"name,omitempty"`

	// Status is the updated status of the asset
	Status Status `json:"status,omitempty"`

	// Metadata contains updated additional custom data
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewUpdateAssetInput creates a new empty UpdateAssetInput.
// This constructor initializes an empty update input that can be customized
// using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdateAssetInput
func NewUpdateAssetInput() *UpdateAssetInput {
	return &UpdateAssetInput{}
}

// WithName sets the name in the update asset input.
// This updates the human-readable name of the asset.
//
// Parameters:
//   - name: The new name for the asset
//
// Returns:
//   - A pointer to the modified UpdateAssetInput for method chaining
func (u *UpdateAssetInput) WithName(name string) *UpdateAssetInput {
	u.Name = name
	return u
}

// WithStatus sets the status in the update asset input.
// This updates the status of the asset.
//
// Parameters:
//   - status: The new status for the asset
//
// Returns:
//   - A pointer to the modified UpdateAssetInput for method chaining
func (u *UpdateAssetInput) WithStatus(status Status) *UpdateAssetInput {
	u.Status = status
	return u
}

// WithMetadata sets the metadata in the update asset input.
// This updates the custom metadata associated with the asset.
//
// Parameters:
//   - metadata: The new metadata map
//
// Returns:
//   - A pointer to the modified UpdateAssetInput for method chaining
func (input *UpdateAssetInput) WithMetadata(metadata map[string]any) *UpdateAssetInput {
	input.Metadata = metadata
	return input
}

// Validate validates the UpdateAssetInput and returns an error if it's invalid.
// This method checks that all fields meet the validation constraints defined by the backend.
// For update operations, fields are optional but must be valid if provided.
//
// Returns:
//   - error: An error if the input is invalid, nil otherwise
func (input *UpdateAssetInput) Validate() error {
	// Name is optional for updates, but if provided must be valid
	if input.Name != "" && len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	// Validate metadata keys and values if present
	if input.Metadata != nil {
		if err := validateMetadata(input.Metadata); err != nil {
			return err
		}
	}

	return nil
}

// ToMmodelUpdateAssetInput converts an SDK UpdateAssetInput to an mmodel UpdateAssetInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdateAssetInput instance with the same values
func (u *UpdateAssetInput) ToMmodelUpdateAssetInput() mmodel.UpdateAssetInput {
	result := mmodel.UpdateAssetInput{
		Name:     u.Name,
		Metadata: u.Metadata,
	}

	if !u.Status.IsEmpty() {
		result.Status = u.Status.ToMmodelStatus()
	}

	return result
}

// Assets represents a list of assets with pagination information.
// This structure is used for paginated responses when listing assets.
type Assets struct {
	// Items is the collection of assets in the current page
	Items []Asset `json:"items"`

	// Page is the current page number
	Page int `json:"page"`

	// Limit is the maximum number of items per page
	Limit int `json:"limit"`
}

// FromMmodelAssets converts an mmodel Assets to an SDK Assets.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - assets: The mmodel.Assets to convert
//
// Returns:
//   - A models.Assets instance with the same values
func FromMmodelAssets(assets mmodel.Assets) Assets {
	result := Assets{
		Page:  assets.Page,
		Limit: assets.Limit,
		Items: make([]Asset, 0, len(assets.Items)),
	}

	for _, asset := range assets.Items {
		result.Items = append(result.Items, FromMmodelAsset(asset))
	}

	return result
}

// AssetFilter for filtering assets in listings.
// This structure defines the criteria for filtering assets when listing them.
type AssetFilter struct {
	// Status is a list of status codes to filter by
	Status []string `json:"status,omitempty"`
}

// ListAssetInput for configuring asset listing requests.
// This structure defines the parameters for listing assets.
type ListAssetInput struct {
	// Page is the page number to retrieve
	Page int `json:"page,omitempty"`

	// PerPage is the number of items per page
	PerPage int `json:"perPage,omitempty"`

	// Filter contains the filtering criteria
	Filter AssetFilter `json:"filter,omitempty"`
}

// ListAssetResponse for asset listing responses.
// This structure represents the response from a list assets request.
type ListAssetResponse struct {
	// Items is the collection of assets in the current page
	Items []Asset `json:"items"`

	// Total is the total number of assets matching the criteria
	Total int `json:"total"`

	// CurrentPage is the current page number
	CurrentPage int `json:"currentPage"`

	// PageSize is the number of items per page
	PageSize int `json:"pageSize"`

	// TotalPages is the total number of pages
	TotalPages int `json:"totalPages"`
}

// AssetRate represents an asset exchange rate in the Midaz Ledger.
// Asset rates define the conversion ratio between two different assets
// and are used for currency conversion and other asset exchange operations.
type AssetRate struct {
	// ID is the unique identifier for the asset rate
	ID string `json:"id"`

	// FromAsset is the source asset code
	FromAsset string `json:"fromAsset"`

	// ToAsset is the target asset code
	ToAsset string `json:"toAsset"`

	// Rate is the exchange rate value (FromAsset to ToAsset)
	Rate float64 `json:"rate"`

	// CreatedAt is the timestamp when the rate was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the rate was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// EffectiveAt is the timestamp when the rate becomes effective
	EffectiveAt time.Time `json:"effectiveAt"`

	// ExpirationAt is the timestamp when the rate expires
	ExpirationAt time.Time `json:"expirationAt"`
}

// NewAssetRate creates a new AssetRate with required fields.
// This constructor ensures that all mandatory fields are provided when creating an asset rate.
//
// Parameters:
//   - id: Unique identifier for the asset rate
//   - fromAsset: Source asset code
//   - toAsset: Target asset code
//   - rate: Exchange rate value
//   - effectiveAt: Timestamp when the rate becomes effective
//   - expirationAt: Timestamp when the rate expires
//
// Returns:
//   - A pointer to the newly created AssetRate
func NewAssetRate(id, fromAsset, toAsset string, rate float64, effectiveAt, expirationAt time.Time) *AssetRate {
	return &AssetRate{
		ID:           id,
		FromAsset:    fromAsset,
		ToAsset:      toAsset,
		Rate:         rate,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		EffectiveAt:  effectiveAt,
		ExpirationAt: expirationAt,
	}
}

// UpdateAssetRateInput is the input for updating an asset rate.
// This structure contains the fields that can be modified when updating an existing asset rate.
type UpdateAssetRateInput struct {
	// FromAsset is the updated source asset code
	FromAsset string `json:"fromAsset"`

	// ToAsset is the updated target asset code
	ToAsset string `json:"toAsset"`

	// Rate is the updated exchange rate value
	Rate float64 `json:"rate"`

	// EffectiveAt is the updated timestamp when the rate becomes effective
	EffectiveAt time.Time `json:"effectiveAt"`

	// ExpirationAt is the updated timestamp when the rate expires
	ExpirationAt time.Time `json:"expirationAt"`
}

// NewUpdateAssetRateInput creates a new UpdateAssetRateInput with required fields.
// This constructor ensures that all mandatory fields are provided when updating an asset rate.
//
// Parameters:
//   - fromAsset: Source asset code
//   - toAsset: Target asset code
//   - rate: Exchange rate value
//   - effectiveAt: Timestamp when the rate becomes effective
//   - expirationAt: Timestamp when the rate expires
//
// Returns:
//   - A pointer to the newly created UpdateAssetRateInput
func NewUpdateAssetRateInput(fromAsset, toAsset string, rate float64, effectiveAt, expirationAt time.Time) *UpdateAssetRateInput {
	return &UpdateAssetRateInput{
		FromAsset:    fromAsset,
		ToAsset:      toAsset,
		Rate:         rate,
		EffectiveAt:  effectiveAt,
		ExpirationAt: expirationAt,
	}
}

// Validate checks that the UpdateAssetRateInput meets all validation requirements.
// It ensures that required fields are present and that all fields meet their
// validation constraints as defined in the API specification.
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (input *UpdateAssetRateInput) Validate() error {
	if input.FromAsset == "" {
		return fmt.Errorf("fromAsset is required")
	}

	if input.ToAsset == "" {
		return fmt.Errorf("toAsset is required")
	}

	if input.Rate <= 0 {
		return fmt.Errorf("rate must be greater than 0, got %f", input.Rate)
	}

	// Validate that EffectiveAt is before ExpirationAt
	if !input.EffectiveAt.IsZero() && !input.ExpirationAt.IsZero() && !input.EffectiveAt.Before(input.ExpirationAt) {
		return fmt.Errorf("effectiveAt must be before expirationAt")
	}

	return nil
}
