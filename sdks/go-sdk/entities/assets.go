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
	// The organizationID and ledgerID parameters specify which organization and ledger to create the asset in.
	// The input parameter contains the asset details such as code, name, and precision.
	// Returns the created asset, or an error if the operation fails.
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
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the AssetsService interface.
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
// The organizationID and ledgerID parameters specify which organization and ledger to create the asset in.
// The input parameter contains the asset details such as code, name, and precision.
// Returns the created asset, or an error if the operation fails.
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
