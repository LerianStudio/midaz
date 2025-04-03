package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/api"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// BalancesService defines the interface for balance-related operations.
// It provides methods to list, retrieve, update, and delete balances
// for both ledgers and specific accounts.
type BalancesService interface {
	// ListBalances retrieves a paginated list of all balances for a specified ledger.
	//
	// This method returns all balances within a ledger, with optional filtering and
	// pagination controls. Balances represent the current state of funds for each
	// account-asset combination in the ledger.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger to retrieve balances from. Must be a valid ledger ID.
	//   - opts: Optional pagination and filtering options:
	//     - Page: The page number to retrieve (1-based indexing)
	//     - Limit: The maximum number of items per page
	//     - Filter: Criteria to filter balances by (e.g., by account ID or asset code)
	//     - Sort: Sorting options for the results
	//     If nil, default pagination settings will be used.
	//
	// Returns:
	//   - *models.ListResponse[models.Balance]: A paginated list of balances, including:
	//     - Items: The array of balance objects for the current page
	//     - Page: The current page number
	//     - Limit: The maximum number of items per page
	//     - Total: The total number of balances matching the filter criteria
	//   - error: An error if the operation fails. Possible errors include:
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization or ledger ID)
	//     - Network or server errors
	//
	// Example - Basic usage:
	//
	//	// List balances with default pagination
	//	balances, err := balancesService.ListBalances(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    nil, // Use default pagination
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to list balances: %v", err)
	//	}
	//
	//	// Process the balances
	//	fmt.Printf("Retrieved %d balances (page %d of %d)\n",
	//	    len(balances.Items), balances.Page, balances.TotalPages)
	//
	//	for _, balance := range balances.Items {
	//	    fmt.Printf("Balance: %s, Asset: %s, Available: %d/%d\n",
	//	        balance.ID, balance.AssetCode, balance.Available, balance.Scale)
	//	}
	//
	// Example - With pagination and filtering:
	//
	//	// Create pagination options with filtering
	//	opts := &models.ListOptions{
	//	    Page: 1,
	//	    Limit: 10,
	//	    Filter: map[string]interface{}{
	//	        "assetCode": "USD", // Only show USD balances
	//	    },
	//	    Sort: []string{"available:desc"}, // Sort by available amount (descending)
	//	}
	//
	//	// List balances with pagination and filtering
	//	balances, err := balancesService.ListBalances(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    opts,
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to list balances: %v", err)
	//	}
	//
	//	// Process the balances
	//	fmt.Printf("Retrieved %d USD balances\n", len(balances.Items))
	ListBalances(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error)

	// ListAccountBalances retrieves a paginated list of all balances for a specific account.
	//
	// This method returns all balances for a single account within a ledger, with optional
	// filtering and pagination controls. Each balance represents a different asset held
	// by the account.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - orgID: The ID of the organization that owns the ledger. Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger containing the account. Must be a valid ledger ID.
	//   - accountID: The ID of the account to retrieve balances for. Must be a valid account ID.
	//   - opts: Optional pagination and filtering options:
	//     - Page: The page number to retrieve (1-based indexing)
	//     - Limit: The maximum number of items per page
	//     - Filter: Criteria to filter balances by (e.g., by asset code)
	//     - Sort: Sorting options for the results
	//     If nil, default pagination settings will be used.
	//
	// Returns:
	//   - *models.ListResponse[models.Balance]: A paginated list of balances for the account, including:
	//     - Items: The array of balance objects for the current page
	//     - Page: The current page number
	//     - Limit: The maximum number of items per page
	//     - Total: The total number of balances matching the filter criteria
	//   - error: An error if the operation fails. Possible errors include:
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization, ledger, or account ID)
	//     - Network or server errors
	//
	// Example - Basic usage:
	//
	//	// List all balances for an account with default pagination
	//	balances, err := balancesService.ListAccountBalances(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    "account-789",
	//	    nil, // Use default pagination
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to list account balances: %v", err)
	//	}
	//
	//	// Process the balances
	//	fmt.Printf("Account has %d different asset balances\n", len(balances.Items))
	//
	//	for _, balance := range balances.Items {
	//	    // Calculate the decimal value of the balance
	//	    decimalValue := float64(balance.Available) / math.Pow10(int(balance.Scale))
	//	    fmt.Printf("Asset: %s, Available: %.2f\n", balance.AssetCode, decimalValue)
	//	}
	//
	// Example - With filtering by asset code:
	//
	//	// Create pagination options with filtering for specific assets
	//	opts := &models.ListOptions{
	//	    Filter: map[string]interface{}{
	//	        "assetCode": []string{"USD", "EUR", "GBP"}, // Only show these currencies
	//	    },
	//	    Sort: []string{"assetCode:asc"}, // Sort alphabetically by asset code
	//	}
	//
	//	// List filtered balances for an account
	//	balances, err := balancesService.ListAccountBalances(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    "account-789",
	//	    opts,
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to list account balances: %v", err)
	//	}
	//
	//	// Process the balances
	//	fmt.Println("Currency balances for account:")
	//	for _, balance := range balances.Items {
	//	    decimalValue := float64(balance.Available) / math.Pow10(int(balance.Scale))
	//	    fmt.Printf("%s: %.2f\n", balance.AssetCode, decimalValue)
	//	}
	ListAccountBalances(ctx context.Context, orgID, ledgerID, accountID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error)

	// GetBalance retrieves a specific balance by its ID.
	// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to retrieve.
	// Returns the balance if found, or an error if the operation fails or the balance doesn't exist.
	GetBalance(ctx context.Context, orgID, ledgerID, balanceID string) (*models.Balance, error)

	// UpdateBalance updates an existing balance.
	// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to update.
	// The input parameter contains the balance details to update, such as amount or metadata.
	// Returns the updated balance, or an error if the operation fails.
	UpdateBalance(ctx context.Context, orgID, ledgerID, balanceID string, input *models.UpdateBalanceInput) (*models.Balance, error)

	// DeleteBalance deletes a balance.
	// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to delete.
	// Returns an error if the operation fails.
	DeleteBalance(ctx context.Context, orgID, ledgerID, balanceID string) error
}

