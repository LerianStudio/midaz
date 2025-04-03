// Package models defines the data models used by the Midaz SDK.
package models

import (
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
)

// Organization represents an organization in the Midaz Ledger.
// Organizations are the top-level entities in the Midaz system that own ledgers,
// accounts, and other resources. Each organization has a legal identity and
// can manage multiple ledgers.
type Organization struct {
	// ID is the unique identifier for the organization
	ID string `json:"id"`

	// LegalName is the official registered name of the organization
	LegalName string `json:"legalName"`

	// LegalDocument is the official identification document (e.g., tax ID, registration number)
	LegalDocument string `json:"legalDocument"`

	// ParentOrganizationID is the reference to the parent organization, if this is a child organization
	ParentOrganizationID *string `json:"parentOrganizationId,omitempty"`

	// DoingBusinessAs is the trading or brand name of the organization, if different from legal name
	DoingBusinessAs *string `json:"doingBusinessAs,omitempty"`

	// Status represents the current status of the organization (e.g., "ACTIVE", "SUSPENDED")
	Status Status `json:"status"`

	// Address is the physical address of the organization
	Address Address `json:"address"`

	// Metadata contains additional custom data associated with the organization
	Metadata map[string]any `json:"metadata,omitempty"`

	// CreatedAt is the timestamp when the organization was created
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is the timestamp when the organization was last updated
	UpdatedAt time.Time `json:"updatedAt"`

	// DeletedAt is the timestamp when the organization was deleted, if applicable
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// NewOrganization creates a new Organization with required fields.
// This constructor ensures that all mandatory fields are provided when creating an organization.
//
// Parameters:
//   - id: Unique identifier for the organization
//   - legalName: Official registered name of the organization
//   - legalDocument: Official identification document (e.g., tax ID, registration number)
//   - status: Current status of the organization
//
// Returns:
//   - A pointer to the newly created Organization
func NewOrganization(id, legalName, legalDocument string, status Status) *Organization {
	return &Organization{
		ID:            id,
		LegalName:     legalName,
		LegalDocument: legalDocument,
		Status:        status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

// WithAddress adds an address to the organization.
// This sets the physical address information for the organization.
//
// Parameters:
//   - address: The physical address of the organization
//
// Returns:
//   - A pointer to the modified Organization for method chaining
func (o *Organization) WithAddress(address Address) *Organization {
	o.Address = address
	return o
}

// WithMetadata adds metadata to the organization.
// Metadata can store additional custom information about the organization.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified Organization for method chaining
func (o *Organization) WithMetadata(metadata map[string]any) *Organization {
	o.Metadata = metadata
	return o
}

// WithParentOrganizationID sets the parent organization ID for the organization.
// This sets the reference to the parent organization, if this is a child organization.
//
// Parameters:
//   - parentOrganizationID: The ID of the parent organization
//
// Returns:
//   - A pointer to the modified Organization for method chaining
func (o *Organization) WithParentOrganizationID(parentOrganizationID string) *Organization {
	o.ParentOrganizationID = &parentOrganizationID
	return o
}

// WithDoingBusinessAs sets the doing business as name for the organization.
// This sets the trading or brand name of the organization, if different from legal name.
//
// Parameters:
//   - doingBusinessAs: The doing business as name of the organization
//
// Returns:
//   - A pointer to the modified Organization for method chaining
func (o *Organization) WithDoingBusinessAs(doingBusinessAs string) *Organization {
	o.DoingBusinessAs = &doingBusinessAs
	return o
}

// FromMmodelOrganization converts an mmodel Organization to an SDK Organization.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - org: The mmodel.Organization to convert
//
// Returns:
//   - A models.Organization instance with the same values
func FromMmodelOrganization(org mmodel.Organization) Organization {
	return Organization{
		ID:                   org.ID,
		LegalName:            org.LegalName,
		LegalDocument:        org.LegalDocument,
		ParentOrganizationID: org.ParentOrganizationID,
		DoingBusinessAs:      org.DoingBusinessAs,
		Status:               FromMmodelStatus(org.Status),
		Address:              FromMmodelAddress(org.Address),
		Metadata:             org.Metadata,
		CreatedAt:            org.CreatedAt,
		UpdatedAt:            org.UpdatedAt,
		DeletedAt:            org.DeletedAt,
	}
}

// ToMmodelOrganization converts an SDK Organization to an mmodel Organization.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.Organization instance with the same values
func (o Organization) ToMmodelOrganization() mmodel.Organization {
	return mmodel.Organization{
		ID:                   o.ID,
		LegalName:            o.LegalName,
		LegalDocument:        o.LegalDocument,
		ParentOrganizationID: o.ParentOrganizationID,
		DoingBusinessAs:      o.DoingBusinessAs,
		Status:               o.Status.ToMmodelStatus(),
		Address:              o.Address.ToMmodelAddress(),
		Metadata:             o.Metadata,
		CreatedAt:            o.CreatedAt,
		UpdatedAt:            o.UpdatedAt,
		DeletedAt:            o.DeletedAt,
	}
}

// CreateOrganizationInput is the input for creating an organization.
// This structure contains all the fields that can be specified when creating a new organization.
type CreateOrganizationInput struct {
	// LegalName is the official registered name of the organization
	LegalName string `json:"legalName"`

	// LegalDocument is the official identification document (e.g., tax ID, registration number)
	LegalDocument string `json:"legalDocument"`

	// ParentOrganizationID is the reference to the parent organization, if this is a child organization
	ParentOrganizationID *string `json:"parentOrganizationId,omitempty"`

	// DoingBusinessAs is the trading or brand name of the organization, if different from legal name
	DoingBusinessAs *string `json:"doingBusinessAs,omitempty"`

	// Status represents the initial status of the organization
	Status Status `json:"status"`

	// Address is the physical address of the organization
	Address Address `json:"address"`

	// Metadata contains additional custom data for the organization
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewCreateOrganizationInput creates a new CreateOrganizationInput with required fields.
// This constructor ensures that all mandatory fields are provided when creating an organization input.
//
// Parameters:
//   - legalName: Official registered name of the organization
//   - legalDocument: Official identification document (e.g., tax ID, registration number)
//
// Returns:
//   - A pointer to the newly created CreateOrganizationInput with default active status
func NewCreateOrganizationInput(legalName, legalDocument string) *CreateOrganizationInput {
	return &CreateOrganizationInput{
		LegalName:     legalName,
		LegalDocument: legalDocument,
		Status:        NewStatus("ACTIVE"), // Default status
	}
}

// WithStatus sets a custom status on the organization input.
// This overrides the default "ACTIVE" status set by the constructor.
//
// Parameters:
//   - status: The status to set for the organization
//
// Returns:
//   - A pointer to the modified CreateOrganizationInput for method chaining
func (input *CreateOrganizationInput) WithStatus(status Status) *CreateOrganizationInput {
	input.Status = status
	return input
}

// WithAddress adds an address to the organization input.
// This sets the physical address information for the organization.
//
// Parameters:
//   - address: The physical address of the organization
//
// Returns:
//   - A pointer to the modified CreateOrganizationInput for method chaining
func (input *CreateOrganizationInput) WithAddress(address Address) *CreateOrganizationInput {
	input.Address = address
	return input
}

// WithMetadata adds metadata to the organization input.
// Metadata can store additional custom information about the organization.
//
// Parameters:
//   - metadata: A map of key-value pairs to store as metadata
//
// Returns:
//   - A pointer to the modified CreateOrganizationInput for method chaining
func (input *CreateOrganizationInput) WithMetadata(metadata map[string]any) *CreateOrganizationInput {
	input.Metadata = metadata
	return input
}

// WithParentOrganizationID sets the parent organization ID for the organization input.
// This sets the reference to the parent organization, if this is a child organization.
//
// Parameters:
//   - parentOrganizationID: The ID of the parent organization
//
// Returns:
//   - A pointer to the modified CreateOrganizationInput for method chaining
func (input *CreateOrganizationInput) WithParentOrganizationID(parentOrganizationID string) *CreateOrganizationInput {
	input.ParentOrganizationID = &parentOrganizationID
	return input
}

// WithDoingBusinessAs sets the doing business as name for the organization input.
// This sets the trading or brand name of the organization, if different from legal name.
//
// Parameters:
//   - doingBusinessAs: The doing business as name of the organization
//
// Returns:
//   - A pointer to the modified CreateOrganizationInput for method chaining
func (input *CreateOrganizationInput) WithDoingBusinessAs(doingBusinessAs string) *CreateOrganizationInput {
	input.DoingBusinessAs = &doingBusinessAs
	return input
}

// ToMmodelCreateOrganizationInput converts an SDK CreateOrganizationInput to an mmodel CreateOrganizationInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.CreateOrganizationInput instance with the same values
func (input CreateOrganizationInput) ToMmodelCreateOrganizationInput() mmodel.CreateOrganizationInput {
	return mmodel.CreateOrganizationInput{
		LegalName:            input.LegalName,
		LegalDocument:        input.LegalDocument,
		ParentOrganizationID: input.ParentOrganizationID,
		DoingBusinessAs:      input.DoingBusinessAs,
		Status:               input.Status.ToMmodelStatus(),
		Address:              input.Address.ToMmodelAddress(),
		Metadata:             input.Metadata,
	}
}

// UpdateOrganizationInput is the input for updating an organization.
// This structure contains all the fields that can be specified when updating an existing organization.
// Only fields that are set will be updated; omitted fields will remain unchanged.
type UpdateOrganizationInput struct {
	// LegalName is the updated official registered name of the organization
	LegalName string `json:"legalName"`

	// ParentOrganizationID is the reference to the parent organization, if this is a child organization
	ParentOrganizationID *string `json:"parentOrganizationId,omitempty"`

	// DoingBusinessAs is the updated trading or brand name of the organization
	DoingBusinessAs string `json:"doingBusinessAs"`

	// Address is the updated physical address of the organization
	Address Address `json:"address"`

	// Status represents the updated status of the organization
	Status Status `json:"status"`

	// Metadata contains updated custom data for the organization
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewUpdateOrganizationInput creates a new empty UpdateOrganizationInput.
// This constructor creates an empty input that can be populated using the With* methods.
//
// Returns:
//   - A pointer to the newly created UpdateOrganizationInput
func NewUpdateOrganizationInput() *UpdateOrganizationInput {
	return &UpdateOrganizationInput{}
}

// WithLegalName sets the legal name on the organization update input.
//
// Parameters:
//   - legalName: The updated official registered name of the organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithLegalName(legalName string) *UpdateOrganizationInput {
	input.LegalName = legalName
	return input
}

// WithParentOrganizationID sets the parent organization ID on the organization update input.
//
// Parameters:
//   - parentOrganizationID: The updated reference to the parent organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithParentOrganizationID(parentOrganizationID string) *UpdateOrganizationInput {
	input.ParentOrganizationID = &parentOrganizationID
	return input
}

// WithDoingBusinessAs sets the doing business as name on the organization update input.
//
// Parameters:
//   - doingBusinessAs: The updated trading or brand name of the organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithDoingBusinessAs(doingBusinessAs string) *UpdateOrganizationInput {
	input.DoingBusinessAs = doingBusinessAs
	return input
}

// WithAddress sets the address on the organization update input.
//
// Parameters:
//   - address: The updated physical address of the organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithAddress(address Address) *UpdateOrganizationInput {
	input.Address = address
	return input
}

// WithStatus sets the status on the organization update input.
//
// Parameters:
//   - status: The updated status of the organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithStatus(status Status) *UpdateOrganizationInput {
	input.Status = status
	return input
}

