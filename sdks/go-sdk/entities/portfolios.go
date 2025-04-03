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
	// The organizationID and ledgerID parameters specify which organization and ledger to create the portfolio in.
	// The input parameter contains the portfolio details such as name and description.
	// Returns the created portfolio, or an error if the operation fails.
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
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the PortfoliosService interface.
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
