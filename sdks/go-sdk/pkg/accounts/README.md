# Accounts Package

The accounts package provides utilities for working with Midaz accounts, making it easier to manage, filter, and display account information.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/accounts"
```

## Core Types

### Account

The `Account` type represents a simplified account structure:

```go
type Account struct {
    ID              string
    Name            string
    ParentAccountID *string
    AssetCode       string
    Type            string
    Alias           *string
    Status          Status
}
```

### Status

The `Status` type represents an account status:

```go
type Status struct {
    Code        string
    Description *string
}
```

### Balance

The `Balance` type represents an account balance:

```go
type Balance struct {
    ID        string
    AccountID string
    AssetCode string
    Available int64
    OnHold    int64
    Scale     int32
}
```

### AccountBalanceSummary

The `AccountBalanceSummary` type provides a human-readable balance summary:

```go
type AccountBalanceSummary struct {
    AccountID    string
    AccountAlias string
    AssetCode    string
    Available    int64
    AvailableStr string
    OnHold       int64
    OnHoldStr    string
    Total        int64
    TotalStr     string
    Scale        int
}
```

## Account Identification

### GetAccountIdentifier

Returns the best identifier for an account (alias if available, ID otherwise).

```go
func GetAccountIdentifier(account *Account) string
```

This function prevents nil pointer exceptions when dealing with the optional Alias field.

## Account Finding Functions

### FindAccountByID

Finds an account with the given ID in a list of accounts.

```go
func FindAccountByID(accounts []Account, id string) *Account
```

Returns nil if no account is found with the given ID.

### FindAccountByAlias

Finds an account with the given alias in a list of accounts.

```go
func FindAccountByAlias(accounts []Account, alias string) *Account
```

Returns nil if no account is found with the given alias.

### FindAccountsByAssetCode

Finds all accounts with the given asset code in a list of accounts.

```go
func FindAccountsByAssetCode(accounts []Account, assetCode string) []Account
```

Returns an empty slice if no accounts are found with the given asset code.

### FindAccountsByStatus

Finds all accounts with the given status in a list of accounts.

```go
func FindAccountsByStatus(accounts []Account, status string) []Account
```

Returns an empty slice if no accounts are found with the given status.

## Account Filtering

### FilterAccounts

Returns accounts that match all given filter criteria.

```go
func FilterAccounts(accounts []Account, filters map[string]string) []Account
```

Supported filter keys:
- `assetCode`: Filter by asset code
- `status`: Filter by status code
- `type`: Filter by account type
- `aliasContains`: Filter by alias containing a substring
- `id`: Filter by account ID
- `parentAccountID`: Filter by parent account ID

## Balance Formatting

### GetAccountBalanceSummary

Creates a human-readable balance summary for an account.

```go
func GetAccountBalanceSummary(account *Account, balance *Balance) (AccountBalanceSummary, error)
```

This function formats the balance amounts into human-readable strings using the scale.

### FormatAccountSummary

Returns a formatted summary string for an account.

```go
func FormatAccountSummary(account *Account) string
```

This function is useful for displaying account information in a user interface.
