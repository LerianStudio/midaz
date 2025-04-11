# Transaction Processing

**Navigation:** [Home](../../) > [Components](../) > [Transaction](./README.md) > Transaction Processing

This document describes the transaction processing flow in the Midaz Transaction Service, explaining how transactions are created, validated, processed, and applied to account balances.

For a visual representation of the transaction processing flow, see the [Transaction Flow Diagram](../../assets/transaction-flow-diagram.md).

## Table of Contents

- [Overview](#overview)
- [Transaction Creation Methods](#transaction-creation-methods)
- [Transaction Processing Flow](#transaction-processing-flow)
- [Validation Process](#validation-process)
- [Balance and Operation Management](#balance-and-operation-management)
- [Asynchronous Processing](#asynchronous-processing)
- [Special Handling Features](#special-handling-features)
- [Error Handling and Recovery](#error-handling-and-recovery)
- [Examples](#examples)

## Overview

The Midaz Transaction Service implements a robust transaction processing system that maintains financial integrity through double-entry accounting principles. Transactions are processed through a multi-stage pipeline that includes validation, balance checking, operation creation, and asynchronous processing.

Key features of the transaction processing system include:

- Multiple interfaces for transaction creation (JSON, DSL, Templates)
- Strong validation ensuring double-entry accounting rules
- Optimistic concurrency control for balance updates
- Event-driven architecture with asynchronous processing
- Support for transaction reversals and idempotent processing
- Comprehensive audit trail through operation records

## Transaction Creation Methods

### JSON Interface

Clients can create transactions by submitting a structured JSON payload:

```json
{
  "chartOfAccountsGroupName": "default",
  "description": "Payment for services",
  "code": "PAY-001",
  "pending": false,
  "metadata": {
    "invoiceNumber": "INV-123",
    "department": "Engineering"
  },
  "send": {
    "amount": 100,
    "scale": 2,
    "assetCode": "BRL",
    "source": {
      "from": [
        {
          "alias": "@customer1",
          "amount": 100,
          "scale": 2
        }
      ]
    },
    "distribute": {
      "to": [
        {
          "alias": "@merchant1",
          "amount": 100,
          "scale": 2
        }
      ]
    }
  }
}
```

The JSON interface provides a programmatic way to create transactions, suitable for system integrations.

### Domain-Specific Language (DSL)

Transactions can be defined using a domain-specific language specifically designed for financial transactions:

```
TRANSACTION
  CHART_OF_ACCOUNTS default
  DESCRIPTION "Payment for services"
  CODE "PAY-001"
  SEND BRL 10000 2
    FROM @customer1 AMOUNT BRL 10000 2
    DISTRIBUTE TO @merchant1 AMOUNT BRL 10000 2
```

The DSL provides a human-readable and writable format for defining transactions, making it easier to create and understand complex financial movements.

The DSL is parsed using an ANTLR4-based parser that converts the textual representation into a structured transaction object.

### Template-Based Transactions

Pre-defined transaction templates can be used with specific variables to create standardized transactions:

```json
{
  "transactionType": "00000000-0000-0000-0000-000000000000",
  "transactionTypeCode": "PAYMENT",
  "variables": {
    "sourceAccount": "@customer1",
    "destinationAccount": "@merchant1",
    "amount": 100.00,
    "currency": "BRL"
  }
}
```

Templates simplify common transaction patterns by allowing users to provide just the variable components of a pre-defined transaction structure.

## Transaction Processing Flow

The transaction processing flow follows these key steps:

### 1. Initiation

- Client submits a transaction request through one of the available interfaces (JSON, DSL, Template)
- The system validates basic request format and authentication

### 2. Parsing and Normalization

- JSON or DSL inputs are parsed into a standardized transaction structure
- Template-based requests are expanded using the template definition
- The transaction is normalized into a canonical form

### 3. Validation

- Business rule validation ensures the transaction follows financial rules
- Balance validation checks if source accounts have sufficient funds
- Structural validation ensures proper account references and asset codes

### 4. Synchronous Processing

- The transaction record is created in the database
- Metadata is stored in MongoDB
- An idempotency key is generated if not provided
- The transaction is queued for asynchronous processing

### 5. Asynchronous Processing

- The transaction is picked up from the queue
- Account balances are locked and verified
- Operations are created for each debit and credit action
- Balances are updated atomically

### 6. Finalization

- Transaction status is updated to COMMITTED
- Audit trails are created
- Notifications are generated for relevant parties

## Validation Process

Transaction validation is a critical step to ensure financial integrity and prevent errors:

### Syntax Validation

For DSL-based transactions, the system performs syntax validation:

- Lexical analysis verifies token structure
- Parser validates grammatical structure
- Type checking ensures proper value formats

### Business Rule Validation

All transactions undergo business rule validation:

- **Double-Entry Compliance**: The sum of debits must equal the sum of credits
- **Balance Availability**: Source accounts must have sufficient available balance
- **Account Permissions**: Accounts must allow sending/receiving transactions
- **Asset Compatibility**: Asset codes must be valid and compatible

### Security Validation

Security validations protect the system from abuse:

- **Authorization**: Verifies the requester has permission to create transactions
- **Resource Access**: Checks if the requester can access specified accounts
- **Rate Limiting**: Prevents excessive transaction creation
- **Idempotency**: Prevents duplicate transaction processing

## Balance and Operation Management

Transactions affect account balances through a carefully controlled process:

### Balance Retrieval

For each account involved in the transaction:

1. The current balance is retrieved and locked (optimistic locking)
2. The system verifies the account exists and is active
3. Permission checks ensure the account can send or receive funds

### Operation Creation

For each financial movement within the transaction:

1. An operation record is created with:
   - Reference to the parent transaction
   - Account identifier
   - Amount and asset code
   - Balance before the operation
   - Operation type (DEBIT or CREDIT)

2. The balance after the operation is calculated based on:
   - For DEBIT operations: `new_balance = old_balance - amount`
   - For CREDIT operations: `new_balance = old_balance + amount`

3. The balance after the operation is recorded in the operation

### Balance Updates

Balance updates follow these rules:

1. Optimistic locking using version numbers prevents race conditions
2. Available and on-hold amounts are tracked separately
3. Balance updates are applied atomically
4. Balance versioning ensures concurrency control

## Asynchronous Processing

The transaction processing system uses asynchronous patterns for scalability and resilience:

### Queue-Based Processing

1. Transactions are submitted to RabbitMQ queues for processing
2. Dedicated workers process transactions from the queues
3. Separate queues handle different stages of transaction processing

### Balance-Transaction-Operation (BTO) Pattern

The system implements the BTO pattern:

1. **Create Transaction**: Record the overall transaction details
2. **Create Operations**: Generate individual debit/credit operations
3. **Update Balances**: Apply operations to account balances

This pattern is applied asynchronously to ensure system resilience.

### Retry Mechanisms

Failed transactions can be retried:

1. Transient failures (e.g., database connectivity) trigger automatic retries
2. Queue dead-letter exchanges capture persistently failing transactions
3. Retry policies control backoff and maximum attempt count

## Special Handling Features

### Transaction Reversals

Transactions can be reversed through a dedicated reversal process:

1. A reversal transaction is created that references the original
2. Credit and debit operations are swapped
3. Account balances are adjusted to reverse the original effects
4. Both transactions maintain their audit trail

### Idempotency

The system supports idempotent transaction processing:

1. Clients can provide idempotency keys with transactions
2. Duplicate submission with the same key returns the original result
3. Idempotency keys have configurable time-to-live (TTL)

### Partial Transactions

Some scenarios support partial transaction processing:

1. Multi-phase transactions for complex scenarios
2. Pending state for transactions awaiting approval
3. Transaction commitment to finalize pending transactions

## Error Handling and Recovery

The transaction system is designed for reliability:

### Error Categorization

Errors are categorized for appropriate handling:

- **Validation Errors**: Returned immediately to clients
- **Resource Errors**: May trigger retries if transient
- **System Errors**: Logged for operational monitoring

### Recovery Mechanisms

Recovery mechanisms include:

- **Automatic Retries**: For transient failures
- **Manual Intervention**: For complex scenarios requiring human review
- **Compensating Transactions**: For correcting committed errors

## Examples

### Basic Transfer Example

A simple money transfer follows this flow:

1. **Client Request**:
   ```
   TRANSACTION
     CHART_OF_ACCOUNTS default
     DESCRIPTION "Payment from Customer to Merchant"
     SEND BRL 100 2
       FROM @customer1 AMOUNT BRL 100 2
       DISTRIBUTE TO @merchant1 AMOUNT BRL 100 2
   ```

2. **System Processing**:
   - Transaction record created with PENDING status
   - Balance check ensures @customer1 has ≥ $100.00
   - Debit operation created for @customer1 (-$100.00)
   - Credit operation created for @merchant1 (+$100.00)
   - Balances updated
   - Transaction status set to COMMITTED

3. **Result**:
   - @customer1 balance decreases by $100.00
   - @merchant1 balance increases by $100.00
   - Complete audit trail through operation records

### Multi-Party Distribution Example

For distributing funds to multiple recipients:

```
TRANSACTION
  CHART_OF_ACCOUNTS default
  DESCRIPTION "Revenue distribution"
  SEND BRL 100 2
    FROM @revenue AMOUNT BRL 100 2
    DISTRIBUTE
      TO @department1 AMOUNT BRL 50 2
      TO @department2 AMOUNT BRL 30 2
      TO @department3 AMOUNT BRL 20 2
```

This creates four operations:
- Debit from @revenue: -$100.00
- Credit to @department1: +$50.00
- Credit to @department2: +$30.00
- Credit to @department3: +$20.00

### Transaction Reversal Example

To reverse transaction T1:

1. Client calls the revert endpoint for T1
2. System creates a new transaction T2 that:
   - References T1 as its parent
   - Creates opposite operations (credits become debits, debits become credits)
   - Marks T1 as REVERSED
   - Sets T2 status to COMMITTED

This maintains a complete audit trail while effectively canceling the original transaction's effects.