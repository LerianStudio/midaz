package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// LedgersService defines the interface for ledger-related operations.
// It provides methods to create, read, update, and delete ledgers
// within an organization.
type LedgersService interface {
	// ListLedgers retrieves a paginated list of ledgers for an organization with optional filters.
	// The organizationID parameter specifies which organization to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the ledgers and pagination information, or an error if the operation fails.
	ListLedgers(ctx context.Context, organizationID string, opts *models.ListOptions) (*models.ListResponse[models.Ledger], error)

	// GetLedger retrieves a specific ledger by its ID.
	// The organizationID parameter specifies which organization the ledger belongs to.
	// The id parameter is the unique identifier of the ledger to retrieve.
	// Returns the ledger if found, or an error if the operation fails or the ledger doesn't exist.
	GetLedger(ctx context.Context, organizationID, id string) (*models.Ledger, error)

	// CreateLedger creates a new ledger in the specified organization.
	//
	// Ledgers are the top-level financial record-keeping systems that contain accounts
	// and track all transactions between those accounts. Each ledger belongs to a specific
	// organization and can have multiple accounts.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - organizationID: The ID of the organization where the ledger will be created.
	//     Must be a valid organization ID.
	//   - input: The ledger details, including required fields:
	//     - Name: The human-readable name of the ledger (max length: 256 characters)
	//     Optional fields include:
	//     - Status: The initial status (defaults to ACTIVE if not specified)
	//     - Metadata: Additional custom information about the ledger
	//
	// Returns:
	//   - *models.Ledger: The created ledger if successful, containing the ledger ID,
	//     name, status, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields or invalid values)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization ID)
	//     - Network or server errors
	//
	// Example - Creating a basic ledger:
	//
	//	// Create a simple ledger with just a name
	//	ledger, err := ledgersService.CreateLedger(
	//	    context.Background(),
	//	    "org-123",
	//	    models.NewCreateLedgerInput("Main Ledger"),
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the ledger
	//	fmt.Printf("Ledger created: %s (status: %s)\n", ledger.ID, ledger.Status.Code)
	//
	// Example - Creating a ledger with metadata:
	//
	//	// Create a ledger with custom status and metadata
	//	ledger, err := ledgersService.CreateLedger(
	//	    context.Background(),
	//	    "org-123",
	//	    models.NewCreateLedgerInput("Finance Ledger").
	//	        WithStatus(models.StatusActive).
	//	        WithMetadata(map[string]any{
	//	            "department": "Finance",
	//	            "fiscalYear": 2025,
	//	            "currency": "USD",
	//	            "description": "Primary ledger for financial operations",
	//	        }),
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the ledger
	//	fmt.Printf("Finance ledger created: %s\n", ledger.ID)
	CreateLedger(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error)

	// UpdateLedger updates an existing ledger.
	// The organizationID parameter specifies which organization the ledger belongs to.
	// The id parameter is the unique identifier of the ledger to update.
	// The input parameter contains the ledger details to update, such as name, description, or status.
	// Returns the updated ledger, or an error if the operation fails.
	UpdateLedger(ctx context.Context, organizationID, id string, input *models.UpdateLedgerInput) (*models.Ledger, error)

	// DeleteLedger deletes a ledger.
	// The organizationID parameter specifies which organization the ledger belongs to.
	// The id parameter is the unique identifier of the ledger to delete.
	// Returns an error if the operation fails.
	DeleteLedger(ctx context.Context, organizationID, id string) error
}

