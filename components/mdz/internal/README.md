# MDZ CLI - Internal

## Overview

The `internal` directory contains the internal implementation of the MDZ CLI, organized following clean architecture principles. It provides domain interfaces and REST API client implementations for interacting with the Midaz platform.

## Architecture

The CLI follows a layered architecture with clear separation of concerns:

```
internal/
├── domain/          # Domain Layer (Interfaces)
│   └── repository/  # Repository interfaces
├── rest/            # Infrastructure Layer (REST clients)
└── model/           # Data models
```

### Clean Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    CLI Commands                          │
│                  (Cobra Commands)                        │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                 Domain Layer                             │
│              (Repository Interfaces)                     │
│                                                          │
│  - Organization, Ledger, Account, Asset                  │
│  - Portfolio, Segment                                    │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│              Infrastructure Layer                        │
│              (REST API Clients)                          │
│                                                          │
│  - HTTP request construction                             │
│  - Authentication handling                               │
│  - Response parsing                                      │
│  - Error formatting                                      │
└─────────────────────────────────────────────────────────┘
```

## Package Structure

### domain/repository/

Repository interfaces that define data operations:

- **organization.go**: Organization CRUD interface
- **ledger.go**: Ledger CRUD interface
- **account.go**: Account CRUD interface
- **asset.go**: Asset CRUD interface
- **portfolio.go**: Portfolio CRUD interface
- **segment.go**: Segment CRUD interface
- **auth.go**: Authentication interface

### rest/

REST API client implementations:

- **organization.go**: Organization REST client
- **ledger.go**: Ledger REST client
- **account.go**: Account REST client
- **asset.go**: Asset REST client
- **portfolio.go**: Portfolio REST client
- **segment.go**: Segment REST client
- **auth.go**: Authentication REST client
- **utils.go**: HTTP utilities and error formatting

### model/

Data models for CLI operations:

- **error.go**: Error structure
- **auth.go**: Authentication models (TokenResponse)

## Key Components

### Repository Pattern

The CLI uses the Repository pattern to abstract data access:

```go
// Domain interface
type Organization interface {
    Create(org CreateOrganizationInput) (*Organization, error)
    Get(limit, page int, ...) (*Organizations, error)
    GetByID(organizationID string) (*Organization, error)
    Update(organizationID string, ...) (*Organization, error)
    Delete(organizationID string) error
}

// REST implementation
type organization struct {
    Factory *factory.Factory
}

func (r *organization) Create(inp CreateOrganizationInput) (*Organization, error) {
    // HTTP POST to /v1/organizations
}
```

### Benefits

- **Testability**: Mock repositories for unit tests
- **Flexibility**: Easy to add new implementations (GraphQL, gRPC)
- **Separation**: Commands don't know about HTTP details
- **Maintainability**: Changes to API don't affect command logic

## HTTP Client Features

### Authentication

All REST clients automatically add authentication headers:

```go
req.Header.Set("Authorization", "Bearer " + r.Factory.Token)
```

### Error Handling

API errors are parsed and formatted for user-friendly display:

```go
Error 0001: Duplicate Ledger
Message: A ledger with this name already exists
Fields:
- name: Ledger name must be unique
```

### Pagination

List operations support pagination with query parameters:

```go
BuildPaginatedURL(baseURL, limit, page, sortOrder, startDate, endDate)
// Returns: /v1/organizations?limit=10&page=1&sort_order=asc
```

## Usage Example

### Command Using Repository

```go
// In a Cobra command
func runCreate(factory *factory.Factory, input CreateOrganizationInput) error {
    // Get repository from factory
    repo := rest.NewOrganization(factory)

    // Call repository method
    org, err := repo.Create(input)
    if err != nil {
        return err
    }

    // Format and print success
    output.FormatAndPrint(factory, org.ID, "organization", output.Created)
    return nil
}
```

## Related Documentation

- [MDZ CLI README](../README.md) - CLI overview and usage
- [pkg/ README](../pkg/README.md) - CLI packages
- [Components README](../../README.md) - All Midaz components
