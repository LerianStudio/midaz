# Midaz Go SDK: Entities Package

The `entities` package provides access to the Midaz API resources and operations. It implements service interfaces for interacting with accounts, assets, ledgers, transactions, and other Midaz platform resources.

## Overview

The entities package serves as the core implementation layer of the Midaz Go SDK, providing:

- Service interfaces for all Midaz API resources
- HTTP client implementation for API communication
- Error handling and response parsing
- Pagination and filtering support
- Validation of required parameters

## Core Components

### Entity

The central type in this package is the `Entity` struct, which provides access to all service interfaces:

```go
// Create a new entity with client configuration
entity, err := entities.NewEntity(
    httpClient,
    authToken,
    map[string]string{
        "onboarding": "https://api.midaz.io/v1",
        "transaction": "https://api.midaz.io/v1",
    },
)

// Use entity services
account, err := entity.Accounts.GetAccount(ctx, "org-123", "ledger-456", "account-789")
```

### HTTP Client

The package includes an internal HTTP client that handles:

- Authentication with bearer tokens
- Request/response serialization
- Error handling and mapping
- Debug logging
- Common headers

## Service Interfaces

### AccountsService

Manages account resources:

```go
// List accounts in a ledger
accounts, err := entity.Accounts.ListAccounts(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
    Filter: map[string]string{"status": "ACTIVE"},
})

// Get an account by ID
account, err := entity.Accounts.GetAccount(ctx, "org-123", "ledger-456", "account-789")

// Get an account by alias
account, err := entity.Accounts.GetAccountByAlias(ctx, "org-123", "ledger-456", "customer:john.doe")

// Create a new account
account, err := entity.Accounts.CreateAccount(ctx, "org-123", "ledger-456", &models.CreateAccountInput{
    Name:      "Checking Account",
    AssetCode: "USD",
    Type:      "ASSET",
})

// Update an account
account, err := entity.Accounts.UpdateAccount(ctx, "org-123", "ledger-456", "account-789", &models.UpdateAccountInput{
    Name:   "Updated Account Name",
    Status: "INACTIVE",
})

// Delete an account
err := entity.Accounts.DeleteAccount(ctx, "org-123", "ledger-456", "account-789")

// Get an account's balance
balance, err := entity.Accounts.GetBalance(ctx, "org-123", "ledger-456", "account-789")
```

### AssetsService

Manages asset resources:

```go
// List assets in a ledger
assets, err := entity.Assets.ListAssets(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
})

// Get an asset by ID
asset, err := entity.Assets.GetAsset(ctx, "org-123", "ledger-456", "asset-789")

// Create a new asset
asset, err := entity.Assets.CreateAsset(ctx, "org-123", "ledger-456", &models.CreateAssetInput{
    Name: "US Dollar",
    Code: "USD",
})

// Update an asset
asset, err := entity.Assets.UpdateAsset(ctx, "org-123", "ledger-456", "asset-789", &models.UpdateAssetInput{
    Name: "Updated Asset Name",
})

// Delete an asset
err := entity.Assets.DeleteAsset(ctx, "org-123", "ledger-456", "asset-789")
```

### AssetRatesService

Manages asset rate resources:

```go
// List asset rates in a ledger
rates, err := entity.AssetRates.ListAssetRates(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
})

// Get an asset rate by ID
rate, err := entity.AssetRates.GetAssetRate(ctx, "org-123", "ledger-456", "rate-789")

// Create a new asset rate
rate, err := entity.AssetRates.CreateAssetRate(ctx, "org-123", "ledger-456", &models.CreateAssetRateInput{
    BaseAsset:  "USD",
    QuoteAsset: "EUR",
    Rate:       0.85,
})

// Delete an asset rate
err := entity.AssetRates.DeleteAssetRate(ctx, "org-123", "ledger-456", "rate-789")
```

### BalancesService

Manages balance resources:

```go
// List balances in a ledger
balances, err := entity.Balances.ListBalances(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
})

// Get a balance by ID
balance, err := entity.Balances.GetBalance(ctx, "org-123", "ledger-456", "balance-789")

// Get a balance by account ID
balance, err := entity.Balances.GetBalanceByAccount(ctx, "org-123", "ledger-456", "account-789")
```

### LedgersService

Manages ledger resources:

```go
// List ledgers in an organization
ledgers, err := entity.Ledgers.ListLedgers(ctx, "org-123", &models.ListOptions{
    Limit: 10,
})

// Get a ledger by ID
ledger, err := entity.Ledgers.GetLedger(ctx, "org-123", "ledger-456")

// Create a new ledger
ledger, err := entity.Ledgers.CreateLedger(ctx, "org-123", &models.CreateLedgerInput{
    Name: "Main Ledger",
})

// Update a ledger
ledger, err := entity.Ledgers.UpdateLedger(ctx, "org-123", "ledger-456", &models.UpdateLedgerInput{
    Name: "Updated Ledger Name",
})

// Delete a ledger
err := entity.Ledgers.DeleteLedger(ctx, "org-123", "ledger-456")
```

### OperationsService

Manages operation resources:

```go
// List operations for an account
operations, err := entity.Operations.ListOperations(ctx, "org-123", "ledger-456", "account-789", &models.ListOptions{
    Limit: 10,
})

// Get an operation by ID
operation, err := entity.Operations.GetOperation(ctx, "org-123", "ledger-456", "account-789", "operation-abc")

// Update an operation
operation, err := entity.Operations.UpdateOperation(ctx, "org-123", "ledger-456", "tx-123", "operation-abc", &models.UpdateOperationInput{
    Metadata: map[string]any{"status": "reconciled"},
})
```

