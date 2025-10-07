# Package mmodel

## Overview

The `mmodel` package provides domain model definitions for the Midaz ledger system. It contains all data structures (models) that represent core business entities, used for API request/response payloads, database representations, and internal data transfer.

## Purpose

This package defines:

- **Entity models**: Complete representations of business entities (Account, Ledger, Organization, etc.)
- **Input models**: Request payloads for create/update operations
- **Collection models**: Paginated lists of entities
- **Response models**: Swagger response wrappers for API documentation
- **Cache models**: Optimized structures for Redis caching
- **Queue models**: Message structures for RabbitMQ communication

## Package Structure

```
mmodel/
├── account.go           # Account entity and related models
├── account-type.go      # Account type classification models
├── asset.go             # Asset (currency/crypto) models
├── balance.go           # Balance tracking models
├── error.go             # Error response models
├── ledger.go            # Ledger entity models
├── operation-route.go   # Operation routing models
├── organization.go      # Organization and address models
├── portfolio.go         # Portfolio grouping models
├── queue.go             # Message queue models
├── segment.go           # Segment division models
├── status.go            # Status type definition
├── transaction-route.go # Transaction routing models
└── README.md           # This file
```

## Entity Hierarchy

The Midaz platform follows this hierarchical structure:

```
Organization (top level)
  └── Ledger
      ├── Asset (currencies, cryptocurrencies, commodities)
      ├── Portfolio (grouping of accounts)
      ├── Segment (logical divisions)
      ├── Account (financial buckets)
      │   └── Balance (asset holdings with available/on-hold amounts)
      ├── AccountType (account classification)
      ├── OperationRoute (transaction routing rules)
      └── TransactionRoute (transaction flow definitions)
```

## Key Entities

### Organization

Top-level entity representing a company or business unit.

**Fields:**

- Legal name and doing business as (DBA) name
- Legal document (tax ID, registration number)
- Physical address (ISO 3166-1 alpha-2 country codes)
- Parent organization support for hierarchies
- Status and metadata

**Models:**

- `CreateOrganizationInput`: Create payload
- `UpdateOrganizationInput`: Update payload
- `Organization`: Complete entity
- `Organizations`: Paginated list

### Ledger

Financial record-keeping system within an organization.

**Fields:**

- Name and organization reference
- Status and metadata
- Timestamps (created, updated, deleted)

**Models:**

- `CreateLedgerInput`: Create payload
- `UpdateLedgerInput`: Update payload
- `Ledger`: Complete entity
- `Ledgers`: Paginated list

### Asset

Types of value (currencies, cryptocurrencies, commodities, stocks).

**Fields:**

- Name, code (e.g., USD, BTC), and type
- Ledger and organization references
- Status and metadata

**Validation:**

- Codes must be uppercase
- Currency codes must follow ISO 4217 standard
- Codes must be unique within a ledger

**Models:**

- `CreateAssetInput`: Create payload
- `UpdateAssetInput`: Update payload
- `Asset`: Complete entity
- `Assets`: Paginated list

### Account

Individual financial entities (bank accounts, cards, expense categories).

**Fields:**

- Name, alias, and type
- Asset code (determines currency)
- Parent account support (for sub-accounts)
- Portfolio and segment associations
- Entity ID for external system linking
- Status and metadata

**Validation:**

- Aliases must match pattern: `^[a-zA-Z0-9@:_-]+$`
- External accounts use `@external/` prefix
- Type cannot be "external" for user-created accounts

**Models:**

- `CreateAccountInput`: Create payload
- `UpdateAccountInput`: Update payload
- `Account`: Complete entity
- `Accounts`: Paginated list

**Methods:**

- `IDtoUUID()`: Converts string ID to UUID type

### Balance

Amount of a specific asset held in an account.

**Fields:**

- Available amount (funds available for transactions)
- On-hold amount (reserved funds)
- Balance key (allows multiple balances per account)
- Version (for optimistic locking)
- Permission flags (allow sending/receiving)
- Account and asset references

