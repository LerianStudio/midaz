package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AssetsService defines the interface for asset-related operations.
// It provides methods to create, read, update, and delete assets.
type AssetsService interface {
	// ListAssets retrieves a paginated list of assets for a ledger with optional filters.
	// The organizationID and ledgerID parameters specify which organization and ledger to query.
	// The opts parameter can be used to specify pagination, sorting, and filtering options.
	// Returns a ListResponse containing the assets and pagination information, or an error if the operation fails.
	ListAssets(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Asset], error)

	// GetAsset retrieves a specific asset by its ID.
	// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
	// The id parameter is the unique identifier of the asset to retrieve.
	// Returns the asset if found, or an error if the operation fails or the asset doesn't exist.
	GetAsset(ctx context.Context, organizationID, ledgerID, id string) (*models.Asset, error)

	// CreateAsset creates a new asset in the specified ledger.
	//
	// Assets represent units of value that can be tracked and transferred within the Midaz
	// ledger system. Each asset has a unique code and can be used in transactions.
	//
	// Parameters:
	//   - ctx: Context for the request, which can be used for cancellation and timeout.
	//   - organizationID: The ID of the organization that owns the ledger. Must be a valid organization ID.
	//   - ledgerID: The ID of the ledger where the asset will be created. Must be a valid ledger ID.
	//   - input: The asset details, including required fields:
	//     - Name: The human-readable name of the asset (e.g., "US Dollar")
	//     - Code: The unique asset code (e.g., "USD")
	//     Optional fields include:
	//     - Type: The asset type (e.g., "CURRENCY", "SECURITY", "COMMODITY")
	//     - Status: The initial status (defaults to ACTIVE if not specified)
	//     - Metadata: Additional custom information about the asset
	//
	// Returns:
	//   - *models.Asset: The created asset if successful, containing the asset ID,
	//     status, and other properties.
	//   - error: An error if the operation fails. Possible errors include:
	//     - Invalid input (missing required fields)
	//     - Authentication failure (invalid auth token)
	//     - Authorization failure (insufficient permissions)
	//     - Resource not found (invalid organization or ledger ID)
	//     - Conflict (asset code already exists)
	//     - Network or server errors
	//
	// Example - Creating a basic currency asset:
	//
	//	// Create a currency asset
	//	asset, err := assetsService.CreateAsset(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    &models.CreateAssetInput{
	//	        Name: "US Dollar",
	//	        Code: "USD",
	//	        Type: "CURRENCY",
	//	    },
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the asset
	//	fmt.Printf("Asset created: %s (code: %s)\n", asset.ID, asset.Code)
	//
	// Example - Creating an asset with metadata:
	//
	//	// Create a security asset with metadata
	//	asset, err := assetsService.CreateAsset(
	//	    context.Background(),
	//	    "org-123",
	//	    "ledger-456",
	//	    models.NewCreateAssetInput("Apple Inc. Stock", "AAPL").
	//	        WithType("SECURITY").
	//	        WithStatus(models.StatusActive).
	//	        WithMetadata(map[string]any{
	//	            "exchange": "NASDAQ",
	//	            "sector": "Technology",
	//	            "currency": "USD",
	//	            "isin": "US0378331005",
	//	        }),
	//	)
	//
	//	if err != nil {
	//	    // Handle error
	//	    return err
	//	}
	//
	//	// Use the asset
	//	fmt.Printf("Security asset created: %s\n", asset.ID)
	CreateAsset(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error)

	// UpdateAsset updates an existing asset.
	// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
	// The id parameter is the unique identifier of the asset to update.
	// The input parameter contains the asset details to update, such as name or status.
	// Returns the updated asset, or an error if the operation fails.
	UpdateAsset(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAssetInput) (*models.Asset, error)

	// DeleteAsset deletes an asset.
	// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
	// The id parameter is the unique identifier of the asset to delete.
	// Returns an error if the operation fails.
	DeleteAsset(ctx context.Context, organizationID, ledgerID, id string) error
}

