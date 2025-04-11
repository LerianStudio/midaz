# Implementing Transactions

**Navigation:** [Home](../) > [Tutorials](./README.md) > Implementing Transactions

This tutorial provides a comprehensive guide to implementing financial transactions in the Midaz platform. You'll learn how to create, execute, and manage different types of transactions using both the API and the Transaction DSL.

## Table of Contents

- [Introduction](#introduction)
- [Transaction Model](#transaction-model)
- [Transaction Processing Flow](#transaction-processing-flow)
- [Transaction DSL](#transaction-dsl)
- [Creating Simple Transactions](#creating-simple-transactions)
- [Advanced Transaction Patterns](#advanced-transaction-patterns)
- [Error Handling](#error-handling)
- [Testing Transactions](#testing-transactions)
- [Best Practices](#best-practices)
- [References](#references)

## Introduction

Transactions are the core of any financial system. In Midaz, transactions represent the movement of funds between accounts and are implemented with double-entry bookkeeping principles to ensure financial integrity. This tutorial will guide you through the process of implementing various types of transactions in Midaz.

### Prerequisites

Before implementing transactions, you should:

1. Have a [financial structure](./creating-financial-structures.md) set up with organizations, ledgers, assets, and accounts
2. Understand the [entity hierarchy](../domain-models/entity-hierarchy.md) of Midaz
3. Be familiar with the [financial model](../domain-models/financial-model.md) principles

## Transaction Model

A transaction in Midaz consists of several key components:

1. **Transaction Entity**: The top-level record containing metadata and status
2. **Operations**: Individual debit/credit entries that affect account balances
3. **Source/Destination**: Accounts involved in the transaction
4. **Amounts**: Values being transferred between accounts
5. **Asset Codes**: The currencies or assets involved

*Note: Transaction model diagram will be provided in a future documentation update.*

### Key Transaction Fields

| Field | Description |
|-------|-------------|
| `id` | Unique identifier for the transaction |
| `externalId` | Optional external reference ID |
| `ledgerId` | ID of the ledger where the transaction takes place |
| `organizationId` | ID of the organization that owns the transaction |
| `operationIds` | IDs of the operations created by this transaction |
| `type` | Type of transaction (e.g., transfer, payment) |
| `status` | Current status (e.g., pending, completed, failed) |
| `metadata` | Custom key-value pairs for additional information |
| `createdAt` | Timestamp when the transaction was created |
| `updatedAt` | Timestamp when the transaction was last updated |

## Transaction Processing Flow

Transactions in Midaz follow a well-defined processing flow:

1. **Request Submission**: Transaction request is submitted via API or using the DSL
2. **Validation**: The transaction is validated for integrity and correctness
3. **Creation**: The transaction record is created in the database
4. **Operations Generation**: Individual operations are created for each account entry
5. **Balance Updates**: Account balances are updated based on operations
6. **Status Updates**: Transaction status is updated throughout the process
7. **Completion**: The transaction is marked as completed or failed

This flow is managed by the Transaction Service, which uses a Balance-Transaction-Operation (BTO) pattern to maintain consistency and atomicity.

*Note: Transaction processing flow diagram will be provided in a future documentation update.*

## Transaction DSL

Midaz provides a domain-specific language (DSL) for defining transactions. The Transaction DSL offers a powerful, expressive way to define complex transactions.

### Basic DSL Syntax

The Transaction DSL uses a structured syntax with the following basic structure:

```
transaction "Name" {
  description "Description of the transaction"
  code "TRANSACTION_CODE"
  
  send USD 100.00 {
    source {
      from "account-id" {
        chart_of_accounts "ACCOUNT_CODE"
        description "Description of the source"
      }
    }
    
    distribute {
      to "account-id" {
        chart_of_accounts "ACCOUNT_CODE"
        description "Description of the destination"
      }
    }
  }
}
```

### DSL Elements

| Element | Description | Example |
|---------|-------------|---------|
| `transaction` | The transaction container | `transaction "Payment" { ... }` |
| `send` | Defines a fund transfer with asset and amount | `send USD 100.00 { ... }` |
| `source` | Container for source accounts | `source { ... }` |
| `from` | Specifies which account to debit | `from "@account123" { ... }` |
| `distribute` | Container for destination accounts | `distribute { ... }` |
| `to` | Specifies which account to credit | `to "@account456" { ... }` |
| `chart_of_accounts` | Account classification code | `chart_of_accounts "1000"` |
| `description` | Human-readable description | `description "Payment for invoice"` |
| `metadata` | Container for key-value metadata | `metadata { ("key" "value") }` |

## Creating Simple Transactions

Let's start by implementing basic transactions in Midaz.

### Direct Transfer Between Accounts

This example shows a simple transfer of funds from one account to another.

#### Using API (JSON)

```json
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json

{
  "type": "transfer",
  "sources": [
    {
      "accountId": "source-account-id",
      "amount": "100.00",
      "assetCode": "USD"
    }
  ],
  "destinations": [
    {
      "accountId": "destination-account-id",
      "amount": "100.00",
      "assetCode": "USD"
    }
  ],
  "metadata": {
    "reference": "Invoice #12345",
    "category": "Payment"
  }
}
```

#### Using Transaction DSL

```
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/dsl

transaction "Simple Transfer" {
  description "Transfer between accounts"
  code "TRANSFER"
  
  send USD 100.00 {
    source {
      from "source-account-id" {
        chart_of_accounts "1000"
        description "Withdrawal from source account"
      }
    }
    
    distribute {
      to "destination-account-id" {
        chart_of_accounts "2000"
        description "Deposit to destination account"
      }
    }
  }
  
  metadata {
    ("reference" "Invoice #12345")
    ("category" "Payment")
  }
}
```

### Using Account Aliases

You can also use account aliases instead of IDs for more readable transactions:

```
transaction "Account Transfer" {
  description "Transfer between named accounts"
  code "TRANSFER"
  
  send USD 100.00 {
    source {
      from "@checking_account" {
        chart_of_accounts "1001"
        description "Withdrawal from checking account"
      }
    }
    
    distribute {
      to "@savings_account" {
        chart_of_accounts "2001"
        description "Deposit to savings account"
      }
    }
  }
}
```

### Checking Transaction Status

After creating a transaction, you can check its status:

```
GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}
```

Response:

```json
{
  "id": "transaction-id",
  "externalId": null,
  "organizationId": "organization-id",
  "ledgerId": "ledger-id",
  "type": "transfer",
  "status": "completed",
  "metadata": {
    "reference": "Invoice #12345",
    "category": "Payment"
  },
  "operationIds": ["op-id-1", "op-id-2"],
  "createdAt": "2023-01-01T12:00:00Z",
  "updatedAt": "2023-01-01T12:00:05Z"
}
```

## Advanced Transaction Patterns

Now let's explore more complex transaction patterns in Midaz.

### Multi-source Transaction

Transfer funds from multiple source accounts to a single destination:

```
transaction "Multi-source Transfer" {
  description "Transfer from multiple accounts"
  code "MULTI_SOURCE"
  
  send USD 100.00 {
    source {
      from "@checking_account" {
        amount USD 50.00
        chart_of_accounts "1001"
        description "Withdrawal from checking account"
      }
      from "@savings_account" {
        amount USD 50.00
        chart_of_accounts "1002"
        description "Withdrawal from savings account"
      }
    }
    
    distribute {
      to "@investment_account" {
        amount USD 100.00
        chart_of_accounts "3001"
        description "Deposit to investment account"
      }
    }
  }
}
```

### Multi-destination Transaction (Distribution)

Distribute funds from a single source to multiple destinations:

```
transaction "Multi-destination Transfer" {
  description "Transfer to multiple accounts"
  code "DISTRIBUTE"
  
  send USD 100.00 {
    source {
      from "@main_account" {
        chart_of_accounts "1000"
        description "Withdrawal from main account"
      }
    }
    
    distribute {
      to "@account1" {
        amount USD 50.00
        chart_of_accounts "2001"
        description "First recipient"
      }
      to "@account2" {
        amount USD 30.00
        chart_of_accounts "2002"
        description "Second recipient"
      }
      to "@account3" {
        amount USD 20.00
        chart_of_accounts "2003"
        description "Third recipient"
      }
    }
  }
}
```

### Percentage-based Distribution

Distribute funds based on percentages:

```
transaction "Percentage Distribution" {
  description "Distribute by percentage"
  code "PERCENT_DIST"
  
  send USD 100.00 {
    source {
      from "@main_account" {
        chart_of_accounts "1000"
        description "Withdrawal from main account"
      }
    }
    
    distribute {
      to "@account1" {
        share 50
        chart_of_accounts "2001"
        description "50% share recipient"
      }
      to "@account2" {
        share 30
        chart_of_accounts "2002"
        description "30% share recipient"
      }
      to "@account3" {
        share 20
        chart_of_accounts "2003"
        description "20% share recipient"
      }
    }
  }
}
```

### Currency Conversion Transaction

Convert between different currencies using asset rates:

```
transaction "Currency Conversion" {
  description "USD to EUR conversion"
  code "FX_CONVERT"
  
  send USD 100.00 {
    source {
      from "@usd_account" {
        chart_of_accounts "1001"
        description "USD account withdrawal"
      }
    }
    
    distribute {
      to "@eur_account" {
        rate "RATE_ID" USD -> EUR 0.85
        chart_of_accounts "1002"
        description "EUR account deposit"
      }
    }
  }
}
```

### Transaction with Pending Status

Create a transaction that requires review before processing:

```
transaction "Pending Payment" {
  description "Payment requiring review"
  code "PENDING_PAY"
  pending true
  
  send USD 100.00 {
    source {
      from "@checking_account" {
        chart_of_accounts "1001"
        description "Withdrawal from checking"
      }
    }
    
    distribute {
      to "@merchant_account" {
        chart_of_accounts "2001"
        description "Payment to merchant"
      }
    }
  }
}

// Later, to commit the pending transaction:
// POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/commit

// Or to revert the pending transaction:
// POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert
```

## Error Handling

Proper error handling is crucial when implementing transactions. Here are the common error types and how to handle them:

### Common Transaction Errors

| Error Code | Description | Handling Strategy |
|------------|-------------|-------------------|
| `ErrInsufficientFunds` | Account doesn't have enough available balance | Check balance before transaction or handle retry logic |
| `ErrInvalidAccount` | Account doesn't exist or is invalid | Validate accounts before transaction |
| `ErrMismatchedAssetCode` | Asset codes don't match between source and destination | Ensure asset codes match or use currency conversion |
| `ErrIdempotencyKey` | Duplicate transaction with same idempotency key | Use a new idempotency key or check the original transaction |
| `ErrTransactionValidation` | Transaction doesn't pass validation rules | Fix the transaction structure based on error details |

### Idempotency Keys

To ensure transactions are processed only once, use idempotency keys:

```json
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json
X-Idempotency-Key: unique-request-id-123

{
  "type": "transfer",
  "sources": [...],
  "destinations": [...]
}
```

### Error Response Example

```json
{
  "error": {
    "code": "ErrInsufficientFunds",
    "message": "Insufficient funds in account",
    "details": {
      "accountId": "source-account-id",
      "available": "50.00",
      "required": "100.00",
      "assetCode": "USD"
    }
  }
}
```

## Testing Transactions

It's important to thoroughly test your transactions before moving to production. Here are some ways to test transactions in Midaz:

### Balance Verification

After a transaction, verify the balances of all involved accounts to ensure they were updated correctly:

```
GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/balances
```

### Transaction Lifecycle Testing

1. Create a transaction
2. Verify that operations were created
3. Check account balances
4. Verify transaction status

### Test Cases for Transactions

| Test Case | Description |
|-----------|-------------|
| Valid transaction | Basic transaction with sufficient funds |
| Insufficient funds | Transaction with more funds than available |
| Invalid account | Transaction with non-existent account |
| Asset mismatch | Transaction with mismatched asset codes |
| Idempotency | Same transaction submitted twice |
| Concurrent transactions | Multiple transactions affecting the same account |

## Best Practices

Follow these best practices when implementing transactions in Midaz:

1. **Use Idempotency Keys**: Always include idempotency keys for production transactions to prevent duplicates.

2. **Transaction Atomicity**: Design transactions to be atomic - either all operations succeed or all fail.

3. **Balance Checking**: Check account balances before attempting large transactions to avoid failures.

4. **Error Handling**: Implement comprehensive error handling for transaction failures.

5. **Transaction Logging**: Include sufficient metadata for auditing and tracking.

6. **Asynchronous Processing**: Be prepared for asynchronous processing; don't assume transactions complete instantly.

7. **Amount Precision**: Be careful with amount calculations to avoid floating-point errors.

8. **Security**: Always validate permissions before executing transactions.

9. **Monitoring**: Implement monitoring for transaction failures and unusual patterns.

10. **Documentation**: Document the purpose and structure of your transaction patterns.

## Advanced Implementation Examples

Here are some complete implementation examples for common transaction scenarios:

### Example 1: Customer Payment Processing

```
transaction "Customer Payment" {
  description "Payment from customer to merchant with fee"
  code "PAYMENT_FEE"
  
  send USD 100.00 {
    source {
      from "@customer_account" {
        chart_of_accounts "1000"
        description "Customer payment"
      }
    }
    
    distribute {
      to "@merchant_account" {
        amount USD 97.00
        chart_of_accounts "2000"
        description "Merchant payment"
      }
      to "@fee_account" {
        amount USD 3.00
        chart_of_accounts "5000"
        description "Transaction fee"
      }
    }
  }
  
  metadata {
    ("payment_id" "PAY-123456")
    ("customer_id" "CUST-789")
    ("merchant_id" "MERCH-456")
    ("fee_percentage" "3")
  }
}
```

### Example 2: Salary Distribution

```
transaction "Salary Payment" {
  description "Monthly salary distribution"
  code "PAYROLL"
  
  send USD 5000.00 {
    source {
      from "@payroll_account" {
        chart_of_accounts "1100"
        description "Payroll account withdrawal"
      }
    }
    
    distribute {
      to "@employee1_account" {
        amount USD 2000.00
        chart_of_accounts "2100"
        description "Senior Engineer salary"
      }
      to "@employee2_account" {
        amount USD 1500.00
        chart_of_accounts "2100"
        description "Engineer salary"
      }
      to "@employee3_account" {
        amount USD 1500.00
        chart_of_accounts "2100"
        description "Engineer salary"
      }
    }
  }
  
  metadata {
    ("payroll_id" "PR-202301")
    ("period" "January 2023")
    ("department" "Engineering")
  }
}
```

### Example 3: Investment Allocation

```
transaction "Investment Allocation" {
  description "Portfolio allocation based on investment strategy"
  code "INVEST_ALLOC"
  
  send USD 10000.00 {
    source {
      from "@investment_pool" {
        chart_of_accounts "1200"
        description "Investment pool withdrawal"
      }
    }
    
    distribute {
      to "@stocks_fund" {
        share 60
        chart_of_accounts "4100"
        description "Equity allocation"
      }
      to "@bonds_fund" {
        share 30
        chart_of_accounts "4200"
        description "Fixed income allocation"
      }
      to "@cash_reserve" {
        share 10
        chart_of_accounts "4300"
        description "Cash reserve allocation"
      }
    }
  }
  
  metadata {
    ("strategy" "Growth")
    ("investor_id" "INV-456")
    ("allocation_date" "2023-01-15")
  }
}
```

## References

- [Transaction Service API](../components/transaction/api.md)
- [Transaction Domain Model](../components/transaction/domain-model.md)
- [Transaction Processing](../components/transaction/transaction-processing.md)
- [Balance Management](../components/transaction/balance-management.md)
- [Financial Model](../domain-models/financial-model.md)
- [Transaction DSL Documentation](../api-reference/transaction-dsl/README.md)
- [Creating Financial Structures](./creating-financial-structures.md)