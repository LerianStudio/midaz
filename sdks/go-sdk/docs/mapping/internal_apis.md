# Midaz Go SDK Internal API Map

This document provides a comprehensive overview of the internal APIs used by the Midaz Go SDK, organized by package and purpose. These APIs are not intended for direct use by SDK consumers but are documented here for SDK maintainers and contributors.

## Table of Contents

- [I. Client Package](#i-client-package-midazclient)
- [II. Resource Clients](#ii-resource-clients-midazclient)
- [III. Builder Implementations](#iii-builder-implementations-midazbuilders)
- [IV. Models Package](#iv-models-package-midazmodels)
- [V. Error Handling](#v-error-handling-midazerrors)
- [VI. Implementation Patterns](#vi-implementation-patterns)

## I. Client Package (`midaz/client`)

The client package provides the foundation for all API interactions, handling HTTP requests, authentication, and error processing.

### HTTP Client

- `httpClient` - Handles all HTTP requests to the Midaz API
  ```go
  type httpClient struct {
      baseURL     string
      authToken   string
      httpClient  *http.Client
      debug       bool
      userAgent   string
  }
  ```

  - `Do(ctx context.Context, req *http.Request) (*http.Response, error)` - Executes an HTTP request with retries and error handling
    ```go
    // Implementation pattern
    func (c *httpClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
        // Add authentication headers
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
        req.Header.Set("User-Agent", c.userAgent)
        
        // Execute request with retry logic
        var resp *http.Response
        var err error
        for attempt := 0; attempt < maxRetries; attempt++ {
            resp, err = c.httpClient.Do(req.WithContext(ctx))
            if err == nil || !isRetryableError(err) {
                break
            }
            time.Sleep(backoffDuration(attempt))
        }
        
        // Handle response errors
        if err != nil {
            return nil, err
        }
        
        if resp.StatusCode >= 400 {
            return resp, handleErrorResponse(resp)
        }
        
        return resp, nil
    }
    ```
  
  - `Get(ctx context.Context, path string, params url.Values) (*http.Response, error)` - Performs an HTTP GET request
  - `Post(ctx context.Context, path string, body interface{}) (*http.Response, error)` - Performs an HTTP POST request
  - `Put(ctx context.Context, path string, body interface{}) (*http.Response, error)` - Performs an HTTP PUT request
  - `Delete(ctx context.Context, path string) (*http.Response, error)` - Performs an HTTP DELETE request
  - `Patch(ctx context.Context, path string, body interface{}) (*http.Response, error)` - Performs an HTTP PATCH request

### API Client

- `apiClient` - Base client for all API operations
  ```go
  type apiClient struct {
      httpClient      *httpClient
      onboardingURL   string
      transactionURL  string
      timeout         time.Duration
      debug           bool
  }
  ```

  - `SetAuthToken(token string)` - Sets the authentication token
    ```go
    func (c *apiClient) SetAuthToken(token string) {
        c.httpClient.authToken = token
    }
    ```
  
  - `SetOnboardingURL(url string)` - Sets the onboarding API URL
  - `SetTransactionURL(url string)` - Sets the transaction API URL
  - `SetTimeout(seconds int)` - Sets the request timeout
  - `SetDebug(debug bool)` - Enables or disables debug mode

## II. Resource Clients (`midaz/client`)

These clients handle specific resource operations and are used by the builders. Each client encapsulates the API operations for a specific resource type.

### Organization Client

- `organizationClient` - Handles organization-related API operations
  ```go
  type organizationClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.Organization], error)` - Lists organizations
    ```go
    // Implementation pattern
    func (c *organizationClient) List(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.Organization], error) {
        path := "/organizations"
        params := url.Values{}
        
        // Add pagination parameters
        if opts != nil {
            if opts.Page > 0 {
                params.Set("page", strconv.Itoa(opts.Page))
            }
            if opts.Limit > 0 {
                params.Set("limit", strconv.Itoa(opts.Limit))
            }
            // Add other filter parameters
        }
        
        resp, err := c.apiClient.httpClient.Get(ctx, path, params)
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()
        
        var result models.ListResponse[models.Organization]
        if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
            return nil, err
        }
        
        return &result, nil
    }
    ```
  
  - `Get(ctx context.Context, id string) (*models.Organization, error)` - Gets an organization by ID
  - `Create(ctx context.Context, input *models.CreateOrganizationInput) (*models.Organization, error)` - Creates a new organization
  - `Update(ctx context.Context, id string, input *models.UpdateOrganizationInput) (*models.Organization, error)` - Updates an organization
  - `Delete(ctx context.Context, id string) error` - Deletes an organization

### Ledger Client

- `ledgerClient` - Handles ledger-related API operations
  ```go
  type ledgerClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID string, opts *models.ListOptions) (*models.ListResponse[models.Ledger], error)` - Lists ledgers for an organization
  - `Get(ctx context.Context, organizationID, id string) (*models.Ledger, error)` - Gets a ledger by ID
  - `Create(ctx context.Context, organizationID string, input *models.CreateLedgerInput) (*models.Ledger, error)` - Creates a new ledger
  - `Update(ctx context.Context, organizationID, id string, input *models.UpdateLedgerInput) (*models.Ledger, error)` - Updates a ledger
  - `Delete(ctx context.Context, organizationID, id string) error` - Deletes a ledger

### Account Client

- `accountClient` - Handles account-related API operations
  ```go
  type accountClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Account], error)` - Lists accounts for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.Account, error)` - Gets an account by ID
  - `GetByAlias(ctx context.Context, organizationID, ledgerID, alias string) (*models.Account, error)` - Gets an account by alias
  - `Create(ctx context.Context, organizationID, ledgerID string, input *models.CreateAccountInput) (*models.Account, error)` - Creates a new account
  - `Update(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAccountInput) (*models.Account, error)` - Updates an account
  - `Delete(ctx context.Context, organizationID, ledgerID, id string) error` - Deletes an account

### Asset Client

- `assetClient` - Handles asset-related API operations
  ```go
  type assetClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Asset], error)` - Lists assets for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.Asset, error)` - Gets an asset by ID
  - `Create(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetInput) (*models.Asset, error)` - Creates a new asset
  - `Update(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAssetInput) (*models.Asset, error)` - Updates an asset
  - `Delete(ctx context.Context, organizationID, ledgerID, id string) error` - Deletes an asset

### Asset Rate Client

- `assetRateClient` - Handles asset rate-related API operations
  ```go
  type assetRateClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.AssetRate], error)` - Lists asset rates for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.AssetRate, error)` - Gets an asset rate by ID
  - `Create(ctx context.Context, organizationID, ledgerID string, input *models.CreateAssetRateInput) (*models.AssetRate, error)` - Creates a new asset rate
  - `Update(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateAssetRateInput) (*models.AssetRate, error)` - Updates an asset rate
  - `Delete(ctx context.Context, organizationID, ledgerID, id string) error` - Deletes an asset rate

### Portfolio Client

- `portfolioClient` - Handles portfolio-related API operations
  ```go
  type portfolioClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Portfolio], error)` - Lists portfolios for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.Portfolio, error)` - Gets a portfolio by ID
  - `Create(ctx context.Context, organizationID, ledgerID string, input *models.CreatePortfolioInput) (*models.Portfolio, error)` - Creates a new portfolio
  - `Update(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdatePortfolioInput) (*models.Portfolio, error)` - Updates a portfolio
  - `Delete(ctx context.Context, organizationID, ledgerID, id string) error` - Deletes a portfolio