// ledgersEntity implements the LedgersService interface.
// It handles the communication with the Midaz API for ledger-related operations.
type ledgersEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewLedgersEntity creates a new ledgers entity.
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
//   - LedgersService: An implementation of the LedgersService interface that provides
//     methods for creating, retrieving, updating, and managing ledgers.
//
// Example:
//
//	// Create a ledgers entity with default HTTP client
//	ledgersEntity := entities.NewLedgersEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to create a new ledger
//	ledger, err := ledgersEntity.CreateLedger(
//	    context.Background(),
//	    "org-123",
//	    models.NewCreateLedgerInput("Main Ledger").
//	        WithMetadata(map[string]any{
//	            "department": "Finance",
//	            "fiscalYear": 2025,
//	        }),
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create ledger: %v", err)
//	}
//
//	fmt.Printf("Ledger created: %s\n", ledger.ID)
func NewLedgersEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) LedgersService {
	return &ledgersEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListLedgers lists all ledgers for an organization with optional filters.
// The organizationID parameter specifies which organization to query.
// The opts parameter can be used to specify pagination, sorting, and filtering options.
// Returns a ListResponse containing the ledgers and pagination information, or an error if the operation fails.
func (e *ledgersEntity) ListLedgers(
	ctx context.Context,
	organizationID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Ledger], error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	url := e.buildURL(organizationID, "")

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

	// Set common headers
	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var response models.ListResponse[models.Ledger]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetLedger gets a ledger by ID.
// The organizationID parameter specifies which organization the ledger belongs to.
// The id parameter is the unique identifier of the ledger to retrieve.
// Returns the ledger if found, or an error if the operation fails or the ledger doesn't exist.
func (e *ledgersEntity) GetLedger(
	ctx context.Context,
	organizationID, id string,
) (*models.Ledger, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	url := e.buildURL(organizationID, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Set common headers
	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var ledger models.Ledger

	if err := json.NewDecoder(resp.Body).Decode(&ledger); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ledger, nil
}

// CreateLedger creates a new ledger in the specified organization.
//
// Ledgers are the top-level financial record-keeping systems that contain accounts
// and track all transactions between those accounts. Each ledger belongs to a specific
// organization and can have multiple accounts.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization where the ledger will be created.
//     Must be a valid organization ID.
//   - input: The ledger details, including required fields:
//   - Name: The human-readable name of the ledger (max length: 256 characters)
//     Optional fields include:
//   - Status: The initial status (defaults to ACTIVE if not specified)
//   - Metadata: Additional custom information about the ledger
//
// Returns:
//   - *models.Ledger: The created ledger if successful, containing the ledger ID,
//     name, status, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization ID)
//   - Network or server errors
//
// Example - Creating a basic ledger:
//
//	// Create a simple ledger with just a name
//	ledger, err := ledgersService.CreateLedger(
//	    context.Background(),
//	    "org-123",
//	    models.NewCreateLedgerInput("Main Ledger"),
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the ledger
//	fmt.Printf("Ledger created: %s (status: %s)\n", ledger.ID, ledger.Status.Code)
//
// Example - Creating a ledger with metadata:
//
//	// Create a ledger with custom status and metadata
//	ledger, err := ledgersService.CreateLedger(
//	    context.Background(),
//	    "org-123",
//	    models.NewCreateLedgerInput("Finance Ledger").
//	        WithStatus(models.StatusActive).
//	        WithMetadata(map[string]any{
//	            "department": "Finance",
//	            "fiscalYear": 2025,
//	            "currency": "USD",
//	            "description": "Primary ledger for financial operations",
//	        }),
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the ledger
//	fmt.Printf("Finance ledger created: %s\n", ledger.ID)
func (e *ledgersEntity) CreateLedger(
	ctx context.Context,
	organizationID string,
	input *models.CreateLedgerInput,
) (*models.Ledger, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("ledger input cannot be nil")
	}

	// Validate the input before making the API call
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ledger input: %v", err)
	}

	url := e.buildURL(organizationID, "")

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Set common headers
	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var ledger models.Ledger

	if err := json.NewDecoder(resp.Body).Decode(&ledger); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ledger, nil
}

// UpdateLedger updates an existing ledger.
// The organizationID parameter specifies which organization the ledger belongs to.
// The id parameter is the unique identifier of the ledger to update.
// The input parameter contains the ledger details to update, such as name, description, or status.
// Returns the updated ledger, or an error if the operation fails.
func (e *ledgersEntity) UpdateLedger(
	ctx context.Context,
	organizationID, id string,
	input *models.UpdateLedgerInput,
) (*models.Ledger, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("ledger input cannot be nil")
	}

	// Validate the input before making the API call
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ledger update input: %v", err)
	}

	url := e.buildURL(organizationID, id)

	body, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(body))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	// Set common headers
	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var ledger models.Ledger

	if err := json.NewDecoder(resp.Body).Decode(&ledger); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ledger, nil
}

// DeleteLedger deletes a ledger.
// The organizationID parameter specifies which organization the ledger belongs to.
// The id parameter is the unique identifier of the ledger to delete.
// Returns an error if the operation fails.
func (e *ledgersEntity) DeleteLedger(
	ctx context.Context,
	organizationID, id string,
) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if id == "" {
		return fmt.Errorf("ledger ID is required")
	}

	url := e.buildURL(organizationID, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)

	if err != nil {
		return fmt.Errorf("internal error: %w", err)
	}

	// Set common headers
	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return nil
}

// buildURL builds the URL for ledgers API calls.
func (e *ledgersEntity) buildURL(organizationID, ledgerID string) string {
	base := e.baseURLs["onboarding"]
	if ledgerID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers", base, organizationID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s", base, organizationID, ledgerID)
}

// setCommonHeaders sets common headers for API requests.
func (e *ledgersEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
