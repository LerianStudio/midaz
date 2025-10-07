# Onboarding Services

## Overview

The `services` package implements the business logic layer for the Midaz onboarding service using the CQRS (Command Query Responsibility Segregation) pattern. This layer orchestrates operations across multiple repositories and enforces business rules.

## Purpose

This package provides:

- **Command operations**: Write operations (create, update, delete)
- **Query operations**: Read operations (get, list, find)
- **Business logic**: Validation, orchestration, and rule enforcement
- **Error handling**: Database error to business error conversion
- **Event publishing**: RabbitMQ message publishing for async processing
- **Metadata management**: MongoDB metadata operations

## Package Structure

```
services/
├── command/              # Command side (write operations)
│   ├── command.go       # UseCase struct and repository aggregation
│   ├── create-*.go      # Create operations (8 files)
│   ├── update-*.go      # Update operations (8 files)
│   ├── delete-*.go      # Delete operations (8 files)
│   ├── create-metadata.go
│   ├── update-metadata.go
│   └── send-account-queue-transaction.go
├── query/                # Query side (read operations)
│   ├── query.go         # UseCase struct and repository aggregation
│   ├── get-*.go         # Get single entity operations
│   └── list-*.go        # List paginated entities operations
├── errors.go             # Shared error handling utilities
└── README.md            # This file
```

## Architecture Pattern: CQRS

### Command Side (Write Operations)

**Package:** `command`

**Responsibilities:**

- Create new entities with validation
- Update existing entities with immutability checks
- Soft-delete entities with cascade rules
- Publish events to RabbitMQ
- Manage metadata in MongoDB
- Invalidate caches in Redis

**Key Characteristics:**

- Validates business rules before writes
- Orchestrates multiple repository calls
- Maintains data consistency
- Publishes domain events
- Uses database transactions at repository layer

### Query Side (Read Operations)

**Package:** `query`

**Responsibilities:**

- Retrieve single entities by ID
- List entities with pagination
- Filter entities by criteria
- Enrich entities with metadata
- Support sorting and date ranges

**Key Characteristics:**

- Optimized for read performance
- Metadata fetched separately and merged
- Supports offset and cursor pagination
- No side effects (read-only)
- Can leverage caching

## Entities Managed

### Organization

- Top-level entity representing companies or business units
- Supports hierarchical structures (parent-child relationships)
- Contains address information (ISO 3166-1 alpha-2 country codes)
- Has legal name, DBA name, and legal document

### Ledger

- Financial record-keeping system within an organization
- Contains assets, accounts, portfolios, and segments
- Names must be unique within an organization

### Asset

- Represents types of value (currencies, cryptocurrencies, commodities)
- Codes must be uppercase and unique within a ledger
- Currency assets must comply with ISO 4217
- Automatically creates external account on creation

### Account

- Financial buckets for tracking balances
- Linked to a specific asset code
- Can have parent-child relationships
- Can belong to portfolios and segments
- Aliases must be unique within a ledger
- External accounts are system-managed

### Portfolio

- Collections of accounts grouped for business purposes
- Used for organizing accounts by business unit, department, or client
- Can link to external entities via entity ID

### Segment

- Logical divisions within a ledger
- Used for categorizing accounts (e.g., by product line, region)
- Names must be unique within a ledger

### AccountType

- Account classification system
- Defines account categories (e.g., "deposit", "loan", "revenue")
- Optional validation during account creation
- Key values must be unique within organization and ledger

## Command Operations

### Create Operations

All create operations follow this pattern:

```go
func (uc *UseCase) Create{Entity}(ctx context.Context, ..., input *mmodel.Create{Entity}Input) (*mmodel.{Entity}, error)
```

**Common Steps:**

1. Validate business rules
2. Set default status (ACTIVE) if not provided
3. Generate UUIDv7 for ID
4. Create entity in PostgreSQL
5. Create metadata in MongoDB
6. Publish events if needed (e.g., accounts to RabbitMQ)
7. Return entity with metadata

**Example:**

```go
input := &mmodel.CreateAccountInput{
    Name:      "Corporate Checking",
    AssetCode: "USD",
    Type:      "deposit",
}
account, err := useCase.CreateAccount(ctx, orgID, ledgerID, input)
```

### Update Operations

All update operations follow this pattern:

```go
func (uc *UseCase) Update{Entity}ByID(ctx context.Context, ..., id uuid.UUID, input *mmodel.Update{Entity}Input) (*mmodel.{Entity}, error)
```

**Common Steps:**