**Key Concepts:**

- **Available**: Funds that can be used immediately
- **On-Hold**: Funds reserved but not yet transferred
- **Version**: Used for optimistic concurrency control
- **Key**: Allows multiple balance types (default, freeze, etc.)

**Models:**

- `CreateAdditionalBalance`: Create additional balance
- `UpdateBalance`: Update balance permissions
- `Balance`: Complete entity
- `Balances`: Paginated list
- `BalanceRedis`: Redis cache representation
- `BalanceOperation`: Balance with operation metadata

**Methods:**

- `IDtoUUID()`: Converts string ID to UUID
- `ConvertToLibBalance()`: Converts to lib-commons format
- `ConvertBalancesToLibBalances()`: Batch conversion
- `ConvertBalanceOperationsToLibBalances()`: Extract and convert from operations
- `UnmarshalJSON()`: Custom JSON unmarshalling for decimal fields

### Portfolio

Collection of accounts grouped for business purposes.

**Fields:**

- Name and entity ID
- Ledger and organization references
- Status and metadata

**Use Cases:**

- Grouping accounts by business unit
- Organizing accounts by department
- Client-specific account collections

**Models:**

- `CreatePortfolioInput`: Create payload
- `UpdatePortfolioInput`: Update payload
- `Portfolio`: Complete entity
- `Portfolios`: Paginated list

### Segment

Logical divisions for organizing accounts (e.g., by product line, region).

**Fields:**

- Name and ledger reference
- Organization reference
- Status and metadata

**Models:**

- `CreateSegmentInput`: Create payload
- `UpdateSegmentInput`: Update payload
- `Segment`: Complete entity
- `Segments`: Paginated list

### AccountType

Classification system for accounts with custom rules.

**Fields:**

- Name, description, and key value
- Ledger and organization references
- Metadata for custom attributes

**Models:**

- `CreateAccountTypeInput`: Create payload
- `UpdateAccountTypeInput`: Update payload
- `AccountType`: Complete entity

### OperationRoute

Defines routing rules for individual operations (source or destination).

**Fields:**

- Title, description, and code
- Operation type (source or destination)
- Account selection rules (by alias or account type)
- Metadata

**Models:**

- `CreateOperationRouteInput`: Create payload
- `UpdateOperationRouteInput`: Update payload
- `OperationRoute`: Complete entity
- `AccountRule`: Account selection rule configuration

### TransactionRoute

Defines complete transaction flow with multiple operation routes.

**Fields:**

- Title and description
- Collection of operation routes (sources and destinations)
- Ledger and organization references
- Metadata

**Models:**

- `CreateTransactionRouteInput`: Create payload
- `UpdateTransactionRouteInput`: Update payload
- `TransactionRoute`: Complete entity
- `TransactionRouteCache`: Redis cache structure

**Methods:**

- `ToCache()`: Converts to cache-optimized structure
- `FromMsgpack()`: Deserializes from msgpack binary
- `ToMsgpack()`: Serializes to msgpack binary

## Common Patterns

### Status Management

All entities use a `Status` struct with standardized codes:

```go
type Status struct {
    Code        string   // ACTIVE, INACTIVE, PENDING, SUSPENDED, DELETED
    Description *string  // Optional human-readable description
}
```

**Methods:**

- `IsEmpty()`: Checks if status is unset

### Metadata Support

Most entities support flexible metadata:

```go
Metadata map[string]any `json:"metadata"`
```

**Constraints:**

- Keys: max 100 characters
- Values: max 2000 characters
- No nested objects allowed
- Validated with: `validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`

### Soft Deletion

Entities support soft deletion via `DeletedAt`:

```go
DeletedAt *time.Time `json:"deletedAt"`
```

- `nil`: Entity is active
- `non-nil`: Entity is soft-deleted (excluded from normal queries)

### Pagination

Collection models include pagination metadata:

