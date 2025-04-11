# Entity Hierarchy

**Navigation:** [Home](../../) > [Domain Models](../) > Entity Hierarchy

This document describes the main domain entities in Midaz, their relationships, and hierarchical structure.

## Entity Hierarchy Overview

Midaz implements a hierarchical financial entity model that reflects real-world financial structures. For a detailed visualization of this hierarchy, see the [Entity Hierarchy Diagram](../assets/entity-hierarchy-diagram.md).

The hierarchy follows this general pattern:

```
Organization
  └── Ledger
       ├── Asset
       ├── Segment
       ├── Portfolio
       └── Account (linked to an Asset)
             └── Balance
```

Additionally, there are transaction-related entities:

```
Transaction
  └── Operation (linked to an Account)
```

## Core Domain Entities

### Organization

Organizations are the top-level entities in the Midaz hierarchy, representing businesses, companies, or divisions.

**Key Attributes:**
- `ID`: Unique identifier
- `legalName`: Official registered name
- `legalDocument`: Tax ID, registration number, or other legal identifier
- `doingBusinessAs`: Trading name (optional)
- `address`: Physical location information
- `status`: Current state (active, inactive, etc.)
- `parentOrganizationID`: Parent organization reference (optional)

**Relationships:**
- Can have parent-child relationships with other Organizations
- Contains multiple Ledgers

**Validation Rules:**
- `legalName` and `legalDocument` are required
- `legalDocument` must follow country-specific formats

### Ledger

Ledgers represent financial record-keeping systems within an Organization.

**Key Attributes:**
- `ID`: Unique identifier
- `name`: Descriptive name
- `organizationID`: Parent organization reference
- `status`: Current state

**Relationships:**
- Belongs to exactly one Organization
- Contains Assets, Segments, Portfolios, and Accounts

**Validation Rules:**
- `name` is required
- `organizationID` must reference a valid Organization

### Asset

Assets represent currencies, cryptocurrencies, or other financial instruments tracked in a Ledger.

**Key Attributes:**
- `ID`: Unique identifier
- `name`: Descriptive name
- `type`: Asset type (e.g., currency, cryptocurrency, commodity)
- `code`: Unique asset code (e.g., USD, BTC)
- `ledgerID`: Parent ledger reference
- `organizationID`: Parent organization reference
- `status`: Current state

**Relationships:**
- Belongs to exactly one Ledger and Organization
- Referenced by Accounts via `assetCode`

**Validation Rules:**
- `name` and `code` are required
- `code` must be unique within a Ledger

### Segment

Segments represent logical divisions within a Ledger, such as business areas or product lines.

**Key Attributes:**
- `ID`: Unique identifier
- `name`: Descriptive name
- `ledgerID`: Parent ledger reference
- `organizationID`: Parent organization reference
- `status`: Current state

**Relationships:**
- Belongs to exactly one Ledger and Organization
- Can contain multiple Accounts

**Validation Rules:**
- `name` is required

### Portfolio

Portfolios represent collections of Accounts grouped for specific purposes.

**Key Attributes:**
- `ID`: Unique identifier
- `name`: Descriptive name
- `entityID`: Entity identifier (optional)
- `ledgerID`: Parent ledger reference
- `organizationID`: Parent organization reference
- `status`: Current state

**Relationships:**
- Belongs to exactly one Ledger and Organization
- Can contain multiple Accounts

**Validation Rules:**
- `name` is required

### Account

Accounts are the basic units for tracking financial resources, always associated with a specific Asset.

**Key Attributes:**
- `ID`: Unique identifier
- `name`: Descriptive name
- `parentAccountID`: Parent account reference (optional)
- `entityID`: Entity identifier (optional)
- `assetCode`: Reference to associated Asset
- `organizationID`: Parent organization reference
- `ledgerID`: Parent ledger reference
- `portfolioID`: Parent portfolio reference (optional)
- `segmentID`: Parent segment reference (optional)
- `status`: Current state
- `alias`: Alternative identifier (optional)
- `type`: Account type (e.g., asset, liability, equity, revenue, expense)

**Relationships:**
- Belongs to exactly one Ledger and Organization
- Can optionally belong to a Portfolio and/or Segment
- Can have parent-child relationships with other Accounts
- Associated with exactly one Asset via `assetCode`
- Has exactly one Balance

