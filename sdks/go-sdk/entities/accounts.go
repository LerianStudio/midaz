package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AccountsService defines the interface for account-related operations.
// It provides methods to create, read, update, and delete accounts,
// as well as manage account balances.
type AccountsService interface {
	// ListAccounts retrieves a paginated list of accounts for a ledger with optional filters.
	// The organizationID and ledgerID parameters specify which organization and ledger to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the accounts and pagination information, or an error if the operation fails.
	ListAccounts(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Account], error)

	// GetAccount retrieves a specific account by its ID.
	// The organizationID and ledgerID parameters specify which organization and ledger the account belongs to.
	// The id parameter is the unique identifier of the account to retrieve.
	// Returns the account if found, or an error if the operation fails or the account doesn't exist.
	GetAccount(ctx context.Context, organizationID, ledgerID, id string) (*models.Account, error)

	// GetAccountByAlias retrieves a specific account by its alias.
	// The organizationID and ledgerID parameters specify which organization and ledger the account belongs to.
	// The alias parameter is the unique alias of the account to retrieve.
	// Returns the account if found, or an error if the operation fails or the account doesn't exist.
	GetAccountByAlias(ctx context.Context, organizationID, ledgerID, alias string) (*models.Account, error)

	// CreateAccount creates a new account in the specified ledger.
	//
	// This method creates a new account in the specified organization and ledger.
	// Accounts are used to track assets and balances within the Midaz system.
	// Each account has a type, name, and can be associated with a specific asset code.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - organizationID: The ID of the organization that owns the ledger. Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger where the account will be created. Must be a valid ledger ID.
	//   - input: The account details, including name, type, asset code, and optional fields.
	//     Required fields in the input are:
	//     - Name: The human-readable name of the account (max 256 characters)
	//     - Type: The account type (e.g., "customer", "revenue", "liability")
	//     - AssetCode: The currency or asset code (e.g., "USD", "EUR") if applicable
	//
	// Returns:
	//   - *models.Account: The created account if successful, containing the account ID,
	//     status, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization or ledger ID)
	//     - Network or server errors
	//
	// Example - Creating a basic customer account:
	//
	//	// Create a customer account
	//	account, err := accountsService.CreateAccount(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    &models.CreateAccountInput{
	//	        Name: "John Doe",
	//	        Type: "customer",
	//	        AssetCode: "USD",
	//	        Metadata: map[string]interface{}{
	//	            "customer_id": "cust-789",
	//	            "email": "john.doe@example.com",
	//	        },
	//	    },
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the account
	//	fmt.Printf("Account created: %s (alias: %s)\n", account.ID, account.Alias)
	//
	// Example - Creating an account with portfolio and segment:
	//
	//	// Create an account within a portfolio and segment
	//	account, err := accountsService.CreateAccount(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    &models.CreateAccountInput{
	//	        Name: "Investment Account",
	//	        Type: "investment",
	//	        AssetCode: "USD",
	//	        PortfolioID: "portfolio-789",
	//	        SegmentID: "segment-012",
	//	        Status: models.StatusActive,
	//	    },
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the account
	//	fmt.Printf("Account created: %s (status: %s)\n", account.ID, account.Status)
	CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error)

	// UpdateAccount updates an existing account.
	// The organizationID and ledgerID parameters specify which organization and ledger the account belongs to.
	// The id parameter is the unique identifier of the account to update.
	// The input parameter contains the account details to update, such as name or status.
	// Returns the updated account, or an error if the operation fails.
	UpdateAccount(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAccountInput) (*models.Account, error)

	// DeleteAccount deletes an account.
	// The organizationID and ledgerID parameters specify which organization and ledger the account belongs to.
	// The id parameter is the unique identifier of the account to delete.
	// Returns an error if the operation fails.
	DeleteAccount(ctx context.Context, organizationID, ledgerID, id string) error

	// GetBalance retrieves the balance for a specific account.
	// The organizationID and ledgerID parameters specify which organization and ledger the account belongs to.
	// The accountID parameter is the unique identifier of the account to get the balance for.
	// Returns the balance information, or an error if the operation fails.
	GetBalance(ctx context.Context, organizationID, ledgerID, accountID string) (*models.Balance, error)
}

