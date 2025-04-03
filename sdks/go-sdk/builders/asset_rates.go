// Package builders provides fluent builder interfaces for the Midaz SDK.
// It implements the builder pattern to simplify the creation and manipulation
// of Midaz resources through a chainable API.
package builders

import (
	"context"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// AssetRateClientInterface defines the minimal client interface required by the asset rate builders.
// Implementations of this interface are responsible for the actual API communication
// with the Midaz backend services for asset rate operations.
type AssetRateClientInterface interface {
	// CreateOrUpdateAssetRate sends a request to create or update an asset rate with the specified parameters.
	// If an asset rate with the same from/to asset pair already exists, it will be updated.
	// Otherwise, a new asset rate will be created.
	// It requires a context for the API request.
	// Returns an error if the API request fails.
	CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID string, input *models.UpdateAssetRateInput) (*models.AssetRate, error)
}

// AssetRateBuilder defines the builder interface for creating or updating asset rates.
// Asset rates define the exchange rate between two assets within a ledger.
// This builder provides a fluent API for configuring and creating/updating asset rate resources.
type AssetRateBuilder interface {
	// WithOrganization sets the organization ID for the asset rate.
	// This is a required field for asset rate creation/update.
	WithOrganization(orgID string) AssetRateBuilder

	// WithLedger sets the ledger ID for the asset rate.
	// This is a required field for asset rate creation/update.
	WithLedger(ledgerID string) AssetRateBuilder

	// WithFromAsset sets the source asset code for the asset rate.
	// This is a required field that specifies the asset being converted from.
	WithFromAsset(assetCode string) AssetRateBuilder

	// WithToAsset sets the destination asset code for the asset rate.
	// This is a required field that specifies the asset being converted to.
	WithToAsset(assetCode string) AssetRateBuilder

	// WithRate sets the exchange rate for the asset rate.
	// This is a required field that specifies the conversion rate from the source asset to the destination asset.
	// For example, if 1 USD = 0.85 EUR, the rate would be 0.85.
	WithRate(rate float64) AssetRateBuilder

	// WithEffectiveAt sets the effective date for the asset rate.
	// This is an optional field that specifies when the rate becomes active.
	// If not specified, the current time will be used.
	WithEffectiveAt(effectiveAt time.Time) AssetRateBuilder

	// WithExpirationAt sets the expiration date for the asset rate.
	// This is an optional field that specifies when the rate expires.
	// If not specified, the rate will expire 30 days from the effective date.
	WithExpirationAt(expirationAt time.Time) AssetRateBuilder

	// CreateOrUpdate executes the asset rate creation or update and returns the created or updated asset rate.
	// It requires a context for the API request.
	// Returns an error if the required fields are not set or if the API request fails.
	CreateOrUpdate(ctx context.Context) (*models.AssetRate, error)
}

// assetRateBuilder implements the AssetRateBuilder interface.
type assetRateBuilder struct {
	client         AssetRateClientInterface
	organizationID string
	ledgerID       string
	fromAsset      string
	toAsset        string
	rate           float64
	effectiveAt    time.Time
	expirationAt   time.Time
	isRateSet      bool
}

// NewAssetRate creates a new builder for creating or updating asset rates.
//
// Asset rates define the exchange rate between two assets within a ledger, allowing for
// currency conversion and multi-currency accounting. This function returns a builder
// that allows for fluent configuration of the asset rate before creation or update.
//
// Parameters:
//   - client: The client interface that will be used to communicate with the Midaz API.
//     This must implement the AssetRateClientInterface with the CreateOrUpdateAssetRate method.
//
// Returns:
//   - AssetRateBuilder: A builder interface for configuring and creating/updating asset rate resources.
//     Use the builder's methods to set required and optional parameters, then call CreateOrUpdate()
//     to perform the asset rate creation or update.
//
// Example:
//
//	// Create a client
//	client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
//	if err != nil {
//	    log.Fatalf("Failed to create client: %v", err)
//	}
//
//	// Create an asset rate builder
//	rateBuilder := builders.NewAssetRate(client)
//
//	// Configure and create/update the asset rate
//	rate, err := rateBuilder.
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithFromAsset("USD").
//	    WithToAsset("EUR").
//	    WithRate(0.85).
//	    WithEffectiveAt(time.Now()).
//	    WithExpirationAt(time.Now().AddDate(0, 1, 0)). // 1 month from now
//	    CreateOrUpdate(context.Background())
//
//	if err != nil {
//	    log.Fatalf("Failed to create/update asset rate: %v", err)
//	}
//
//	fmt.Printf("Asset rate created/updated: %s -> %s at rate %.4f\n",
//	    rate.FromAsset, rate.ToAsset, rate.Rate)
func NewAssetRate(client AssetRateClientInterface) AssetRateBuilder {
	now := time.Now()
	return &assetRateBuilder{
		client:       client,
		effectiveAt:  now,
		expirationAt: now.AddDate(0, 1, 0), // Default expiration: 30 days from now
	}
}

