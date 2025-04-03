# Midaz Go SDK: Builders Package

The `builders` package provides fluent builder interfaces for the Midaz SDK, implementing the builder pattern to simplify the creation and manipulation of Midaz resources through a chainable API.

## Overview

The builders package offers a more intuitive and developer-friendly way to interact with the Midaz API by:

- Providing method chaining for progressive configuration
- Enforcing required parameters through compile-time type checking
- Offering clear validation with descriptive error messages
- Simplifying complex operations with a consistent interface

## Main Builder Types

The package provides a central `Builder` type that acts as a factory for creating specific builders:

```go
// Create a new builder with a client
builder := builders.NewBuilder(
    client, // AccountClientInterface
    client, // AssetClientInterface
    client, // AssetRateClientInterface
    client, // BalanceClientInterface
    client, // LedgerClientInterface
    client, // OrganizationClientInterface
    client, // PortfolioClientInterface
    client, // SegmentClientInterface
    client, // ClientInterface (for transactions)
)
```

## Resource Builders

### Account Builders

Create and update account resources:

```go
// Create an account
account, err := builder.
    NewAccount().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithName("Checking Account").
    WithAssetCode("USD").
    WithType("ASSET").
    Create(context.Background())

// Update an account
updatedAccount, err := builder.
    NewAccountUpdate("org-123", "ledger-456", "account-789").
    WithName("Updated Account Name").
    WithStatus("INACTIVE").
    Update(context.Background())
```

### Asset Builders

Create and update asset resources:

```go
// Create an asset
asset, err := builder.
    NewAsset().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithName("US Dollar").
    WithCode("USD").
    Create(context.Background())

// Update an asset
updatedAsset, err := builder.
    NewAssetUpdate("org-123", "ledger-456", "asset-789").
    WithName("Updated Asset Name").
    Update(context.Background())
```

### Asset Rate Builders

Create asset rate resources:

```go
// Create an asset rate
assetRate, err := builder.
    NewAssetRate().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithBaseAsset("USD").
    WithQuoteAsset("EUR").
    WithRate(0.85).
    Create(context.Background())
```

### Ledger Builders

Create and update ledger resources:

```go
// Create a ledger
ledger, err := builder.
    NewLedger().
    WithOrganization("org-123").
    WithName("Main Ledger").
    Create(context.Background())

// Update a ledger
updatedLedger, err := builder.
    NewLedgerUpdate("org-123", "ledger-456").
    WithName("Updated Ledger Name").
    Update(context.Background())
```

### Organization Builders

Create and update organization resources:

```go
// Create an organization
organization, err := builder.
    NewOrganization().
    WithName("ACME Corporation").
    Create(context.Background())

// Update an organization
updatedOrg, err := builder.
    NewOrganizationUpdate("org-123").
    WithName("Updated Organization Name").
    Update(context.Background())
```

### Portfolio Builders

Create and update portfolio resources:

```go
// Create a portfolio
portfolio, err := builder.
    NewPortfolio().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithName("Investment Portfolio").
    Create(context.Background())

// Update a portfolio
updatedPortfolio, err := builder.
    NewPortfolioUpdate("org-123", "ledger-456", "portfolio-789").
    WithName("Updated Portfolio Name").
    Update(context.Background())
```

### Segment Builders

Create and update segment resources:

```go
// Create a segment
segment, err := builder.
    NewSegment().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithName("Retail Segment").
    Create(context.Background())

// Update a segment
updatedSegment, err := builder.
    NewSegmentUpdate("org-123", "ledger-456", "portfolio-789", "segment-abc").
    WithName("Updated Segment Name").
    WithStatus("INACTIVE").
    Update(context.Background())
```

## Transaction Builders

The builders package provides specialized builders for different transaction types:

### Deposit Builder

Create deposit transactions that increase account balances:

```go
// Create a deposit transaction
tx, err := builder.
    NewDeposit().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(10000, 2). // $100.00
    WithAssetCode("USD").
    WithDescription("Customer deposit").
    WithMetadata(map[string]any{
        "reference": "invoice-789",
        "channel": "web",
    }).
    WithTag("customer-deposit").
    WithExternalID("ext-deposit-123").
    WithIdempotencyKey("deposit-idempotency-key-123").
    ToAccount("customer:john.doe").
    Execute(context.Background())
```

### Withdrawal Builder

Create withdrawal transactions that decrease account balances:

```go
// Create a withdrawal transaction
tx, err := builder.
    NewWithdrawal().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(5000, 2). // $50.00
    WithAssetCode("USD").
    WithDescription("Customer withdrawal").
    WithMetadata(map[string]any{
        "reference": "withdrawal-789",
        "channel": "mobile",
    }).
    WithExternalID("ext-withdrawal-123").
    FromAccount("customer:john.doe").
    Execute(context.Background())
```

### Transfer Builder

Create transfer transactions that move funds between accounts:

