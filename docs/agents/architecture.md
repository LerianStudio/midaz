# Architecture Guide

## System Overview

Midaz implements **Hexagonal Architecture (Ports & Adapters)** with **CQRS pattern** for a clean separation of concerns in a financial ledger system. The architecture ensures that business logic is isolated from infrastructure concerns, making the system testable, maintainable, and adaptable.

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP/gRPC Layer                      │
│         (Primary Adapters - Inbound)                    │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│              Application Services                       │
│  ┌──────────────────┐    ┌──────────────────┐         │
│  │   Commands       │    │    Queries       │         │
│  │  (Write Ops)     │    │   (Read Ops)     │         │
│  └──────────────────┘    └──────────────────┘         │
│                Domain Logic                             │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│         Infrastructure Adapters                         │
│         (Secondary Adapters - Outbound)                 │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ │
│  │PostgreSQL│ │ MongoDB  │ │RabbitMQ  │ │  Valkey  │ │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ │
└─────────────────────────────────────────────────────────┘
```

## Key Components

### Components (Services)

- **onboarding** (`components/onboarding/`): Entity management - Organizations, Ledgers, Assets, Portfolios, Segments, Accounts
- **transaction** (`components/transaction/`): Transaction processing, balance calculations, double-entry accounting
- **crm** (`components/crm/`): Customer relationship management
- **console** (`components/console/`): Frontend UI (Next.js)

### Shared Packages

- **pkg/assert** (`pkg/assert/`): Domain invariant assertions - CRITICAL for validation
- **pkg/constant** (`pkg/constant/`): Error codes, constants
- **pkg/gold** (`pkg/gold/`): Transaction DSL parser (ANTLR-based Gold language)
- **pkg/mgrpc** (`pkg/mgrpc/`): gRPC protobuf definitions for inter-service communication
- **pkg/mruntime** (`pkg/mruntime/`): Safe goroutine handling with panic recovery
- **pkg/mmodel** (`pkg/mmodel/`): Domain models

## Hexagonal Architecture Pattern

### Directory Structure (Example: onboarding component)

```
components/onboarding/
├── internal/
│   ├── adapters/              # Infrastructure adapters
│   │   ├── http/in/           # Primary adapters (inbound HTTP handlers)
│   │   ├── grpc/in/           # Primary adapters (inbound gRPC handlers)
│   │   ├── grpc/out/          # Secondary adapters (outbound gRPC clients)
│   │   ├── postgres/          # Secondary adapters (PostgreSQL repositories)
│   │   ├── mongodb/           # Secondary adapters (MongoDB repositories)
│   │   ├── rabbitmq/          # Secondary adapters (message queue)
│   │   └── redis/             # Secondary adapters (cache)
│   ├── bootstrap/             # Dependency injection & server setup
│   └── services/              # Application layer (business logic)
│       ├── command/           # Write operations (CQRS commands)
│       └── query/             # Read operations (CQRS queries)
└── cmd/
    └── main.go                # Application entry point
```

### Primary Adapters (Inbound)

**HTTP Handlers** - Entry points for REST API requests

Example: `components/onboarding/internal/adapters/http/in/account.go`

```go
// Handler receives HTTP requests and delegates to use cases
type AccountHandler struct {
    Command *command.UseCase
    Query   *query.UseCase
}

