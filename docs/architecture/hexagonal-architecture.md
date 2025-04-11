# Hexagonal Architecture

**Navigation:** [Home](../../) > [Architecture](../) > Hexagonal Architecture

This document describes how Midaz implements the Hexagonal Architecture pattern (also known as Ports and Adapters) to achieve separation of concerns and maintainable code.

For a visual representation of this architecture, see the [Hexagonal Architecture Diagram](../assets/hexagonal-architecture-diagram.md).

## Overview

Hexagonal Architecture is a software design pattern that aims to create loosely coupled application components that can be easily connected to their software environment by means of ports and adapters. This allows applications to be equally driven by users, programs, automated tests, or batch scripts, and to be developed and tested in isolation from their runtime devices and databases.

In Midaz, this architecture enables:
- Clear separation between business logic and external concerns
- Improved testability through dependency inversion
- Flexibility to change infrastructure without affecting domain logic
- Consistent structure across services

## Core Concepts

### 1. Domain Logic (Core)

The innermost layer contains the business logic and domain models:

- **Domain Models**: Pure data structures representing business entities
- **Business Rules**: Validation and domain-specific logic
- **Service Logic**: Application use cases and workflows

Domain logic has no dependencies on external systems and is expressed purely in terms of the business domain.

### 2. Ports (Interfaces)

Ports are interfaces that define how the domain interacts with the outside world:

- **Primary/Driving Ports**: APIs that allow external actors to use the domain
- **Secondary/Driven Ports**: Interfaces that the domain needs to interact with external systems

Ports are defined as Go interfaces that abstract away implementation details.

### 3. Adapters (Implementations)

Adapters connect the domain to the outside world by implementing ports:

- **Primary/Driving Adapters**: Implement primary ports to drive the application (e.g., HTTP controllers)
- **Secondary/Driven Adapters**: Implement secondary ports to connect to external systems (e.g., database repositories)

## Implementation in Midaz

Midaz implements Hexagonal Architecture across its services. Here's the general structure:

```
components/
  └── [service]/
      └── internal/
          ├── domain/           # Domain models and interfaces (ports)
          ├── services/         # Application services (use cases)
          │   ├── command/      # Write operations
          │   └── query/        # Read operations
          ├── adapters/         # Infrastructure adapters
          │   ├── http/         # HTTP controllers (primary adapters)
          │   │   └── in/       # Inbound HTTP endpoints
          │   ├── postgres/     # PostgreSQL adapters (secondary adapters)
          │   ├── mongodb/      # MongoDB adapters (secondary adapters)
          │   ├── rabbitmq/     # RabbitMQ adapters (secondary adapters)
          │   └── redis/        # Redis adapters (secondary adapters)
          └── bootstrap/        # Application wiring and dependency injection
```

### Domain Models

Domain models in Midaz are defined in the shared `pkg/mmodel` package and include:

- `Organization`
- `Ledger`
- `Asset`
- `Account`
- `Transaction`
- `Operation`
- `Balance`

These models are pure Go structs without dependencies on infrastructure.

### Ports (Interfaces)

Ports are defined as Go interfaces that specify how the domain interacts with external systems:

```go
// Example: Organization repository interface (port)
type Repository interface {
    Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error)
    Update(ctx context.Context, id uuid.UUID, organization *mmodel.Organization) (*mmodel.Organization, error)
    Find(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error)
    FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Organization, error)
    Delete(ctx context.Context, id uuid.UUID) error
}
```

These interfaces allow the domain to remain agnostic of how data is stored or retrieved.

### Adapters (Implementations)

Adapters implement the ports to provide concrete functionality:

#### Primary Adapters

HTTP controllers serve as primary adapters, accepting user input and translating it into domain operations:

```go
// Example: HTTP controller (primary adapter)
func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
    // Parse request
    var request dto.OrganizationRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        httputils.WriteError(w, errors.NewBadRequestError("invalid request body"))
        return
    }
    
    // Map to domain model
    organization := &mmodel.Organization{
        Name: request.Name,
        // ...other fields
    }
    
    // Call domain service
    result, err := c.createOrgUseCase.Execute(r.Context(), organization)
    if err != nil {
        httputils.WriteError(w, err)
        return
    }
    
    // Return response
    httputils.WriteJSON(w, http.StatusCreated, result)
}
```

#### Secondary Adapters

Database repositories serve as secondary adapters, implementing storage operations:

```go
// Example: PostgreSQL repository (secondary adapter)
func (r *PostgreSQLRepository) Create(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
    // Implementation that saves to PostgreSQL
    query := `INSERT INTO organizations (id, name, legal_name, status) VALUES ($1, $2, $3, $4) RETURNING id`
    // ...database operations
    return organization, nil
}
```

### Application Services

Services in Midaz are split into commands (write) and queries (read) following the CQRS pattern:

```go
// Example: Create organization command
type CreateOrganizationUseCase struct {
    OrgRepo  domain.OrganizationRepository
    MetaRepo domain.MetadataRepository
}

func (u *CreateOrganizationUseCase) Execute(ctx context.Context, organization *mmodel.Organization) (*mmodel.Organization, error) {
    // Validate
    if err := validate(organization); err != nil {
        return nil, err
    }
    
    // Create in repository
    result, err := u.OrgRepo.Create(ctx, organization)
    if err != nil {
        return nil, err
    }
    
    // Store metadata if any
    if organization.Metadata != nil {
        err = u.MetaRepo.Store(ctx, "organization", result.ID, organization.Metadata)
        if err != nil {
            // Handle error
        }
    }
    
    return result, nil
}
```

### Dependency Injection

The bootstrap package wires everything together using dependency injection:

```go
// Example: Wiring in bootstrap
func createServices(cfg *Config) (*Services, error) {
    // Create repositories (adapters)
    orgRepo := organization.NewPostgreSQLRepository(db)
    metaRepo := metadata.NewMongoDBRepository(mongoClient)
    
    // Create use cases with injected dependencies
    createOrgUseCase := &command.CreateOrganizationUseCase{
        OrgRepo:  orgRepo,
        MetaRepo: metaRepo,
    }
    
    // Create HTTP controllers with injected use cases
    orgController := &http.OrganizationController{
        CreateUseCase: createOrgUseCase,
        // ...other use cases
    }
    
    return &Services{
        OrgController: orgController,
        // ...other controllers
    }, nil
}
```

## Benefits in Midaz

The hexagonal architecture provides Midaz with several benefits:

1. **Testability**: Domain logic can be tested in isolation without external dependencies
2. **Maintainability**: Clear boundaries make the code easier to understand and modify
3. **Flexibility**: Infrastructure can be changed without affecting domain logic
4. **Consistency**: Common patterns across services make development predictable

## Example Flow

Here's how a typical request flows through the hexagonal architecture in Midaz:

1. **HTTP Request** → Controller (Primary Adapter)
2. **Controller** → Use Case (Application Service)
3. **Use Case** → Domain Logic and Repository Interface (Port)
4. **Repository Interface** → PostgreSQL Implementation (Secondary Adapter)
5. **PostgreSQL Implementation** → Database

This separation ensures that changes to the database or API don't affect the core business logic.

## Next Steps

- [Event-Driven Design](./event-driven-design.md) - How events flow through the architecture
- [Component Integration](./component-integration.md) - How components interact
- [Data Flow](./data-flow/) - Detailed data flow documentation