### Segment Client

- `segmentClient` - Handles segment-related API operations
  ```go
  type segmentClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID, portfolioID string, opts *models.ListOptions) (*models.ListResponse[models.Segment], error)` - Lists segments for a portfolio
  - `Get(ctx context.Context, organizationID, ledgerID, portfolioID, id string) (*models.Segment, error)` - Gets a segment by ID
  - `Create(ctx context.Context, organizationID, ledgerID, portfolioID string, input *models.CreateSegmentInput) (*models.Segment, error)` - Creates a new segment
  - `Update(ctx context.Context, organizationID, ledgerID, portfolioID, id string, input *models.UpdateSegmentInput) (*models.Segment, error)` - Updates a segment
  - `Delete(ctx context.Context, organizationID, ledgerID, portfolioID, id string) error` - Deletes a segment

### Transaction Client

- `transactionClient` - Handles transaction-related API operations
  ```go
  type transactionClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error)` - Lists transactions for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.Transaction, error)` - Gets a transaction by ID
  - `Create(ctx context.Context, organizationID, ledgerID string, input *models.CreateTransactionInput) (*models.Transaction, error)` - Creates a new transaction
    ```go
    // Implementation pattern
    func (c *transactionClient) Create(ctx context.Context, organizationID, ledgerID string, input *models.CreateTransactionInput) (*models.Transaction, error) {
        path := fmt.Sprintf("/organizations/%s/ledgers/%s/transactions", organizationID, ledgerID)
        
        resp, err := c.apiClient.httpClient.Post(ctx, path, input)
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()
        
        var result models.Transaction
        if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
            return nil, err
        }
        
        return &result, nil
    }
    ```
  
  - `Commit(ctx context.Context, organizationID, ledgerID, id string) (*models.Transaction, error)` - Commits a pending transaction
  - `Cancel(ctx context.Context, organizationID, ledgerID, id string) (*models.Transaction, error)` - Cancels a pending transaction

### Balance Client

- `balanceClient` - Handles balance-related API operations
  ```go
  type balanceClient struct {
      apiClient *apiClient
  }
  ```

  - `List(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Balance], error)` - Lists balances for a ledger
  - `Get(ctx context.Context, organizationID, ledgerID, id string) (*models.Balance, error)` - Gets a balance by ID
  - `Update(ctx context.Context, organizationID, ledgerID, id string, input *models.UpdateBalanceInput) (*models.Balance, error)` - Updates a balance

## III. Builder Implementations (`midaz/builders`)

The builders package provides fluent interfaces for creating and updating resources, implemented as private structs with public interfaces. These implementations handle the complexity of constructing valid API requests while providing a clean, chainable API for SDK users.

### Common Builder Patterns

All builders follow similar implementation patterns:

```go
// Public interface exposed to SDK users
type SomeBuilder interface {
    WithField1(value string) SomeBuilder
    WithField2(value int) SomeBuilder
    Create(ctx context.Context) (*models.SomeResource, error)
}

// Private implementation
type someBuilderImpl struct {
    client *client.Client
    input  *models.CreateSomeResourceInput
}