// WithMetadata sets the metadata on the organization update input.
//
// Parameters:
//   - metadata: The updated custom data for the organization
//
// Returns:
//   - A pointer to the modified UpdateOrganizationInput for method chaining
func (input *UpdateOrganizationInput) WithMetadata(metadata map[string]any) *UpdateOrganizationInput {
	input.Metadata = metadata
	return input
}

// ToMmodelUpdateOrganizationInput converts an SDK UpdateOrganizationInput to an mmodel UpdateOrganizationInput.
// This method is used internally to convert between SDK and backend models.
//
// Returns:
//   - An mmodel.UpdateOrganizationInput instance with the same values
func (input UpdateOrganizationInput) ToMmodelUpdateOrganizationInput() mmodel.UpdateOrganizationInput {
	return mmodel.UpdateOrganizationInput{
		LegalName:            input.LegalName,
		ParentOrganizationID: input.ParentOrganizationID,
		DoingBusinessAs:      input.DoingBusinessAs,
		Address:              input.Address.ToMmodelAddress(),
		Status:               input.Status.ToMmodelStatus(),
		Metadata:             input.Metadata,
	}
}

// FromMmodelUpdateOrganizationInput converts an mmodel UpdateOrganizationInput to an SDK UpdateOrganizationInput.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - input: The mmodel.UpdateOrganizationInput to convert
//
// Returns:
//   - A models.UpdateOrganizationInput instance with the same values
func FromMmodelUpdateOrganizationInput(input mmodel.UpdateOrganizationInput) UpdateOrganizationInput {
	return UpdateOrganizationInput{
		LegalName:            input.LegalName,
		ParentOrganizationID: input.ParentOrganizationID,
		DoingBusinessAs:      input.DoingBusinessAs,
		Address:              FromMmodelAddress(input.Address),
		Status:               FromMmodelStatus(input.Status),
		Metadata:             input.Metadata,
	}
}

