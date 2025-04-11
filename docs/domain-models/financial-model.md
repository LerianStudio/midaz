# Financial Model

**Navigation:** [Home](../) > [Domain Models](./) > Financial Model

This document describes the financial model implemented in the Midaz system, covering the core financial entities, accounting principles, and transaction processing mechanisms.

## Overview

Midaz implements a comprehensive financial accounting model designed to handle complex financial operations with high integrity and flexibility. The model follows double-entry accounting principles and provides a structured approach to managing financial entities, transactions, and balances.

The system enables:
- Hierarchical organization of financial entities
- Double-entry accounting with balanced transactions
- Flexible transaction templates using a domain-specific language
- Precise balance tracking with optimistic concurrency control
- Multiple asset types with scale-aware calculations

## Core Financial Entities

### Organization Structure

The financial model starts with a hierarchical organization structure:

1. **Organizations**
   - Top-level entity representing a business or individual
   - Can have parent-child relationships
   - Contains ledgers

2. **Ledgers**
   - Financial book of records for an organization
   - Container for accounts, assets, and transactions
   - Separates financial data into distinct contexts

3. **Portfolios & Segments**
   - Optional grouping constructs for accounts
   - Enable classification and reporting
   - Support organizational hierarchies

### Financial Instruments

Midaz supports different financial instruments through:

1. **Assets**
   - Represent currencies, cryptocurrencies, or other financial instruments
   - Properties:
     - Code/Symbol (e.g., "BRL", "USD", "BTC")
     - Type (currency, cryptocurrency, etc.)
     - Scale (decimal precision)
   - Can be used across multiple accounts

2. **Accounts**
   - Individual financial entities within a ledger
   - Properties:
     - Type (checking, savings, credit card, expense)
     - Asset code (currency used)
     - Alias (optional unique identifier, e.g., "@person1")
     - Hierarchical structure (parent-child relationships)
   - Associated with specific assets, portfolios, and segments

3. **Balances**
   - Current financial position of accounts
   - Properties:
     - Available amount (actual balance)
     - On-hold amount (reserved funds)
     - Version number (for concurrency control)
     - Permissions (sending/receiving capabilities)
   - Each account can have multiple balances (one per asset)

## Double-Entry Accounting Model

Midaz implements double-entry accounting through a Balance-Transaction-Operation (BTO) pattern:

### Transactions

Transactions are the core financial events that move value between accounts:

```
┌─────────────────────────────────────────────┐
│ Transaction                                 │
│ - ID                                        │
│ - Parent Transaction ID (optional)          │
│ - Description                               │
│ - Status (CREATED, APPROVED, PENDING, etc.) │
│ - Asset Code                                │
│ - Amount                                    │
│ - Chart of Accounts Group                   │
│ - Body (Transaction DSL)                    │
└─────────────────────┬───────────────────────┘
                      │
                      │ contains
                      ▼
┌─────────────────────────────────────────────┐
│ Operations (2 or more)                      │
│ - ID                                        │
│ - Transaction ID                            │
│ - Type (DEBIT/CREDIT)                       │
│ - Account ID                                │
│ - Amount                                    │
│ - Balance Before/After                      │
└─────────────────────────────────────────────┘
```

Key principles:
- Every transaction must have balanced debits and credits
- Transactions record both sides of a financial movement
- Transactions can be atomic or part of a larger transaction group
- Transactions follow a hierarchical parent-child relationship

### Balance Processing

Balance management follows these principles:

1. **Atomic Updates**
   - Balance updates are atomic and version-controlled
   - Uses optimistic concurrency control with version numbers
   - Prevents race conditions during concurrent transactions

2. **Balance Tracking**
   - Operations record pre and post balance states
   - Provides audit trail of all balance changes
   - Enables reconciliation and verification

3. **Two-phase Balance Updates**
   - First phase: validate and lock balances
   - Second phase: update balances and create operations
   - Ensures consistency across complex transactions

## Transaction DSL

Midaz implements a domain-specific language (DSL) for defining transactions, providing a flexible way to express complex financial operations:

### DSL Syntax

