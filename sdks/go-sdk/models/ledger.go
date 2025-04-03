// Package models defines the data models used by the Midaz SDK.
package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Ledger represents a ledger in the Midaz system.
// A ledger is a financial record-keeping system that contains accounts
// and tracks all transactions between those accounts. Each ledger belongs
// to a specific organization and can have multiple accounts.
type Ledger struct {
	// ID is the unique identifier for the ledger
	ID string `json:"id"`

	// Name is the human-readable name of the ledger (max length: 256 characters)
	Name string `json:"name"`

	// OrganizationID is the ID of the organization that owns this ledger
	OrganizationID string `json:"organizationId"`

	// Status represents the current status of the ledger (e.g., "ACTIVE", "INACTIVE")
	Status Status `json:"status"`

	// Metadata contains additional custom data associated with the ledger
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the ledger was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the ledger was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the ledger was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// NewLedger creates a new Ledger with required fields.
// This constructor ensures that all mandatory fields are provided when creating a ledger.
//
// Parameters:
//   - id: Unique identifier for the ledger
//   - name: Human-readable name for the ledger
//   - organizationID: ID of the organization that owns this ledger
//   - status: Current status of the ledger
//
// Returns:
//   - A pointer to the newly created Ledger
func NewLedger(id, name, organizationID string, status Status) *Ledger {
	return &Ledger{
		ID:             id,
		Name:           name,
		OrganizationID: organizationID,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// WithMetadata adds metadata to the ledger.
// Metadata can store additional custom information about the ledger.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified Ledger for method chaining
func (l *Ledger) WithMetadata(metadata map[string]any) *Ledger {
	l.Metadata = metadata
	return l
}

// FromMmodelLedger converts an mmodel Ledger to an SDK Ledger.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - ledger: The mmodel.Ledger to convert
//
// Returns:
//   - A models.Ledger instance with the same values
func FromMmodelLedger(ledger mmodel.Ledger) Ledger {
	return Ledger{
		ID:             ledger.ID,
		Name:           ledger.Name,
		OrganizationID: ledger.OrganizationID,
		Status:         FromMmodelStatus(ledger.Status),
		Metadata:       ledger.Metadata,
		CreatedAt:      ledger.CreatedAt,
		UpdatedAt:      ledger.UpdatedAt,
		DeletedAt:      ledger.DeletedAt,
	}
}

// ToMmodelLedger converts an SDK Ledger to an mmodel Ledger.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Ledger instance with the same values
func (l Ledger) ToMmodelLedger() mmodel.Ledger {
	return mmodel.Ledger{
		ID:             l.ID,
		Name:           l.Name,
		OrganizationID: l.OrganizationID,
		Status:         l.Status.ToMmodelStatus(),
		Metadata:       l.Metadata,
		CreatedAt:      l.CreatedAt,
		UpdatedAt:      l.UpdatedAt,
		DeletedAt:      l.DeletedAt,
	}
}

// CreateLedgerInput is the input for creating a ledger.
// This structure contains all the fields that can be specified when creating a new ledger.
type CreateLedgerInput struct {
	// Name is the human-readable name for the ledger (required, max length: 256 characters)
	Name string `json:"name"`

	// Status represents the initial status of the ledger (defaults to ACTIVE if not specified)
	Status Status `json:"status"`

	// Metadata contains additional custom data for the ledger
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewCreateLedgerInput creates a new CreateLedgerInput with required fields.
// This constructor ensures that all mandatory fields are provided when creating a ledger input.
//
// Parameters:
//   - name: Human-readable name for the ledger (required, max length: 256 characters)
//
// Returns:
//   - A pointer to the newly created CreateLedgerInput with default active status
//
// Example - Basic usage:
//
//	// Create a simple ledger input with just the required name
//	input := models.NewCreateLedgerInput("Main Ledger")
//
//	// Validate the input
//	if err := input.Validate(); err != nil {
//	    log.Fatalf("Invalid ledger input: %v", err)
//	}
//
// Example - With all options:
//
//	// Create a ledger with custom status and metadata
//	input := models.NewCreateLedgerInput("Finance Ledger")
//	    .WithStatus(models.NewStatus("PENDING"))
//	    .WithMetadata(map[string]any{
//	        "department": "Finance",
//	        "fiscalYear": 2025,
//	        "currency": "USD",
//	    })
//
//	// Validate the input
//	if err := input.Validate(); err != nil {
//	    log.Fatalf("Invalid ledger input: %v", err)
//	}
func NewCreateLedgerInput(name string) *CreateLedgerInput {
	return &CreateLedgerInput{
		Name:   name,
		Status: NewStatus("ACTIVE"), // Default status
	}
}

// WithStatus sets a custom status on the ledger input.
// This overrides the default "ACTIVE" status set by the constructor.
//
// Parameters:
//   - status: The status to set for the ledger
//
// Returns:
//   - A pointer to the modified CreateLedgerInput for method chaining
func (input *CreateLedgerInput) WithStatus(status Status) *CreateLedgerInput {
	input.Status = status
	return input
}

// WithMetadata adds metadata to the ledger input.
// Metadata can store additional custom information about the ledger.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified CreateLedgerInput for method chaining
func (input *CreateLedgerInput) WithMetadata(metadata map[string]any) *CreateLedgerInput {
	input.Metadata = metadata
	return input
}

// Validate validates the CreateLedgerInput and returns an error if it's invalid.
// This method checks that all required fields are present and that all fields
// meet the validation constraints defined by the backend.
//
// Returns:
//   - error: An error if the input is invalid, nil otherwise
func (input *CreateLedgerInput) Validate() error {
	if input.Name == "" {
		return errors.New("name is required")
	}
	if len(input.Name) > 256 {
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

// ToMmodelCreateLedgerInput converts an SDK CreateLedgerInput to an mmodel CreateLedgerInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.CreateLedgerInput instance with the same values
func (input CreateLedgerInput) ToMmodelCreateLedgerInput() mmodel.CreateLedgerInput {
	return mmodel.CreateLedgerInput{
		Name:     input.Name,
		Status:   input.Status.ToMmodelStatus(),
		Metadata: input.Metadata,
	}
}

// UpdateLedgerInput is the input for updating a ledger.
// This structure contains the fields that can be modified when updating an existing ledger.
type UpdateLedgerInput struct {
	// Name is the updated human-readable name for the ledger (max length: 256 characters)
	Name string `json:"name"`

	// Status is the updated status of the ledger
	Status Status `json:"status"`

	// Metadata contains updated additional custom data
	// Keys max length: 100 characters, Values max length: 2000 characters
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewUpdateLedgerInput creates a new UpdateLedgerInput.
// This constructor initializes an empty update input that can be customized
// using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdateLedgerInput
//
// Example - Update name only:
//
//	// Create an update to change just the ledger name
//	input := models.NewUpdateLedgerInput().WithName("Updated Ledger Name")
//
//	// Validate the input
//	if err := input.Validate(); err != nil {
//	    log.Fatalf("Invalid update input: %v", err)
//	}
//
// Example - Multiple field update:
//
//	// Update multiple fields at once
//	input := models.NewUpdateLedgerInput()
//	    .WithName("Archive Ledger")
//	    .WithStatus(models.NewStatus("INACTIVE"))
//	    .WithMetadata(map[string]any{
//	        "archivedDate": time.Now().Format(time.RFC3339),
//	        "archivedBy": "user123",
//	        "reason": "End of fiscal year",
//	    })
//
//	// Validate the input
//	if err := input.Validate(); err != nil {
//	    log.Fatalf("Invalid update input: %v", err)
//	}
func NewUpdateLedgerInput() *UpdateLedgerInput {
	return &UpdateLedgerInput{}
}

// WithName sets the name on the update ledger input.
// This updates the human-readable name of the ledger.
//
// Parameters:
//   - name: The new name for the ledger (max length: 256 characters)
//
// Returns:
//   - A pointer to the modified UpdateLedgerInput for method chaining
//
// Example:
//
//	input := models.NewUpdateLedgerInput().WithName("Quarterly Ledger Q1 2025")
func (input *UpdateLedgerInput) WithName(name string) *UpdateLedgerInput {
	input.Name = name
	return input
}

// WithStatus sets the status on the update ledger input.
// This updates the status of the ledger.
//
// Parameters:
//   - status: The new status for the ledger
//
// Returns:
//   - A pointer to the modified UpdateLedgerInput for method chaining
func (input *UpdateLedgerInput) WithStatus(status Status) *UpdateLedgerInput {
	input.Status = status
	return input
}

// WithMetadata sets the metadata on the update ledger input.
// This updates the custom metadata associated with the ledger.
//
// Parameters:
//   - metadata: The new metadata map
//
// Returns:
//   - A pointer to the modified UpdateLedgerInput for method chaining
func (input *UpdateLedgerInput) WithMetadata(metadata map[string]any) *UpdateLedgerInput {
	input.Metadata = metadata
	return input
}

// Validate validates the UpdateLedgerInput and returns an error if it's invalid.
// This method checks that all fields meet the validation constraints defined by the backend.
// For update operations, fields are optional but must be valid if provided.
//
// Returns:
//   - error: An error if the input is invalid, nil otherwise
func (input *UpdateLedgerInput) Validate() error {
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

// ToMmodelUpdateLedgerInput converts an SDK UpdateLedgerInput to an mmodel UpdateLedgerInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdateLedgerInput instance with the same values
func (input UpdateLedgerInput) ToMmodelUpdateLedgerInput() mmodel.UpdateLedgerInput {
	return mmodel.UpdateLedgerInput{
		Name:     input.Name,
		Status:   input.Status.ToMmodelStatus(),
		Metadata: input.Metadata,
	}
}

// validateMetadata validates the metadata map against the backend constraints.
// This is a helper function used by both CreateLedgerInput and UpdateLedgerInput.
//
// Parameters:
//   - metadata: The metadata map to validate
//
// Returns:
//   - error: An error if the metadata is invalid, nil otherwise
func validateMetadata(metadata map[string]any) error {
	for key, value := range metadata {
		// Validate key length
		if len(key) > 100 {
			return fmt.Errorf("metadata key '%s' exceeds maximum length of 100 characters", key)
		}

		// Validate value
		if strValue, ok := value.(string); ok {
			if len(strValue) > 2000 {
				return fmt.Errorf("metadata value for key '%s' exceeds maximum length of 2000 characters", key)
			}
		}

		// Check for nested objects (not allowed)
		if _, ok := value.(map[string]any); ok {
			return fmt.Errorf("nested objects are not allowed in metadata (key: '%s')", key)
		}
	}

	return nil
}

// Ledgers represents a list of ledgers with pagination information.
// This structure is used for paginated responses when listing ledgers.
type Ledgers struct {
	// Items is the collection of ledgers in the current page
	Items []Ledger `json:"items"`

	// Page is the current page number
	Page int `json:"page"`

	// Limit is the maximum number of items per page
	Limit int `json:"limit"`
}

// FromMmodelLedgers converts an mmodel Ledgers to an SDK Ledgers.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - ledgers: The mmodel.Ledgers to convert
//
// Returns:
//   - A models.Ledgers instance with the same values
func FromMmodelLedgers(ledgers mmodel.Ledgers) Ledgers {
	items := make([]Ledger, 0)

	for _, ledger := range ledgers.Items {
		items = append(items, FromMmodelLedger(ledger))
	}

	return Ledgers{
		Items: items,
		Page:  ledgers.Page,
		Limit: ledgers.Limit,
	}
}

// LedgerFilter for filtering ledgers in listings.
// This structure defines the criteria for filtering ledgers when listing them.
type LedgerFilter struct {
	// Status is a list of status codes to filter by
	Status []string `json:"status,omitempty"`
}

// ListLedgerInput for configuring ledger listing requests.
// This structure defines the parameters for listing ledgers.
type ListLedgerInput struct {
	// Page is the page number to retrieve
	Page int `json:"page,omitempty"`

	// PerPage is the number of items per page
	PerPage int `json:"perPage,omitempty"`

	// Filter contains the filtering criteria
	Filter LedgerFilter `json:"filter,omitempty"`
}

// ListLedgerResponse for ledger listing responses.
// This structure represents the response from a list ledgers request.
type ListLedgerResponse struct {
	// Items is the collection of ledgers in the current page
	Items []Ledger `json:"items"`

	// Total is the total number of ledgers matching the criteria
	Total int `json:"total"`

	// CurrentPage is the current page number
	CurrentPage int `json:"currentPage"`

	// PageSize is the number of items per page
	PageSize int `json:"pageSize"`

	// TotalPages is the total number of pages
	TotalPages int `json:"totalPages"`
}