```go
type Accounts struct {
    Items []Account  // Entity records
    Page  int        // Current page (1-indexed)
    Limit int        // Items per page (1-100)
}
```

The `X-Total-Count` HTTP header provides total count across all pages.

### UUID Handling

Entity IDs are stored as strings but can be converted to UUID types:

```go
account := &Account{ID: "123e4567-e89b-12d3-a456-426614174000"}
accountUUID := account.IDtoUUID()
```

## Usage Examples

### Creating an Organization

```go
input := mmodel.CreateOrganizationInput{
    LegalName:     "Acme Corp",
    LegalDocument: "12345678901234",
    Address: mmodel.Address{
        Line1:   "123 Main St",
        City:    "New York",
        State:   "NY",
        Country: "US",
        ZipCode: "10001",
    },
    Status: mmodel.Status{
        Code: "ACTIVE",
    },
    Metadata: map[string]any{
        "industry": "Technology",
    },
}
```

### Creating an Account

```go
input := mmodel.CreateAccountInput{
    Name:      "Corporate Checking",
    AssetCode: "USD",
    Type:      "deposit",
    Alias:     ptr.String("@corporate_checking"),
    Status: mmodel.Status{
        Code: "ACTIVE",
    },
    Metadata: map[string]any{
        "department": "Treasury",
    },
}
```

### Working with Balances

```go
// Create balance
balance := &mmodel.Balance{
    AccountID:      accountID,
    AssetCode:      "USD",
    Available:      decimal.NewFromInt(10000),
    OnHold:         decimal.NewFromInt(0),
    Version:        1,
    AllowSending:   true,
    AllowReceiving: true,
}

// Convert to lib-commons format
libBalance := balance.ConvertToLibBalance()

// Batch convert
balances := []*mmodel.Balance{balance1, balance2}
libBalances := mmodel.ConvertBalancesToLibBalances(balances)
```

### Checking Status

```go
status := mmodel.Status{Code: "ACTIVE"}
if !status.IsEmpty() {
    // Status was provided, use it
} else {
    // Use default status
}
```

### Working with Transaction Routes

```go
// Create transaction route
route := &mmodel.TransactionRoute{
    Title: "Payment Settlement",
    OperationRoutes: []mmodel.OperationRoute{
        {
            OperationType: "source",
            Account: &mmodel.AccountRule{
                RuleType: "alias",
                ValidIf:  "@customer_account",
            },
        },
        {
            OperationType: "destination",
            Account: &mmodel.AccountRule{
                RuleType: "account_type",
                ValidIf:  []string{"revenue", "income"},
            },
        },
    },
}

// Convert to cache format
cache := route.ToCache()

// Serialize for Redis
data, err := cache.ToMsgpack()
if err != nil {
    // Handle error
}

// Store in Redis
redis.Set(cacheKey, data, ttl)
```

## Validation

All input models include validation tags:

- `required`: Field must be present
- `max=N`: Maximum length constraint
- `uuid`: Must be valid UUID format
- `dive`: Validates nested structures
- `keymax=N`: Maximum key length in maps
- `valuemax=N`: Maximum value length in maps
- `nonested`: Prevents nested objects in metadata
- `omitempty`: Field is optional

Example:

```go
Name string `json:"name" validate:"required,max=256"`
```

## Swagger Integration

All models include Swagger annotations for API documentation:

- `swagger:model`: Defines the model name
- `@Description`: Describes the model purpose
- `@example`: Provides example values
- `example:`: Field-level examples
- `format:`: Data format (uuid, date-time, etc.)
- `minimum:`, `maximum:`: Numeric constraints
- `minLength:`, `maxLength:`: String constraints

These annotations are used by swaggo/swag to generate OpenAPI documentation.

## Cache Models

### BalanceRedis

Optimized balance representation for Redis caching:

```go
type BalanceRedis struct {
    ID             string
    Available      decimal.Decimal
    OnHold         decimal.Decimal
    Version        int64
    AllowSending   int  // 1=true, 0=false
    AllowReceiving int  // 1=true, 0=false
    // ... other fields
}
```

