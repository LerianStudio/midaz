package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// PortfoliosService defines the interface for portfolio-related operations.
// It provides methods to create, read, update, and delete portfolios
// within a ledger and organization.
type PortfoliosService interface {
	// ListPortfolios retrieves a paginated list of portfolios for a ledger with optional filters.
	// The organizationID and ledgerID parameters specify which organization and ledger to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the portfolios and pagination information, or an error if the operation fails.
	ListPortfolios(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Portfolio], error)

	// GetPortfolio retrieves a specific portfolio by its ID.
	// The organizationID and ledgerID parameters specify which organization and ledger the portfolio belongs to.
	// The id parameter is the unique identifier of the portfolio to retrieve.
	// Returns the portfolio if found, or an error if the operation fails or the portfolio doesn't exist.
	GetPortfolio(ctx context.Context, organizationID, ledgerID, id string) (*models.Portfolio, error)

	// CreatePortfolio creates a new portfolio in the specified ledger.
	//
	// Portfolios are collections of accounts that belong to a specific entity within
	// an organization and ledger. They help organize accounts for better management
	// and reporting.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - organizationID: The ID of the organization where the portfolio will be created.
	//     Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger where the portfolio will be created.
	//     Must be a valid ledger ID within the specified organization.
	//   - input: The portfolio details, including required fields:
	//     - EntityID: The ID of the entity that will own this portfolio
	//     - Name: The human-readable name of the portfolio
	//     Optional fields include:
	//     - Status: The initial status (defaults to ACTIVE if not specified)
	//     - Metadata: Additional custom information about the portfolio
	//
	// Returns:
	//   - *models.Portfolio: The created portfolio if successful, containing the portfolio ID,
	//     name, entity ID, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields or invalid values)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization or ledger ID)
	//     - Network or server errors
	//
	// Example - Creating a basic portfolio:
	//
	//	// Create a simple portfolio with just required fields
	//	portfolio, err := portfoliosService.CreatePortfolio(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    models.NewCreatePortfolioInput(
	//	        "entity-789",
	//	        "Investment Portfolio",
	//	    ),
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create portfolio: %v", err)
	//	}
	//
	//	// Use the portfolio
	//	fmt.Printf("Portfolio created: %s (status: %s)\n",
	//	    portfolio.ID, portfolio.Status.Code)
	//
	// Example - Creating a portfolio with metadata:
	//
	//	// Create a portfolio with custom metadata
	//	input := models.NewCreatePortfolioInput(
	//	    "entity-789",
	//	    "Retirement Portfolio",
	//	).WithStatus(
	//	    models.StatusActive,
	//	).WithMetadata(
	//	    map[string]any{
	//	        "portfolioType": "retirement",
	//	        "riskProfile": "moderate",
	//	        "targetYear": 2045,
	//	        "manager": "Jane Smith",
	//	    },
	//	)
	//
	//	portfolio, err := portfoliosService.CreatePortfolio(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    input,
	//	)
	//
	//	if err != nil {
	//	    log.Fatalf("Failed to create portfolio: %v", err)
	//	}
	//
	//	// Use the portfolio
	//	fmt.Printf("Portfolio created: %s\n", portfolio.ID)
	//	fmt.Printf("Name: %s\n", portfolio.Name)
	//	fmt.Printf("Entity: %s\n", portfolio.EntityID)
	CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error)

	// UpdatePortfolio updates an existing portfolio.
	// The organizationID and ledgerID parameters specify which organization and ledger the portfolio belongs to.
	// The id parameter is the unique identifier of the portfolio to update.
	// The input parameter contains the portfolio details to update, such as name, description, or status.
	// Returns the updated portfolio, or an error if the operation fails.
	UpdatePortfolio(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdatePortfolioInput) (*models.Portfolio, error)

	// DeletePortfolio deletes a portfolio.
	// The organizationID and ledgerID parameters specify which organization and ledger the portfolio belongs to.
	// The id parameter is the unique identifier of the portfolio to delete.
	// Returns an error if the operation fails.
	DeletePortfolio(ctx context.Context, organizationID, ledgerID, id string) error
}

// portfoliosEntity implements the PortfoliosService interface.
// It handles the communication with the Midaz API for portfolio-related operations.
type portfoliosEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewPortfoliosEntity creates a new portfolios entity.
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
//   - PortfoliosService: An implementation of the PortfoliosService interface that provides
//     methods for creating, retrieving, updating, and managing portfolios.
//
// Example:
//
//	// Create a portfolios entity with default HTTP client
//	portfoliosEntity := entities.NewPortfoliosEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to retrieve portfolios
//	portfolios, err := portfoliosEntity.ListPortfolios(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    nil, // No pagination options
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to retrieve portfolios: %v", err)
//	}
//
//	fmt.Printf("Retrieved %d portfolios\n", len(portfolios.Items))
func NewPortfoliosEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) PortfoliosService {
	return &portfoliosEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListPortfolios lists portfolios for a ledger with optional filters.
func (e *portfoliosEntity) ListPortfolios(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Portfolio], error) {
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

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var response models.ListResponse[models.Portfolio]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetPortfolio gets a portfolio by ID.
func (e *portfoliosEntity) GetPortfolio(ctx context.Context, organizationID, ledgerID, id string) (*models.Portfolio, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, id)

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

	var portfolio models.Portfolio

	if err := json.NewDecoder(resp.Body).Decode(&portfolio); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &portfolio, nil
}