// Pattern: Extract context → Start span → Call use case → Handle response
func (handler *AccountHandler) CreateAccount(c *fiber.Ctx) error {
    ctx := c.Context()

    // Extract tracking context for logging/tracing
    tracking := libCommons.NewTrackingFromContext(ctx)

    // Start OpenTelemetry span
    ctx, span := tracer.Start(ctx, "handler.create_account")
    defer span.End()

    // Parse request
    var req mmodel.CreateAccountInput
    if err := c.BodyParser(&req); err != nil {
        return http.WithError(c, err)
    }

    // Delegate to command service
    account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, req)
    if err != nil {
        return http.WithError(c, err)
    }

    return http.Created(c, account)
}
```

**Key Pattern Elements:**
- Context propagation through the entire call chain
- OpenTelemetry span for distributed tracing
- Consistent error handling via `http.WithError()`
- Swagger annotations for API documentation (see actual file)

### Application Services (Business Logic)

**Commands** - Write operations that modify state

Example: `components/onboarding/internal/services/command/create-account.go`

```go
func (uc *UseCase) CreateAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, input mmodel.CreateAccountInput) (*mmodel.Account, error) {
    // 1. Domain validation using assert package
    assert.NotEmpty(input.Name, "account name")
    assert.ValidUUID(organizationID, "organization ID")

    // 2. Check business rules (e.g., parent account exists)
    if input.ParentAccountID != nil {
        parent, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, *input.ParentAccountID)
        if err != nil {
            return nil, fmt.Errorf("validating parent account: %w", err)
        }
        assert.NotNil(parent, "parent account")
    }

    // 3. Create domain entity
    account := &mmodel.Account{
        ID:             uuid.New(),
        Name:           input.Name,
        Type:           input.Type,
        OrganizationID: organizationID,
        LedgerID:       ledgerID,
        CreatedAt:      time.Now(),
    }

    // 4. Persist via repository
    if err := uc.AccountRepo.Create(ctx, account); err != nil {
        return nil, fmt.Errorf("creating account: %w", err)
    }

    // 5. Publish domain event (if needed)
    // uc.EventPublisher.Publish(ctx, events.AccountCreated{...})

    return account, nil
}
```

**Queries** - Read operations that don't modify state

Example: `components/onboarding/internal/services/query/get-all-accounts.go`

```go
func (uc *UseCase) GetAllAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, filter Filter) (*Pagination, error) {
    // Simple read - no domain logic, just data retrieval
    accounts, err := uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, filter)
    if err != nil {
        return nil, fmt.Errorf("fetching accounts: %w", err)
    }

    return accounts, nil
}
```

### Secondary Adapters (Outbound)

**Repository Pattern** - Data persistence abstraction

1. **Interface Definition** (Port)
   - Location: `components/onboarding/internal/adapters/postgres/account/account.go`
   - Defines contract for data access

2. **PostgreSQL Implementation**
   - Location: `components/onboarding/internal/adapters/postgres/account/account.postgresql.go`
   - Actual SQL queries and database interaction

3. **Mock Implementation**
   - Location: `components/onboarding/internal/adapters/postgres/account/account.postgresql_mock.go`
   - Generated with uber/mock for testing

**Example Interface:**
```go
type Repository interface {
    Create(ctx context.Context, account *mmodel.Account) error
    Find(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (*mmodel.Account, error)
    FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter Filter) ([]*mmodel.Account, error)
    Update(ctx context.Context, account *mmodel.Account) error
    Delete(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) error
}
```

## CQRS Pattern

### Command/Query Separation

**Commands (Write Side)**
- Location: `internal/services/command/`
- Modify system state
- Validate business rules
- Trigger side effects (events, notifications)
- Return created/modified entities

**Queries (Read Side)**
- Location: `internal/services/query/`
- Read-only operations
- No side effects
- Optimized for specific read patterns
- Can use different data models than commands

### File Naming Convention

Pattern: `{verb}-{entity}.go`

Commands:
- `create-account.go`
- `update-account.go`
- `delete-account.go`

Queries:
- `get-account-by-id.go`
- `get-all-accounts.go`
- `get-account-balance.go`

## Data Flow

### Typical Request Flow

```
1. HTTP Request → Handler (Primary Adapter)
   ├─ Extract context (tracking, auth)
   ├─ Start OpenTelemetry span
   └─ Validate request structure

2. Handler → Use Case (Application Service)
   ├─ Domain validation (assert package)
   ├─ Business rule checks
   └─ Orchestrate operations

3. Use Case → Repository (Secondary Adapter)
   ├─ Data persistence (PostgreSQL)
   ├─ Metadata storage (MongoDB)
   └─ Cache operations (Valkey)

4. Use Case → Event Publisher (Secondary Adapter)
   └─ Publish domain events (RabbitMQ)

5. Repository → Database
   └─ Execute SQL queries

6. Response Flow (reverse)
   ├─ Database → Repository → Use Case
   ├─ Use Case → Handler
   └─ Handler → HTTP Response
```

### Cross-Service Communication

Services communicate via gRPC:

```
Onboarding Service ─┐
                    ├─→ gRPC Client (Secondary Adapter)
                    │      ↓
                    │   [Network]
                    │      ↓
