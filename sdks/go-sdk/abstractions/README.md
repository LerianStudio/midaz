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
// Create an abstraction with a client's CreateTransactionWithDSL method
txAbstraction := abstractions.NewAbstraction(client.CreateTransactionWithDSL)

// Use the abstraction to create a deposit
tx, err := txAbstraction.CreateDeposit(
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
tx, err := txAbstraction.CreateDeposit(
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
tx, err := txAbstraction.CreateWithdrawal(
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
tx, err := txAbstraction.CreateTransfer(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    "merchant:acme",
    2500, 2, "USD",
    "Payment for services",
    abstractions.WithMetadata(map[string]any{"reference": "INV12345"}),
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

Add an idempotency key to ensure transaction uniqueness:

```go
// Using a UUID as an idempotency key
import "github.com/google/uuid"

idempotencyKey := uuid.New().String()
abstractions.WithIdempotencyKey(idempotencyKey)
```

### WithPending

Mark a transaction as pending, requiring explicit commitment later:

```go
// Create a pending transaction
tx, err := txAbstraction.CreateTransfer(
    ctx, orgID, ledgerID,
    sourceAccount, targetAccount,
    amount, scale, asset, description,
    abstractions.WithPending(true),
)

// Later, after verification or approval:
err = client.Transactions.CommitTransaction(ctx, orgID, ledgerID, tx.ID)
```

### WithExternalID

Set an external ID for the transaction:

```go
abstractions.WithExternalID("PO-12345")
```

### WithCode

Set a custom transaction code:

```go
abstractions.WithCode("SUBS-RENEW")
```

### WithNotes

Add detailed notes to a transaction:

```go
abstractions.WithNotes("Customer requested refund due to damaged product.")
```

### WithChartOfAccountsGroupName

Set the chart of accounts group for accounting integration:

```go
abstractions.WithChartOfAccountsGroupName("revenue:subscription")
```

### WithRequestID

Attach a unique request ID for tracking:

```go
abstractions.WithRequestID("req-abc-123-xyz")
```

## Advanced Usage

### Implementing an Approval Workflow

```go
// Create a pending high-value transfer
tx, err := txAbstraction.CreateTransfer(
    ctx, orgID, ledgerID,
    "account:treasury", "account:investments",
    1000000, 2, "USD", // $10,000.00
    "Investment allocation",
    abstractions.WithPending(true),
    abstractions.WithMetadata(map[string]any{
        "requires_approval": true,
        "approval_level": "director",
        "requested_by": "jane.doe",
    }),
)

// Store the transaction ID for the approval process
pendingTransactionID := tx.ID

// Later, in the approval handler:
if approved {
    err = client.Transactions.CommitTransaction(ctx, orgID, ledgerID, pendingTransactionID)
} else {
    err = client.Transactions.DeleteTransaction(ctx, orgID, ledgerID, pendingTransactionID)
}
```

### Error Handling

All transaction methods validate inputs and return descriptive errors:

```go
tx, err := txAbstraction.CreateDeposit(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    10000, 2, "USD",
    "Customer deposit",
)

if err != nil {
    // Handle specific error types
    switch {
    case strings.Contains(err.Error(), "amount must be greater than zero"):
        // Handle invalid amount
    case strings.Contains(err.Error(), "asset code is required"):
        // Handle missing asset code
    default:
        // Handle other errors
    }
    return err
}
```

## Integration with the Client

The abstractions package is typically used through the TransactionService in the client package:

```go
// Create a client
client, err := midaz.New(midaz.WithAuthToken("your-auth-token"))
if err != nil {
    log.Fatalf("Failed to create client: %v", err)
}

// Use the client's transaction service to create a deposit
tx, err := client.Transactions.CreateDeposit(
    ctx,
    "org-123", "ledger-456",
    "customer:john.doe",
    10000, 2, "USD",
    "Customer deposit",
    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
)
```

## Testing Support

The package includes testing helpers to facilitate unit testing with the abstractions:

```go
// Create a mock abstraction for testing
mockAbstraction := abstractions.NewMockAbstraction()

// Configure the mock to return a specific transaction
mockAbstraction.On("CreateDeposit", mock.Anything, "org-123", "ledger-456", "account:test", int64(1000), 2, "USD", "Test deposit", mock.Anything).
    Return(&models.Transaction{ID: "tx-123", Status: models.StatusCompleted}, nil)

// Use the mock in your tests
tx, err := mockAbstraction.CreateDeposit(
    context.Background(),
    "org-123", "ledger-456",
    "account:test",
    1000, 2, "USD",
    "Test deposit",
)
```
