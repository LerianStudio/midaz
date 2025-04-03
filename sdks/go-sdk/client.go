// Package midaz provides a Go client for the Midaz API.
package midaz

import (
	"context"
	"net/http"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/abstractions"
	"github.com/LerianStudio/midaz/sdks/go-sdk/builders"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// Client is the main entry point for the Midaz SDK.
type Client struct {
	// HTTP client configuration
	httpClient     *http.Client
	authToken      string
	userAgent      string
	debug          bool
	ctx            context.Context
	baseURLs       map[string]string
	onboardingURL  string
	transactionURL string

	// Optional API interfaces
	Entity      *entities.Entity
	Builder     *builders.Builder
	Abstraction *abstractions.Abstraction

	// API interface flags
	useEntity      bool
	useBuilder     bool
	useAbstraction bool
}

// transactionServiceAdapter adapts the entities.TransactionsService to builders.ClientInterface
type transactionServiceAdapter struct {
	service entities.TransactionsService
}

// CreateTransaction implements builders.ClientInterface by converting DSL input to standard input
func (a *transactionServiceAdapter) CreateTransaction(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
	return a.service.CreateTransactionWithDSL(ctx, orgID, ledgerID, input)
}

// New creates a new Midaz client with the provided options.
func New(options ...Option) (*Client, error) {
	// Create a new client with default settings
	client := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		ctx:       context.Background(),
		userAgent: "midaz-go-sdk/1.0",
		baseURLs:  make(map[string]string),
	}

	// Apply the provided options
	for _, option := range options {
		if err := option(client); err != nil {
			return nil, err
		}
	}

	// Set default URLs if not provided
	if client.onboardingURL == "" {
		client.onboardingURL = "https://api.midaz.io/onboarding"
	}
	if client.transactionURL == "" {
		client.transactionURL = "https://api.midaz.io/transaction"
	}

	// Set base URLs
	client.baseURLs["onboarding"] = client.onboardingURL
	client.baseURLs["transaction"] = client.transactionURL

	// Initialize requested interfaces
	var err error

	// Create a single Entity instance to be shared across all interfaces
	var entity *entities.Entity
	if client.useEntity || client.useBuilder || client.useAbstraction {
		entity, err = entities.NewEntity(client.httpClient, client.authToken, client.baseURLs)
		if err != nil {
			return nil, err
		}
	}

	// Initialize Entity if requested
	if client.useEntity {
		client.Entity = entity
	}

	// Initialize Builder if requested
	if client.useBuilder {
		// Create adapter for transactions service
		txAdapter := &transactionServiceAdapter{service: entity.Transactions}

		// Use the entity's interfaces for the builder
		client.Builder = builders.NewBuilder(
			entity.Accounts,      // AccountClientInterface
			entity.Assets,        // AssetClientInterface
			entity.AssetRates,    // AssetRateClientInterface
			entity.Balances,      // BalanceClientInterface
			entity.Ledgers,       // LedgerClientInterface
			entity.Organizations, // OrganizationClientInterface
			entity.Portfolios,    // PortfolioClientInterface
			entity.Segments,      // SegmentClientInterface
			txAdapter,            // ClientInterface
		)
	}

	// Initialize Abstraction if requested
	if client.useAbstraction {
		// Initialize abstraction with entity's CreateTransactionWithDSL method
		client.Abstraction = abstractions.NewAbstraction(entity.Transactions.CreateTransactionWithDSL)
	}

	return client, nil
}

// Option defines a function type for configuring the client.
type Option func(*Client) error

// WithAuthToken sets the authentication token for the client.
func WithAuthToken(token string) Option {
	return func(c *Client) error {
		c.authToken = token
		return nil
	}
}

// WithOnboardingURL sets the base URL for the onboarding API.
func WithOnboardingURL(url string) Option {
	return func(c *Client) error {
		c.onboardingURL = url
		return nil
	}
}

// WithTransactionURL sets the base URL for the transaction API.
func WithTransactionURL(url string) Option {
	return func(c *Client) error {
		c.transactionURL = url
		return nil
	}
}

// WithHTTPClient sets a custom HTTP client for the client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) error {
		c.httpClient = httpClient
		return nil
	}
}

// WithTimeout sets the timeout for requests made by the client.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		if c.httpClient == nil {
			c.httpClient = &http.Client{}
		}
		c.httpClient.Timeout = timeout
		return nil
	}
}

// WithDebug enables or disables debug mode for the client.
func WithDebug(debug bool) Option {
	return func(c *Client) error {
		c.debug = debug
		return nil
	}
}

// UseEntity enables the Entity API interface.
func UseEntity() Option {
	return func(c *Client) error {
		c.useEntity = true
		return nil
	}
}

// UseBuilder enables the Builder API interface.
func UseBuilder() Option {
	return func(c *Client) error {
		c.useBuilder = true
		return nil
	}
}

// UseAbstraction enables the Abstraction API interface.
func UseAbstraction() Option {
	return func(c *Client) error {
		c.useAbstraction = true
		return nil
	}
}

// UseAllAPIs enables all API interfaces.
func UseAllAPIs() Option {
	return func(c *Client) error {
		c.useEntity = true
		c.useBuilder = true
		c.useAbstraction = true
		return nil
	}
}