1. Validate entity exists
2. Validate business rules (e.g., immutability, external account protection)
3. Update entity in PostgreSQL
4. Merge metadata in MongoDB (RFC 7396)
5. Return updated entity with merged metadata

**Example:**

```go
input := &mmodel.UpdateAccountInput{
    Name:   "Updated Account Name",
    Status: mmodel.Status{Code: "INACTIVE"},
}
account, err := useCase.UpdateAccount(ctx, orgID, ledgerID, nil, accountID, input)
```

### Delete Operations

All delete operations follow this pattern:

```go
func (uc *UseCase) Delete{Entity}ByID(ctx context.Context, ..., id uuid.UUID) error
```

**Common Steps:**

1. Validate entity exists
2. Validate business rules (e.g., external account protection)
3. Perform soft delete (set DeletedAt timestamp)
4. Entity remains in database for audit

**Example:**

```go
err := useCase.DeleteAccountByID(ctx, orgID, ledgerID, nil, accountID)
```

## Query Operations

### Get Operations

Retrieve single entities by ID:

```go
func (uc *UseCase) Get{Entity}ByID(ctx context.Context, ..., id uuid.UUID) (*mmodel.{Entity}, error)
```

**Steps:**

1. Fetch entity from PostgreSQL
2. Fetch metadata from MongoDB
3. Merge metadata into entity
4. Return enriched entity

**Example:**

```go
account, err := useCase.GetAccountByID(ctx, orgID, ledgerID, nil, accountID)
```

### List Operations

Retrieve paginated collections:

```go
func (uc *UseCase) List{Entities}(ctx context.Context, ..., filter QueryFilter) (*mmodel.{Entities}, error)
```

**Steps:**

1. Validate query parameters
2. Fetch entities from PostgreSQL with pagination
3. Fetch metadata for all entities
4. Merge metadata into entities
5. Return paginated collection

**Example:**

```go
filter := QueryFilter{
    Limit:     50,
    Page:      1,
    SortOrder: "desc",
}
accounts, err := useCase.ListAccounts(ctx, orgID, ledgerID, filter)
```

## Error Handling

### Database Error Conversion

The `errors.go` file provides utilities for converting database errors to business errors:

```go
// Sentinel error for not found conditions
var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// Convert PostgreSQL constraint errors
func ValidatePGError(pgErr *pgconn.PgError, entityType string) error
```

**Usage Pattern:**

```go
entity, err := repo.Find(ctx, id)
if err != nil {
    if errors.Is(err, services.ErrDatabaseItemNotFound) {
        return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")
    }
    return err
}
```

### Constraint Mapping

PostgreSQL constraints are mapped to business errors:

- `organization_parent_organization_id_fkey` → ErrParentOrganizationIDNotFound
- `account_parent_account_id_fkey` → ErrInvalidParentAccountID
- `account_asset_code_fkey` → ErrAssetCodeNotFound
- `account_portfolio_id_fkey` → ErrPortfolioIDNotFound
- `account_segment_id_fkey` → ErrSegmentIDNotFound
- `*_ledger_id_fkey` → ErrLedgerIDNotFound
- `*_organization_id_fkey` → ErrOrganizationIDNotFound
- `idx_account_type_unique_key_value` → ErrDuplicateAccountTypeKeyValue

## Business Rules

### Status Management

All entities support status codes:

- Default status: **ACTIVE** (if not provided)
- Common statuses: ACTIVE, INACTIVE, PENDING, SUSPENDED
- Status can be updated via update operations

### Metadata Management

Metadata is stored separately in MongoDB:

- **Create**: Stores new metadata document
- **Update**: Merges with existing metadata (RFC 7396 JSON Merge Patch)
- **Delete**: Metadata remains for audit (soft delete)

**Merge Semantics:**

```go
// Existing: {"dept": "Finance", "region": "US"}
// Update:   {"region": "EU", "cost": 123}
// Result:   {"dept": "Finance", "region": "EU", "cost": 123}
```

### External Accounts

External accounts are system-managed:

- Created automatically when assets are created
- Alias format: `@external/{ASSET_CODE}`
- Type: "external"
- **Cannot be updated or deleted by users**
- Used for tracking transactions with external systems

### Account Type Validation

Optional account type validation:

- Enabled per organization:ledger via `ACCOUNT_TYPE_VALIDATION` env var
- Format: `"orgID:ledgerID,orgID:ledgerID"`
- When enabled, account type must exist before creating accounts
- External accounts bypass this validation

### Immutable Fields

Certain fields cannot be changed after creation:

- Organization: Legal document
- Ledger: Organization ID
- Asset: Code, type
- Account: Asset code, type, alias
- All entities: ID, created_at

