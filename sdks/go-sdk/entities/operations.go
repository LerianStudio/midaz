package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// OperationsService defines the interface for operation-related operations.
// It provides methods to list, retrieve, and update operations
// associated with accounts and transactions.
type OperationsService interface {
	// ListOperations retrieves a paginated list of operations for an account with optional filters.
	// The orgID, ledgerID, and accountID parameters specify which organization, ledger, and account to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the operations and pagination information, or an error if the operation fails.
	ListOperations(ctx context.Context, orgID, ledgerID, accountID string, opts *models.ListOptions) (*models.ListResponse[models.Operation], error)

	// GetOperation retrieves a specific operation by its ID.
	// The orgID, ledgerID, and accountID parameters specify which organization, ledger, and account the operation belongs to.
	// The operationID parameter is the unique identifier of the operation to retrieve.
	// Returns the operation if found, or an error if the operation fails or the operation doesn't exist.
	GetOperation(ctx context.Context, orgID, ledgerID, accountID, operationID string) (*models.Operation, error)

	// UpdateOperation updates an existing operation.
	// The orgID, ledgerID, and transactionID parameters specify which organization, ledger, and transaction the operation belongs to.
	// The operationID parameter is the unique identifier of the operation to update.
	// The input parameter contains the operation details to update.
	// Returns the updated operation, or an error if the operation fails.
	UpdateOperation(ctx context.Context, orgID, ledgerID, transactionID, operationID string, input any) (*models.Operation, error)
}

// operationsEntity implements the OperationsService interface.
// It handles the communication with the Midaz API for operation-related operations.
type operationsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewOperationsEntity creates a new operations entity.
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the OperationsService interface.
func NewOperationsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) OperationsService {
	return &operationsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListOperations lists operations for an account with optional filters.
func (e *operationsEntity) ListOperations(ctx context.Context, orgID, ledgerID, accountID string, opts *models.ListOptions) (*models.ListResponse[models.Operation], error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	url := e.buildURL(orgID, ledgerID, accountID, "")

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

	var response models.ListResponse[models.Operation]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return &response, nil
}

// GetOperation retrieves an operation by its ID.
func (e *operationsEntity) GetOperation(ctx context.Context, orgID, ledgerID, accountID, operationID string) (*models.Operation, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	if operationID == "" {
		return nil, fmt.Errorf("operation ID is required")
	}

	url := e.buildURL(orgID, ledgerID, accountID, operationID)

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

	var operation models.Operation

	if err := json.NewDecoder(resp.Body).Decode(&operation); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return &operation, nil
}

// UpdateOperation updates an operation.
func (e *operationsEntity) UpdateOperation(ctx context.Context, orgID, ledgerID, transactionID, operationID string, input any) (*models.Operation, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if transactionID == "" {
		return nil, fmt.Errorf("transaction ID is required")
	}

	if operationID == "" {
		return nil, fmt.Errorf("operation ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}

	url := fmt.Sprintf("%s/organizations/%s/ledgers/%s/transactions/%s/operations/%s", e.baseURLs["transaction"], orgID, ledgerID, transactionID, operationID)

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

	var operation models.Operation

	if err := json.NewDecoder(resp.Body).Decode(&operation); err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	return &operation, nil
}

// buildURL builds the URL for operations API calls.
func (e *operationsEntity) buildURL(orgID, ledgerID, accountID, operationID string) string {
	base := e.baseURLs["transaction"]
	if operationID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/accounts/%s/operations", base, orgID, ledgerID, accountID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/accounts/%s/operations/%s", base, orgID, ledgerID, accountID, operationID)
}

// setCommonHeaders sets common headers for API requests.
func (e *operationsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
