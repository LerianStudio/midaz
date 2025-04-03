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
	//
	// Organizations are the top-level entities in the Midaz system that own ledgers,
	// accounts, and other resources. Each organization has a legal identity and
	// can manage multiple ledgers.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - input: The organization details, including required fields:
	//     - LegalName: The official registered name of the organization
	//     - LegalDocument: The official identification document (e.g., tax ID)
	//     Optional fields include:
	//     - Status: The initial status (defaults to ACTIVE if not specified)
	//     - Address: The physical address of the organization
	//     - Metadata: Additional custom information about the organization
	//     - ParentOrganizationID: Reference to a parent organization, if applicable
	//     - DoingBusinessAs: Trading or brand name, if different from legal name
	//
	// Returns:
	//   - *models.Organization: The created organization if successful, containing the organization ID,
	//     legal name, status, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields or invalid values)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Network or server errors
	//
	// Example - Creating a basic organization:
	//
	//	// Create a simple organization with just required fields
	//	organization, err := organizationsService.CreateOrganization(
	//	    context.Background(),
	//	    models.NewCreateOrganizationInput(
	//	        "Acme Corporation",
	//	        "123456789",
	//	    ),
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create organization: %v", err)
	//	}
	//
	//	// Use the organization
	//	fmt.Printf("Organization created: %s (status: %s)\n",
	//	    organization.ID, organization.Status.Code)
	//
	// Example - Creating an organization with all options:
	//
	//	// Create an organization with all available options
	//	input := models.NewCreateOrganizationInput(
	//	    "Acme Corporation",
	//	    "123456789",
	//	).WithStatus(
	//	    models.StatusActive,
	//	).WithAddress(
	//	    models.Address{
	//	        Line1:      "123 Main Street",
	//	        City:       "San Francisco",
	//	        State:      "CA",
	//	        PostalCode: "94105",
	//	        Country:    "US",
	//	    },
	//	).WithMetadata(
	//	    map[string]any{
	//	        "industry": "Technology",
	//	        "founded": 2023,
	//	        "website": "https://acme.example.com",
	//	    },
	//	).WithDoingBusinessAs(
	//	    "Acme Tech",
	//	)
	//
	//	organization, err := organizationsService.CreateOrganization(
	//	    context.Background(),
	//	    input,
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create organization: %v", err)
	//	}
	//
	//	// Use the organization
	//	fmt.Printf("Organization created: %s\n", organization.ID)
	//	fmt.Printf("Legal name: %s\n", organization.LegalName)
	//	if organization.DoingBusinessAs != nil {
	//	    fmt.Printf("DBA: %s\n", *organization.DoingBusinessAs)
	//	}
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
//
// Parameters:
//   - httpClient: The HTTP client used for API requests. Can be configured with custom timeouts
//     and transport options. If nil, a default client will be used.
//   - authToken: The authentication token for API authorization. Must be a valid JWT token
//     issued by the Midaz authentication service.
//   - baseURLs: Map of service names to base URLs. Must include an "onboarding" key with
//     the URL of the onboarding service (e.g., "https://api.midaz.io/v1").
//
// Returns:
//   - OrganizationsService: An implementation of the OrganizationsService interface that provides
//     methods for creating, retrieving, updating, and managing organizations.
//
// Example:
//
//	// Create an organizations entity with default HTTP client
//	organizationsEntity := entities.NewOrganizationsEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to retrieve organizations
//	organizations, err := organizationsEntity.ListOrganizations(
//	    context.Background(),
//	    nil, // No pagination options
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to retrieve organizations: %v", err)
//	}
//
//	fmt.Printf("Retrieved %d organizations\n", len(organizations.Items))
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
//
// Organizations are the top-level entities in the Midaz system that own ledgers,
// accounts, and other resources. Each organization has a legal identity and
// can manage multiple ledgers.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - input: The organization details, including required fields:
//   - LegalName: The official registered name of the organization
//   - LegalDocument: The official identification document (e.g., tax ID)
//     Optional fields include:
//   - Status: The initial status (defaults to ACTIVE if not specified)
//   - Address: The physical address of the organization
//   - Metadata: Additional custom information about the organization
//   - ParentOrganizationID: Reference to a parent organization, if applicable
//   - DoingBusinessAs: Trading or brand name, if different from legal name
//
// Returns:
//   - *models.Organization: The created organization if successful, containing the organization ID,
//     legal name, status, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Network or server errors
//
// Example - Creating a basic organization:
//
//	// Create a simple organization with just required fields
//	organization, err := organizationsService.CreateOrganization(
//	    context.Background(),
//	    models.NewCreateOrganizationInput(
//	        "Acme Corporation",
//	        "123456789",
//	    ),
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create organization: %v", err)
//	}
//
//	// Use the organization
//	fmt.Printf("Organization created: %s (status: %s)\n",
//	    organization.ID, organization.Status.Code)
//
// Example - Creating an organization with all options:
//
//	// Create an organization with all available options
//	input := models.NewCreateOrganizationInput(
//	    "Acme Corporation",
//	    "123456789",
//	).WithStatus(
//	    models.StatusActive,
//	).WithAddress(
//	    models.Address{
//	        Line1:      "123 Main Street",
//	        City:       "San Francisco",
//	        State:      "CA",
//	        PostalCode: "94105",
//	        Country:    "US",
//	    },
//	).WithMetadata(
//	    map[string]any{
//	        "industry": "Technology",
//	        "founded": 2023,
//	        "website": "https://acme.example.com",
//	    },
//	).WithDoingBusinessAs(
//	    "Acme Tech",
//	)
//
//	organization, err := organizationsService.CreateOrganization(
//	    context.Background(),
//	    input,
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create organization: %v", err)
//	}
//
//	// Use the organization
//	fmt.Printf("Organization created: %s\n", organization.ID)
//	fmt.Printf("Legal name: %s\n", organization.LegalName)
//	if organization.DoingBusinessAs != nil {
//	    fmt.Printf("DBA: %s\n", *organization.DoingBusinessAs)
//	}
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