// accountsEntity implements the AccountsService interface.
// It handles the communication with the Midaz API for account-related operations.
type accountsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewAccountsEntity creates a new accounts entity.
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
//   - AccountsService: An implementation of the AccountsService interface that provides
//     methods for creating, retrieving, updating, and managing accounts.
//
// Example:
//
//	// Create an accounts entity with default HTTP client
//	accountsEntity := entities.NewAccountsEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to create an account
//	account, err := accountsEntity.CreateAccount(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateAccountInput{
//	        Name: "Customer Account",
//	        Type: "customer",
//	        AssetCode: "USD",
//	    },
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create account: %v", err)
//	}
//
//	fmt.Printf("Account created: %s\n", account.ID)
func NewAccountsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) AccountsService {
	return &accountsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListAccounts lists accounts for a ledger with optional filters.
func (e *accountsEntity) ListAccounts(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Account], error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, "")

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

	var response models.ListResponse[models.Account]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetAccount gets an account by ID.
func (e *accountsEntity) GetAccount(ctx context.Context, organizationID, ledgerID, id string) (*models.Account, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, id)

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

	var account models.Account

	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &account, nil
}

// GetAccountByAlias gets an account by alias.
func (e *accountsEntity) GetAccountByAlias(ctx context.Context, organizationID, ledgerID, alias string) (*models.Account, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if alias == "" {
		return nil, fmt.Errorf("account alias is required")
	}

	url := e.buildURL(organizationID, ledgerID, "alias/"+alias)

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

	var account models.Account

	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &account, nil
}

// CreateAccount creates a new account in the specified ledger.
//
// This method creates a new account in the specified organization and ledger.
// Accounts are used to track assets and balances within the Midaz system.
// Each account has a type, name, and can be associated with a specific asset code.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the account will be created. Must be a valid ledger ID.
//   - input: The account details, including name, type, asset code, and optional fields.
//     Required fields in the input are:
//   - Name: The human-readable name of the account (max 256 characters)
//   - Type: The account type (e.g., "customer", "revenue", "liability")
//   - AssetCode: The currency or asset code (e.g., "USD", "EUR") if applicable
//
// Returns:
//   - *models.Account: The created account if successful, containing the account ID,
//     status, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Network or server errors
//
// Example - Creating a basic customer account:
//
//	// Create a customer account
//	account, err := accountsService.CreateAccount(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateAccountInput{
//	        Name: "John Doe",
//	        Type: "customer",
//	        AssetCode: "USD",
//	        Metadata: map[string]interface{}{
//	            "customer_id": "cust-789",
//	            "email": "john.doe@example.com",
//	        },
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the account
//	fmt.Printf("Account created: %s (alias: %s)\n", account.ID, account.Alias)
//
// Example - Creating an account with portfolio and segment:
//
//	// Create an account within a portfolio and segment
//	account, err := accountsService.CreateAccount(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateAccountInput{
//	        Name: "Investment Account",
//	        Type: "investment",
//	        AssetCode: "USD",
//	        PortfolioID: "portfolio-789",
//	        SegmentID: "segment-012",
//	        Status: models.StatusActive,
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the account
//	fmt.Printf("Account created: %s (status: %s)\n", account.ID, account.Status)
func (e *accountsEntity) CreateAccount(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("account input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account input: %v", err)
	}

	url := e.buildURL(organizationID, ledgerID, "")

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

	var account models.Account

	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &account, nil
}

// UpdateAccount updates an existing account.
func (e *accountsEntity) UpdateAccount(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAccountInput) (*models.Account, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("account input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account update input: %v", err)
	}

	url := e.buildURL(organizationID, ledgerID, id)

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

	var account models.Account

	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &account, nil
}

// DeleteAccount deletes an account.
func (e *accountsEntity) DeleteAccount(ctx context.Context, organizationID, ledgerID, id string) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return fmt.Errorf("account ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, id)

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

// GetBalance gets an account's balance.
func (e *accountsEntity) GetBalance(ctx context.Context, organizationID, ledgerID, accountID string) (*models.Balance, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	// First get the account details to get the alias
	account, err := e.GetAccount(ctx, organizationID, ledgerID, accountID)

	if err != nil {
		return nil, err
	}

	if account.Alias == nil || *account.Alias == "" {
		return nil, fmt.Errorf("account has no alias")
	}

	// Build URL with balance endpoint using alias instead of ID
	base := e.baseURLs["transaction"]
	urlPath := path.Join("v1", "organizations", organizationID, "ledgers", ledgerID, "balances")

	url := fmt.Sprintf("%s/%s?account=%s", base, urlPath, *account.Alias)

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

	var balance models.Balance

	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &balance, nil
}

// buildURL builds the URL for accounts API calls.
func (e *accountsEntity) buildURL(organizationID, ledgerID, accountID string) string {
	base := e.baseURLs["onboarding"]
	if accountID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/accounts", base, organizationID, ledgerID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/accounts/%s", base, organizationID, ledgerID, accountID)
}

// setCommonHeaders sets common headers for API requests.
func (e *accountsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