Transaction Service ─┘   gRPC Server (Primary Adapter)
```

Example: Creating a transaction requires account validation from onboarding service.

## Dependency Injection

### Bootstrap Pattern

Location: `components/onboarding/internal/bootstrap/service.go`

```go
func InitializeService() *Service {
    // 1. Initialize infrastructure
    dbConn := initPostgresConnection()
    mongoConn := initMongoConnection()
    redisClient := initRedisClient()
    rabbitConn := initRabbitMQConnection()

    // 2. Create repositories (secondary adapters)
    accountRepo := postgres.NewAccountRepository(dbConn)
    metadataRepo := mongodb.NewMetadataRepository(mongoConn)

    // 3. Create use cases (business logic)
    commandUC := command.NewUseCase(accountRepo, metadataRepo, ...)
    queryUC := query.NewUseCase(accountRepo, metadataRepo, ...)

    // 4. Create handlers (primary adapters)
    accountHandler := httphandler.NewAccountHandler(commandUC, queryUC)

    // 5. Wire up HTTP routes
    router := setupRoutes(accountHandler, ...)

    return &Service{Router: router}
}
```

## Adding New Features

### Step-by-Step Guide

**1. Define Domain Model**
- Add struct to `pkg/mmodel/` or component-specific model
- Include JSON/database tags
- Add validation rules

**2. Create Repository Interface (if new entity)**
- Define interface in `internal/adapters/postgres/{entity}/{entity}.go`
- Include all CRUD operations needed

**3. Implement Repository**
- Create `{entity}.postgresql.go` with SQL queries
- Use parameterized queries to prevent SQL injection
- Handle errors consistently

**4. Generate Repository Mock**
- Run: `go generate ./...` (uses uber/mock directives)
- Mock file: `{entity}.postgresql_mock.go`

**5. Create Use Cases**

Commands (if write operation):
- Create `internal/services/command/create-{entity}.go`
- Add domain validation with `assert` package
- Implement business rules
- Call repository methods
- Wrap errors with context

Queries (if read operation):
- Create `internal/services/query/get-{entity}.go`
- Simple data retrieval
- Apply filters/pagination

**6. Create HTTP Handler**
- Add methods to handler in `internal/adapters/http/in/{entity}.go`
- Add Swagger annotations (see existing files)
- Extract context and start spans
- Delegate to use cases
- Return consistent responses

**7. Register Routes**
- Add route in `internal/bootstrap/routes.go` or similar
- Apply middleware (auth, rate limiting, etc.)

**8. Write Tests**
- Unit tests for use cases (`*_test.go`)
- Integration tests in `tests/integration/`
- See `docs/agents/testing.md` for details

**Example Reference:**
- Full implementation: Account entity in `components/onboarding/`
- Handler: `components/onboarding/internal/adapters/http/in/account.go:1`
- Use Case: `components/onboarding/internal/services/command/create-account.go:1`
- Repository: `components/onboarding/internal/adapters/postgres/account/account.postgresql.go:1`

## Design Principles

### Single Responsibility
- Each layer has one reason to change
- Handlers: HTTP concerns only
- Use Cases: Business logic only
- Repositories: Data access only

### Dependency Inversion
- Business logic depends on interfaces (ports)
- Infrastructure implements interfaces (adapters)
- Dependencies point inward (toward business logic)

### Open/Closed Principle
- Easy to add new adapters without changing business logic
- Example: Can swap PostgreSQL for another database by implementing the repository interface

## Common Patterns

### Error Handling at Boundaries
- Handlers log errors and return HTTP responses
- Use cases return wrapped errors
- Repositories return infrastructure errors

### Context Propagation
- Always pass `context.Context` as first parameter
- Extract tracking info at handler level
- Propagate through all layers

### Validation Layers
- Request structure validation: Handler level
- Domain invariants: Use case level with `assert` package
- Database constraints: Repository/migration level

## Anti-Patterns to Avoid

❌ **Business logic in handlers**
```go
// BAD
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    // Don't do business logic here!
    if account.Balance < 0 {
        return errors.New("negative balance")
    }
}
```

✅ **Business logic in use cases**
```go
// GOOD - Handler delegates to use case
func (h *Handler) CreateAccount(c *fiber.Ctx) error {
    account, err := h.Command.CreateAccount(ctx, input)
    return http.WithResponse(c, account, err)
}

// Business logic in use case
func (uc *UseCase) CreateAccount(ctx context.Context, input Input) (*Account, error) {
    assert.That(input.Balance >= 0, "balance must be non-negative")
    // ... business logic
}
```

❌ **Use cases depending on HTTP/Fiber types**
```go
// BAD
func (uc *UseCase) CreateAccount(c *fiber.Ctx) error {
    // Use cases should not know about HTTP framework
}
```

✅ **Use cases depend only on domain types**
```go
// GOOD
func (uc *UseCase) CreateAccount(ctx context.Context, input CreateAccountInput) (*Account, error) {
    // Only domain types, no framework dependencies
}
```

❌ **Direct database access in use cases**
```go
// BAD
func (uc *UseCase) CreateAccount(ctx context.Context) error {
    db.Exec("INSERT INTO accounts ...")  // Direct SQL
}
```

✅ **Use repository abstraction**
```go
// GOOD
func (uc *UseCase) CreateAccount(ctx context.Context) error {
    uc.AccountRepo.Create(ctx, account)  // Via interface
}
```

## Architecture Decision Records (ADRs)

Key decisions:
1. **Hexagonal Architecture**: Chosen for testability and flexibility
2. **CQRS**: Separate read/write optimizes for different access patterns
3. **Repository Pattern**: Abstracts data access for easier testing and swapping
4. **gRPC for inter-service**: Type-safe, efficient communication
5. **OpenTelemetry**: Standard observability across all services

## Related Documentation

- Error Handling: `docs/agents/error-handling.md`
- Testing Strategies: `docs/agents/testing.md`
- Database Patterns: `docs/agents/database.md`
- API Design: `docs/agents/api-design.md`
- Observability: `docs/agents/observability.md`
