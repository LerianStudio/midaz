# pkg/mmodel - Domain Models

**Location**: `pkg/mmodel/`
**Priority**: 🔥 High Use - Core domain entities
**Status**: Production-ready domain models

Core domain entities shared across Midaz components (onboarding, transaction, CRM).

## Key Domain Entities

### Account
```go
type Account struct {
    ID               uuid.UUID
    Name             string
    ParentAccountID  *uuid.UUID
    EntityID         *uuid.UUID
    AssetCode        string
    OrganizationID   uuid.UUID
    LedgerID         uuid.UUID
    PortfolioID      *uuid.UUID
    SegmentID        *uuid.UUID
    Type             string
    Status           Status
    Alias            *string
    CreatedAt        time.Time
    UpdatedAt        time.Time
    DeletedAt        *time.Time
    Metadata         map[string]any
}
```

### Ledger
```go
type Ledger struct {
    ID             uuid.UUID
    Name           string
    OrganizationID uuid.UUID
    Status         Status
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
    Metadata       map[string]any
}
```

### Organization
```go
type Organization struct {
    ID                   uuid.UUID
    ParentOrganizationID *uuid.UUID
    LegalName            string
    DoingBusinessAs      *string
    LegalDocument        string
    Address              Address
    Status               Status
    CreatedAt            time.Time
    UpdatedAt            time.Time
    DeletedAt            *time.Time
    Metadata             map[string]any
}
```

### Portfolio
```go
type Portfolio struct {
    ID             uuid.UUID
    Name           string
    EntityID       uuid.UUID
    LedgerID       uuid.UUID
    OrganizationID uuid.UUID
    Status         Status
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
    Metadata       map[string]any
}
```

### Segment
```go
type Segment struct {
    ID             uuid.UUID
    Name           string
    LedgerID       uuid.UUID
    OrganizationID uuid.UUID
    Status         Status
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
}
```

### Asset
```go
type Asset struct {
    ID             uuid.UUID
    Name           string
    Type           string  // crypto, currency, commodity, others
    Code           string  // Uppercase, e.g., USD, BTC
    LedgerID       uuid.UUID
    OrganizationID uuid.UUID
    Status         Status
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      *time.Time
    Metadata       map[string]any
}
```

### Balance
```go
type Balance struct {
    ID                     uuid.UUID
    AccountID              uuid.UUID
    AccountAlias           string
    AssetCode              string
    Amount                 decimal.Decimal
    Scale                  int
    AvailableAmount        decimal.Decimal
    OnHoldAmount           decimal.Decimal
    BalanceAvailable       decimal.Decimal
    BalanceOnHold          decimal.Decimal
    Version                int
    CreatedAt              time.Time
    UpdatedAt              time.Time
}
```

## Common Types

### Status
```go
type Status struct {
    Code        string
    Description *string
}

// Status codes
const (
    ACTIVE   = "ACTIVE"
    INACTIVE = "INACTIVE"
    DELETED  = "DELETED"
    PENDING  = "PENDING"
)
```

### Address
```go
type Address struct {
    Line1      string
    Line2      *string
    Neighborhood *string
    ZipCode    string
    City       string
    State      string
    Country    string  // ISO 3166-1 alpha-2
}
```

## Input/Output Models

Each entity typically has corresponding input models:

```go
// Create inputs
type CreateAccountInput struct {
    Name            string
    ParentAccountID *uuid.UUID
    AssetCode       string
    PortfolioID     *uuid.UUID
    SegmentID       *uuid.UUID
    Type            string
    Alias           *string
    Metadata        map[string]any
}

// Update inputs
type UpdateAccountInput struct {
    Name     string
    Status   Status
    Alias    *string
    Metadata map[string]any
}
```

## Usage Patterns

### Creating Entities

```go
account := &mmodel.Account{
    ID:             uuid.New(),
    Name:           input.Name,
    AssetCode:      input.AssetCode,
    OrganizationID: organizationID,
    LedgerID:       ledgerID,
    PortfolioID:    input.PortfolioID,
    Type:           input.Type,
    Status: mmodel.Status{
        Code:        mmodel.ACTIVE,
        Description: utils.StringPtr("Active account"),
    },
    CreatedAt:      time.Now(),
    UpdatedAt:      time.Now(),
    Metadata:       input.Metadata,
}
```

### Status Management

```go
// Check status
if account.Status.Code == mmodel.ACTIVE {
    // Process active account
}

// Update status
account.Status = mmodel.Status{
    Code:        mmodel.INACTIVE,
    Description: utils.StringPtr("Account closed by user"),
}
account.UpdatedAt = time.Now()
```

### Soft Delete Pattern

```go
// Soft delete
now := time.Now()
account.DeletedAt = &now
account.Status = mmodel.Status{
    Code:        mmodel.DELETED,
    Description: utils.StringPtr("Account deleted"),
}

// Check if deleted
if account.DeletedAt != nil {
    // Entity is soft-deleted
}
```

## References

- **Source**: `pkg/mmodel/*.go`
- **Tests**: `pkg/mmodel/*_test.go`
- **Related**: [`utils.md`](./utils.md) for validation
- **Related**: [`errors.md`](./errors.md) for error handling

## Summary

`pkg/mmodel` provides domain entities:

1. **Core entities** - Account, Ledger, Organization, Portfolio, Segment, Asset, Balance
2. **Common types** - Status, Address
3. **Input models** - CreateXInput, UpdateXInput
4. **Shared patterns** - Status management, soft deletes, metadata
5. **Type safety** - Strongly typed domain models

Use these models consistently across all components.