**Custom JSON Handling:**

- Supports multiple decimal formats (float64, string, json.Number)
- Handles integer flags for boolean fields

### TransactionRouteCache

Optimized transaction route structure for Redis:

```go
type TransactionRouteCache struct {
    Source      map[string]OperationRouteCache  // Source routes by ID
    Destination map[string]OperationRouteCache  // Destination routes by ID
}
```

**Serialization:**

- Uses msgpack for efficient binary encoding
- Provides O(1) lookup by operation route ID
- Pre-categorizes routes by type for faster validation

## Queue Models

### Queue

Message structure for RabbitMQ communication:

```go
type Queue struct {
    OrganizationID uuid.UUID
    LedgerID       uuid.UUID
    AuditID        uuid.UUID
    AccountID      uuid.UUID
    QueueData      []QueueData
}
```

### Event

Event structure for event-driven architecture:

```go
type Event struct {
    Source         string          // Event source (e.g., "midaz")
    EventType      string          // Type of event (e.g., "transaction")
    Action         string          // Action performed (e.g., "APPROVED")
    TimeStamp      time.Time       // Event timestamp
    Version        string          // API version
    OrganizationID string          // Organization context
    LedgerID       string          // Ledger context
    Payload        json.RawMessage // Event payload
}
```

## Design Principles

1. **Immutability**: Entity IDs and certain fields cannot be changed after creation
2. **Soft Deletion**: Entities are marked as deleted rather than physically removed
3. **Versioning**: Balances use optimistic locking with version numbers
4. **Validation**: All inputs include comprehensive validation rules
5. **Extensibility**: Metadata fields allow custom attributes
6. **Type Safety**: Strong typing with UUID and decimal types
7. **Documentation**: Comprehensive Swagger annotations

## Best Practices

### Working with Decimals

Always use `decimal.Decimal` for financial amounts:

```go
import "github.com/shopspring/decimal"

// Create from integer (cents)
amount := decimal.NewFromInt(10000)  // $100.00

// Create from float (use with caution)
amount := decimal.NewFromFloat(100.50)

// Create from string (recommended for precision)
amount, err := decimal.NewFromString("100.50")
```

**Why decimals?**

- Avoids floating-point precision errors
- Ensures accurate financial calculations
- Maintains precision for monetary values

### Working with UUIDs

Convert string IDs to UUID types when needed:

```go
account := &mmodel.Account{ID: "123e4567-e89b-12d3-a456-426614174000"}
accountUUID := account.IDtoUUID()  // Returns uuid.UUID
```

**Note:** `IDtoUUID()` panics on invalid UUIDs. Ensure IDs are valid before calling.

### Handling Optional Fields

Use pointers for optional fields:

```go
// Optional field present
alias := "my_alias"
account.Alias = &alias

// Optional field absent
account.Alias = nil
```

### Validating Status

Check if status was provided:

```go
if input.Status.IsEmpty() {
    // Use default status
    input.Status = mmodel.Status{Code: "ACTIVE"}
}
```

### Validating Address

Check if address was provided:

```go
if !input.Address.IsEmpty() {
    // Validate address fields
    if input.Address.Country != "" {
        // Validate ISO 3166-1 alpha-2 format
    }
}
```

### Working with Metadata

```go
// Create with metadata
input := mmodel.CreateAccountInput{
    Name: "Account",
    Metadata: map[string]any{
        "department": "Sales",
        "region":     "EMEA",
        "cost_center": 12345,
    },
}

// Metadata constraints:
// - Keys: max 100 chars
// - Values: max 2000 chars
// - No nested objects
```

### Pagination

```go
// Request with pagination
page := 1
limit := 50

// Response
accounts := mmodel.Accounts{
    Items: []mmodel.Account{...},
    Page:  page,
    Limit: limit,
}

// Total count in X-Total-Count header
```

## Conversion Functions

### Balance Conversions

