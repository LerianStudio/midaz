package models

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Segment represents a segment in the Midaz system for more granular organization.
// Segments allow for further categorization and grouping of accounts or other entities
// within a ledger, enabling more detailed reporting and management.
type Segment struct {
	// ID is the unique identifier for the segment
	ID string `json:"id"`

	// Name is the human-readable name of the segment
	Name string `json:"name"`

	// LedgerID is the identifier of the ledger that contains this segment
	LedgerID string `json:"ledgerId"`

	// OrganizationID is the identifier of the organization that owns this segment
	OrganizationID string `json:"organizationId"`

	// Status represents the current status of the segment (e.g., "ACTIVE", "INACTIVE")
	Status Status `json:"status"`

	// CreatedAt is the timestamp when the segment was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the segment was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the segment was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// Metadata contains additional custom data associated with the segment
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewSegment creates a new Segment with required fields.
// This constructor ensures that all mandatory fields are provided when creating a segment.
//
// Parameters:
//   - id: Unique identifier for the segment
//   - name: Human-readable name for the segment
//   - ledgerID: Identifier of the ledger that contains this segment
//   - organizationID: Identifier of the organization that owns this segment
//   - status: Current status of the segment
//
// Returns:
//   - A pointer to the newly created Segment
func NewSegment(id, name, ledgerID, organizationID string, status Status) *Segment {
	return &Segment{
		ID:             id,
		Name:           name,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// WithMetadata adds metadata to the segment.
// Metadata can store additional custom information about the segment.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified Segment for method chaining
func (s *Segment) WithMetadata(metadata map[string]any) *Segment {
	s.Metadata = metadata
	return s
}

// FromMmodelSegment converts an mmodel Segment to an SDK Segment.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - segment: The mmodel.Segment to convert
//
// Returns:
//   - A models.Segment instance with the same values
func FromMmodelSegment(segment mmodel.Segment) Segment {
	result := Segment{
		ID:             segment.ID,
		Name:           segment.Name,
		LedgerID:       segment.LedgerID,
		OrganizationID: segment.OrganizationID,
		Status:         FromMmodelStatus(segment.Status),
		CreatedAt:      segment.CreatedAt,
		UpdatedAt:      segment.UpdatedAt,
		Metadata:       segment.Metadata,
	}

	if segment.DeletedAt != nil {
		deletedAt := *segment.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// ToMmodelSegment converts an SDK Segment to an mmodel Segment.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Segment instance with the same values
func (s *Segment) ToMmodelSegment() mmodel.Segment {
	result := mmodel.Segment{
		ID:             s.ID,
		Name:           s.Name,
		LedgerID:       s.LedgerID,
		OrganizationID: s.OrganizationID,
		Status:         s.Status.ToMmodelStatus(),
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
		Metadata:       s.Metadata,
	}

	if s.DeletedAt != nil {
		deletedAt := *s.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// CreateSegmentInput is the input for creating a segment.
// This structure contains all the fields that can be specified when creating a new segment.
type CreateSegmentInput struct {
	// Name is the human-readable name for the segment
	Name string `json:"name"`

	// Status represents the initial status of the segment
	Status Status `json:"status,omitempty"`

	// Metadata contains additional custom data for the segment
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks that the CreateSegmentInput meets all validation requirements.
// It ensures that required fields are present and that all fields meet their
// validation constraints as defined in the API specification.
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (input *CreateSegmentInput) Validate() error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	// Validate metadata if present
	if input.Metadata != nil {
		for key, value := range input.Metadata {
			if len(key) > 100 {
				return fmt.Errorf("metadata key must be at most 100 characters, got %d for key %s", len(key), key)
			}

			// Check value length for string values
			if strValue, ok := value.(string); ok {
				if len(strValue) > 2000 {
					return fmt.Errorf("metadata value must be at most 2000 characters, got %d for key %s", len(strValue), key)
				}
			}
		}
	}

	return nil
}

// NewCreateSegmentInput creates a new CreateSegmentInput with required fields.
// This constructor ensures that all mandatory fields are provided when creating a segment input.
//
// Parameters:
//   - name: Human-readable name for the segment
//
// Returns:
//   - A pointer to the newly created CreateSegmentInput
func NewCreateSegmentInput(name string) *CreateSegmentInput {
	return &CreateSegmentInput{
		Name: name,
	}
}

// WithStatus adds a status to the create segment input.
// This sets the initial status of the segment.
//
// Parameters:
//   - status: The status to set for the segment
//
// Returns:
//   - A pointer to the modified CreateSegmentInput for method chaining
func (c *CreateSegmentInput) WithStatus(status Status) *CreateSegmentInput {
	c.Status = status
	return c
}

// WithMetadata adds metadata to the create segment input.
// Metadata can store additional custom information about the segment.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified CreateSegmentInput for method chaining
func (c *CreateSegmentInput) WithMetadata(metadata map[string]any) *CreateSegmentInput {
	c.Metadata = metadata
	return c
}

// ToMmodelCreateSegmentInput converts an SDK CreateSegmentInput to an mmodel CreateSegmentInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.CreateSegmentInput instance with the same values
func (c *CreateSegmentInput) ToMmodelCreateSegmentInput() mmodel.CreateSegmentInput {
	result := mmodel.CreateSegmentInput{
		Name:     c.Name,
		Metadata: c.Metadata,
	}

	if !c.Status.IsEmpty() {
		result.Status = c.Status.ToMmodelStatus()
	}

	return result
}

// UpdateSegmentInput is the input for updating a segment.
// This structure contains the fields that can be modified when updating an existing segment.
type UpdateSegmentInput struct {
	// Name is the updated human-readable name for the segment
	Name string `json:"name,omitempty"`

	// Status represents the updated status of the segment
	Status Status `json:"status,omitempty"`

	// Metadata contains updated or additional custom data for the segment
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks that the UpdateSegmentInput meets all validation requirements.
// It ensures that all fields meet their validation constraints as defined in the API specification.
//
// Returns:
//   - error: An error if validation fails, nil otherwise
func (input *UpdateSegmentInput) Validate() error {
	// Validate name if provided
	if input.Name != "" && len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	// Validate metadata if present
	if input.Metadata != nil {
		for key, value := range input.Metadata {
			if len(key) > 100 {
				return fmt.Errorf("metadata key must be at most 100 characters, got %d for key %s", len(key), key)
			}

			// Check value length for string values
			if strValue, ok := value.(string); ok {
				if len(strValue) > 2000 {
					return fmt.Errorf("metadata value must be at most 2000 characters, got %d for key %s", len(strValue), key)
				}
			}
		}
	}

	return nil
}

// NewUpdateSegmentInput creates a new empty UpdateSegmentInput.
// This constructor initializes an empty update input that can be customized
// using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdateSegmentInput
func NewUpdateSegmentInput() *UpdateSegmentInput {
	return &UpdateSegmentInput{}
}

// WithName sets the name in the update segment input.
// This updates the human-readable name of the segment.
//
// Parameters:
//   - name: The new name for the segment
//
// Returns:
//   - A pointer to the modified UpdateSegmentInput for method chaining
func (u *UpdateSegmentInput) WithName(name string) *UpdateSegmentInput {
	u.Name = name
	return u
}

// WithStatus sets the status in the update segment input.
// This updates the status of the segment.
//
// Parameters:
//   - status: The new status for the segment
//
// Returns:
//   - A pointer to the modified UpdateSegmentInput for method chaining
func (u *UpdateSegmentInput) WithStatus(status Status) *UpdateSegmentInput {
	u.Status = status
	return u
}

// WithMetadata sets the metadata in the update segment input.
// This updates the custom metadata associated with the segment.
//
// Parameters:
//   - metadata: The new metadata map
//
// Returns:
//   - A pointer to the modified UpdateSegmentInput for method chaining
func (u *UpdateSegmentInput) WithMetadata(metadata map[string]any) *UpdateSegmentInput {
	u.Metadata = metadata
	return u
}

// ToMmodelUpdateSegmentInput converts an SDK UpdateSegmentInput to an mmodel UpdateSegmentInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdateSegmentInput instance with the same values
func (u *UpdateSegmentInput) ToMmodelUpdateSegmentInput() mmodel.UpdateSegmentInput {
	result := mmodel.UpdateSegmentInput{
		Name:     u.Name,
		Metadata: u.Metadata,
	}

	if !u.Status.IsEmpty() {
		result.Status = u.Status.ToMmodelStatus()
	}

	return result
}