// balancesEntity implements the BalancesService interface.
// It handles the communication with the Midaz API for balance-related operations.
type balancesEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewBalancesEntity creates a new balances entity.
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
//   - BalancesService: An implementation of the BalancesService interface that provides
//     methods for retrieving, updating, and managing account balances.
//
// Example:
//
//	// Create a balances entity with default HTTP client
//	balancesEntity := entities.NewBalancesEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to retrieve account balances
//	balances, err := balancesEntity.ListAccountBalances(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "account-789",
//	    nil, // No pagination options
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to retrieve balances: %v", err)
//	}
//
//	fmt.Printf("Retrieved %d balances\n", len(balances.Items))
func NewBalancesEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) BalancesService {
	return &balancesEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListBalances lists all balances for a ledger.
// The orgID and ledgerID parameters specify which organization and ledger to query.
// The opts parameter can be used to specify pagination, sorting, and filtering options.
// Returns a ListResponse containing the balances and pagination information, or an error if the operation fails.
func (e *balancesEntity) ListBalances(
	ctx context.Context,
	orgID,
	ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Balance], error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	url := e.buildURL(orgID, ledgerID, "")

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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var response models.ListResponse[models.Balance]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// ListAccountBalances lists all balances for a specific account.
// The orgID, ledgerID, and accountID parameters specify which organization, ledger, and account to query.
// The opts parameter can be used to specify pagination, sorting, and filtering options.
// Returns a ListResponse containing the account balances and pagination information, or an error if the operation fails.
func (e *balancesEntity) ListAccountBalances(
	ctx context.Context,
	orgID,
	ledgerID,
	accountID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Balance], error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	url := e.buildAccountURL(orgID, ledgerID, accountID)

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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var response models.ListResponse[models.Balance]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetBalance retrieves a balance by its ID.
// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to retrieve.
// Returns the balance if found, or an error if the operation fails or the balance doesn't exist.
func (e *balancesEntity) GetBalance(
	ctx context.Context,
	orgID,
	ledgerID,
	balanceID string,
) (*models.Balance, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if balanceID == "" {
		return nil, fmt.Errorf("balance ID is required")
	}

	url := e.buildURL(orgID, ledgerID, balanceID)

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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var balance models.Balance

	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &balance, nil
}

// UpdateBalance updates an existing balance.
// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to update.
// The input parameter contains the balance details to update, such as amount or metadata.
// Returns the updated balance, or an error if the operation fails.
func (e *balancesEntity) UpdateBalance(
	ctx context.Context,
	orgID,
	ledgerID,
	balanceID string,
	input *models.UpdateBalanceInput,
) (*models.Balance, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if balanceID == "" {
		return nil, fmt.Errorf("balance ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid balance update input: %v", err)
	}

	url := e.buildURL(orgID, ledgerID, balanceID)

	payload, err := json.Marshal(input)

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(payload))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var balance models.Balance

	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &balance, nil
}

// DeleteBalance deletes a balance.
// The orgID, ledgerID, and balanceID parameters specify which organization, ledger, and balance to delete.
// Returns an error if the operation fails.
func (e *balancesEntity) DeleteBalance(
	ctx context.Context,
	orgID,
	ledgerID,
	balanceID string,
) error {
	if orgID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if balanceID == "" {
		return fmt.Errorf("balance ID is required")
	}

	url := e.buildURL(orgID, ledgerID, balanceID)

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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return api.ErrorFromResponse(resp, respBody)
	}

	return nil
}

// buildURL builds the URL for balances API calls.
// The orgID and ledgerID parameters specify which organization and ledger to query.
// The balanceID parameter is the unique identifier of the balance to retrieve, or an empty string for a list of balances.
// Returns the built URL.
func (e *balancesEntity) buildURL(organizationID, ledgerID, balanceID string) string {
	base := e.baseURLs["onboarding"]
	if balanceID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/balances", base, organizationID, ledgerID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/balances/%s", base, organizationID, ledgerID, balanceID)
}

// buildAccountURL builds the URL for account balances API calls.
// The orgID, ledgerID, and accountID parameters specify which organization, ledger, and account to query.
// Returns the built URL for retrieving balances for a specific account.
func (e *balancesEntity) buildAccountURL(orgID, ledgerID, accountID string) string {
	base := e.baseURLs["onboarding"]
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/accounts/%s/balances", base, orgID, ledgerID, accountID)
}

// setCommonHeaders sets common headers for API requests.
// It sets the Content-Type header to application/json, the Authorization header with the client's auth token,
// and the User-Agent header with the client's user agent.
func (e *balancesEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