```
TRANSACTION
  CHART_OF_ACCOUNTS <chart_id>
  [DESCRIPTION "<description>"]
  [CODE <code>]
  [PENDING <true|false>]
  [METADATA { <key>: <value>, ... }]
  SEND <asset_code> <amount> <scale>
    FROM
      <account> [AMOUNT <asset> <value> <scale> | SHARE <percent> | REMAINING]
      [RATE <external_id> <from_asset> <to_asset> <value> <scale>]
      [DESCRIPTION "<description>"]
      [METADATA { <key>: <value>, ... }]
    [FROM ...]
    DISTRIBUTE TO
      <account> [AMOUNT <asset> <value> <scale> | SHARE <percent> | REMAINING]
      [RATE <external_id> <from_asset> <to_asset> <value> <scale>]
      [DESCRIPTION "<description>"]
      [METADATA { <key>: <value>, ... }]
    [TO ...]
```

### Key DSL Features

1. **Multiple Sources and Destinations**
   - Support for multiple FROM and TO entries
   - Allows complex multi-party transactions

2. **Flexible Amount Distribution**
   - Fixed amounts with AMOUNT
   - Percentages with SHARE
   - Remaining balances with REMAINING

3. **Currency Conversion**
   - RATE keyword for currency conversions
   - Asset rate tracking and conversion

4. **Metadata and Documentation**
   - Per-transaction and per-operation metadata
   - Description fields for audit and reporting

### DSL Processing

The DSL is processed using an ANTLR4 parser that:
1. Validates transaction syntax and semantics
2. Converts the DSL into executable operations
3. Ensures balanced debits and credits

## Asset Rates and Currency Handling

The system manages different currencies and assets with:

1. **Asset Rates**
   - Tracks exchange rates between assets
   - Supports point-in-time rate snapshots
   - Used for currency conversion in transactions

2. **Scale Management**
   - Each asset defines its decimal scale (e.g., 2 for dollars, 8 for Bitcoin)
   - All calculations preserve proper scale
   - Amounts stored as integers with scale information

## Event-Driven Processing

Financial operations follow an event-driven pattern:

1. **Asynchronous Transaction Processing**
   - Account creation triggers balance creation
   - Transactions are processed asynchronously
   - Uses message queues (RabbitMQ) for reliability

2. **Idempotent Operations**
   - Transaction processing is idempotent
   - Support for retries without duplicate entries
   - Ensures at-least-once delivery semantics

## Data Model

### Transaction Schema

```sql
CREATE TABLE IF NOT EXISTS "transaction" (
    id                                  UUID PRIMARY KEY NOT NULL,
    parent_transaction_id               UUID,
    description                         TEXT NOT NULL,
    template                            TEXT NOT NULL,
    status                              TEXT NOT NULL,
    status_description                  TEXT,
    amount                              BIGINT NOT NULL,
    amount_scale                        BIGINT NOT NULL,
    asset_code                          TEXT NOT NULL,
    chart_of_accounts_group_name        TEXT NOT NULL,
    ledger_id                           UUID NOT NULL,
    organization_id                     UUID NOT NULL,
    body                                JSONB NOT NULL,
    created_at                          TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                          TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_transaction_id) REFERENCES "transaction" (id)
);
```

### Operation Schema

```sql
CREATE TABLE IF NOT EXISTS operation (
    id                                 UUID PRIMARY KEY NOT NULL,
    transaction_id                     UUID NOT NULL,
    description                        TEXT NOT NULL,
    type                               TEXT NOT NULL,
    asset_code                         TEXT NOT NULL,
    amount                             BIGINT NOT NULL DEFAULT 0,
    amount_scale                       BIGINT NOT NULL DEFAULT 0,
    available_balance                  BIGINT NOT NULL DEFAULT 0,
    on_hold_balance                    BIGINT NOT NULL DEFAULT 0,
    balance_scale                      BIGINT NOT NULL DEFAULT 0,
    available_balance_after            BIGINT NOT NULL DEFAULT 0,
    on_hold_balance_after              BIGINT NOT NULL DEFAULT 0,
    balance_scale_after                BIGINT NOT NULL DEFAULT 0,
    status                             TEXT NOT NULL,
    status_description                 TEXT NULL,
    account_id                         UUID NOT NULL,
    account_alias                      TEXT NOT NULL,
    balance_id                         UUID NOT NULL,
    chart_of_accounts                  TEXT NOT NULL,
    organization_id                    UUID NOT NULL,
    ledger_id                          UUID NOT NULL,
    created_at                         TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                         TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (transaction_id) REFERENCES "transaction" (id)
);
```

