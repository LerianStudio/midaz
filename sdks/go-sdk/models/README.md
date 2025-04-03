# Midaz Go SDK: Models Package

The `models` package defines the data models used by the Midaz SDK. It provides structures that either directly align with backend types from `pkg/mmodel` where possible, or implement SDK-specific types only where necessary.

## Overview

The models package serves as the foundation of the Midaz Go SDK, providing:

- Data structures representing core Midaz resources
- Input/output models for API operations
- Validation methods for ensuring data integrity
- Conversion utilities between SDK and backend models
- Common types used across the SDK

## Core Model Types

### Account

Represents an account in the Midaz system, which is a fundamental entity for tracking assets and balances:

```go
// Get an account
account := &models.Account{
    ID:          "account-123",
    Name:        "Checking Account",
    Alias:       "customer:john.doe",
    AssetCode:   "USD",
    Type:        "ASSET",
    Status:      models.NewStatus("ACTIVE"),
    Description: "Primary checking account",
}

// Create account input
input := &models.CreateAccountInput{
    Name:      "Savings Account",
    AssetCode: "USD",
    Type:      "ASSET",
}
```

### Asset

Represents a type of value that can be tracked and transferred within the Midaz system:

```go
// Get an asset
asset := &models.Asset{
    ID:   "asset-123",
    Name: "US Dollar",
    Code: "USD",
}

// Create asset input
input := &models.CreateAssetInput{
    Name: "Euro",
    Code: "EUR",
}
```

### Balance

Represents the current state of an account's holdings for a specific asset:

```go
// Get a balance
balance := &models.Balance{
    ID:        "balance-123",
    AccountID: "account-456",
    AssetCode: "USD",
    Amount:    10000, // $100.00
    Scale:     2,
}
```

### Ledger

Represents a collection of accounts and transactions within an organization:

```go
// Get a ledger
ledger := &models.Ledger{
    ID:   "ledger-123",
    Name: "Main Ledger",
}

// Create ledger input
input := &models.CreateLedgerInput{
    Name: "Secondary Ledger",
}
```

### Organization

Represents a business entity that owns ledgers, accounts, and other resources:

```go
// Get an organization
organization := &models.Organization{
    ID:   "org-123",
    Name: "ACME Corporation",
}

// Create organization input
input := &models.CreateOrganizationInput{
    Name: "New Company LLC",
}
```

### Portfolio

Represents a collection of accounts that belong to a specific entity within an organization and ledger:

```go
// Get a portfolio
portfolio := &models.Portfolio{
    ID:   "portfolio-123",
    Name: "Investment Portfolio",
}

// Create portfolio input
input := &models.CreatePortfolioInput{
    Name: "Retirement Portfolio",
}
```

### Segment

Represents a categorization unit for more granular organization of accounts:

```go
// Get a segment
segment := &models.Segment{
    ID:   "segment-123",
    Name: "Retail Segment",
}

// Create segment input
input := &models.CreateSegmentInput{
    Name: "Corporate Segment",
}
```

### Transaction

Represents a financial event that affects one or more accounts through operations:

```go
// Get a transaction
transaction := &models.Transaction{
    ID:        "tx-123",
    Amount:    10000,
    Scale:     2,
    AssetCode: "USD",
    Status:    models.NewStatus("COMPLETED"),
}

// Create transaction with standard format
input := &models.CreateTransactionInput{
    Amount:    10000,
    Scale:     2,
    AssetCode: "USD",
    Operations: []models.CreateOperationInput{
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
}

// Create transaction with DSL format
dslInput := &models.TransactionDSLInput{
    Send: &models.DSLSend{
        Asset: "USD",
        Value: 10000,
        Scale: 2,
        Source: &models.DSLSource{
            From: []models.DSLFromTo{
                {Account: "account:source"},
            },
        },
        Distribute: &models.DSLDistribute{
            To: []models.DSLFromTo{
                {Account: "account:target"},
            },
        },
    },
    Description: "Transfer between accounts",
}
```

### Operation

Represents an individual accounting entry within a transaction:

```go
// Get an operation
operation := &models.Operation{
    ID:        "op-123",
    AccountID: "account-456",
    Amount:    10000,
    Scale:     2,
    AssetCode: "USD",
}
```

## Common Types

### Status

Represents the status of an entity in the Midaz system:

```go
// Create a status
status := models.NewStatus("ACTIVE")

// Add a description
status = status.WithDescription("Account is active and operational")
```

### Address

Represents a physical address:

```go
// Create an address
address := models.NewAddress(
    "123 Main St",
    "12345",
    "Anytown",
    "CA",
    "US",
)

// Add optional line2
address = address.WithLine2("Suite 100")
```

### ListOptions

Represents options for list operations, including pagination, filtering, and sorting:

```go
// Create list options with pagination
options := &models.ListOptions{
    Limit:  10,
    Offset: 20,
}

// Add filtering
options.Filters = map[string]string{
    "status": "ACTIVE",
    "type":   "ASSET",
}

// Add sorting
options.OrderBy = "createdAt"
options.OrderDirection = "desc"

// Convert to query parameters
params := options.ToQueryParams()
```

### ListResponse

Represents a paginated response from a list operation:

```go
// Process a list response
response := &models.ListResponse[models.Account]{
    Items: []models.Account{
        // Account objects
    },
    Pagination: models.Pagination{
        Limit:  10,
        Offset: 20,
        Total:  100,
    },
}

// Access items and pagination
accounts := response.Items
totalAccounts := response.Pagination.Total
```

## Validation

Many model types include validation methods to ensure data integrity:

```go
// Validate transaction input
input := &models.CreateTransactionInput{
    // ... set fields
}
if err := input.Validate(); err != nil {
    // Handle validation error
}

// Validate DSL transaction input
dslInput := &models.TransactionDSLInput{
    // ... set fields
}
if err := dslInput.Validate(); err != nil {
    // Handle validation error
}
```

## Model Conversion

The package provides utilities for converting between SDK and backend models:

```go
// Convert from backend model to SDK model
sdkStatus := models.FromMmodelStatus(backendStatus)

// Convert from SDK model to backend model
backendStatus := sdkStatus.ToMmodelStatus()
```

## Integration with Other Packages

The models package is used throughout the SDK:

- **entities**: Uses models for API request/response structures
- **builders**: Uses models for input/output parameters
- **abstractions**: Uses models for transaction operations
- **client**: Uses models for service method parameters and return values

## Thread Safety

All models in this package are designed to be used in a single goroutine and are not thread-safe by default. When sharing models across goroutines, proper synchronization should be implemented.