### OrganizationsService

Manages organization resources:

```go
// List organizations
organizations, err := entity.Organizations.ListOrganizations(ctx, &models.ListOptions{
    Limit: 10,
})

// Get an organization by ID
organization, err := entity.Organizations.GetOrganization(ctx, "org-123")

// Create a new organization
organization, err := entity.Organizations.CreateOrganization(ctx, &models.CreateOrganizationInput{
    Name: "ACME Corporation",
})

// Update an organization
organization, err := entity.Organizations.UpdateOrganization(ctx, "org-123", &models.UpdateOrganizationInput{
    Name: "Updated Organization Name",
})

// Delete an organization
err := entity.Organizations.DeleteOrganization(ctx, "org-123")
```

### PortfoliosService

Manages portfolio resources:

```go
// List portfolios in a ledger
portfolios, err := entity.Portfolios.ListPortfolios(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
})

// Get a portfolio by ID
portfolio, err := entity.Portfolios.GetPortfolio(ctx, "org-123", "ledger-456", "portfolio-789")

// Create a new portfolio
portfolio, err := entity.Portfolios.CreatePortfolio(ctx, "org-123", "ledger-456", &models.CreatePortfolioInput{
    Name: "Investment Portfolio",
})

// Update a portfolio
portfolio, err := entity.Portfolios.UpdatePortfolio(ctx, "org-123", "ledger-456", "portfolio-789", &models.UpdatePortfolioInput{
    Name: "Updated Portfolio Name",
})

// Delete a portfolio
err := entity.Portfolios.DeletePortfolio(ctx, "org-123", "ledger-456", "portfolio-789")
```

### SegmentsService

Manages segment resources:

```go
// List segments in a portfolio
segments, err := entity.Segments.ListSegments(ctx, "org-123", "ledger-456", "portfolio-789", &models.ListOptions{
    Limit: 10,
})

// Get a segment by ID
segment, err := entity.Segments.GetSegment(ctx, "org-123", "ledger-456", "portfolio-789", "segment-abc")

// Create a new segment
segment, err := entity.Segments.CreateSegment(ctx, "org-123", "ledger-456", "portfolio-789", &models.CreateSegmentInput{
    Name: "Retail Segment",
})

// Update a segment
segment, err := entity.Segments.UpdateSegment(ctx, "org-123", "ledger-456", "portfolio-789", "segment-abc", &models.UpdateSegmentInput{
    Name: "Updated Segment Name",
})

// Delete a segment
err := entity.Segments.DeleteSegment(ctx, "org-123", "ledger-456", "portfolio-789", "segment-abc")
```

### TransactionsService

Manages transaction resources:

```go
// List transactions in a ledger
transactions, err := entity.Transactions.ListTransactions(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit: 10,
})

// Get a transaction by ID
transaction, err := entity.Transactions.GetTransaction(ctx, "org-123", "ledger-456", "tx-123")

// Create a transaction with standard format
transaction, err := entity.Transactions.CreateTransaction(ctx, "org-123", "ledger-456", &models.CreateTransactionInput{
    Entries: []models.TransactionEntry{
        {
            AccountID: "account-source",
            Amount:    -10000,
            AssetCode: "USD",
        },
        {
            AccountID: "account-target",
            Amount:    10000,
            AssetCode: "USD",
        },
    },
    Description: "Transfer between accounts",
})

// Create a transaction with DSL format
transaction, err := entity.Transactions.CreateTransactionWithDSL(ctx, "org-123", "ledger-456", &models.TransactionDSLInput{
    Script: `
        send {
            asset: "USD",
            amount: 100.00,
            from: "account:source",
            to: "account:target"
        }
    `,
    Metadata: map[string]any{"reference": "TX12345"},
})

// Update a transaction
transaction, err := entity.Transactions.UpdateTransaction(ctx, "org-123", "ledger-456", "tx-123", &models.UpdateTransactionInput{
    Description: "Updated transaction description",
})

// Commit a pending transaction
transaction, err := entity.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", "tx-123")

// Commit a pending transaction by external ID
transaction, err := entity.Transactions.CommitTransactionWithExternalID(ctx, "org-123", "ledger-456", "ext-tx-123")
```

## Error Handling

The entities package provides consistent error handling across all services:

```go
account, err := entity.Accounts.GetAccount(ctx, "org-123", "ledger-456", "account-789")
if err != nil {
    // Handle specific error types
    switch {
    case strings.Contains(err.Error(), "not found"):
        // Handle not found error
    case strings.Contains(err.Error(), "permission denied"):
        // Handle permission error
    default:
        // Handle other errors
    }
    return err
}
```

## Pagination and Filtering

All list operations support pagination and filtering through the `ListOptions` type:

```go
// List with pagination
accounts, err := entity.Accounts.ListAccounts(ctx, "org-123", "ledger-456", &models.ListOptions{
    Limit:  10,
    Offset: 20,
})

// List with filtering
accounts, err := entity.Accounts.ListAccounts(ctx, "org-123", "ledger-456", &models.ListOptions{
    Filter: map[string]string{
        "status": "ACTIVE",
        "type":   "ASSET",
    },
})

// List with sorting
accounts, err := entity.Accounts.ListAccounts(ctx, "org-123", "ledger-456", &models.ListOptions{
    Sort: "created_at:desc",
})
```

## Integration with the Client

The entities package is typically used through the client package, which provides a higher-level interface:

```go
// Create a client
client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}

// Use the client's services
account, err := client.Accounts.GetAccount(ctx, "org-123", "ledger-456", "account-789")
```
