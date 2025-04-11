# Transaction Domain Model

**Navigation:** [Home](../../) > [Components](../) > [Transaction](./README.md) > Domain Model

This document describes the domain model of the Transaction Service, explaining the core entities, their relationships, domain patterns, and business rules that govern the financial transaction processing.

For a visual representation of the transaction model and its key components, see the [Transaction Model Diagram](../../assets/transaction-model-diagram.md).

## Table of Contents

- [Core Entities](#core-entities)
  - [Transaction](#transaction)
  - [Operation](#operation)
  - [Balance](#balance)
  - [Asset Rate](#asset-rate)
- [Entity Relationships](#entity-relationships)
- [Domain Patterns](#domain-patterns)
  - [Double-Entry Accounting](#double-entry-accounting)
  - [Balance-Transaction-Operation (BTO) Pattern](#balance-transaction-operation-bto-pattern)
  - [Transaction DSL](#transaction-dsl)
  - [Balance Management](#balance-management)
- [Transaction Lifecycle](#transaction-lifecycle)
- [Domain Rules](#domain-rules)
- [Implementation Considerations](#implementation-considerations)

## Core Entities

### Transaction

A Transaction represents a financial movement between accounts, following double-entry accounting principles.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `ParentTransactionID`: Optional reference to a parent transaction (for related transactions)
- `Description`: Human-readable description of the transaction
- `Template`: Template name or pattern used for the transaction
- `Status`: Current state of the transaction (CREATED, PENDING, COMMITTED, REVERSED, etc.)
- `Amount`: Transaction amount (integer representation)
- `AmountScale`: Decimal places for the amount (for precision)
- `AssetCode`: Code of the primary asset being transacted
- `ChartOfAccountsGroupName`: Accounting classification reference
- `Source`: Array of source account identifiers (where money comes from)
- `Destination`: Array of destination account identifiers (where money goes to)
- `LedgerID`: Reference to the containing ledger
- `OrganizationID`: Reference to the owning organization
- `Body`: Structured representation of the transaction details (DSL parsed)
- `Metadata`: Custom key-value attributes
- `Operations`: Collection of operations created from this transaction

**Special Features:**
- Maintains parent-child relationships for transaction reversals and related transactions
- Contains the transaction DSL body for audit and reconstruction
- Enforces double-entry accounting rules (debits = credits)
- Supports transaction templates for standardized operations

### Operation

An Operation represents a single accounting entry (debit or credit) within a transaction, affecting a specific account's balance.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `TransactionID`: Reference to the parent transaction
- `Description`: Human-readable description of the operation
- `Type`: Operation type (DEBIT or CREDIT)
- `AssetCode`: Code of the asset being operated on
- `Amount`: Operation amount (integer representation)
- `AmountScale`: Decimal places for the amount
- `Balance`: Balance state before the operation
- `BalanceAfter`: Balance state after the operation
- `Status`: Current state of the operation (COMPLETED, FAILED, etc.)
- `AccountID`: Reference to the affected account
- `AccountAlias`: Human-readable account identifier
- `BalanceID`: Reference to the specific balance record
- `ChartOfAccounts`: Accounting classification reference
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `Metadata`: Custom key-value attributes

**Special Features:**
- Records both pre-operation and post-operation balance states
- Creates an immutable audit trail of financial movements
- Links account entries to their originating transaction
- Typed as either DEBIT (decrease in balance) or CREDIT (increase in balance)

### Balance

A Balance represents the current financial position of an account for a specific asset.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `AccountID`: Reference to the account
- `Alias`: Human-readable account identifier
- `AssetCode`: Code of the asset this balance represents
- `Available`: Currently available amount (integer representation)
- `OnHold`: Amount reserved but not yet finalized (integer representation)
- `Scale`: Decimal places for the amounts
- `Version`: Optimistic concurrency control counter
- `AccountType`: Type of the associated account
- `AllowSending`: Flag controlling whether the account can send funds
- `AllowReceiving`: Flag controlling whether the account can receive funds
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `Metadata`: Custom key-value attributes

**Special Features:**
- Uses optimistic concurrency control with version tracking
- Separates available balance from on-hold amounts
- Enforces account permissions for sending and receiving
- Maintains account-level control flags

### Asset Rate

An Asset Rate represents an exchange rate between two assets, enabling multi-currency operations.

**Key Attributes:**
- `ID`: Unique identifier (UUID)
- `ExternalID`: External reference ID
- `From`: Source asset code
- `To`: Target asset code
- `Rate`: Exchange rate value (integer representation)
- `RateScale`: Decimal places for the rate
- `TTL`: Time-to-live in seconds for rate validity
- `OrganizationID`: Reference to the owning organization
- `LedgerID`: Reference to the containing ledger
- `ExpiresAt`: Timestamp when the rate expires
- `Metadata`: Custom key-value attributes

**Special Features:**
- Supports time-limited validity with TTL and expiration
- Linked to external reference systems
- Enables multi-currency transactions

## Entity Relationships

The Transaction domain model has the following relationships:

1. **Transaction to Operations**:
   - One-to-many: Each Transaction generates multiple Operations
   - Each Operation must belong to exactly one Transaction

2. **Operation to Balance**:
   - Many-to-one: Multiple Operations can affect the same Balance
   - Each Operation references exactly one Balance

3. **Balance to Account**:
   - One-to-one (per asset): Each Account has one Balance per Asset
   - Balance changes reflect in the Account's financial position

4. **Transaction Hierarchy**:
   - Self-referential: Transactions can have parent-child relationships
   - Used for reversals, adjustments, and related transaction grouping

5. **Asset Rate Relationships**:
   - Asset Rates define conversions between Assets
   - Used during multi-currency transactions

## Domain Patterns

### Double-Entry Accounting

Midaz implements strict double-entry accounting principles:

1. **Balance Equilibrium**:
   - For every transaction, the sum of debits equals the sum of credits
   - Any imbalance causes transaction rejection

2. **Dual Operations**:
   - Each financial movement creates at least two operations:
     - Debit operation (reducing an account's balance)
     - Credit operation (increasing another account's balance)
   - Multi-party transactions create multiple operation pairs

3. **Operation Atomicity**:
   - All operations in a transaction are processed atomically
   - Either all operations succeed, or none are applied

4. **Chart of Accounts**:
   - Operations reference the chart of accounts for accounting classification
   - Enables standard accounting reports and categorization

### Balance-Transaction-Operation (BTO) Pattern

The Transaction Service implements the Balance-Transaction-Operation (BTO) pattern:

1. **Balance**:
   - Represents the current state of an account
   - Must be verified before transactions
   - Subject to update concurrency controls

2. **Transaction**:
   - Contains the overall financial movement
   - Ensures business rules and accounting principles
   - Links all related operations

3. **Operation**:
   - Records individual debit/credit entries
   - Maintains the pre- and post-balance states
   - Forms the immutable audit trail

This pattern ensures:
- Complete audit history through operational immutability
- Accurate balance tracking with optimistic concurrency
- Proper transaction isolation and atomicity

### Transaction DSL

The Transaction Service uses a domain-specific language (DSL) for expressing transactions:

1. **Grammar-Based Parsing**:
   - Defines valid transaction syntax
   - Processed through an ANTLR-based parser
   - Converted to structured domain objects

2. **Expression Examples**:
   ```
   TRANSACTION
     CHART_OF_ACCOUNTS default
     DESCRIPTION "Payment for services"
     SEND BRL 100.00
       FROM @customer1 AMOUNT BRL 100.00
       DISTRIBUTE TO @merchant1 AMOUNT BRL 100.00
   ```

3. **Features**:
   - Multi-source and multi-destination support
   - Exact amount specification
   - Transaction metadata
   - Support for templates and variables

### Balance Management

Balance management follows specific patterns:

1. **Optimistic Concurrency Control**:
   - Balances have version counters
   - Incremented with each update
   - Prevents race conditions

2. **Available vs. On-Hold**:
   - Available balance: Ready for use
   - On-hold balance: Reserved but not finalized
   - Prevents double-spending while allowing pending transactions

3. **Permission Controls**:
   - AllowSending: Controls outgoing transfers
   - AllowReceiving: Controls incoming transfers
   - Can be toggled for account management

4. **Balance History**:
   - Operations record pre- and post-balances
   - Enables balance reconstruction at any point in time
   - Facilitates auditing and reconciliation

## Transaction Lifecycle

Transactions follow a defined lifecycle with multiple states:

1. **Creation Phase**:
   - **CREATED**: Initial state when a transaction is defined
   - DSL is parsed and validated
   - Available balances are checked

2. **Processing Phase**:
   - **PENDING**: Transaction is valid but not yet finalized
   - Operations are created
   - Balances may be placed on hold

3. **Finalization Phase**:
   - **COMMITTED**: Transaction is successfully completed
   - Balance changes are finalized
   - Operations are marked as COMPLETED

4. **Alternate Paths**:
   - **FAILED**: Transaction processing encountered an error
   - **REVERSED**: Transaction was reversed by a follow-up transaction
   - **DECLINED**: Transaction was rejected due to business rules

5. **State Transitions**:
   ```
   CREATED → PENDING → COMMITTED
      ↓         ↓
   FAILED    DECLINED
                ↓
              REVERSED
   ```

## Domain Rules

The Transaction domain enforces numerous business rules:

### Balance Integrity Rules

1. **Sufficient Funds**:
   - Debits require sufficient available balance
   - Operations verify balance availability

2. **Balance Protection**:
   - Accounts with AllowSending=false cannot be debited
   - Accounts with AllowReceiving=false cannot be credited

3. **Concurrency Protection**:
   - Balance updates check version numbers
   - Concurrent modifications are detected and rejected

### Transaction Validation Rules

1. **Double-Entry Balance**:
   - Sum of all debits must equal sum of all credits
   - Enforced during transaction creation

2. **Asset Consistency**:
   - Cross-asset transactions must have valid exchange rates
   - Amounts must be properly scaled and converted

3. **Transaction Completion**:
   - All operations in a transaction must complete successfully
   - Partial completion is not allowed

### Operation Rules

1. **Operation Immutability**:
   - Operations, once created, cannot be modified
   - Corrections require compensating transactions

2. **Operation Atomicity**:
   - All operations in a transaction succeed or fail together
   - Database transactions ensure atomicity

3. **Operation Types**:
   - Operations must be either DEBIT or CREDIT
   - Debit operations decrease balance
   - Credit operations increase balance

## Implementation Considerations

### Scale-Aware Calculations

Financial amounts are stored as integers with a separate scale field to prevent floating-point errors:

```
// Representing $123.45
amount = 12345
scale = 2
```

This ensures precise financial calculations without rounding errors.

### Transaction Reversals

Transactions can be reversed through a counteracting transaction:

1. The reversal transaction references the original as its parent
2. Debits and credits are swapped (original sources become destinations)
3. The original transaction is marked as REVERSED
4. The reversal maintains a reference to what it reversed

### Optimistic Concurrency Control

To handle concurrent balance updates:

1. Balances include a version field incremented with each update
2. Updates include a WHERE clause checking the expected version
3. If the version doesn't match, the update fails with a concurrency exception
4. The operation is retried with the new balance state

### Idempotency

Transaction processing supports idempotency:

1. External transaction IDs can be provided
2. Duplicate transaction attempts are detected
3. Identical repeated requests return the same result without duplicate processing