### Balance Schema

```go
type Balance struct {
    ID             string         `json:"id"`
    OrganizationID string         `json:"organizationId"`
    LedgerID       string         `json:"ledgerId"`
    AccountID      string         `json:"accountId"`
    Alias          string         `json:"alias"`
    AssetCode      string         `json:"assetCode"`
    Available      int64          `json:"available"`
    OnHold         int64          `json:"onHold"`
    Scale          int64          `json:"scale"`
    Version        int64          `json:"version"`
    AccountType    string         `json:"accountType"`
    AllowSending   bool           `json:"allowSending"`
    AllowReceiving bool           `json:"allowReceiving"`
    CreatedAt      time.Time      `json:"createdAt"`
    UpdatedAt      time.Time      `json:"updatedAt"`
    DeletedAt      *time.Time     `json:"deletedAt"`
    Metadata       map[string]any `json:"metadata,omitempty"`
}
```

## Transaction Processing Flow

The transaction processing follows this flow:

1. **Transaction Creation**
   - Transaction is created with DSL body and metadata
   - Initial status: CREATED

2. **Transaction Validation**
   - DSL is parsed and validated
   - Accounts and balances are verified
   - Required permissions are checked

3. **Balance Locking**
   - Balances are locked with SELECT FOR UPDATE
   - Optimistic concurrency control with version checks

4. **Operation Creation**
   - Source account operations (debits) are created
   - Destination account operations (credits) are created
   - Pre and post balance states are recorded

5. **Balance Updates**
   - Balances are updated atomically
   - Version numbers are incremented

6. **Transaction Completion**
   - Transaction status updated to APPROVED
   - Metadata is updated
   - Event notifications may be triggered

## Common Transaction Patterns

### Simple Transfer

A basic transfer between two accounts:

```
TRANSACTION
  CHART_OF_ACCOUNTS chart-id-12345
  DESCRIPTION "Payment for services"
  SEND BRL 10000 2
    FROM
      @person1 AMOUNT BRL 10000 2
      DESCRIPTION "Payment sent"
    DISTRIBUTE TO
      @person2 AMOUNT BRL 10000 2
      DESCRIPTION "Payment received"
```

### Multi-party Distribution

Distribution of funds to multiple recipients:

```
TRANSACTION
  CHART_OF_ACCOUNTS chart-id-12345
  DESCRIPTION "Salary payment with benefits split"
  SEND BRL 500000 2
    FROM
      @company AMOUNT BRL 500000 2
      DESCRIPTION "Salary payment"
    DISTRIBUTE TO
      @employee AMOUNT BRL 400000 2
      DESCRIPTION "Net salary"
      @tax AMOUNT BRL 80000 2
      DESCRIPTION "Income tax"
      @retirement AMOUNT BRL 20000 2
      DESCRIPTION "Retirement contribution"
```

### Currency Conversion

Transaction with currency conversion:

```
TRANSACTION
  CHART_OF_ACCOUNTS chart-id-12345
  DESCRIPTION "USD to BRL conversion"
  SEND USD 10000 2
    FROM
      @account-usd AMOUNT USD 10000 2
      DESCRIPTION "USD withdrawal"
    DISTRIBUTE TO
      @account-brl AMOUNT BRL 52000 2
      RATE rate-id-12345 USD BRL 520 2
      DESCRIPTION "BRL deposit"
```

## Best Practices

1. **Balance Consistency**
   - Always use transactions for balance modifications
   - Implement proper error handling for failed transactions
   - Regularly reconcile balances against operations

2. **Asset Management**
   - Define assets with appropriate scale for precision
   - Keep exchange rates updated for accurate conversions
   - Consider precision requirements for financial calculations

3. **Transaction Design**
   - Use the DSL for complex transaction patterns
   - Include descriptive information for audit purposes
   - Structure transactions with appropriate chart of accounts

4. **Concurrency Handling**
   - Implement optimistic concurrency control
   - Handle version conflicts gracefully
   - Use appropriate database isolation levels

## Related Documentation

- [Entity Hierarchy](entity-hierarchy.md)
- [Transaction Lifecycle](../architecture/data-flow/transaction-lifecycle.md)
- [Event-Driven Design](../architecture/event-driven-design.md)