// assetsEntity implements the AssetsService interface.
// It handles the communication with the Midaz API for asset-related operations.
type assetsEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewAssetsEntity creates a new assets entity.
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
//   - AssetsService: An implementation of the AssetsService interface that provides
//     methods for creating, retrieving, updating, and managing assets.
//
// Example:
//
//	// Create an assets entity with default HTTP client
//	assetsEntity := entities.NewAssetsEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to create an asset
//	asset, err := assetsEntity.CreateAsset(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateAssetInput{
//	        Name: "US Dollar",
//	        Code: "USD",
//	        Type: "CURRENCY",
//	    },
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create asset: %v", err)
//	}
//
//	fmt.Printf("Asset created: %s\n", asset.ID)
func NewAssetsEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) AssetsService {
	return &assetsEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// ListAssets lists assets for a ledger with optional filters.
// The organizationID and ledgerID parameters specify which organization and ledger to query.
// The opts parameter can be used to specify pagination, sorting, and filtering options.
// Returns a ListResponse containing the assets and pagination information, or an error if the operation fails.
func (e *assetsEntity) ListAssets(
	ctx context.Context,
	organizationID, ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Asset], error) {
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

	var response models.ListResponse[models.Asset]

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetAsset gets an asset by ID.
// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
// The id parameter is the unique identifier of the asset to retrieve.
// Returns the asset if found, or an error if the operation fails or the asset doesn't exist.
func (e *assetsEntity) GetAsset(
	ctx context.Context,
	organizationID, ledgerID, id string,
) (*models.Asset, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("asset ID is required")
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

	var asset models.Asset

	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &asset, nil
}

// CreateAsset creates a new asset in the specified ledger.
//
// Assets represent units of value that can be tracked and transferred within the Midaz
// ledger system. Each asset has a unique code and can be used in transactions.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the asset will be created. Must be a valid ledger ID.
//   - input: The asset details, including required fields:
//   - Name: The human-readable name of the asset (e.g., "US Dollar")
//   - Code: The unique asset code (e.g., "USD")
//     Optional fields include:
//   - Type: The asset type (e.g., "CURRENCY", "SECURITY", "COMMODITY")
//   - Status: The initial status (defaults to ACTIVE if not specified)
//   - Metadata: Additional custom information about the asset
//
// Returns:
//   - *models.Asset: The created asset if successful, containing the asset ID,
//     status, and other properties.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization or ledger ID)
//   - Conflict (asset code already exists)
//   - Network or server errors
//
// Example - Creating a basic currency asset:
//
//	// Create a currency asset
//	asset, err := assetsService.CreateAsset(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.CreateAssetInput{
//	        Name: "US Dollar",
//	        Code: "USD",
//	        Type: "CURRENCY",
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset
//	fmt.Printf("Asset created: %s (code: %s)\n", asset.ID, asset.Code)
//
// Example - Creating an asset with metadata:
//
//	// Create a security asset with metadata
//	asset, err := assetsService.CreateAsset(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    models.NewCreateAssetInput("Apple Inc. Stock", "AAPL").
//	        WithType("SECURITY").
//	        WithStatus(models.StatusActive).
//	        WithMetadata(map[string]any{
//	            "exchange": "NASDAQ",
//	            "sector": "Technology",
//	            "currency": "USD",
//	            "isin": "US0378331005",
//	        }),
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset
//	fmt.Printf("Security asset created: %s\n", asset.ID)
func (e *assetsEntity) CreateAsset(
	ctx context.Context,
	organizationID, ledgerID string,
	input *models.CreateAssetInput,
) (*models.Asset, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("asset input cannot be nil")
	}

	// Validate the input before making the API call
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset input: %v", err)
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

	var asset models.Asset

	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &asset, nil
}

// UpdateAsset updates an existing asset.
// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
// The id parameter is the unique identifier of the asset to update.
// The input parameter contains the asset details to update, such as name or status.
// Returns the updated asset, or an error if the operation fails.
func (e *assetsEntity) UpdateAsset(
	ctx context.Context,
	organizationID, ledgerID, id string,
	input *models.UpdateAssetInput,
) (*models.Asset, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return nil, fmt.Errorf("asset ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("asset input cannot be nil")
	}

	// Validate the input before making the API call
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset update input: %v", err)
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

	var asset models.Asset

	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &asset, nil
}

// DeleteAsset deletes an asset.
// The organizationID and ledgerID parameters specify which organization and ledger the asset belongs to.
// The id parameter is the unique identifier of the asset to delete.
// Returns an error if the operation fails.
func (e *assetsEntity) DeleteAsset(
	ctx context.Context,
	organizationID, ledgerID, id string,
) error {
	if organizationID == "" {
		return fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return fmt.Errorf("ledger ID is required")
	}

	if id == "" {
		return fmt.Errorf("asset ID is required")
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

// buildURL builds the URL for assets API calls.
// The organizationID and ledgerID parameters specify which organization and ledger to query.
// The assetID parameter is the unique identifier of the asset to retrieve, or an empty string for a list of assets.
// Returns the built URL.
func (e *assetsEntity) buildURL(organizationID, ledgerID, assetID string) string {
	base := e.baseURLs["onboarding"]
	if assetID == "" {
		return fmt.Sprintf("%s/organizations/%s/ledgers/%s/assets", base, organizationID, ledgerID)
	}
	return fmt.Sprintf("%s/organizations/%s/ledgers/%s/assets/%s", base, organizationID, ledgerID, assetID)
}

// setCommonHeaders sets common headers for API requests.
// It sets the Content-Type header to application/json, the Authorization header with the client's auth token,
// and the User-Agent header with the client's user agent.
func (e *assetsEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
