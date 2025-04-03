package models

import (
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Portfolio represents a portfolio in the Midaz system.
// A portfolio is a collection of accounts that belong to a specific entity
// within an organization and ledger. Portfolios help organize accounts
// for better management and reporting.
type Portfolio struct {
	// ID is the unique identifier for the portfolio
	ID string `json:"id"`

	// Name is the human-readable name of the portfolio
	Name string `json:"name"`

	// EntityID is the identifier of the entity that owns this portfolio
	EntityID string `json:"entityId"`

	// LedgerID is the ID of the ledger that contains this portfolio
	LedgerID string `json:"ledgerId"`

	// OrganizationID is the ID of the organization that owns this portfolio
	OrganizationID string `json:"organizationId"`

	// Status represents the current status of the portfolio (e.g., "ACTIVE", "INACTIVE")
	Status Status `json:"status"`

	// CreatedAt is the timestamp when the portfolio was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the portfolio was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the portfolio was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// Metadata contains additional custom data associated with the portfolio
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewPortfolio creates a new Portfolio with required fields.
// This constructor ensures that all mandatory fields are provided when creating a portfolio.
//
// Parameters:
//   - id: Unique identifier for the portfolio
//   - name: Human-readable name for the portfolio
//   - entityID: Identifier of the entity that owns this portfolio
//   - ledgerID: ID of the ledger that contains this portfolio
//   - organizationID: ID of the organization that owns this portfolio
//   - status: Current status of the portfolio
//
// Returns:
//   - A pointer to the newly created Portfolio
func NewPortfolio(id, name, entityID, ledgerID, organizationID string, status Status) *Portfolio {
	return &Portfolio{
		ID:             id,
		Name:           name,
		EntityID:       entityID,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// WithMetadata adds metadata to the portfolio.
// Metadata can store additional custom information about the portfolio.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified Portfolio for method chaining
func (p *Portfolio) WithMetadata(metadata map[string]any) *Portfolio {
	p.Metadata = metadata
	return p
}

// FromMmodelPortfolio converts an mmodel Portfolio to an SDK Portfolio.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - portfolio: The mmodel.Portfolio to convert
//
// Returns:
//   - A models.Portfolio instance with the same values
func FromMmodelPortfolio(portfolio mmodel.Portfolio) Portfolio {
	result := Portfolio{
		ID:             portfolio.ID,
		Name:           portfolio.Name,
		EntityID:       portfolio.EntityID,
		LedgerID:       portfolio.LedgerID,
		OrganizationID: portfolio.OrganizationID,
		Status:         FromMmodelStatus(portfolio.Status),
		CreatedAt:      portfolio.CreatedAt,
		UpdatedAt:      portfolio.UpdatedAt,
		Metadata:       portfolio.Metadata,
	}

	if portfolio.DeletedAt != nil {
		deletedAt := *portfolio.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// ToMmodelPortfolio converts an SDK Portfolio to an mmodel Portfolio.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Portfolio instance with the same values
func (p *Portfolio) ToMmodelPortfolio() mmodel.Portfolio {
	result := mmodel.Portfolio{
		ID:             p.ID,
		Name:           p.Name,
		EntityID:       p.EntityID,
		LedgerID:       p.LedgerID,
		OrganizationID: p.OrganizationID,
		Status:         p.Status.ToMmodelStatus(),
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		Metadata:       p.Metadata,
	}

	if p.DeletedAt != nil {
		deletedAt := *p.DeletedAt

		result.DeletedAt = &deletedAt
	}

	return result
}

// CreatePortfolioInput is the input for creating a portfolio.
// This structure contains all the fields that can be specified when creating a new portfolio.
type CreatePortfolioInput struct {
	// EntityID is the identifier of the entity that will own this portfolio.
	// Max length: 256 characters.
	EntityID string `json:"entityId"`

	// Name is the human-readable name for the portfolio.
	// Required. Max length: 256 characters.
	Name string `json:"name"`

	// Status represents the initial status of the portfolio.
	Status Status `json:"status,omitempty"`

	// Metadata contains additional custom data for the portfolio.
	// Keys max length: 100 characters, Values max length: 2000 characters.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks if the CreatePortfolioInput meets the validation requirements.
// It returns an error if any of the validation checks fail.
func (input *CreatePortfolioInput) Validate() error {
	// Check required fields
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Check length constraints
	if len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	if len(input.EntityID) > 256 {
		return fmt.Errorf("entity ID must be at most 256 characters, got %d", len(input.EntityID))
	}

	// Metadata validation would typically be more complex
	// For now, we'll just ensure it's not nil if provided

	return nil
}

// NewCreatePortfolioInput creates a new CreatePortfolioInput with required fields.
// This constructor ensures that all mandatory fields are provided when creating a portfolio input.
//
// Parameters:
//   - entityID: Identifier of the entity that will own this portfolio
//   - name: Human-readable name for the portfolio
//
// Returns:
//   - A pointer to the newly created CreatePortfolioInput
func NewCreatePortfolioInput(entityID, name string) *CreatePortfolioInput {
	return &CreatePortfolioInput{
		EntityID: entityID,
		Name:     name,
	}
}

// WithStatus adds a status to the create portfolio input.
// This sets the initial status of the portfolio.
//
// Parameters:
//   - status: The status to set for the portfolio
//
// Returns:
//   - A pointer to the modified CreatePortfolioInput for method chaining
func (c *CreatePortfolioInput) WithStatus(status Status) *CreatePortfolioInput {
	c.Status = status
	return c
}

// WithMetadata adds metadata to the create portfolio input.
// Metadata can store additional custom information about the portfolio.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified CreatePortfolioInput for method chaining
func (c *CreatePortfolioInput) WithMetadata(metadata map[string]any) *CreatePortfolioInput {
	c.Metadata = metadata
	return c
}

// ToMmodelCreatePortfolioInput converts an SDK CreatePortfolioInput to an mmodel CreatePortfolioInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.CreatePortfolioInput instance with the same values
func (c *CreatePortfolioInput) ToMmodelCreatePortfolioInput() mmodel.CreatePortfolioInput {
	result := mmodel.CreatePortfolioInput{
		EntityID: c.EntityID,
		Name:     c.Name,
		Metadata: c.Metadata,
	}

	if !c.Status.IsEmpty() {
		result.Status = c.Status.ToMmodelStatus()
	}

	return result
}

// UpdatePortfolioInput is the input for updating a portfolio.
// This structure contains the fields that can be modified when updating an existing portfolio.
type UpdatePortfolioInput struct {
	// Name is the updated human-readable name for the portfolio.
	// Max length: 256 characters.
	Name string `json:"name,omitempty"`

	// Status is the updated status of the portfolio.
	Status Status `json:"status,omitempty"`

	// Metadata contains updated additional custom data.
	// Keys max length: 100 characters, Values max length: 2000 characters.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Validate checks if the UpdatePortfolioInput meets the validation requirements.
// It returns an error if any of the validation checks fail.
func (input *UpdatePortfolioInput) Validate() error {
	// Check length constraints for optional fields
	if len(input.Name) > 256 {
		return fmt.Errorf("name must be at most 256 characters, got %d", len(input.Name))
	}

	// Metadata validation would typically be more complex
	// For now, we'll just ensure it's not nil if provided

	return nil
}

// NewUpdatePortfolioInput creates a new empty UpdatePortfolioInput.
// This constructor initializes an empty update input that can be customized
// using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdatePortfolioInput
func NewUpdatePortfolioInput() *UpdatePortfolioInput {
	return &UpdatePortfolioInput{}
}

// WithName sets the name in the update portfolio input.
// This updates the human-readable name of the portfolio.
//
// Parameters:
//   - name: The new name for the portfolio
//
// Returns:
//   - A pointer to the modified UpdatePortfolioInput for method chaining
func (u *UpdatePortfolioInput) WithName(name string) *UpdatePortfolioInput {
	u.Name = name
	return u
}

// WithStatus sets the status in the update portfolio input.
// This updates the status of the portfolio.
//
// Parameters:
//   - status: The new status for the portfolio
//
// Returns:
//   - A pointer to the modified UpdatePortfolioInput for method chaining
func (u *UpdatePortfolioInput) WithStatus(status Status) *UpdatePortfolioInput {
	u.Status = status
	return u
}

// WithMetadata sets the metadata in the update portfolio input.
// This updates the custom metadata associated with the portfolio.
//
// Parameters:
//   - metadata: The new metadata map
//
// Returns:
//   - A pointer to the modified UpdatePortfolioInput for method chaining
func (u *UpdatePortfolioInput) WithMetadata(metadata map[string]any) *UpdatePortfolioInput {
	u.Metadata = metadata
	return u
}

// ToMmodelUpdatePortfolioInput converts an SDK UpdatePortfolioInput to an mmodel UpdatePortfolioInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdatePortfolioInput instance with the same values
func (u *UpdatePortfolioInput) ToMmodelUpdatePortfolioInput() mmodel.UpdatePortfolioInput {
	result := mmodel.UpdatePortfolioInput{
		Name:     u.Name,
		Metadata: u.Metadata,
	}

	if !u.Status.IsEmpty() {
		result.Status = u.Status.ToMmodelStatus()
	}

	return result
}

// Portfolios represents a collection of portfolios with pagination information.
// This structure is used for paginated responses when listing portfolios.
type Portfolios struct {
	// Items is the collection of portfolios in the current page
	Items []Portfolio `json:"items"`

	// Page is the current page number
	Page int `json:"page"`

	// Limit is the maximum number of items per page
	Limit int `json:"limit"`
}

// FromMmodelPortfolios converts an mmodel Portfolios to an SDK Portfolios.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - portfolios: The mmodel.Portfolios to convert
//
// Returns:
//   - A models.Portfolios instance with the same values
func FromMmodelPortfolios(portfolios mmodel.Portfolios) Portfolios {
	result := Portfolios{
		Page:  portfolios.Page,
		Limit: portfolios.Limit,
		Items: make([]Portfolio, 0, len(portfolios.Items)),
	}

	for _, portfolio := range portfolios.Items {
		result.Items = append(result.Items, FromMmodelPortfolio(portfolio))
	}

	return result
}
