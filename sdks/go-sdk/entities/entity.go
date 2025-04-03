// Package entities provides access to the Midaz API resources and operations.
// It implements service interfaces for interacting with accounts, assets, ledgers,
// transactions, and other Midaz platform resources.
package entities

import (
	"net/http"
)

// Entity provides a centralized access point to all entity types in the Midaz SDK.
// It acts as a factory for creating specific entity interfaces for different resource types
// and operations.
type Entity struct {
	// HTTP client configuration
	httpClient *httpClient
	baseURLs   map[string]string

	// Service interfaces for different resource types
	Accounts      AccountsService
	Assets        AssetsService
	AssetRates    AssetRatesService
	Balances      BalancesService
	Ledgers       LedgersService
	Operations    OperationsService
	Organizations OrganizationsService
	Portfolios    PortfoliosService
	Segments      SegmentsService
	Transactions  TransactionsService
}

// NewEntity creates a new Entity instance with the provided client configuration.
// This constructor initializes an Entity that provides access to all entity types
// in the Midaz SDK.
//
// Parameters:
//   - client: The HTTP client to use for API requests
//   - authToken: The authentication token for API requests
//   - baseURLs: Map of base URLs for different API endpoints
//   - options: Optional configuration options
//
// Returns:
//   - *Entity: A pointer to the newly created Entity, ready to interact with the Midaz API
//   - error: An error if the client initialization fails
func NewEntity(client *http.Client, authToken string, baseURLs map[string]string, options ...Option) (*Entity, error) {
	// Create a new entity with the provided configuration
	entity := &Entity{
		httpClient: newHTTPClient(client, authToken, "midaz-go-sdk/1.0", false),
		baseURLs:   baseURLs,
	}

	// Apply the provided options
	for _, option := range options {
		if err := option(entity); err != nil {
			return nil, err
		}
	}

	// Initialize service interfaces
	entity.initServices()

	return entity, nil
}

// initServices initializes the service interfaces for the entity.
func (e *Entity) initServices() {
	// Initialize service interfaces with the entity configuration
	e.Accounts = NewAccountsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Assets = NewAssetsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.AssetRates = NewAssetRatesEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Balances = NewBalancesEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Ledgers = NewLedgersEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Operations = NewOperationsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Organizations = NewOrganizationsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Portfolios = NewPortfoliosEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Segments = NewSegmentsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
	e.Transactions = NewTransactionsEntity(e.httpClient.client, e.httpClient.authToken, e.baseURLs)
}
