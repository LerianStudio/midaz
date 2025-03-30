# Midaz Data Models

This document provides a detailed overview of the core data models in the Midaz system.

## Core Models

### Organization

Organizations are the top-level entities in the Midaz system, representing companies or business units.

```go
type Organization struct {
    ID                   string         // Unique identifier
    ParentOrganizationID *string        // Parent organization ID (optional)
    LegalName            string         // Legal name
    DoingBusinessAs      *string        // DBA name (optional)
    LegalDocument        string         // Legal document (e.g., tax ID)
    Address              Address        // Physical address
    Status               Status         // Status (active, inactive, etc.)
    CreatedAt            time.Time      // Creation timestamp
    UpdatedAt            time.Time      // Last update timestamp
    DeletedAt            *time.Time     // Deletion timestamp (optional)
    Metadata             map[string]any // Custom metadata
}
```

### Ledger

Ledgers are financial ledgers within organizations.

```go
type Ledger struct {
    ID             string         // Unique identifier
    Name           string         // Ledger name
    OrganizationID string         // Parent organization ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

### Account

Accounts are financial accounts within ledgers.

```go
type Account struct {
    ID              string         // Unique identifier
    Name            string         // Account name
    ParentAccountID *string        // Parent account ID (optional)
    EntityID        *string        // Associated entity ID (optional)
    AssetCode       string         // Asset code (e.g., USD, BTC)
    OrganizationID  string         // Parent organization ID
    LedgerID        string         // Parent ledger ID
    PortfolioID     *string        // Associated portfolio ID (optional)
    SegmentID       *string        // Associated segment ID (optional)
    Status          Status         // Status (active, inactive, etc.)
    Alias           *string        // Account alias (optional)
    Type            string         // Account type
    CreatedAt       time.Time      // Creation timestamp
    UpdatedAt       time.Time      // Last update timestamp
    DeletedAt       *time.Time     // Deletion timestamp (optional)
    Metadata        map[string]any // Custom metadata
}
```

### Asset

Assets represent different types of assets (currencies, commodities, etc.).

```go
type Asset struct {
    ID             string         // Unique identifier
    Code           string         // Asset code (e.g., USD, BTC)
    Name           string         // Asset name
    Symbol         string         // Asset symbol
    Decimals       int            // Number of decimal places
    OrganizationID string         // Parent organization ID
    LedgerID       string         // Parent ledger ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

### Portfolio

Portfolios are collections of accounts for grouping and reporting.

```go
type Portfolio struct {
    ID             string         // Unique identifier
    Name           string         // Portfolio name
    OrganizationID string         // Parent organization ID
    LedgerID       string         // Parent ledger ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

### Segment

Segments are business segments for categorizing accounts.

```go
type Segment struct {
    ID             string         // Unique identifier
    Name           string         // Segment name
    OrganizationID string         // Parent organization ID
    LedgerID       string         // Parent ledger ID
    Status         Status         // Status (active, inactive, etc.)
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

### Transaction

Transactions are financial transactions between accounts.

```go
type Transaction struct {
    ID                       string                     // Unique identifier
    ParentTransactionID      *string                    // Parent transaction ID (optional)
    Description              string                     // Transaction description
    Template                 string                     // Transaction template
    Status                   Status                     // Status
    Amount                   *int64                     // Transaction amount
    AmountScale              *int64                     // Amount scale (decimal places)
    AssetCode                string                     // Asset code (e.g., USD, BTC)
    ChartOfAccountsGroupName string                     // Chart of accounts group
    Source                   []string                   // Source accounts
    Destination              []string                   // Destination accounts
    LedgerID                 string                     // Parent ledger ID
    OrganizationID           string                     // Parent organization ID
    Body                     libTransaction.Transaction // Transaction body
    CreatedAt                time.Time                  // Creation timestamp
    UpdatedAt                time.Time                  // Last update timestamp
    DeletedAt                *time.Time                 // Deletion timestamp (optional)
    Metadata                 map[string]any             // Custom metadata
    Operations               []*operation.Operation     // Associated operations
}
```

### Operation

Operations are individual debit/credit operations within transactions.

```go
type Operation struct {
    ID              string         // Unique identifier
    TransactionID   string         // Parent transaction ID
    AccountID       string         // Account ID
    AccountAlias    *string        // Account alias (optional)
    Type            string         // Operation type (debit, credit)
    Amount          int64          // Operation amount
    AmountScale     int64          // Amount scale (decimal places)
    AssetCode       string         // Asset code (e.g., USD, BTC)
    OrganizationID  string         // Parent organization ID
    LedgerID        string         // Parent ledger ID
    CreatedAt       time.Time      // Creation timestamp
    UpdatedAt       time.Time      // Last update timestamp
    DeletedAt       *time.Time     // Deletion timestamp (optional)
    Metadata        map[string]any // Custom metadata
}
```

### Balance

Balances are account balances resulting from transactions.

```go
type Balance struct {
    ID             string         // Unique identifier
    AccountID      string         // Account ID
    Amount         int64          // Balance amount
    AmountScale    int64          // Amount scale (decimal places)
    AssetCode      string         // Asset code (e.g., USD, BTC)
    OrganizationID string         // Parent organization ID
    LedgerID       string         // Parent ledger ID
    CreatedAt      time.Time      // Creation timestamp
    UpdatedAt      time.Time      // Last update timestamp
    DeletedAt      *time.Time     // Deletion timestamp (optional)
    Metadata       map[string]any // Custom metadata
}
```

## Relationships

### Organization Relationships

- An organization can have multiple ledgers
- An organization can have a parent organization

### Ledger Relationships

- A ledger belongs to an organization
- A ledger can have multiple accounts, assets, portfolios, and segments

### Account Relationships

- An account belongs to a ledger
- An account can have a parent account
- An account can belong to a portfolio
- An account can belong to a segment
- An account can have multiple operations
- An account can have multiple balances

### Transaction Relationships

- A transaction belongs to a ledger
- A transaction can have a parent transaction
- A transaction can have multiple operations

### Operation Relationships

- An operation belongs to a transaction
- An operation affects an account

### Balance Relationships

- A balance belongs to an account

## Data Flow

### Organization and Ledger Setup

1. Create an organization
2. Create a ledger within the organization
3. Create accounts, assets, portfolios, and segments within the ledger

### Transaction Processing

1. Create a transaction with source and destination accounts
2. Create operations for each source and destination account
3. Update account balances based on operations

### Query Processing

1. Query organizations, ledgers, accounts, assets, portfolios, and segments
2. Query transactions, operations, and balances
3. Generate reports based on the queried data