// Constructor function that returns the interface
func NewSome(client *client.Client) SomeBuilder {
    return &someBuilderImpl{
        client: client,
        input: &models.CreateSomeResourceInput{},
    }
}

// Method implementations that return the interface for chaining
func (b *someBuilderImpl) WithField1(value string) SomeBuilder {
    b.input.Field1 = value
    return b
}

func (b *someBuilderImpl) WithField2(value int) SomeBuilder {
    b.input.Field2 = value
    return b
}

// Terminal method that performs the API call
func (b *someBuilderImpl) Create(ctx context.Context) (*models.SomeResource, error) {
    // Validate input
    if err := b.validate(); err != nil {
        return nil, err
    }
    
    // Call the API client
    return b.client.SomeResources.Create(ctx, b.input)
}

// Private validation method
func (b *someBuilderImpl) validate() error {
    if b.input.Field1 == "" {
        return errors.New("field1 is required")
    }
    return nil
}
```

### Organization Builders

- `baseOrganizationBuilder` - Base implementation for organization builders
  ```go
  type baseOrganizationBuilder struct {
      client *client.Client
      input  *models.CreateOrganizationInput
  }
  ```
  - Contains common fields and methods for organization creation and updates
  - Implements the `WithLegalName()`, `WithLegalDocument()`, etc. methods

- `organizationBuilder` - Implementation of the OrganizationBuilder interface
  ```go
  type organizationBuilder struct {
      baseOrganizationBuilder
  }
  ```
  - Extends baseOrganizationBuilder
  - Implements `Create()` to create a new organization
    ```go
    func (b *organizationBuilder) Create(ctx context.Context) (*models.Organization, error) {
        // Validate input
        if err := b.validate(); err != nil {
            return nil, err
        }
        
        // Call the API client
        return b.client.Organizations.Create(ctx, b.input)
    }
    ```

- `organizationUpdateBuilder` - Implementation of the OrganizationUpdateBuilder interface
  ```go
  type organizationUpdateBuilder struct {
      baseOrganizationBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends baseOrganizationBuilder
  - Tracks which fields have been updated using the updatedFields map
  - Implements `Update()` to update an existing organization
    ```go
    func (b *organizationUpdateBuilder) Update(ctx context.Context) (*models.Organization, error) {
        // Validate input
        if err := b.validate(); err != nil {
            return nil, err
        }
        
        // Create update input with only the fields that were changed
        updateInput := &models.UpdateOrganizationInput{}
        if b.updatedFields["legalName"] {
            updateInput.LegalName = b.input.LegalName
        }
        if b.updatedFields["legalDocument"] {
            updateInput.LegalDocument = b.input.LegalDocument
        }
        // ... other fields
        
        // Call the API client
        return b.client.Organizations.Update(ctx, b.id, updateInput)
    }
    ```

### Ledger Builders

- `baseLedgerBuilder` - Base implementation for ledger builders
  ```go
  type baseLedgerBuilder struct {
      client        *client.Client
      input         *models.CreateLedgerInput
      organizationID string
  }
  ```
  - Contains common fields and methods for ledger creation and updates
  - Implements the `WithName()`, `WithOrganization()`, etc. methods

- `ledgerBuilder` - Implementation of the LedgerBuilder interface
  ```go
  type ledgerBuilder struct {
      baseLedgerBuilder
  }
  ```
  - Extends baseLedgerBuilder
  - Implements `Create()` to create a new ledger
    ```go
    func (b *ledgerBuilder) Create(ctx context.Context) (*models.Ledger, error) {
        // Validate input
        if err := b.validate(); err != nil {
            return nil, err
        }
        
        // Call the API client
        return b.client.Ledgers.Create(ctx, b.organizationID, b.input)
    }
    ```

- `ledgerUpdateBuilder` - Implementation of the LedgerUpdateBuilder interface
  ```go
  type ledgerUpdateBuilder struct {
      baseLedgerBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends baseLedgerBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing ledger

### Account Builders

- `baseAccountBuilder` - Base implementation for account builders
  ```go
  type baseAccountBuilder struct {
      client        *client.Client
      input         *models.CreateAccountInput
      organizationID string
      ledgerID      string
  }
  ```
  - Contains common fields and methods for account creation and updates
  - Implements the `WithName()`, `WithAssetCode()`, etc. methods

- `accountBuilder` - Implementation of the AccountBuilder interface
  ```go
  type accountBuilder struct {
      baseAccountBuilder
  }
  ```
  - Extends baseAccountBuilder
  - Implements `Create()` to create a new account
    ```go
    func (b *accountBuilder) Create(ctx context.Context) (*models.Account, error) {
        // Validate input
        if err := b.validate(); err != nil {
            return nil, err
        }
        
        // Call the API client
        return b.client.Accounts.Create(ctx, b.organizationID, b.ledgerID, b.input)
    }
    ```

- `accountUpdateBuilder` - Implementation of the AccountUpdateBuilder interface
  ```go
  type accountUpdateBuilder struct {
      baseAccountBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends baseAccountBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing account

### Asset Builders

- `baseAssetBuilder` - Base implementation for asset builders
  ```go
  type baseAssetBuilder struct {
      client        *client.Client
      input         *models.CreateAssetInput
      organizationID string
      ledgerID      string
  }
  ```
  - Contains common fields and methods for asset creation and updates
  - Implements the `WithName()`, `WithCode()`, etc. methods

- `assetBuilder` - Implementation of the AssetBuilder interface
  ```go
  type assetBuilder struct {
      baseAssetBuilder
  }
  ```
  - Extends baseAssetBuilder
  - Implements `Create()` to create a new asset

- `assetUpdateBuilder` - Implementation of the AssetUpdateBuilder interface
  ```go
  type assetUpdateBuilder struct {
      baseAssetBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends baseAssetBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing asset

### Asset Rate Builders

- `assetRateBuilder` - Implementation of the AssetRateBuilder interface
  ```go
  type assetRateBuilder struct {
      client        *client.Client
      input         *models.CreateAssetRateInput
      organizationID string
      ledgerID      string
  }
  ```
  - Contains fields for asset rate creation and updates
  - Implements the `WithBaseAsset()`, `WithQuoteAsset()`, etc. methods
  - Implements `CreateOrUpdate()` to create or update an asset rate
    ```go
    func (b *assetRateBuilder) CreateOrUpdate(ctx context.Context) (*models.AssetRate, error) {
        // Validate input
        if err := b.validate(); err != nil {
            return nil, err
        }
        
        // Try to find existing asset rate
        opts := &models.ListOptions{
            Filters: map[string]string{
                "baseAssetCode":  b.input.BaseAssetCode,
                "quoteAssetCode": b.input.QuoteAssetCode,
            },
        }
        
        rates, err := b.client.AssetRates.List(ctx, b.organizationID, b.ledgerID, opts)
        if err != nil {
            return nil, err
        }
        
        // If rate exists, update it
        if len(rates.Items) > 0 {
            updateInput := &models.UpdateAssetRateInput{
                Rate:         b.input.Rate,
                EffectiveAt:  b.input.EffectiveAt,
                ExpirationAt: b.input.ExpirationAt,
                Metadata:     b.input.Metadata,
            }
            return b.client.AssetRates.Update(ctx, b.organizationID, b.ledgerID, rates.Items[0].ID, updateInput)
        }
        
        // Otherwise create a new rate
        return b.client.AssetRates.Create(ctx, b.organizationID, b.ledgerID, b.input)
    }
    ```

### Portfolio Builders

- `basePortfolioBuilder` - Base implementation for portfolio builders
  ```go
  type basePortfolioBuilder struct {
      client        *client.Client
      input         *models.CreatePortfolioInput
      organizationID string
      ledgerID      string
  }
  ```
  - Contains common fields and methods for portfolio creation and updates
  - Implements the `WithName()`, `WithOrganization()`, etc. methods

- `portfolioBuilder` - Implementation of the PortfolioBuilder interface
  ```go
  type portfolioBuilder struct {
      basePortfolioBuilder
  }
  ```
  - Extends basePortfolioBuilder
  - Implements `Create()` to create a new portfolio

- `portfolioUpdateBuilder` - Implementation of the PortfolioUpdateBuilder interface
  ```go
  type portfolioUpdateBuilder struct {
      basePortfolioBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends basePortfolioBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing portfolio

### Segment Builders

- `baseSegmentBuilder` - Base implementation for segment builders
  ```go
  type baseSegmentBuilder struct {
      client        *client.Client
      input         *models.CreateSegmentInput
      organizationID string
      ledgerID      string
      portfolioID   string
  }
  ```
  - Contains common fields and methods for segment creation and updates
  - Implements the `WithName()`, `WithPortfolio()`, etc. methods

- `segmentBuilder` - Implementation of the SegmentBuilder interface
  ```go
  type segmentBuilder struct {
      baseSegmentBuilder
  }
  ```
  - Extends baseSegmentBuilder
  - Implements `Create()` to create a new segment

- `segmentUpdateBuilder` - Implementation of the SegmentUpdateBuilder interface
  ```go
  type segmentUpdateBuilder struct {
      baseSegmentBuilder
      id            string
      updatedFields map[string]bool
  }
  ```
  - Extends baseSegmentBuilder
  - Tracks which fields have been updated
  - Implements `Update()` to update an existing segment

### Transaction Builders

- `baseTransactionBuilder` - Base implementation for transaction builders
  ```go
  type baseTransactionBuilder struct {
      client        *client.Client
      input         *models.CreateTransactionInput
      organizationID string
      ledgerID      string
  }
  ```
  - Contains common fields and methods for transaction creation
  - Implements the `WithDescription()`, `WithExternalID()`, etc. methods

- `transactionBuilder` - Implementation of the TransactionBuilder interface
  ```go
  type transactionBuilder struct {
      baseTransactionBuilder
  }
  ```
  - Extends baseTransactionBuilder
  - Implements `AddOperation()` to add operations to the transaction
  - Implements `Create()` to create a new transaction
  - Implements `CreateAndCommit()` to create and commit a transaction in one step
    ```go
    func (b *transactionBuilder) CreateAndCommit(ctx context.Context) (*models.Transaction, error) {
        // Create the transaction
        tx, err := b.Create(ctx)
        if err != nil {
            return nil, err
        }
        
        // Commit the transaction
        return b.client.Transactions.Commit(ctx, b.organizationID, b.ledgerID, tx.ID)
    }
    ```

- `depositBuilder` - Implementation of the DepositBuilder interface
  ```go
  type depositBuilder struct {
      baseTransactionBuilder
      sourceAccountID      string
      destinationAccountID string
      amount               int64
      assetCode            string
      scale                int
  }
  ```
  - Extends baseTransactionBuilder
  - Implements specialized methods for deposits
  - Implements `Execute()` to create and commit a deposit transaction
    ```go
    func (b *depositBuilder) Execute(ctx context.Context) (*models.Transaction, error) {
        // Set up the operations for a deposit
        b.input.Operations = []models.CreateOperationInput{
            {
                Type:      "credit",
                AccountID: b.sourceAccountID,
                Amount:    b.amount,
                AssetCode: b.assetCode,
                Scale:     b.scale,
            },
            {
                Type:      "debit",
                AccountID: b.destinationAccountID,
                Amount:    b.amount,
                AssetCode: b.assetCode,
                Scale:     b.scale,
            },
        }
        
        // Create and commit the transaction
        return b.CreateAndCommit(ctx)
    }
    ```

- `withdrawalBuilder` - Implementation of the WithdrawalBuilder interface
  ```go
  type withdrawalBuilder struct {
      baseTransactionBuilder
      sourceAccountID      string
      destinationAccountID string
      amount               int64
      assetCode            string
      scale                int
  }
  ```
  - Extends baseTransactionBuilder
  - Implements specialized methods for withdrawals
  - Implements `Execute()` to create and commit a withdrawal transaction

- `transferBuilder` - Implementation of the TransferBuilder interface
  ```go
  type transferBuilder struct {
      baseTransactionBuilder
      sourceAccountID      string
      destinationAccountID string
      amount               int64
      assetCode            string
      scale                int
  }
  ```
  - Extends baseTransactionBuilder
  - Implements specialized methods for transfers
  - Implements `Execute()` to create and commit a transfer transaction

### Balance Builders

- `balanceUpdateBuilder` - Implementation of the BalanceUpdateBuilder interface
  ```go
  type balanceUpdateBuilder struct {
      client        *client.Client
      input         *models.UpdateBalanceInput
      organizationID string
      ledgerID      string
      id            string
  }
  ```
  - Implements methods for updating balance settings
  - Implements `Update()` to update a balance
    ```go
    func (b *balanceUpdateBuilder) Update(ctx context.Context) (*models.Balance, error) {
        return b.client.Balances.Update(ctx, b.organizationID, b.ledgerID, b.id, b.input)
    }
    ```

## IV. Models Package (`midaz/models`)

The models package defines the data structures used throughout the SDK, representing API resources, request inputs, and responses.

### Resource Models

These structs represent the core resources in the Midaz API:

- `Organization` - Represents an organization in the system
  ```go
  type Organization struct {
      ID            string     `json:"id"`
      LegalName     string     `json:"legalName"`
      LegalDocument string     `json:"legalDocument,omitempty"`
      Status        Status     `json:"status"`
      Address       *Address   `json:"address,omitempty"`
      CreatedAt     time.Time  `json:"createdAt"`
      UpdatedAt     time.Time  `json:"updatedAt"`
      DeletedAt     *time.Time `json:"deletedAt,omitempty"`
      Metadata      map[string]any `json:"metadata,omitempty"`
      Tags          []string   `json:"tags,omitempty"`
  }
  ```

- `Ledger` - Represents a ledger in the system
  ```go
  type Ledger struct {
      ID             string     `json:"id"`
      Name           string     `json:"name"`
      OrganizationID string     `json:"organizationId"`
      Status         Status     `json:"status"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      DeletedAt      *time.Time `json:"deletedAt,omitempty"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `Account` - Represents an account in the system
  ```go
  type Account struct {
      ID             string     `json:"id"`
      Name           string     `json:"name"`
      Type           string     `json:"type"`
      AssetCode      string     `json:"assetCode"`
      ParentID       string     `json:"parentId,omitempty"`
      EntityID       string     `json:"entityId,omitempty"`
      PortfolioID    string     `json:"portfolioId,omitempty"`
      SegmentID      string     `json:"segmentId,omitempty"`
      Alias          string     `json:"alias,omitempty"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      Status         Status     `json:"status"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      DeletedAt      *time.Time `json:"deletedAt,omitempty"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `Asset` - Represents an asset in the system
  ```go
  type Asset struct {
      ID             string     `json:"id"`
      Name           string     `json:"name"`
      Code           string     `json:"code"`
      Type           string     `json:"type"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      Status         Status     `json:"status"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      DeletedAt      *time.Time `json:"deletedAt,omitempty"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `AssetRate` - Represents an exchange rate between assets
  ```go
  type AssetRate struct {
      ID             string     `json:"id"`
      BaseAssetCode  string     `json:"baseAssetCode"`
      QuoteAssetCode string     `json:"quoteAssetCode"`
      Rate           float64    `json:"rate"`
      EffectiveAt    time.Time  `json:"effectiveAt"`
      ExpirationAt   *time.Time `json:"expirationAt,omitempty"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `Portfolio` - Represents a portfolio in the system
  ```go
  type Portfolio struct {
      ID             string     `json:"id"`
      Name           string     `json:"name"`
      EntityID       string     `json:"entityId,omitempty"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      Status         Status     `json:"status"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      DeletedAt      *time.Time `json:"deletedAt,omitempty"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `Segment` - Represents a segment in the system
  ```go
  type Segment struct {
      ID             string     `json:"id"`
      Name           string     `json:"name"`
      PortfolioID    string     `json:"portfolioId"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      Status         Status     `json:"status"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
      DeletedAt      *time.Time `json:"deletedAt,omitempty"`
      Metadata       map[string]any `json:"metadata,omitempty"`
      Tags           []string   `json:"tags,omitempty"`
  }
  ```

- `Transaction` - Represents a transaction in the system
  ```go
  type Transaction struct {
      ID             string      `json:"id"`
      ExternalID     string      `json:"externalId,omitempty"`
      Description    string      `json:"description,omitempty"`
      Status         string      `json:"status"`
      AssetCode      string      `json:"assetCode"`
      Amount         int64       `json:"amount"`
      Scale          int         `json:"scale"`
      Operations     []Operation `json:"operations"`
      LedgerID       string      `json:"ledgerId"`
      OrganizationID string      `json:"organizationId"`
      CreatedAt      time.Time   `json:"createdAt"`
      UpdatedAt      time.Time   `json:"updatedAt"`
      CommittedAt    *time.Time  `json:"committedAt,omitempty"`
      CanceledAt     *time.Time  `json:"canceledAt,omitempty"`
      Metadata       map[string]any  `json:"metadata,omitempty"`
      Tags           []string    `json:"tags,omitempty"`
  }
  ```

- `Operation` - Represents an operation within a transaction
  ```go
  type Operation struct {
      ID          string    `json:"id"`
      Type        string    `json:"type"`
      AccountID   string    `json:"accountId"`
      Amount      int64     `json:"amount"`
      AssetCode   string    `json:"assetCode"`
      Scale       int       `json:"scale"`
      CreatedAt   time.Time `json:"createdAt"`
      Description string    `json:"description,omitempty"`
  }
  ```

- `Balance` - Represents an account balance
  ```go
  type Balance struct {
      ID             string     `json:"id"`
      AccountID      string     `json:"accountId"`
      AssetCode      string     `json:"assetCode"`
      Available      int64      `json:"available"`
      Pending        int64      `json:"pending"`
      Reserved       int64      `json:"reserved"`
      Scale          int        `json:"scale"`
      AllowSending   bool       `json:"allowSending"`
      AllowReceiving bool       `json:"allowReceiving"`
      LedgerID       string     `json:"ledgerId"`
      OrganizationID string     `json:"organizationId"`
      CreatedAt      time.Time  `json:"createdAt"`
      UpdatedAt      time.Time  `json:"updatedAt"`
  }
  ```

### Input Models

These structs represent the input data for API operations:

- `CreateOrganizationInput` - Input for creating an organization
  ```go
  type CreateOrganizationInput struct {
      LegalName     string      `json:"legalName"`
      LegalDocument string      `json:"legalDocument,omitempty"`
      Status        Status      `json:"status,omitempty"`
      Address       *Address    `json:"address,omitempty"`
      Metadata      map[string]any  `json:"metadata,omitempty"`
      Tags          []string    `json:"tags,omitempty"`
  }
  ```

- `UpdateOrganizationInput` - Input for updating an organization
  ```go
  type UpdateOrganizationInput struct {
      LegalName     *string     `json:"legalName,omitempty"`
      LegalDocument *string     `json:"legalDocument,omitempty"`
      Status        *Status     `json:"status,omitempty"`
      Address       *Address    `json:"address,omitempty"`
      Metadata      map[string]any  `json:"metadata,omitempty"`
      Tags          []string    `json:"tags,omitempty"`
  }
  ```

- `CreateLedgerInput` - Input for creating a ledger
- `UpdateLedgerInput` - Input for updating a ledger
- `CreateAccountInput` - Input for creating an account
- `UpdateAccountInput` - Input for updating an account
- `CreateAssetInput` - Input for creating an asset
- `UpdateAssetInput` - Input for updating an asset
- `CreateAssetRateInput` - Input for creating an asset rate
- `UpdateAssetRateInput` - Input for updating an asset rate
- `CreatePortfolioInput` - Input for creating a portfolio
- `UpdatePortfolioInput` - Input for updating a portfolio
- `CreateSegmentInput` - Input for creating a segment
- `UpdateSegmentInput` - Input for updating a segment
- `CreateTransactionInput` - Input for creating a transaction
  ```go
  type CreateTransactionInput struct {
      ExternalID  string                 `json:"externalId,omitempty"`
      Description string                 `json:"description,omitempty"`
      Status      string                 `json:"status,omitempty"`
      AssetCode   string                 `json:"assetCode"`
      Amount      int64                  `json:"amount"`
      Scale       int                    `json:"scale"`
      Operations  []CreateOperationInput `json:"operations"`
      Metadata    map[string]any         `json:"metadata,omitempty"`
      Tags        []string               `json:"tags,omitempty"`
  }
  ```

- `CreateOperationInput` - Input for creating an operation
  ```go
  type CreateOperationInput struct {
      Type        string `json:"type"`
      AccountID   string `json:"accountId"`
      Amount      int64  `json:"amount"`
      AssetCode   string `json:"assetCode"`
      Scale       int    `json:"scale"`
      Description string `json:"description,omitempty"`
  }
  ```

- `UpdateBalanceInput` - Input for updating a balance
  ```go
  type UpdateBalanceInput struct {
      AllowSending   *bool `json:"allowSending,omitempty"`
      AllowReceiving *bool `json:"allowReceiving,omitempty"`
  }
  ```

### Response Models

These structs represent API responses:

- `ListResponse` - Generic paginated response for list operations
  ```go
  type ListResponse[T any] struct {
      Items      []T   `json:"items"`
      Page       int   `json:"page"`
      Limit      int   `json:"limit"`
      TotalItems int   `json:"totalItems"`
      TotalPages int   `json:"totalPages"`
  }
  ```

- `ListOptions` - Options for list operations
  ```go
  type ListOptions struct {
      Page    int               `json:"page,omitempty"`
      Limit   int               `json:"limit,omitempty"`
      Filters map[string]string `json:"filters,omitempty"`
      Sort    map[string]string `json:"sort,omitempty"`
  }
  ```

### Common Types and Constants

- `Status` - Enum for resource status
  ```go
  type Status string

  const (
      StatusActive   Status = "active"
      StatusInactive Status = "inactive"
      StatusPending  Status = "pending"
  )
  ```

- `Address` - Struct for address information
  ```go
  type Address struct {
      Line1      string `json:"line1"`
      Line2      string `json:"line2,omitempty"`
      City       string `json:"city"`
      State      string `json:"state"`
      PostalCode string `json:"postalCode"`
      Country    string `json:"country"`
  }
  ```

- Transaction status constants
  ```go
  const (
      TransactionStatusPending   = "pending"
      TransactionStatusCommitted = "committed"
      TransactionStatusCanceled  = "canceled"
  )
  ```

- Operation type constants
  ```go
  const (
      OperationTypeDebit  = "debit"
      OperationTypeCredit = "credit"
  )
  ```

- Asset type constants
  ```go
  const (
      AssetTypeCurrency = "currency"
      AssetTypeStock    = "stock"
      AssetTypeCrypto   = "crypto"
  )
  ```

## V. Error Handling (`midaz/errors`)

The errors package provides standardized error types and utilities for handling errors in a consistent way.

### Error Types

- `MidazError` - Custom error type with additional context
  ```go
  type MidazError struct {
      Code      string
      Message   string
      Err       error
      Resource  string
      RequestID string
  }
  
  // Implements the error interface
  func (e *MidazError) Error() string {
      if e.Resource != "" {
          return fmt.Sprintf("%s: %s (resource: %s)", e.Code, e.Message, e.Resource)
      }
      return fmt.Sprintf("%s: %s", e.Code, e.Message)
  }
  
  // Implements the Unwrap interface for errors.Is and errors.As
  func (e *MidazError) Unwrap() error {
      return e.Err
  }
  ```

### Standard Error Codes

- `ErrNotFound` - Returned when a resource is not found
  ```go
  var ErrNotFound = &MidazError{
      Code:    "not_found",
      Message: "Resource not found",
  }
  ```

- `ErrValidation` - Returned when a request fails validation
  ```go
  var ErrValidation = &MidazError{
      Code:    "validation_error",
      Message: "Validation failed",
  }
  ```

- `ErrTimeout` - Returned when a request times out
  ```go
  var ErrTimeout = &MidazError{
      Code:    "timeout",
      Message: "Request timed out",
  }
  ```

- `ErrAuthentication` - Returned when authentication fails
  ```go
  var ErrAuthentication = &MidazError{
      Code:    "authentication_error",
      Message: "Authentication failed",
  }
  ```

- `ErrPermission` - Returned when the user does not have permission
  ```go
  var ErrPermission = &MidazError{
      Code:    "permission_error",
      Message: "Permission denied",
  }
  ```

- `ErrRateLimit` - Returned when the API rate limit is exceeded
  ```go
  var ErrRateLimit = &MidazError{
      Code:    "rate_limit_exceeded",
      Message: "Rate limit exceeded",
  }
  ```

- `ErrInternal` - Returned when an unexpected error occurs
  ```go
  var ErrInternal = &MidazError{
      Code:    "internal_error",
      Message: "Internal server error",
  }
  ```

### Transaction-Specific Errors

- `ErrAccountEligibility` - Returned when accounts are not eligible for a transaction
  ```go
  var ErrAccountEligibility = &MidazError{
      Code:    "account_eligibility_error",
      Message: "One or more accounts are not eligible for this transaction",
  }
  ```

- `ErrAssetMismatch` - Returned when accounts have different asset types
  ```go
  var ErrAssetMismatch = &MidazError{
      Code:    "asset_mismatch",
      Message: "Asset mismatch between accounts",
  }
  ```

- `ErrInsufficientBalance` - Returned when a transaction would result in a negative balance
  ```go
  var ErrInsufficientBalance = &MidazError{
      Code:    "insufficient_balance",
      Message: "Insufficient balance for transaction",
  }
  ```

### Error Handling Utilities

- `NewError()` - Creates a new MidazError with the given code and error
  ```go
  func NewError(code string, err error) *MidazError {
      return &MidazError{
          Code:    code,
          Message: err.Error(),
          Err:     err,
      }
  }
  ```

- `NewErrorf()` - Creates a new MidazError with the given code and formatted message
  ```go
  func NewErrorf(code string, format string, args ...interface{}) *MidazError {
      return &MidazError{
          Code:    code,
          Message: fmt.Sprintf(format, args...),
      }
  }
  ```

- `APIErrorToError()` - Converts an internal API error type to a public error
  ```go
  func APIErrorToError(apiErr *APIError) error {
      switch apiErr.Code {
      case "not_found":
          return ErrNotFound
      case "validation_error":
          return ErrValidation
      // ... other error types
      default:
          return ErrInternal
      }
  }
  ```

### Error Type Checking

- `IsNotFoundError()` - Checks if the error is a not found error
  ```go
  func IsNotFoundError(err error) bool {
      var midazErr *MidazError
      if errors.As(err, &midazErr) {
          return midazErr.Code == ErrNotFound.Code
      }
      return false
  }
  ```

- `IsValidationError()` - Checks if the error is a validation error
  ```go
  func IsValidationError(err error) bool {
      var midazErr *MidazError
      if errors.As(err, &midazErr) {
          return midazErr.Code == ErrValidation.Code
      }
      return false
  }
  ```

- `IsAccountEligibilityError()` - Checks if the error is related to account eligibility
- `IsInsufficientBalanceError()` - Checks if the error is an insufficient balance error
- `IsAssetMismatchError()` - Checks if the error is related to asset mismatch
- `IsTimeoutError()` - Checks if the error is related to timeout
- `IsAuthenticationError()` - Checks if the error is related to authentication
- `IsPermissionError()` - Checks if the error is related to permissions
- `IsRateLimitError()` - Checks if the error is related to rate limiting
- `IsInternalError()` - Checks if the error is an internal error

### Transaction Error Utilities

- `FormatTransactionError()` - Produces a standardized error message for transaction errors
  ```go
  func FormatTransactionError(err error) string {
      if IsInsufficientBalanceError(err) {
          return "Transaction failed: Insufficient balance"
      } else if IsAssetMismatchError(err) {
          return "Transaction failed: Asset mismatch between accounts"
      } else if IsAccountEligibilityError(err) {
          return "Transaction failed: One or more accounts are not eligible"
      } else {
          return fmt.Sprintf("Transaction failed: %v", err)
      }
  }
  ```

- `CategorizeTransactionError()` - Provides the error category as a string
- `GetTransactionErrorContext()` - Returns detailed context information for transaction errors
- `IsTransactionRetryable()` - Determines if a transaction error can be safely retried

## VI. Implementation Patterns

This section describes common implementation patterns used throughout the SDK.

### Interface-Based Design

The SDK uses interfaces to define the public API, with private implementations:

```go
// Public interface
type SomeService interface {
    List(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.SomeResource], error)
    Get(ctx context.Context, id string) (*models.SomeResource, error)
    Create(ctx context.Context, input *models.CreateSomeResourceInput) (*models.SomeResource, error)
    Update(ctx context.Context, id string, input *models.UpdateSomeResourceInput) (*models.SomeResource, error)
    Delete(ctx context.Context, id string) error
}

// Private implementation
type someServiceImpl struct {
    client *client.Client
}

// Constructor that returns the interface
func NewSomeService(client *client.Client) SomeService {
    return &someServiceImpl{
        client: client,
    }
}

// Implementation methods
func (s *someServiceImpl) List(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.SomeResource], error) {
    // Implementation
}

// ... other methods
```

This pattern allows for:
- Clean separation between public API and implementation details
- Easier testing through interface mocking
- Future extensibility without breaking changes

### Builder Pattern

The SDK uses the builder pattern for constructing complex objects:

```go
// Example usage
resource, err := builders.NewResource(client).
    WithName("Example").
    WithStatus(models.StatusActive).
    WithMetadata(map[string]any{
        "key": "value",
    }).
    Create(ctx)
```

This pattern provides:
- Progressive disclosure of options
- Method chaining for a fluent API
- Clear separation between construction and execution
- Validation at execution time

### Error Wrapping

The SDK uses error wrapping to preserve context:

```go
func (c *someClient) Get(ctx context.Context, id string) (*models.SomeResource, error) {
    resp, err := c.httpClient.Get(ctx, fmt.Sprintf("/resources/%s", id), nil)
    if err != nil {
        return nil, fmt.Errorf("failed to get resource: %w", err)
    }
    
    // Process response
}
```

This pattern allows:
- Preserving the original error for inspection with `errors.Is` and `errors.As`
- Adding context to errors for better debugging
- Standardized error handling throughout the SDK

### Pagination Handling

The SDK uses a consistent pattern for handling paginated results:

```go
func (c *someClient) List(ctx context.Context, opts *models.ListOptions) (*models.ListResponse[models.SomeResource], error) {
    path := "/resources"
    params := url.Values{}
    
    // Add pagination parameters
    if opts != nil {
        if opts.Page > 0 {
            params.Set("page", strconv.Itoa(opts.Page))
        }
        if opts.Limit > 0 {
            params.Set("limit", strconv.Itoa(opts.Limit))
        }
        
        // Add filters
        for key, value := range opts.Filters {
            params.Set(key, value)
        }
        
        // Add sorting
        for key, value := range opts.Sort {
            params.Set(fmt.Sprintf("sort[%s]", key), value)
        }
    }
    
    // Execute request
    resp, err := c.httpClient.Get(ctx, path, params)
    if err != nil {
        return nil, err
    }
    
    // Parse response
    var result models.ListResponse[models.SomeResource]
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return &result, nil
}
```

This pattern provides:
- Consistent pagination across all list operations
- Support for filtering and sorting
- Clear separation of concerns between pagination, filtering, and API calls