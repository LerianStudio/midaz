# Midaz Go SDK: Abstractions Package

The `abstractions` package provides high-level transaction operations for the Midaz platform. It abstracts away the complexities of the underlying Domain-Specific Language (DSL) format used by the Midaz API, offering simplified methods for common transaction types.

## Overview

The abstractions package simplifies transaction creation by:

- Providing intuitive methods for common transaction types (deposits, withdrawals, transfers)
- Handling the complexities of the DSL format internally
- Offering a consistent interface for transaction options
- Validating inputs before sending requests to the API
- Supporting advanced features like idempotency, metadata, and pending transactions

## Core Components

### Abstraction

The central type in this package is the `Abstraction` struct, which provides high-level transaction operations:

```go
// Create an abstraction with a client's CreateTransactionWithDSL method and transactions service
txAbstraction := abstractions.NewAbstraction(
    client.Transactions.CreateTransactionWithDSL,
    client.Transactions,
)

// Use the abstraction to create a deposit
tx, err := txAbstraction.Deposits.Create(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    10000, 2, "USD",
    "Customer deposit",
    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
)
```

## Transaction Types

### Deposit Transactions

Deposits represent money coming into the system from an external source:

```go
// Deposit $100.00 to a customer's account
tx, err := txAbstraction.Deposits.Create(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    10000, 2, "USD",
    "Customer deposit",
    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
)
```

### Withdrawal Transactions

Withdrawals represent money leaving the system to an external destination:

```go
// Withdraw $50.00 from a customer's account
tx, err := txAbstraction.Withdrawals.Create(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    5000, 2, "USD",
    "Customer withdrawal",
    abstractions.WithMetadata(map[string]any{"reference": "WD12345"}),
)
```

### Transfer Transactions

Transfers move funds between two internal accounts:

```go
// Transfer $25.00 between two accounts
tx, err := txAbstraction.Transfers.Create(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    "merchant:acme",
    2500, 2, "USD",
    "Payment for services",
    abstractions.WithMetadata(map[string]any{"reference": "INV12345"}),
)
```

## Transaction Management

The abstraction package provides methods for managing transactions beyond just creation:

### Listing Transactions

```go
// List deposit transactions
deposits, err := txAbstraction.Deposits.List(
    ctx,
    "org-123", "ledger-456",
    &models.ListOptions{
        Limit: 10,
        Offset: 0,
        Filters: map[string]string{
            "status": "completed",
        },
    },
)
```

### Retrieving Transactions

```go
// Get a specific deposit transaction
deposit, err := txAbstraction.Deposits.Get(
    ctx,
    "org-123", "ledger-456",
    "tx-123",
)
```

### Updating Transactions

```go
// Update a deposit transaction
updated, err := txAbstraction.Deposits.Update(
    ctx,
    "org-123", "ledger-456",
    "tx-123",
    &models.UpdateTransactionInput{
        Status: "completed",
        Metadata: map[string]interface{}{
            "processed_by": "system",
        },
    },
)
```

## Transaction Options

The package provides a rich set of options for customizing transactions:

### WithMetadata

Add structured metadata to a transaction:

```go
abstractions.WithMetadata(map[string]any{
    "invoice_id": "INV-123",
    "customer_ref": "CUST-456",
    "channel": "web",
    "payment_method": "credit_card",
})
```

### WithIdempotencyKey

Ensure transaction uniqueness with a client-generated key:

```go
abstractions.WithIdempotencyKey("payment-2023-03-15-12345")
```

### WithPending

Create a transaction in a pending state, requiring explicit commitment:

```go
// Create a pending transaction
tx, err := txAbstraction.Deposits.Create(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    10000, 2, "USD",
    "Customer deposit",
    abstractions.WithPending(true),
)

// Later, after verification or approval:
err = client.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", tx.ID)
```

### Other Options

- `WithExternalID`: Link the transaction to an external system identifier
- `WithChartOfAccountsGroupName`: Categorize the transaction for accounting purposes
- `WithCode`: Add a custom transaction code for categorization
- `WithNotes`: Add detailed notes about the transaction
- `WithRequestID`: Track the transaction with a request identifier

## Error Handling

The package provides utilities for handling transaction errors:

```go
if err != nil {
    if abstractions.IsInsufficientBalanceError(err) {
        // Handle insufficient balance error
    } else if abstractions.IsValidationError(err) {
        // Handle validation error
    } else {
        // Handle other errors
    }
}