The package provides utilities for converting between Midaz and lib-commons formats:

```go
// Single balance conversion
libBalance := balance.ConvertToLibBalance()

// Batch conversion
libBalances := mmodel.ConvertBalancesToLibBalances(balances)

// Extract from operations
libBalances := mmodel.ConvertBalanceOperationsToLibBalances(operations)
```

### Cache Conversions

Transaction routes can be converted to cache format:

```go
// Convert to cache structure
cache := transactionRoute.ToCache()

// Serialize to msgpack
data, err := cache.ToMsgpack()

// Deserialize from msgpack
var cache mmodel.TransactionRouteCache
err := cache.FromMsgpack(data)
```

## Error Responses

All entity types have corresponding error response models:

```go
type AccountErrorResponse struct {
    Body struct {
        Code    int            // Error code
        Message string         // Error message
        Details map[string]any // Additional details
    }
}
```

Error response types:

- `AccountErrorResponse`
- `AssetErrorResponse`
- `BalanceErrorResponse`
- `LedgerErrorResponse`
- `OrganizationErrorResponse`
- `PortfolioErrorResponse`
- `SegmentErrorResponse`
- `ErrorResponse` (generic)

## Validation Constraints

### Common Constraints

- **Names**: max 256 characters
- **Descriptions**: max 500 characters
- **Codes**: max 100 characters
- **Asset codes**: 2-10 characters, uppercase
- **Aliases**: max 100 characters, pattern: `^[a-zA-Z0-9@:_-]+$`
- **Country codes**: exactly 2 characters (ISO 3166-1 alpha-2)
- **Metadata keys**: max 100 characters
- **Metadata values**: max 2000 characters

### Pagination Limits

- **Page**: minimum 1
- **Limit**: 1-100 items per page

## Dependencies

This package depends on:

- `github.com/google/uuid`: UUID generation and parsing
- `github.com/shopspring/decimal`: Precise decimal arithmetic
- `github.com/vmihailenco/msgpack/v5`: Binary serialization
- `github.com/LerianStudio/lib-commons/v2`: Shared transaction types
- `time` (standard library): Timestamp handling
- `encoding/json` (standard library): JSON serialization

## Related Packages

- `pkg/constant`: Error codes and constants used in models
- `pkg`: Error types used in error responses
- `components/onboarding`: Uses models for onboarding service
- `components/transaction`: Uses models for transaction processing

## Testing

The package includes comprehensive tests:

- `account_test.go`: Account model tests
- `balance_test.go`: Balance conversion and validation tests
- `organization_test.go`: Address validation tests
- `status_test.go`: Status validation tests

Run tests:

```bash
go test ./pkg/mmodel/...
```

## Performance Considerations

### Decimal Operations

Decimal operations are slower than native float64 but provide accuracy:

- Use decimals for all monetary calculations
- Cache decimal values when possible
- Avoid repeated string parsing

### UUID Parsing

`IDtoUUID()` panics on invalid UUIDs:

- Ensure IDs are validated before calling
- Use in trusted contexts only
- Consider error-returning alternatives for user input

### Cache Serialization

Msgpack is faster and more compact than JSON:

- Use msgpack for Redis caching
- Use JSON for API responses
- Pre-compute cache structures when possible

## Security Considerations

### Metadata Validation

Metadata is validated to prevent:

- Excessively long keys/values (DoS via memory)
- Nested objects (complexity attacks)
- Injection attacks via key names

### External Accounts

External accounts have special restrictions:

- Cannot be directly created by users
- Cannot be modified or deleted
- Use `@external/` prefix for identification

### Optimistic Locking

Balance updates use version numbers:

- Prevents race conditions
- Ensures balance consistency
- Detects concurrent modifications

## Future Enhancements

Potential additions:

- Validation helper functions
- Builder patterns for complex models
- Deep copy methods
- Diff/patch utilities
- Schema migration support

## Version History

This package follows semantic versioning as part of the Midaz v3 module.