// Organizations represents a list of organizations with pagination information.
// This structure is used for paginated responses when listing organizations.
type Organizations struct {
	// Items is the collection of organizations in the current page
	Items []Organization `json:"items"`

	// Page is the current page number
	Page int `json:"page"`

	// Limit is the maximum number of items per page
	Limit int `json:"limit"`
}

// FromMmodelOrganizations converts an mmodel Organizations to an SDK Organizations.
// This function is used internally to convert between backend and SDK models.
//
// Parameters:
//   - orgs: The mmodel.Organizations to convert
//
// Returns:
//   - A models.Organizations instance with the same values
func FromMmodelOrganizations(orgs mmodel.Organizations) Organizations {
	items := make([]Organization, 0)

	for _, org := range orgs.Items {
		items = append(items, FromMmodelOrganization(org))
	}

	return Organizations{
		Items: items,
		Page:  orgs.Page,
		Limit: orgs.Limit,
	}
}

// OrganizationFilter for filtering organizations in listings.
// This structure defines the criteria for filtering organizations when listing them.
type OrganizationFilter struct {
	// Status is a list of status codes to filter by
	Status []string `json:"status,omitempty"`
}

// ListOrganizationInput for configuring organization listing requests.
// This structure defines the parameters for listing organizations.
type ListOrganizationInput struct {
	// Page is the page number to retrieve
	Page int `json:"page,omitempty"`

	// PerPage is the number of items per page
	PerPage int `json:"perPage,omitempty"`

	// Filter contains the filtering criteria
	Filter OrganizationFilter `json:"filter,omitempty"`
}

// ListOrganizationResponse for organization listing responses.
// This structure represents the response from a list organizations request.
type ListOrganizationResponse struct {
	// Items is the collection of organizations in the current page
	Items []Organization `json:"items"`

	// Total is the total number of organizations matching the criteria
	Total int `json:"total"`

	// CurrentPage is the current page number
	CurrentPage int `json:"currentPage"`

	// PageSize is the number of items per page
	PageSize int `json:"pageSize"`

	// TotalPages is the total number of pages
	TotalPages int `json:"totalPages"`
}