## Event Publishing

### RabbitMQ Integration

Accounts are published to RabbitMQ for transaction service processing:

```go
uc.SendAccountQueueTransaction(ctx, organizationID, ledgerID, account)
```

**Message Structure:**

```go
type Queue struct {
    OrganizationID uuid.UUID
    LedgerID       uuid.UUID
    AccountID      uuid.UUID
    QueueData      []QueueData
}
```

**Configuration:**

- `RABBITMQ_EXCHANGE`: Exchange name
- `RABBITMQ_KEY`: Routing key

**CRITICAL BUG:** Uses `logger.Fatalf` which crashes the application on failure. See BUGS.md.

## Data Storage

### PostgreSQL (Primary Data)

Tables:

- `organizations`: Organization records
- `ledgers`: Ledger records
- `assets`: Asset records
- `accounts`: Account records
- `portfolios`: Portfolio records
- `segments`: Segment records
- `account_types`: Account type records

**Features:**

- ACID transactions
- Foreign key constraints
- Unique constraints
- Soft delete support (deleted_at column)

### MongoDB (Metadata)

Collections (one per entity type):

- Flexible schema
- Document structure:
  ```json
  {
    "entity_id": "uuid",
    "entity_name": "Account",
    "data": { "key": "value" },
    "created_at": "timestamp",
    "updated_at": "timestamp"
  }
  ```

### RabbitMQ (Events)

Exchanges and queues for async processing:

- Account creation events
- Routed to transaction service
- Enables balance initialization

### Redis (Caching)

Available for:

- Caching frequently accessed entities
- Distributed locking
- Session management

## OpenTelemetry Integration

All operations are traced:

**Span Naming Convention:**

- Command: `command.{operation}_{entity}`
- Query: `query.{operation}_{entity}`
- Example: `command.create_account`, `query.get_account_by_id`

**Span Events:**

- Business errors recorded as span events
- Includes error type and message
- Helps with debugging and monitoring

**Example:**

```go
ctx, span := tracer.Start(ctx, "command.create_account")
defer span.End()

if err != nil {
    libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create account", err)
    return nil, err
}
```

## Usage Patterns

### Command Pattern

```go
// In HTTP handler
func createAccountHandler(p any, c *fiber.Ctx) error {
    input := p.(*mmodel.CreateAccountInput)

    // Extract path parameters
    orgID := c.Locals("organization_id").(uuid.UUID)
    ledgerID := c.Locals("ledger_id").(uuid.UUID)

    // Call command use case
    account, err := commandUC.CreateAccount(c.Context(), orgID, ledgerID, input)
    if err != nil {
        return http.WithError(c, err)
    }

    return http.Created(c, account)
}
```

### Query Pattern

```go
// In HTTP handler
func getAccountHandler(c *fiber.Ctx) error {
    // Extract path parameters
    orgID := c.Locals("organization_id").(uuid.UUID)
    ledgerID := c.Locals("ledger_id").(uuid.UUID)
    accountID := c.Locals("id").(uuid.UUID)

    // Call query use case
    account, err := queryUC.GetAccountByID(c.Context(), orgID, ledgerID, nil, accountID)
    if err != nil {
        return http.WithError(c, err)
    }

    return http.OK(c, account)
}
```

### List Pattern with Pagination

```go
func listAccountsHandler(c *fiber.Ctx) error {
    // Parse query parameters
    query, err := http.ValidateParameters(c.AllParams())
    if err != nil {
        return http.WithError(c, err)
    }

    orgID := c.Locals("organization_id").(uuid.UUID)
    ledgerID := c.Locals("ledger_id").(uuid.UUID)

    // Call query use case
    accounts, total, err := queryUC.ListAccounts(c.Context(), orgID, ledgerID, query)
    if err != nil {
        return http.WithError(c, err)
    }

    // Set total count header
    c.Set(constant.XTotalCount, strconv.Itoa(total))

    return http.OK(c, accounts)
}
```

## Validation Rules

### Asset Validation

- **Type**: Must be "currency", "crypto", "commodities", or "others"
- **Code**: Alphanumeric, uppercase, at least one letter
- **Currency**: Must comply with ISO 4217 if type is "currency"
- **Uniqueness**: Name and code must be unique within ledger

### Account Validation

- **Asset Code**: Must exist in the ledger
- **Parent Account**: Must exist and have matching asset code
- **Alias**: Must be unique, match pattern `^[a-zA-Z0-9@:_-]+$`
- **Type**: Must exist if accounting validation is enabled
- **External**: Cannot create, update, or delete external accounts

### Organization Validation

