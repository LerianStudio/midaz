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
//   - AssetRatesService: An implementation of the AssetRatesService interface that provides
//     methods for creating, retrieving, and managing asset exchange rates.
//
// Example:
//
//	// Create an asset rates entity with default HTTP client
//	assetRatesEntity := entities.NewAssetRatesEntity(
//	    &http.Client{Timeout: 30 * time.Second},
//	    "your-auth-token",
//	    map[string]string{"onboarding": "https://api.midaz.io/v1"},
//	)
//
//	// Use the entity to create or update an exchange rate
//	rate, err := assetRatesEntity.CreateOrUpdateAssetRate(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.UpdateAssetRateInput{
//	        FromAsset: "USD",
//	        ToAsset: "EUR",
//	        Rate: 0.92,
//	        EffectiveAt: time.Now(),
//	        ExpirationAt: time.Now().Add(24 * time.Hour),
//	    },
//	)
//
//	if err != nil {
//	    log.Fatalf("Failed to create asset rate: %v", err)
//	}
//
//	fmt.Printf("Asset rate created: %s (rate: %.4f)\n", rate.ID, rate.Rate)
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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

	var assetRate models.AssetRate

	if err := json.NewDecoder(resp.Body).Decode(&assetRate); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &assetRate, nil
}

// CreateOrUpdateAssetRate creates or updates an asset rate.
//
// Asset rates define the exchange ratio between two assets within the same ledger.
// If a rate already exists for the specified asset pair, it will be updated; otherwise,
// a new rate will be created.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//   - organizationID: The ID of the organization that owns the ledger. Must be a valid organization ID.
//   - ledgerID: The ID of the ledger where the asset rate will be created/updated. Must be a valid ledger ID.
//   - input: The asset rate details, including:
//   - FromAsset: The source asset code (e.g., "USD")
//   - ToAsset: The target asset code (e.g., "EUR")
//   - Rate: The exchange rate value (e.g., 0.92 means 1 unit of FromAsset = 0.92 units of ToAsset)
//   - EffectiveAt: The timestamp when the rate becomes effective
//   - ExpirationAt: The timestamp when the rate expires
//
// Returns:
//   - *models.AssetRate: The created or updated asset rate if successful, containing the rate ID,
//     source and target assets, rate value, and effective/expiration dates.
//   - error: An error if the operation fails. Possible errors include:
//   - Invalid input (missing required fields or invalid values)
//   - Authentication failure (invalid auth token)
//   - Authorization failure (insufficient permissions)
//   - Resource not found (invalid organization, ledger, or asset codes)
//   - Network or server errors
//
// Example - Creating a new currency exchange rate:
//
//	// Create an exchange rate between USD and EUR
//	rate, err := assetRatesService.CreateOrUpdateAssetRate(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    models.NewUpdateAssetRateInput(
//	        "USD",
//	        "EUR",
//	        0.92,
//	        time.Now(),
//	        time.Now().Add(24 * time.Hour),
//	    ),
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset rate
//	fmt.Printf("Exchange rate created: %s (1 %s = %.4f %s)\n",
//	    rate.ID, rate.FromAsset, rate.Rate, rate.ToAsset)
//
// Example - Updating an existing exchange rate:
//
//	// Update the USD to EUR exchange rate
//	tomorrow := time.Now().Add(24 * time.Hour)
//	nextWeek := time.Now().Add(7 * 24 * time.Hour)
//
//	rate, err := assetRatesService.CreateOrUpdateAssetRate(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.UpdateAssetRateInput{
//	        FromAsset: "USD",
//	        ToAsset: "EUR",
//	        Rate: 0.93, // Updated rate
//	        EffectiveAt: tomorrow,
//	        ExpirationAt: nextWeek,
//	    },
//	)
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the updated asset rate
//	fmt.Printf("Exchange rate updated: %s (new rate: %.4f, effective: %s)\n",
//	    rate.ID, rate.Rate, rate.EffectiveAt.Format(time.RFC3339))
//
// Example - Creating multiple rates for different time periods:
//
//	// Create current rate
//	now := time.Now()
//	tomorrow := now.Add(24 * time.Hour)
//
//	currentRate, err := assetRatesService.CreateOrUpdateAssetRate(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.UpdateAssetRateInput{
//	        FromAsset: "USD",
//	        ToAsset: "EUR",
//	        Rate: 0.92,
//	        EffectiveAt: now,
//	        ExpirationAt: tomorrow,
//	    },
//	)
//
//	if err != nil {
//	    return err
//	}
//
//	// Create future rate (effective tomorrow)
//	nextWeek := tomorrow.Add(6 * 24 * time.Hour)
//
//	futureRate, err := assetRatesService.CreateOrUpdateAssetRate(
//	    context.Background(),
//	    "org-123",
//	    "ledger-456",
//	    &models.UpdateAssetRateInput{
//	        FromAsset: "USD",
//	        ToAsset: "EUR",
//	        Rate: 0.94, // Expected future rate
//	        EffectiveAt: tomorrow,
//	        ExpirationAt: nextWeek,
//	    },
//	)
//
//	if err != nil {
//	    return err
//	}
//
//	fmt.Printf("Created current rate (%.4f) and future rate (%.4f)\n",
//	    currentRate.Rate, futureRate.Rate)
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

	// Check if the response status code indicates an error
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, api.ErrorFromResponse(resp, respBody)
	}

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
