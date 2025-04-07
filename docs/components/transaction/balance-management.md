# Balance Management

**Navigation:** [Home](../../) > [Components](../) > [Transaction](./README.md) > Balance Management

This document describes the balance management system in the Midaz Transaction Service, explaining how account balances are structured, updated, and controlled during financial transactions.

## Table of Contents

- [Overview](#overview)
- [Balance Structure](#balance-structure)
- [Balance States](#balance-states)
- [Concurrency Control](#concurrency-control)
- [Balance Operations](#balance-operations)
- [Transaction Integration](#transaction-integration)
- [Security Controls](#security-controls)
- [Performance Considerations](#performance-considerations)
- [Examples](#examples)

## Overview

The Balance Management system in Midaz provides a robust foundation for tracking and managing financial balances across accounts. It ensures data integrity, prevents race conditions, and maintains a complete audit trail of balance changes.

Key features of the balance management system include:

- Dual-state balance tracking (available vs. on-hold funds)
- Optimistic concurrency control with version tracking
- Permission controls for sending and receiving funds
- Scale-aware balance calculations for precise financial handling
- Integration with the transaction processing system
- Support for multiple assets per account

## Balance Structure

Each balance record represents the financial state of an account for a specific asset and includes the following key attributes:

### Core Attributes

- `ID`: Unique identifier (UUID)
- `AccountID`: Reference to the account this balance belongs to
- `Alias`: Human-readable account identifier (e.g., "@customer1")
- `AssetCode`: Code of the asset this balance represents (e.g., "BRL", "USD")
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger

### Amount Fields

- `Available`: Currently available amount (integer representation)
- `OnHold`: Amount reserved but not yet finalized (integer representation)
- `Scale`: Decimal places for the amounts (for precision)

### Control Fields

- `Version`: Counter for optimistic concurrency control
- `AccountType`: Type of the associated account
- `AllowSending`: Flag controlling whether the account can send funds
- `AllowReceiving`: Flag controlling whether the account can receive funds

### Audit Fields

- `CreatedAt`: Timestamp when the balance was created
- `UpdatedAt`: Timestamp when the balance was last updated
- `DeletedAt`: Timestamp when the balance was soft-deleted (nullable)
- `Metadata`: Custom key-value attributes

## Balance States

The balance management system tracks two distinct balance states:

### Available Balance

The available balance represents funds that are immediately accessible for use in transactions:

- Decreases during debit operations
- Increases during credit operations
- Must be non-negative for transactions to succeed
- Represents the "spendable" portion of an account

### On-Hold Balance

The on-hold balance represents funds that are reserved for pending operations:

- Used during multi-phase transactions
- Allows for reservation before final settlement
- Prevents the same funds from being used in multiple transactions
- Provides atomicity guarantees across asynchronous operations

### Balance Lifecycle

1. **Creation**: Balances are created when an account is created, typically with zero initial values
2. **Updates**: Balances change as transactions occur, with corresponding operations recorded
3. **Archiving**: Balances can be soft-deleted but are never physically removed

## Concurrency Control

The balance management system uses optimistic concurrency control to handle multiple simultaneous updates:

### Version Tracking

- Each balance record includes a version number
- The version is incremented with each update
- Updates include version checking to detect concurrent modifications

### Update Approaches

The system provides two approaches for updating balances:

1. **SelectForUpdate**:
   - Uses explicit row locking with `SELECT FOR UPDATE`
   - Provides serializable isolation level
   - Prevents concurrent modifications during critical operations

2. **BalancesUpdate**:
   - Uses optimistic locking with version checking
   - More scalable approach for most scenarios
   - Updates only succeed if the version hasn't changed

### Atomicity

- All balance updates within a transaction are atomic
- Either all balance changes succeed or none are applied
- Maintains consistency between related accounts

## Balance Operations

The balance management system supports several operations on balances:

### Creation

Balances are automatically created when accounts are created:

```go
balance := &mmodel.Balance{
    ID:             libCommons.GenerateUUIDv7().String(),
    Alias:          *account.Alias,
    OrganizationID: account.OrganizationID,
    LedgerID:       account.LedgerID,
    AccountID:      account.ID,
    AssetCode:      account.AssetCode,
    AccountType:    account.Type,
    AllowSending:   true,
    AllowReceiving: true,
    CreatedAt:      time.Now(),
    UpdatedAt:      time.Now(),
}
```

### Updates

Balances are updated during transaction processing using the `OperateBalances` function:

```go
// For debit operations
newBalance = OperateBalances(amount, currentBalance, "DEBIT")

// For credit operations
newBalance = OperateBalances(amount, currentBalance, "CREDIT")
```

The function updates the balance based on the operation type:
- DEBIT: Decreases available balance
- CREDIT: Increases available balance

### Queries

The system provides several methods to query balances:

- `GetAllBalances`: List all balances with pagination
- `GetBalanceByID`: Retrieve a specific balance by ID
- `GetAllBalancesByAccountID`: Get all balances for a specific account

## Transaction Integration

The balance management system is tightly integrated with transaction processing:

### Pre-Transaction Checks

Before processing a transaction, the system:
1. Retrieves current balances for all involved accounts
2. Verifies that source accounts have sufficient available funds
3. Checks that accounts allow sending/receiving as required

### Transaction Processing

During transaction processing:
1. Balances are locked to prevent concurrent modifications
2. Operations are created to record the pre-state of each balance
3. Balance updates are applied based on the transaction details
4. Operations record the post-state of each balance
5. The transaction record is finalized

### Balance Verification

The system ensures financial integrity through several checks:
1. Double-entry accounting (debits = credits)
2. Non-negative balance constraint
3. Permission verification for each account
4. Optimistic concurrency control

## Security Controls

The balance management system includes several security controls:

### Permission Flags

Accounts have two permission flags:

- `AllowSending`: Controls whether funds can be debited from the account
- `AllowReceiving`: Controls whether funds can be credited to the account

These flags can be updated independently to restrict account operations:

```go
// Freeze an account to prevent any outgoing transfers
updateBalance := mmodel.UpdateBalance{
    AllowSending: &false,
}
```

### Validation Rules

The balance system enforces several validation rules:

1. **Sufficient Funds**: Available balance must be sufficient for debits
2. **Permission Checks**: Accounts must have appropriate permissions
3. **Concurrency Control**: Version checks prevent lost updates
4. **Scale Consistency**: Amount scales must match balance scales

## Performance Considerations

The balance management system is designed for performance:

### Caching

- Redis caching for frequently accessed balances
- BalanceRedis model for optimized storage
- Cache invalidation on updates

### Query Optimization

- Indexes on AccountID, Alias, and other frequently filtered fields
- Pagination for large result sets
- Metadata filtering capabilities

### Batch Processing

- Bulk balance updates for better throughput
- Transaction grouping for related operations
- Asynchronous processing for non-critical updates

## Examples

### Basic Balance Creation

When a new account is created, a corresponding balance is automatically created:

```json
{
  "id": "98765432-0000-0000-0000-000000000000",
  "accountId": "12345678-0000-0000-0000-000000000000",
  "alias": "@customer1",
  "assetCode": "BRL",
  "available": 0,
  "onHold": 0,
  "scale": 2,
  "version": 1,
  "accountType": "checking",
  "allowSending": true,
  "allowReceiving": true,
  "organizationId": "00000000-0000-0000-0000-000000000001",
  "ledgerId": "00000000-0000-0000-0000-000000000002",
  "createdAt": "2023-01-01T10:00:00Z",
  "updatedAt": "2023-01-01T10:00:00Z",
  "metadata": {
    "accountOrigin": "web",
    "tier": "standard"
  }
}
```

### Balance Update During Transaction

When a transaction occurs, the balance is updated through a series of steps:

1. **Initial State**:
   ```json
   {
     "id": "98765432-0000-0000-0000-000000000000",
     "available": 10000,
     "onHold": 0,
     "scale": 2,
     "version": 1
   }
   ```

2. **Debit Operation**:
   ```json
   {
     "transactionId": "00000000-1111-0000-0000-000000000000",
     "accountId": "12345678-0000-0000-0000-000000000000",
     "type": "DEBIT",
     "amount": 5000,
     "scale": 2,
     "balance": {
       "available": 10000,
       "onHold": 0,
       "scale": 2
     },
     "balanceAfter": {
       "available": 5000,
       "onHold": 0,
       "scale": 2
     }
   }
   ```

3. **Final State**:
   ```json
   {
     "id": "98765432-0000-0000-0000-000000000000",
     "available": 5000,
     "onHold": 0,
     "scale": 2,
     "version": 2
   }
   ```

The version number is incremented with each update to track changes and prevent concurrent modifications.

### Account Restriction

To restrict an account from sending funds:

```json
{
  "allowSending": false,
  "allowReceiving": true
}
```

This update prevents the account from being debited while still allowing it to receive funds.