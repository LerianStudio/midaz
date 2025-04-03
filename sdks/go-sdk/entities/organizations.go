package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// OrganizationsService defines the interface for organization-related operations.
// It provides methods to create, read, update, and delete organizations
// in the Midaz platform.
type OrganizationsService interface {
	// ListOrganizations retrieves a paginated list of organizations with optional filters.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the organizations and pagination information, or an error if the operation fails.
	ListOrganizations(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.Organization], error)

	// GetOrganization retrieves a specific organization by its ID.
	// The id parameter is the unique identifier of the organization to retrieve.
	// Returns the organization if found, or an error if the operation fails or the organization doesn't exist.
	GetOrganization(ctx context.Context, id string) (*models.Organization, error)

	// CreateOrganization creates a new organization.
	// The input parameter contains the organization details such as name and description.
	// Returns the created organization, or an error if the operation fails.
	CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error)

	// UpdateOrganization updates an existing organization.
	// The id parameter is the unique identifier of the organization to update.
	// The input parameter contains the organization details to update, such as name, description, or status.
	// Returns the updated organization, or an error if the operation fails.
	UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error)

	// DeleteOrganization deletes an organization.
	// The id parameter is the unique identifier of the organization to delete.
	// Returns an error if the operation fails.
	DeleteOrganization(ctx context.Context, id string) error
}

// organizationsEntity implements the OrganizationsService interface.
// It handles the communication with the Midaz API for organization-related operations.
type organizationsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewOrganizationsEntity creates a new organizations entity.
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the OrganizationsService interface.
func NewOrganizationsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) OrganizationsService {
	return &organizationsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListOrganizations lists organizations with optional filters.
func (e *organizationsEntity) ListOrganizations(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.Organization], error) {
	url := e.buildURL("")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Add query parameters if provided
	if opts != nil {
		q := req.URL.Query()

		for key, value := range opts.ToQueryParams() {
			q.Add(key, value)
		}

		req.URL.RawQuery = q.Encode()
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response models.ListResponse[models.Organization]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetOrganization gets an organization by ID.
func (e *organizationsEntity) GetOrganization(ctx context.Context, id string) (*models.Organization, error) {
	if id == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	url := e.buildURL(id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var organization models.Organization

	if err := json.NewDecoder(resp.Body).Decode(&organization); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &organization, nil
}

// CreateOrganization creates a new organization.
func (e *organizationsEntity) CreateOrganization(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error) {
	if input == nil {
		return nil, fmt.Errorf("organization input cannot be nil")
	}

	url := e.buildURL("")

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var organization models.Organization

	if err := json.NewDecoder(resp.Body).Decode(&organization); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &organization, nil
}

// UpdateOrganization updates an existing organization.
func (e *organizationsEntity) UpdateOrganization(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error) {
	if id == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("organization input cannot be nil")
	}

	url := e.buildURL(id)

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var organization models.Organization

	if err := json.NewDecoder(resp.Body).Decode(&organization); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &organization, nil
}

// DeleteOrganization deletes an organization.
func (e *organizationsEntity) DeleteOrganization(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("organization ID is required")
	}

	url := e.buildURL(id)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)

	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// buildURL builds the URL for organizations API calls.
func (e *organizationsEntity) buildURL(id string) string {
	base := e.baseURLs["onboarding"]
	if id == "" {
		return fmt.Sprintf("%s/organizations", base)
	}
	return fmt.Sprintf("%s/organizations/%s", base, id)
}

// setCommonHeaders sets common headers for API requests.
func (e *organizationsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
