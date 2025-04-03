package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AssetRatesService defines the interface for asset rate-related operations.
// It provides methods to manage exchange rates between different assets.
type AssetRatesService interface {
	// GetAssetRate retrieves the exchange rate between two assets.
	// The organizationID and ledgerID parameters specify which organization and ledger the assets belong to.
	// The sourceAssetCode and destinationAssetCode parameters specify the assets for which to get the exchange rate.
	// Returns the asset rate if found, or an error if the operation fails or the rate doesn't exist.
	GetAssetRate(ctx context.Context, organizationID, ledgerID, sourceAssetCode, destinationAssetCode string) (*models.AssetRate, error)

	// CreateOrUpdateAssetRate creates a new asset rate or updates an existing one.
	// The organizationID and ledgerID parameters specify which organization and ledger to create/update the asset rate in.
	// The input parameter contains the asset rate details such as source asset, destination asset, and rate.
	// Returns the created or updated asset rate, or an error if the operation fails.
	CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error)
}

// assetRatesEntity implements the AssetRatesService interface.
// It handles the communication with the Midaz API for asset rate-related operations.
type assetRatesEntity struct {
	httpClient *http.Client
	authToken  string
	baseURLs   map[string]string
}

// NewAssetRatesEntity creates a new asset rates entity.
// It takes the HTTP client, auth token, and base URLs which are used to make HTTP requests to the Midaz API.
// Returns an implementation of the AssetRatesService interface.
func NewAssetRatesEntity(httpClient *http.Client, authToken string, baseURLs map[string]string) AssetRatesService {
	return &assetRatesEntity{
		httpClient: httpClient,
		authToken:  authToken,
		baseURLs:   baseURLs,
	}
}

// GetAssetRate gets an asset rate by source and destination asset codes.
func (e *assetRatesEntity) GetAssetRate(ctx context.Context, organizationID, ledgerID, sourceAssetCode, destinationAssetCode string) (*models.AssetRate, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if sourceAssetCode == "" {
		return nil, fmt.Errorf("source asset code is required")
	}

	if destinationAssetCode == "" {
		return nil, fmt.Errorf("destination asset code is required")
	}

	url := e.buildURL(organizationID, ledgerID, fmt.Sprintf("?source=%s&destination=%s", sourceAssetCode, destinationAssetCode))

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

	var assetRate models.AssetRate

	if err := json.NewDecoder(resp.Body).Decode(&assetRate); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &assetRate, nil
}

// CreateOrUpdateAssetRate creates or updates an asset rate.
func (e *assetRatesEntity) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if input == nil {
		return nil, fmt.Errorf("asset rate input cannot be nil")
	}

	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset rate input: %v", err)
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

	var assetRate models.AssetRate

	if err := json.NewDecoder(resp.Body).Decode(&assetRate); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &assetRate, nil
}

// buildURL builds the URL for asset rates API calls.
func (e *assetRatesEntity) buildURL(organizationID, ledgerID, query string) string {
	base := e.baseURLs["onboarding"]
	url := fmt.Sprintf("%s/organizations/%s/ledgers/%s/assets/rates", base, organizationID, ledgerID)
	if query != "" {
		url += query
	}
	return url
}

// setCommonHeaders sets common headers for API requests.
func (e *assetRatesEntity) setCommonHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e.authToken))
}
