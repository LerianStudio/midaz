package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// BalancesService defines the interface for balance-related operations.
// It provides methods to list, retrieve, update, and delete balances
// for both ledgers and specific accounts.
type BalancesService interface {
	// ListBalances retrieves a paginated list of all balances for a ledger.
	// The orgID and ledgerID parameters specify which organization and ledger to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the balances and pagination information, or an error if the operation fails.
	ListBalances(ctx context.Context, orgID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error)

	// ListAccountBalances retrieves a paginated list of all balances for a specific account.
	// The orgID, ledgerID, and accountID parameters specify which organization, ledger, and account to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the account balances and pagination information, or an error if the operation fails.
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
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the BalancesService interface.
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(payload))

	if err != nil {
		return nil, fmt.Errorf("internal error: %w", err)
	}

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

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