- **Country Code**: Must be valid ISO 3166-1 alpha-2 (2 letters)
- **Parent**: Cannot be self-referential
- **Legal Document**: Required and immutable

### Ledger Validation

- **Name**: Must be unique within organization
- **Organization**: Must exist

### Portfolio/Segment Validation

- **Name**: Must be unique within ledger (segments only)
- **Ledger**: Must exist

## Dependencies

### Repositories

Command UseCase depends on:

- `OrganizationRepo`: Organization CRUD
- `LedgerRepo`: Ledger CRUD
- `AssetRepo`: Asset CRUD
- `AccountRepo`: Account CRUD
- `PortfolioRepo`: Portfolio CRUD
- `SegmentRepo`: Segment CRUD
- `AccountTypeRepo`: Account type CRUD
- `MetadataRepo`: MongoDB metadata operations
- `RabbitMQRepo`: Event publishing
- `RedisRepo`: Caching and locking

Query UseCase depends on:

- Same as command, minus RabbitMQRepo

### External Packages

- `github.com/LerianStudio/lib-commons/v2`: Shared utilities
- `github.com/LerianStudio/midaz/v3/pkg`: Error handling, models, constants
- `github.com/google/uuid`: UUID generation and parsing

## Configuration

### Environment Variables

- **ACCOUNT_TYPE_VALIDATION**: Enable account type validation
  - Format: "orgID:ledgerID,orgID:ledgerID"
  - Example: "123e4567-...:987f6543-...,..."
- **RABBITMQ_EXCHANGE**: RabbitMQ exchange name
- **RABBITMQ_KEY**: RabbitMQ routing key

## Error Handling Best Practices

### Convert Database Errors

```go
entity, err := repo.Create(ctx, entity)
if err != nil {
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        return services.ValidatePGError(pgErr, "Entity")
    }
    return err
}
```

### Handle Not Found

```go
entity, err := repo.Find(ctx, id)
if err != nil {
    if errors.Is(err, services.ErrDatabaseItemNotFound) {
        return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")
    }
    return err
}
```

### Trace Errors

```go
if err != nil {
    libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Operation failed", err)
    logger.Errorf("Error: %v", err)
    return nil, err
}
```

## Testing

### Unit Testing

Mock repositories for testing use cases:

```go
mockRepo := &MockAccountRepo{}
useCase := &command.UseCase{
    AccountRepo: mockRepo,
    // ... other repos
}

mockRepo.On("Create", mock.Anything, mock.Anything).Return(account, nil)
result, err := useCase.CreateAccount(ctx, orgID, ledgerID, input)
```

### Integration Testing

Test with real databases:

- PostgreSQL test database
- MongoDB test database
- RabbitMQ test instance

## Performance Considerations

### Metadata Operations

- Metadata is fetched separately (additional query)
- Consider caching frequently accessed metadata
- Batch metadata fetches for list operations

### UUID Generation

- Uses UUIDv7 for time-ordered IDs
- Better database performance than UUIDv4
- Maintains chronological ordering

### Transaction Boundaries

- Database transactions handled at repository layer
- Each use case method is a single business transaction
- Rollback on any error

## Security Considerations

### External Account Protection

External accounts are protected:

- Cannot be created by users
- Cannot be updated
- Cannot be deleted
- Checks enforced in update and delete operations

### Validation

All inputs validated:

- At HTTP layer (struct validation)
- At service layer (business rules)
- At database layer (constraints)

### Audit Trail

Soft deletes preserve audit trail:

- All operations logged
- Deleted entities remain in database
- Timestamps track all changes

## Known Issues

### Critical

1. **logger.Fatalf in SendAccountQueueTransaction**
   - Crashes entire application on queue failure
   - Should return error instead
   - See BUGS.md for details

### Low Priority

2. **Inconsistent span naming**
   - DeleteOrganizationByID uses "usecase." instead of "command."
   - Should be standardized

## Future Enhancements

Potential improvements:

- Batch operations for multiple entities
- Async metadata operations
- Cache-aside pattern for reads
- Event sourcing for audit trail
- Saga pattern for distributed transactions
- Retry logic for RabbitMQ publishing

## Related Documentation

- [Bootstrap Layer](../bootstrap/README.md): Application initialization
- [Adapters Layer](../adapters/README.md): Infrastructure implementations
- [HTTP Layer](../adapters/http/in/README.md): HTTP handlers
- [pkg/mmodel](../../../../pkg/mmodel/README.md): Domain models
- [pkg/constant](../../../../pkg/constant/README.md): Error codes

## Version History

This package follows semantic versioning as part of the Midaz v3 module.
