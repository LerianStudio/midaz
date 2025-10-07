# MDZ CLI - Domain Layer

## Overview

The `domain` directory contains the domain layer of the MDZ CLI, following clean architecture principles. This layer defines repository interfaces that abstract data access operations, making the CLI independent of specific API implementations.

## Purpose

This layer provides:

- **Repository interfaces**: Abstract data operations
- **Domain contracts**: Define what operations are available
- **Implementation independence**: Commands don't know about HTTP
- **Testability**: Easy to mock for unit tests

## Package Structure

```
domain/
└── repository/          # Repository interfaces
    ├── organization.go  # Organization operations
    ├── ledger.go       # Ledger operations
    ├── account.go      # Account operations
    ├── asset.go        # Asset operations
    ├── portfolio.go    # Portfolio operations
    ├── segment.go      # Segment operations
    └── auth.go         # Authentication operations
```

## Repository Interfaces

### Organization Repository

```go
type Organization interface {
    Create(org CreateOrganizationInput) (*Organization, error)
    Get(limit, page int, sortOrder, startDate, endDate string) (*Organizations, error)
    GetByID(organizationID string) (*Organization, error)
    Update(organizationID string, orgInput UpdateOrganizationInput) (*Organization, error)
    Delete(organizationID string) error
}
```

**Operations:**

- Create: Create new organization
- Get: List organizations with pagination
- GetByID: Get single organization
- Update: Update organization fields
- Delete: Delete organization

### Common Pattern

All repository interfaces follow the same pattern:

```go
type Entity interface {
    Create(input CreateInput) (*Entity, error)
    Get(limit, page int, ...) (*Entities, error)
    GetByID(id string) (*Entity, error)
    Update(id string, input UpdateInput) (*Entity, error)
    Delete(id string) error
}
```

## Implementation

Repository interfaces are implemented by the `rest` package:

```
domain/repository/organization.go  (interface)
    ↓
internal/rest/organization.go      (implementation)
```

### Example Implementation

```go
// Domain interface
package repository

type Organization interface {
    Create(org CreateOrganizationInput) (*Organization, error)
}

// REST implementation
package rest

type organization struct {
    Factory *factory.Factory
}

func (r *organization) Create(inp CreateOrganizationInput) (*Organization, error) {
    // HTTP POST to /v1/organizations
    // Handle authentication
    // Parse response
    return organization, nil
}
```

## Benefits

### 1. Testability

Commands can be tested with mock repositories:

```go
type mockOrganization struct{}

func (m *mockOrganization) Create(inp CreateOrganizationInput) (*Organization, error) {
    return &Organization{ID: "test-id"}, nil
}

// Test command with mock
func TestCreateCommand(t *testing.T) {
    mockRepo := &mockOrganization{}
    // Test command logic without HTTP calls
}
```

### 2. Flexibility

Easy to add new implementations:

- REST API client (current)
- GraphQL client (future)
- gRPC client (future)
- Local file storage (testing)

### 3. Separation of Concerns

Commands focus on user interaction, not HTTP details:

```go
// Command only knows about repository interface
func runCreate(factory *factory.Factory, input CreateInput) error {
    repo := getRepository(factory) // Returns interface
    entity, err := repo.Create(input)
    // Handle result
}
```

## Usage in Commands

### Getting Repository

```go
// In command execution
func runCreate(f *factory.Factory) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        // Get REST implementation
        repo := rest.NewOrganization(f)

        // Use interface methods
        org, err := repo.Create(input)
        if err != nil {
            return err
        }

        // Format output
        output.FormatAndPrint(f, org.ID, "organization", output.Created)
        return nil
    }
}
```

## Related Documentation

- [MDZ CLI README](../../README.md) - CLI overview
- [Internal README](../README.md) - Internal architecture
- [REST Package](../rest/) - REST implementations
