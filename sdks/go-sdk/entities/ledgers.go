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
	// The organizationID parameter specifies which organization to create the ledger in.
	// The input parameter contains the ledger details such as name and description.
	// Returns the created ledger, or an error if the operation fails.
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
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the LedgersService interface.
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
// The organizationID parameter specifies which organization to create the ledger in.
// The input parameter contains the ledger details such as name and description.
// Returns the created ledger, or an error if the operation fails.
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
