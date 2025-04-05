# Account Transfer Example

This example demonstrates how to use the Midaz Go SDK to perform account transfers using the Transaction DSL. It shows the complete workflow from creating an organization and ledger to transferring funds between accounts.

## What This Example Does

1. **Organization Creation**: Creates a new organization with required fields
2. **Ledger Creation**: Creates a new ledger within the organization
3. **Asset Creation**: Creates a USD asset for use in transactions
4. **Account Creation**: Creates source and destination accounts
5. **Account Transfer**: Transfers $100.00 USD from the source account to the destination account

## Running the Example

### Prerequisites

- Go 1.18 or later
- Access to a running Midaz API (or use mock mode for testing)

### Configuration

Create a `.env` file in the example directory with the following variables:

```
# Midaz API URLs
MIDAZ_ONBOARDING_URL=http://127.0.0.1:3000/v1
MIDAZ_TRANSACTION_URL=http://127.0.0.1:3001/v1

# Authentication
MIDAZ_AUTH_TOKEN=your-auth-token

# Debug mode
MIDAZ_DEBUG=true

# Timeout in seconds
MIDAZ_TIMEOUT=30

# Mock mode (set to true for testing without a running server)
MIDAZ_MOCK_MODE=false
```

### Running the Example

```bash
go run main.go
```

## Key Concepts

### Transaction DSL

The Transaction DSL (Domain Specific Language) is a powerful way to create transactions in Midaz. It allows you to define:

- Source accounts (where funds come from)
- Destination accounts (where funds go to)
- Asset type, amount, and scale
- Metadata and other transaction properties

Example DSL for a simple transfer:

```go
dslInput := &models.TransactionDSLInput{
    Description: "Transfer from source to destination",
    Code:        "TRANSFER",
    Metadata: map[string]any{
        "source":      "go-sdk-example",
        "transferType": "account-to-account",
    },
    Send: &models.DSLSend{
        Asset: "USD",
        Value: 10000, // $100.00
        Scale: 2,     // 2 decimal places
        Source: &models.DSLSource{
            From: []models.DSLFromTo{
                {
                    Account: sourceAccountID,
                },
            },
        },
        Distribute: &models.DSLDistribute{
            To: []models.DSLFromTo{
                {
                    Account: destAccountID,
                },
            },
        },
    },
}
```

### Mock Mode

The example supports a mock mode for testing without a running server. Set `MIDAZ_MOCK_MODE=true` in your `.env` file to enable this mode.

## Error Handling

The example demonstrates proper error handling for:

- Input validation errors
- API errors
- Empty or invalid responses

## Further Reading

For more information on using the Midaz Go SDK, refer to:

- [Midaz Go SDK Documentation](https://docs.midaz.io/go-sdk)
- [Transaction DSL Reference](https://docs.midaz.io/transaction-dsl)
- [Validation Architecture](https://docs.midaz.io/validation)