// CreatePortfolio creates a new portfolio in the specified ledger.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization where the portfolio will be created.
//     Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the portfolio will be created.
//     Must be a valid ledger ID within the specified organization.
//   - input: The portfolio details, including required fields:
//   - EntityID: The ID of the entity that will own this portfolio
//   - Name: The human-readable name of the portfolio
//     Optional fields include:
//   - Status: The initial status (defaults to ACTIVE if not specified)
//   - Metadata: Additional custom information about the portfolio
//
// Returns:
//   - *models.Portfolio: The created portfolio if successful, containing the portfolio ID,
//     name, entity ID, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Network or server errors
//
// Example - Creating a basic portfolio:
//
//	// Create a simple portfolio with just required fields
//	portfolio, err := portfoliosService.CreatePortfolio(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    models.NewCreatePortfolioInput(
//	        "entity-789",
//	        "Investment Portfolio",
//	    ),
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create portfolio: %v", err)
//	}
//
//	// Use the portfolio
//	fmt.Printf("Portfolio created: %s (status: %s)\n",
//	    portfolio.ID, portfolio.Status.Code)
//
// Example - Creating a portfolio with metadata:
//
//	// Create a portfolio with custom metadata
//	input := models.NewCreatePortfolioInput(
//	    "entity-789",
//	    "Retirement Portfolio",
//	).WithStatus(
//	    models.StatusActive,
//	).WithMetadata(
//	    map[string]any{
//	        "portfolioType": "retirement",
//	        "riskProfile": "moderate",
//	        "targetYear": 2045,
//	        "manager": "Jane Smith",
//	    },
//	)
//
//	portfolio, err := portfoliosService.CreatePortfolio(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    input,
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create portfolio: %v", err)
//	}
//
//	// Use the portfolio
//	fmt.Printf("Portfolio created: %s\n", portfolio.ID)
//	fmt.Printf("Name: %s\n", portfolio.Name)
//	fmt.Printf("Entity: %s\n", portfolio.EntityID)
func (e *portfoliosEntity) CreatePortfolio(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("portfolio input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid portfolio input: %v", err)
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

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var portfolio models.Portfolio

	if err := json.NewDecoder(resp.Body).Decode(&portfolio); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &portfolio, nil
}

// UpdatePortfolio updates an existing portfolio.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization where the portfolio is located.
//     Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the portfolio is located.
//     Must be a valid ledger ID within the specified organization.
//   - id: The ID of the portfolio to update.
//   - input: The updated portfolio details, including optional fields:
//   - Name: The human-readable name of the portfolio
//   - Status: The updated status
//   - Metadata: Additional custom information about the portfolio
//
// Returns:
//   - *models.Portfolio: The updated portfolio if successful, containing the portfolio ID,
//     name, entity ID, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Network or server errors
//
// Example - Updating a portfolio:
//
//	// Update a portfolio with new name and status
//	input := models.NewUpdatePortfolioInput(
//	    "New Portfolio Name",
//	).WithStatus(
//	    models.StatusInactive,
//	)
//
//	portfolio, err := portfoliosService.UpdatePortfolio(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    "portfolio-789",
//	    input,
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to update portfolio: %v", err)
//	}
//
//	// Use the updated portfolio
//	fmt.Printf("Portfolio updated: %s (status: %s)\n",
//	    portfolio.ID, portfolio.Status.Code)
func (e *portfoliosEntity) UpdatePortfolio(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdatePortfolioInput) (*models.Portfolio, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("portfolio ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("portfolio input cannot be nil")
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid portfolio update input: %v", err)
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

	e.setCommonHeaders(req)

	resp, err := e.httpClient.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var portfolio models.Portfolio

	if err := json.NewDecoder(resp.Body).Decode(&portfolio); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &portfolio, nil
}

// DeletePortfolio deletes a portfolio.
func (e *portfoliosEntity) DeletePortfolio(ctx context.Context, organizationID, ledgerID, id string) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return fmt.Errorf("portfolio ID is required")
	}

	url := e.buildURL(organizationID, ledgerID, id)

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

// buildURL builds the URL for portfolios API calls.
func (e *portfoliosEntity) buildURL(organizationID, ledgerID, portfolioID string) string {
	base := e.baseURLs["onboarding"]
	if portfolioID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/portfolios", base, organizationID, ledgerID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/portfolios/%s", base, organizationID, ledgerID, portfolioID)
}

// setCommonHeaders sets common headers for API requests.
func (e *portfoliosEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
