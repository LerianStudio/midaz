# Code Organization

**Navigation:** [Home](../) > [Developer Guide](../) > Code Organization

This document describes how code is organized within the Midaz codebase, covering the project structure, architectural patterns, and code conventions.

## Table of Contents

- [Overview](#overview)
- [Project Structure](#project-structure)
- [Hexagonal Architecture](#hexagonal-architecture)
- [Repository Pattern](#repository-pattern)
- [CQRS Pattern](#cqrs-pattern)
- [Dependency Injection](#dependency-injection)
- [Naming Conventions](#naming-conventions)
- [Testing Organization](#testing-organization)
- [References](#references)

## Overview

Midaz follows a well-structured approach to code organization based on several key principles:

1. **Component-Based Structure**: The codebase is divided into distinct components (microservices), each responsible for a specific domain area.
2. **Hexagonal Architecture**: Each component follows the hexagonal (ports and adapters) architecture pattern.
3. **Domain-Driven Design**: The core domain model is kept independent of external concerns.
4. **Command Query Responsibility Segregation (CQRS)**: Command (write) and query (read) operations are separated.
5. **Interface-Based Design**: Dependencies are defined through interfaces, promoting loose coupling.

## Project Structure

The Midaz codebase is organized into the following top-level directories:

```
midaz/
├── components/           # Main microservices components
│   ├── infra/            # Infrastructure setup (databases, message queues)
│   ├── mdz/              # CLI component
│   ├── onboarding/       # Onboarding API component
│   └── transaction/      # Transaction processing component
├── pkg/                  # Shared packages used across components
│   ├── constant/         # Shared constants
│   ├── gold/             # Transaction DSL parser
│   ├── mmodel/           # Shared model definitions
│   ├── net/              # Network utilities
│   └── shell/            # Shell utilities
├── docs/                 # Documentation
├── scripts/              # Build and utility scripts
└── ...
```

Each microservice component follows a similar internal structure:

```
component/
├── api/                  # API definitions (OpenAPI)
├── cmd/                  # Entry points
├── internal/             # Component-specific code (not exported)
│   ├── adapters/         # Infrastructure adapters
│   │   ├── http/         # HTTP adapters (in/out)
│   │   ├── mongodb/      # MongoDB adapters
│   │   ├── postgres/     # PostgreSQL adapters
│   │   └── rabbitmq/     # RabbitMQ adapters
│   ├── bootstrap/        # Application bootstrap code
│   └── services/         # Business services
│       ├── command/      # Command handlers (write operations)
│       └── query/        # Query handlers (read operations)
├── migrations/           # Database migrations
└── ...
```

## Hexagonal Architecture

Midaz implements the hexagonal architecture pattern (also known as ports and adapters) to separate business logic from external concerns. This architecture allows the core business logic to remain isolated from infrastructure details.

### Core Concepts

1. **Domain Layer**: Contains the core business logic and entities.
2. **Ports Layer**: Defines interfaces (ports) that the domain layer uses to interact with external systems.
3. **Adapters Layer**: Implements the interfaces defined by the ports layer to connect to external systems.

### Implementation

In Midaz, this pattern is implemented as follows:

1. **Domain Interfaces** (`internal/domain/repository/*.go`): Define the contracts for data access and external services.
2. **Service Layer** (`internal/services/{command,query}/*.go`): Contains the business logic that uses these interfaces.
3. **Infrastructure Adapters** (`internal/adapters/{http,postgres,mongodb,rabbitmq}/*.go`): Implement the interfaces to connect to real systems.

Example domain interface (port):

```go
// internal/domain/repository/organization.go
type OrganizationRepository interface {
    Create(ctx context.Context, organization *model.Organization) (*model.Organization, error)
    GetByID(ctx context.Context, id string) (*model.Organization, error)
    List(ctx context.Context, filter map[string]interface{}) ([]*model.Organization, error)
    Update(ctx context.Context, organization *model.Organization) (*model.Organization, error)
    Delete(ctx context.Context, id string) error
}
```

Example adapter implementation:

```go
// internal/adapters/postgres/organization/organization.postgresql.go
type PostgresOrganizationRepository struct {
    db *sqlx.DB
}

func (r *PostgresOrganizationRepository) Create(ctx context.Context, organization *model.Organization) (*model.Organization, error) {
    // Implementation details for PostgreSQL
}

// Other method implementations...
```

## Repository Pattern

Midaz uses the repository pattern to abstract data access logic. Repositories provide a clean API for the domain layer to interact with data storage without being concerned with the underlying implementation.

### Key Characteristics

1. **Interface-Driven**: Each repository is defined by an interface.
2. **Single Responsibility**: Each repository deals with a single domain entity.
3. **Storage Agnostic**: Domain code doesn't need to know if data is stored in PostgreSQL, MongoDB, or elsewhere.

### Implementation

Each domain entity has its own repository interface:

```go
// Example repository interface for Account entity
type AccountRepository interface {
    Create(ctx context.Context, account *model.Account) (*model.Account, error)
    GetByID(ctx context.Context, id string) (*model.Account, error)
    List(ctx context.Context, filter map[string]interface{}) ([]*model.Account, error)
    // Other methods...
}
```

These repositories are implemented for specific storage engines:

```go
// PostgreSQL implementation for Account repository
type PostgresAccountRepository struct {
    db *sqlx.DB
}

func (r *PostgresAccountRepository) Create(ctx context.Context, account *model.Account) (*model.Account, error) {
    // PostgreSQL-specific implementation
}

// Other method implementations...
```

## CQRS Pattern

Midaz implements the Command Query Responsibility Segregation (CQRS) pattern to separate operations that modify state (commands) from operations that read state (queries).

### Implementation

In the Midaz codebase, CQRS is implemented with separate packages for commands and queries:

```
services/
├── command/          # Write operations
│   ├── create-*.go   # Create operations
│   ├── update-*.go   # Update operations
│   └── delete-*.go   # Delete operations
└── query/            # Read operations
    ├── get-*.go      # Get single entity
    └── get-all-*.go  # Get multiple entities
```

Command example:

```go
// internal/services/command/create-organization.go
type CreateOrganizationCommand struct {
    repo domain.OrganizationRepository
}

func (c *CreateOrganizationCommand) Execute(ctx context.Context, params CreateOrganizationParams) (*model.Organization, error) {
    // Validate input
    // Create domain entity
    // Save via repository
    // Return result
}
```

Query example:

```go
// internal/services/query/get-id-organization.go
type GetOrganizationByIDQuery struct {
    repo domain.OrganizationRepository
}

func (q *GetOrganizationByIDQuery) Execute(ctx context.Context, id string) (*model.Organization, error) {
    // Fetch from repository
    // Return result
}
```

## Dependency Injection

Midaz uses a form of dependency injection to provide components with their dependencies. This approach promotes loose coupling and testability.

### Implementation

Dependencies are typically injected via constructors:

```go
// Constructor injection for a service
func NewCreateOrganizationCommand(repo domain.OrganizationRepository) *CreateOrganizationCommand {
    return &CreateOrganizationCommand{
        repo: repo,
    }
}
```

Application bootstrap code wires up all dependencies:

```go
// Simplified example from bootstrap code
func SetupServices(db *sqlx.DB) *Services {
    // Create repositories
    orgRepo := postgres.NewPostgresOrganizationRepository(db)
    
    // Create services
    createOrgCmd := command.NewCreateOrganizationCommand(orgRepo)
    getOrgByIDQuery := query.NewGetOrganizationByIDQuery(orgRepo)
    
    // Return services container
    return &Services{
        CreateOrganization: createOrgCmd,
        GetOrganizationByID: getOrgByIDQuery,
        // Other services...
    }
}
```

## Naming Conventions

Midaz follows consistent naming conventions to make the codebase more predictable and navigable:

1. **Files**: Named after the primary entity or functionality they contain.
   - Entity repositories: `entity.go`
   - Entity repository implementations: `entity.postgresql.go`, `entity.mongodb.go`
   - Command handlers: `command-name.go` (e.g., `create-organization.go`)
   - Query handlers: `get-[qualifier]-entity.go` (e.g., `get-id-organization.go`)

2. **Interfaces**: Named with a descriptive noun followed by the word "Repository" or "Service".
   - `OrganizationRepository`
   - `TransactionService`

3. **Implementations**: Named with the technology, followed by the interface name.
   - `PostgresOrganizationRepository`
   - `MongoDBMetadataRepository`

4. **Test Files**: Same name as the implementation file with `_test` suffix.
   - `organization.go` → `organization_test.go`
   - `create-organization.go` → `create-organization_test.go`

## Testing Organization

Midaz organizes tests alongside the code they test, following Go's standard approach:

1. **Unit Tests**: Placed in the same package as the code they test, with `_test.go` suffix.
2. **Integration Tests**: For component-level tests that span multiple packages.
3. **Golden File Tests**: Used for testing CLI output and complex serialization.

### Mock Generation

For testing with dependencies, Midaz generates mock implementations of interfaces:

```go
// account_mock.go (generated)
type MockAccountRepository struct {
    mock.Mock
}

func (m *MockAccountRepository) Create(ctx context.Context, account *model.Account) (*model.Account, error) {
    args := m.Called(ctx, account)
    return args.Get(0).(*model.Account), args.Error(1)
}

// Other method implementations...
```

### Test Helper Functions

Common test helper functions are provided in packages like `mockutil`:

```go
// Example test helper function
func CreateTestAccount(t *testing.T) *model.Account {
    return &model.Account{
        ID: uuid.New().String(),
        // Other fields...
    }
}
```

## References

- [STRUCTURE.md](../../STRUCTURE.md) - Top-level code structure documentation
- [Domain Models](../domain-models/) - Documentation on domain models
- [Hexagonal Architecture](https://en.wikipedia.org/wiki/Hexagonal_architecture_(software)) - External reference on hexagonal architecture
- [CQRS Pattern](https://martinfowler.com/bliki/CQRS.html) - Martin Fowler's article on CQRS