**Validation Rules:**
- `assetCode` and `type` are required
- `assetCode` must reference a valid Asset in the same Ledger

### Balance

Balances track the financial position of an Account in a specific Asset.

**Key Attributes:**
- `ID`: Unique identifier
- `accountID`: Parent account reference
- `assetCode`: Asset code
- `available`: Available balance amount
- `onHold`: Amount on hold (unavailable)
- `scale`: Decimal scale for the balance amount
- `version`: Optimistic locking version
- `accountType`: Type of account (mirrors Account.type)
- `allowSending`: Whether account can send funds
- `allowReceiving`: Whether account can receive funds

**Relationships:**
- Belongs to exactly one Account
- Referenced by Operations

**Lifecycle:**
- Created automatically when an Account is created
- Updated by Operations within Transactions

### Transaction

Transactions represent financial operations involving one or more Accounts.

**Key Attributes:**
- `ID`: Unique identifier
- `parentTransactionID`: Parent transaction reference (optional)
- `description`: Transaction description
- `template`: Transaction template information
- `status`: Current state
- `amount`: Transaction amount
- `amountScale`: Decimal scale for the amount
- `assetCode`: Primary asset code for the transaction
- `chartOfAccountsGroupName`: Accounting group name
- `ledgerID`: Parent ledger reference
- `organizationID`: Parent organization reference

**Relationships:**
- Can have parent-child relationships with other Transactions
- Contains multiple Operations
- Associated with a Ledger and Organization

**Validation Rules:**
- Must have at least one Operation
- Sum of debit Operations must equal sum of credit Operations

### Operation

Operations represent individual account movements within a Transaction.

**Key Attributes:**
- `ID`: Unique identifier
- `transactionID`: Parent transaction reference
- `description`: Operation description
- `type`: Operation type (debit or credit)
- `assetCode`: Asset code
- `amount`: Operation amount
- `amountScale`: Decimal scale for the amount
- `accountID`: Associated account reference
- `accountAlias`: Account alias (optional)
- `balanceID`: Associated balance reference
- `chartOfAccounts`: Accounting classification
- `organizationID`: Parent organization reference
- `ledgerID`: Parent ledger reference

**Relationships:**
- Belongs to exactly one Transaction
- References exactly one Account and its Balance

**Validation Rules:**
- `amount` must be positive
- `type` must be either debit or credit
- Account must allow sending for debit operations
- Account must allow receiving for credit operations

## Entity Relationships Diagram

```
┌─────────────────┐          ┌─────────────────┐
│   Organization  │◄─────────┤   Organization  │
└───────┬─────────┘          │     (Parent)    │
        │                    └─────────────────┘
        │ 1:n
        ▼
┌─────────────────┐
│     Ledger      │
└───────┬─────────┘
        │ 1:n
        ├───────────────────┬───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│      Asset      │ │     Segment     │ │    Portfolio    │
└───────┬─────────┘ └───────┬─────────┘ └───────┬─────────┘
        │ 1:n               │ 1:n               │ 1:n
        │                   │                   │
        │                   ▼                   ▼
        │            ┌─────────────────┐
        └───────────►│     Account     │◄────────┐
                     └───────┬─────────┘         │
                             │ 1:1               │
                             ▼                   │
                     ┌─────────────────┐         │
                     │     Balance     │         │
                     └─────────────────┘         │
                                                 │
┌─────────────────┐          ┌─────────────────┐ │
│   Transaction   │◄─────────┤   Transaction   │ │
└───────┬─────────┘          │     (Parent)    │ │
        │                    └─────────────────┘ │
        │ 1:n                                    │
        ▼                                        │
┌─────────────────┐                              │
│    Operation    │─────────────────────────────┘
└─────────────────┘
```

## Lifecycle Management

All entities in Midaz share common lifecycle attributes and behaviors:

- **Creation**: Entities are created with required fields and validated
- **Updates**: Entities can be updated with full or partial updates
- **Soft Deletion**: Entities are never physically deleted, only marked as deleted via `deletedAt` field
- **Status Tracking**: Entities have status fields to track their current state
- **Metadata**: Entities can have custom metadata as key-value pairs
- **Timestamps**: All entities track creation and update times

## Next Steps

- Learn more about the [Financial Model](./financial-model.md)
- Understand how [Metadata](./metadata-approach.md) extends the entity model
- Explore [Onboarding Service](../components/onboarding/README.md) for entity management implementation