```go
// Create a transfer transaction
tx, err := builder.
    NewTransfer().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(2500, 2). // $25.00
    WithAssetCode("USD").
    WithDescription("Transfer between accounts").
    WithMetadata(map[string]any{
        "reference": "transfer-789",
        "purpose": "monthly-allocation",
    }).
    WithIdempotencyKey("transfer-idempotency-key-123").
    FromAccount("customer:john.doe").
    ToAccount("merchant:acme").
    Execute(context.Background())
```

## Common Options

All transaction builders support the following common options:

### Required Parameters

- `WithOrganization(orgID string)` - Set the organization ID
- `WithLedger(ledgerID string)` - Set the ledger ID
- `WithAmount(amount int64, scale int)` - Set the transaction amount and scale
- `WithAssetCode(assetCode string)` - Set the asset code

### Optional Parameters

- `WithDescription(description string)` - Add a human-readable description
- `WithMetadata(metadata map[string]any)` - Add metadata key-value pairs
- `WithTag(tag string)` - Add a single tag
- `WithTags(tags []string)` - Add multiple tags
- `WithExternalID(externalID string)` - Set an external ID for reference
- `WithIdempotencyKey(key string)` - Set an idempotency key for safe retries

## Error Handling

All builder methods that perform API operations (like `Create()`, `Update()`, or `Execute()`) return both the result and an error. Possible error types include:

- Validation errors (missing required parameters)
- Authentication errors
- Permission errors
- Not found errors (organization, ledger, account not found)
- Internal errors

Example of proper error handling:

```go
tx, err := builder.
    NewDeposit().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(10000, 2).
    WithAssetCode("USD").
    ToAccount("customer:john.doe").
    Execute(context.Background())

if err != nil {
    // Handle specific error types
    switch {
    case errors.Is(err, errors.ErrValidation):
        // Handle validation errors (missing required fields)
        fmt.Printf("Validation error: %v\n", err)
    case errors.Is(err, errors.ErrAuthentication):
        // Handle authentication errors (invalid token)
        fmt.Printf("Authentication error: %v\n", err)
    case errors.Is(err, errors.ErrPermission):
        // Handle permission errors (insufficient privileges)
        fmt.Printf("Permission error: %v\n", err)
    case errors.Is(err, errors.ErrNotFound):
        // Handle not found errors (invalid organization, ledger, or account)
        fmt.Printf("Resource not found: %v\n", err)
    case strings.Contains(err.Error(), "insufficient funds"):
        // Handle insufficient funds errors
        fmt.Printf("Insufficient funds: %v\n", err)
    default:
        // Handle other errors
        fmt.Printf("Unexpected error: %v\n", err)
    }
    return err
}

// Use the transaction
fmt.Printf("Created deposit transaction: %s (status: %s)\n", tx.ID, tx.Status)
```

## Advanced Usage Examples

### Handling Idempotency

```go
// Attempt to create a transfer with an existing idempotency key
tx, err := builder.
    NewTransfer().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(5000, 2). // $50.00
    WithAssetCode("USD").
    WithDescription("Recurring payment").
    WithIdempotencyKey("payment-may-2023").
    FromAccount("customer-account").
    ToAccount("revenue-account").
    Execute(context.Background())

if err != nil {
    if strings.Contains(err.Error(), "idempotency key already exists") {
        // If the transaction already exists, retrieve it
        existingTx, findErr := client.Transactions.GetTransactionByIdempotencyKey(
            context.Background(),
            "org-123",
            "ledger-456",
            "payment-may-2023",
        )

        if findErr == nil {
            // Use the existing transaction
            fmt.Printf("Found existing transaction: %s (created: %s)\n",
                existingTx.ID, existingTx.CreatedAt)
            return nil
        }
    }
    return fmt.Errorf("transfer failed: %w", err)
}

fmt.Printf("Created new transaction: %s\n", tx.ID)
```

### Creating Pending Transactions

```go
// Create a pending high-value transfer
tx, err := builder.
    NewTransfer().
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(1000000, 2). // $10,000.00
    WithAssetCode("USD").
    WithDescription("Investment allocation").
    WithMetadata(map[string]any{
        "requires_approval": true,
        "approval_level": "director",
        "requested_by": "jane.doe",
    }).
    WithPending(true). // Mark as pending
    FromAccount("account:treasury").
    ToAccount("account:investments").
    Execute(context.Background())

// Store the transaction ID for the approval process
pendingTransactionID := tx.ID

// Later, in the approval handler:
if approved {
    err = client.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", pendingTransactionID)
} else {
    err = client.Transactions.DeleteTransaction(ctx, "org-123", "ledger-456", pendingTransactionID)
}
```

## Direct Builder Creation

While the main `Builder` type is the recommended way to access builders, you can also create specific builders directly:

```go
// Create a deposit builder directly
deposit := builders.NewDeposit(client)

// Configure and execute the deposit
tx, err := deposit.
    WithOrganization("org-123").
    WithLedger("ledger-456").
    WithAmount(1000, 2).
    WithAssetCode("USD").
    ToAccount("customer-account").
    Execute(context.Background())
```

## Thread Safety

All builders in this package are designed to be used in a single goroutine and are not thread-safe. Create a new builder instance for each concurrent operation.