// WithOrganization sets the organization ID for the asset rate.
// This is a required field for asset rate creation/update.
func (b *assetRateBuilder) WithOrganization(orgID string) AssetRateBuilder {
	b.organizationID = orgID
	return b
}

// WithLedger sets the ledger ID for the asset rate.
// This is a required field for asset rate creation/update.
func (b *assetRateBuilder) WithLedger(ledgerID string) AssetRateBuilder {
	b.ledgerID = ledgerID
	return b
}

// WithFromAsset sets the source asset code for the asset rate.
// This is a required field that specifies the asset being converted from.
func (b *assetRateBuilder) WithFromAsset(assetCode string) AssetRateBuilder {
	b.fromAsset = assetCode
	return b
}

// WithToAsset sets the destination asset code for the asset rate.
// This is a required field that specifies the asset being converted to.
func (b *assetRateBuilder) WithToAsset(assetCode string) AssetRateBuilder {
	b.toAsset = assetCode
	return b
}

// WithRate sets the exchange rate for the asset rate.
// This is a required field that specifies the conversion rate from the source asset to the destination asset.
// For example, if 1 USD = 0.85 EUR, the rate would be 0.85.
func (b *assetRateBuilder) WithRate(rate float64) AssetRateBuilder {
	b.rate = rate
	b.isRateSet = true
	return b
}

// WithEffectiveAt sets the effective date for the asset rate.
// This is an optional field that specifies when the rate becomes active.
// If not specified, the current time will be used.
func (b *assetRateBuilder) WithEffectiveAt(effectiveAt time.Time) AssetRateBuilder {
	b.effectiveAt = effectiveAt
	return b
}

// WithExpirationAt sets the expiration date for the asset rate.
// This is an optional field that specifies when the rate expires.
// If not specified, the rate will expire 30 days from the effective date.
func (b *assetRateBuilder) WithExpirationAt(expirationAt time.Time) AssetRateBuilder {
	b.expirationAt = expirationAt
	return b
}

// CreateOrUpdate executes the asset rate creation or update and returns the created or updated asset rate.
//
// This method validates all required parameters, constructs the asset rate input,
// and sends the request to the Midaz API to create or update the asset rate. If an asset rate
// with the same from/to asset pair already exists, it will be updated. Otherwise, a new
// asset rate will be created.
//
// Parameters:
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//     This context is passed to the underlying API client.
//
// Returns:
//   - *models.AssetRate: The created or updated asset rate object if successful.
//     This contains all details about the asset rate, including the from/to assets,
//     rate value, and effective/expiration dates.
//   - error: An error if the operation fails. Possible error types include:
//   - errors.ErrValidation: If required parameters are missing or invalid
//   - errors.ErrAuthentication: If authentication fails
//   - errors.ErrPermission: If the client lacks permission
//   - errors.ErrNotFound: If the organization, ledger, or assets are not found
//   - errors.ErrInternal: For other internal errors
//
// Example:
//
//	// Create a basic asset rate
//	rate, err := builders.NewAssetRate(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithFromAsset("USD").
//	    WithToAsset("EUR").
//	    WithRate(0.85).
//	    CreateOrUpdate(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset rate
//	fmt.Printf("Asset rate: 1 %s = %.4f %s\n", rate.FromAsset, rate.Rate, rate.ToAsset)
//
//	// Create an asset rate with custom effective and expiration dates
//	now := time.Now()
//	rate, err = builders.NewAssetRate(client).
//	    WithOrganization("org-123").
//	    WithLedger("ledger-456").
//	    WithFromAsset("EUR").
//	    WithToAsset("GBP").
//	    WithRate(0.88).
//	    WithEffectiveAt(now).
//	    WithExpirationAt(now.AddDate(0, 3, 0)). // 3 months from now
//	    CreateOrUpdate(context.Background())
//
//	if err != nil {
//	    // Handle error
//	    return err
//	}
//
//	// Use the asset rate
//	fmt.Printf("Asset rate valid from %s to %s\n",
//	    rate.EffectiveAt.Format(time.RFC3339),
//	    rate.ExpirationAt.Format(time.RFC3339))
func (b *assetRateBuilder) CreateOrUpdate(ctx context.Context) (*models.AssetRate, error) {
	// Validate required fields
	if b.organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if b.ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	if b.fromAsset == "" {
		return nil, fmt.Errorf("from asset is required")
	}

	if b.toAsset == "" {
		return nil, fmt.Errorf("to asset is required")
	}

	if !b.isRateSet {
		return nil, fmt.Errorf("rate is required")
	}

	// Create asset rate input
	input := &models.UpdateAssetRateInput{
		FromAsset:    b.fromAsset,
		ToAsset:      b.toAsset,
		Rate:         b.rate,
		EffectiveAt:  b.effectiveAt,
		ExpirationAt: b.expirationAt,
	}

	// Validate the input using the model's validation method
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset rate input: %v", err)
	}

	// Execute asset rate creation/update
	return b.client.CreateOrUpdateAssetRate(ctx, b.organizationID, b.ledgerID, input)
}
