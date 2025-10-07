# Package http

## Overview

The `http` package provides HTTP utilities and helpers for the Midaz ledger system. It contains functions for request/response handling, validation, error conversion, and middleware utilities built on top of the Fiber web framework.

## Purpose

This package provides:

- **Error handling**: Convert domain errors to HTTP responses
- **Request validation**: Decode and validate JSON request bodies
- **Response helpers**: Standardized HTTP response functions
- **Query parameter parsing**: Parse and validate query parameters
- **Middleware**: UUID validation, body decoding, and more
- **File handling**: DSL file upload and validation
- **Idempotency**: Support for idempotent requests

## Package Structure

```
http/
├── errors.go      # Error handling and conversion
├── response.go    # HTTP response helper functions
├── httputils.go   # Query parameter parsing and utilities
├── withBody.go    # Request body decoding and validation
└── README.md      # This file
```

## Key Components

### Error Handling (errors.go)

#### WithError

Central error handling function that converts domain errors to HTTP responses:

```go
func WithError(c *fiber.Ctx, err error) error
```

**Error Type Mappings:**

- `EntityNotFoundError` → 404 Not Found
- `EntityConflictError` → 409 Conflict
- `ValidationError` → 400 Bad Request
- `UnprocessableOperationError` → 422 Unprocessable Entity
- `UnauthorizedError` → 401 Unauthorized
- `ForbiddenError` → 403 Forbidden
- `InternalServerError` → 500 Internal Server Error

**Example:**

```go
account, err := service.GetAccount(id)
if err != nil {
    return http.WithError(c, err)
}
return http.OK(c, account)
```

### Response Helpers (response.go)

Standardized response functions for common HTTP status codes:

#### Success Responses

- **OK** (200): Successful GET, PUT, PATCH operations
- **Created** (201): Successful POST operations
- **Accepted** (202): Asynchronous operations accepted
- **NoContent** (204): Successful DELETE operations
- **PartialContent** (206): Partial data responses

#### Error Responses

- **BadRequest** (400): Malformed or invalid requests
- **Unauthorized** (401): Missing or invalid authentication
- **Forbidden** (403): Insufficient permissions
- **NotFound** (404): Resource not found
- **Conflict** (409): Duplicate or conflicting resource
- **UnprocessableEntity** (422): Business logic errors
- **RangeNotSatisfiable** (416): Invalid range request
- **InternalServerError** (500): Unexpected server errors
- **NotImplemented** (501): Feature not implemented

**Examples:**

```go
// Success responses
return http.OK(c, account)
return http.Created(c, newAccount)
return http.NoContent(c)

// Error responses
return http.NotFound(c, "0052", "Account Not Found", "The account ID does not exist.")
return http.Conflict(c, "0020", "Alias Unavailable", "The alias is already in use.")
return http.UnprocessableEntity(c, "0018", "Insufficient Funds", "Balance too low.")
```

### Query Parameter Parsing (httputils.go)

#### QueryHeader

Struct representing parsed query parameters:

```go
type QueryHeader struct {
    Metadata      *bson.M   // MongoDB metadata filter
    Limit         int       // Items per page (default: 10, max: 100)
    Page          int       // Page number (default: 1)
    Cursor        string    // Cursor for cursor-based pagination
    SortOrder     string    // "asc" or "desc" (default: "asc")
    StartDate     time.Time // Date range start
    EndDate       time.Time // Date range end
    UseMetadata   bool      // Metadata filtering enabled
    PortfolioID   string    // Portfolio filter
    OperationType string    // Operation type filter
    ToAssetCodes  []string  // Asset code filters
}
```

#### ValidateParameters

Parses and validates query parameters:

```go
func ValidateParameters(params map[string]string) (*QueryHeader, error)
```

**Supported Parameters:**

- `limit`: Items per page (1-100)
- `page`: Page number (1+)
- `cursor`: Base64-encoded cursor
- `sort_order`: "asc" or "desc"
- `start_date`: Date in "yyyy-mm-dd" format
- `end_date`: Date in "yyyy-mm-dd" format
- `portfolio_id`: UUID filter
- `type`: Operation type filter
- `to`: Comma-separated asset codes
- `metadata.*`: Metadata filters

**Example:**

```go
params := map[string]string{
    "limit": "50",
    "page": "2",
    "sort_order": "desc",
    "start_date": "2024-01-01",
    "end_date": "2024-12-31",
}
query, err := http.ValidateParameters(params)
if err != nil {
    return http.WithError(c, err)
}
```

#### Pagination Methods

Convert QueryHeader to pagination formats:

```go
// Offset-based pagination (page numbers)
pagination := query.ToOffsetPagination()

// Cursor-based pagination (cursor tokens)
pagination := query.ToCursorPagination()
```

#### Idempotency Support

Extract idempotency headers:

```go
func GetIdempotencyKeyAndTTL(c *fiber.Ctx) (string, time.Duration)
```

**Example:**

```go
ikey, ttl := http.GetIdempotencyKeyAndTTL(c)
if ikey != "" {
    // Check for duplicate request
    if exists := redis.Exists(ikey); exists {
        return http.Conflict(c, "0084", "Duplicate Request", "Already processed")
    }
    // Process and store result
    result, err := service.CreateTransaction(input)
    redis.Set(ikey, result, ttl)
}
```

#### DSL File Handling

Extract DSL files from multipart form uploads:

```go
func GetFileFromHeader(ctx *fiber.Ctx) (string, error)
```

**Example:**

```go
dslContent, err := http.GetFileFromHeader(c)
if err != nil {
    return http.WithError(c, err)
}
transaction, err := parser.Parse(dslContent)
```

### Request Body Handling (withBody.go)

#### WithBody Decorator

Wraps handlers with automatic JSON decoding and validation:

```go
func WithBody(s any, h DecodeHandlerFunc) fiber.Handler
```

**Features:**

- Automatic JSON unmarshalling
- Unknown field detection
- Struct validation using tags
- Null byte security validation
- RFC 7396 JSON Merge Patch support for metadata

**Example:**

```go
func createAccountHandler(p any, c *fiber.Ctx) error {
    input := p.(*mmodel.CreateAccountInput)
    account, err := service.CreateAccount(input)
    if err != nil {
        return http.WithError(c, err)
    }
    return http.Created(c, account)
}

app.Post("/accounts",
    http.WithBody(&mmodel.CreateAccountInput{}, createAccountHandler))
```

#### WithDecode Decorator

Similar to WithBody but uses a constructor function:

```go
func WithDecode(c ConstructorFunc, h DecodeHandlerFunc) fiber.Handler
```

**Example:**

```go
func newCreateAccountInput() any {
    return &mmodel.CreateAccountInput{
        Status: mmodel.Status{Code: "ACTIVE"}, // Default status
    }
}

app.Post("/accounts",
    http.WithDecode(newCreateAccountInput, createAccountHandler))
```

#### ParseUUIDPathParameters Middleware

Validates UUID path parameters:

```go
func ParseUUIDPathParameters(entityName string) fiber.Handler
```

**Example:**

```go
app.Get("/v1/organizations/:organization_id/ledgers/:id",
    http.ParseUUIDPathParameters("ledger"),
    getLedgerHandler)

func getLedgerHandler(c *fiber.Ctx) error {
    orgID := c.Locals("organization_id").(uuid.UUID)
    ledgerID := c.Locals("id").(uuid.UUID)
    // Use parsed UUIDs...
}
```

#### ValidateStruct

Validates structs using go-playground/validator:

```go
func ValidateStruct(s any) error
```

**Custom Validation Tags:**

- `keymax`: Maximum metadata key length
- `valuemax`: Maximum metadata value length
- `nonested`: Prevents nested metadata objects
- `singletransactiontype`: One transaction type per entry
- `prohibitedexternalaccountprefix`: Prevents @external/ prefix
- `invalidstrings`: Prevents specific strings
- `invalidaliascharacters`: Validates alias format
- `invalidaccounttype`: Validates account type format
- `nowhitespaces`: Prevents whitespace

**Example:**

```go
input := &mmodel.CreateAccountInput{
    Name: "Account",
    AssetCode: "USD",
}
if err := http.ValidateStruct(input); err != nil {
    return http.BadRequest(c, err)
}
```

#### FindUnknownFields

Detects fields in JSON that aren't in the struct:

```go
func FindUnknownFields(original, marshaled map[string]any) map[string]any
```

**Example:**

```go
original := map[string]any{"name": "Account", "extra": "field"}
marshaled := map[string]any{"name": "Account"}
unknown := FindUnknownFields(original, marshaled)
// Returns: {"extra": "field"}
```

## Usage Patterns

### Basic Handler Pattern

```go
func createAccountHandler(p any, c *fiber.Ctx) error {
    // Type assert the payload
    input := p.(*mmodel.CreateAccountInput)

    // Call business logic
    account, err := service.CreateAccount(input)
    if err != nil {
        return http.WithError(c, err)
    }

    // Return success response
    return http.Created(c, account)
}

// Register route with body decoding
app.Post("/accounts",
    http.WithBody(&mmodel.CreateAccountInput{}, createAccountHandler))
```

### List Handler Pattern

```go
func listAccountsHandler(c *fiber.Ctx) error {
    // Parse query parameters
    query, err := http.ValidateParameters(c.AllParams())
    if err != nil {
        return http.WithError(c, err)
    }

    // Call business logic
    accounts, total, err := service.ListAccounts(query)
    if err != nil {
        return http.WithError(c, err)
    }

    // Set total count header
    c.Set(constant.XTotalCount, strconv.Itoa(total))

    // Return paginated response
    return http.OK(c, accounts)
}

app.Get("/accounts", listAccountsHandler)
```

### Update Handler Pattern

```go
func updateAccountHandler(p any, c *fiber.Ctx) error {
    // Get ID from path
    accountID := c.Locals("id").(uuid.UUID)

    // Type assert the payload
    input := p.(*mmodel.UpdateAccountInput)

    // Call business logic
    account, err := service.UpdateAccount(accountID, input)
    if err != nil {
        return http.WithError(c, err)
    }

    // Return updated resource
    return http.OK(c, account)
}

app.Patch("/accounts/:id",
    http.ParseUUIDPathParameters("account"),
    http.WithBody(&mmodel.UpdateAccountInput{}, updateAccountHandler))
```

### Delete Handler Pattern

```go
func deleteAccountHandler(c *fiber.Ctx) error {
    // Get ID from path
    accountID := c.Locals("id").(uuid.UUID)

    // Call business logic
    if err := service.DeleteAccount(accountID); err != nil {
        return http.WithError(c, err)
    }

    // Return no content
    return http.NoContent(c)
}

app.Delete("/accounts/:id",
    http.ParseUUIDPathParameters("account"),
    deleteAccountHandler)
```

### Idempotent Handler Pattern

```go
func createTransactionHandler(p any, c *fiber.Ctx) error {
    input := p.(*mmodel.CreateTransactionInput)

    // Get idempotency key
    ikey, ttl := http.GetIdempotencyKeyAndTTL(c)
    if ikey != "" {
        // Check if already processed
        if result, exists := redis.Get(ikey); exists {
            return http.OK(c, result)
        }
    }

    // Process transaction
    transaction, err := service.CreateTransaction(input)
    if err != nil {
        return http.WithError(c, err)
    }

    // Store idempotency record
    if ikey != "" {
        redis.Set(ikey, transaction, ttl)
    }

    return http.Created(c, transaction)
}
```

### DSL File Upload Pattern

```go
func createTransactionFromDSLHandler(c *fiber.Ctx) error {
    // Extract DSL file
    dslContent, err := http.GetFileFromHeader(c)
    if err != nil {
        return http.WithError(c, err)
    }

    // Parse DSL
    transaction, err := parser.Parse(dslContent)
    if err != nil {
        return http.WithError(c, err)
    }

    // Process transaction
    result, err := service.CreateTransaction(transaction)
    if err != nil {
        return http.WithError(c, err)
    }

    return http.Created(c, result)
}

app.Post("/transactions/dsl", createTransactionFromDSLHandler)
```

## Validation

### Struct Validation

The package uses go-playground/validator with custom rules:

```go
type CreateAccountInput struct {
    Name      string         `json:"name" validate:"required,max=256"`
    AssetCode string         `json:"assetCode" validate:"required,max=100"`
    Alias     *string        `json:"alias" validate:"omitempty,invalidaliascharacters"`
    Type      string         `json:"type" validate:"required,invalidstrings=external"`
    Metadata  map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}
```

### Custom Validation Rules

#### keymax

Validates metadata key length:

```go
Metadata map[string]any `validate:"dive,keys,keymax=100"`
```

#### valuemax

Validates metadata value length:

```go
Metadata map[string]any `validate:"dive,keys,keymax=100,endkeys,valuemax=2000"`
```

#### nonested

Prevents nested objects in metadata:

```go
Metadata map[string]any `validate:"nonested"`
```

#### invalidaliascharacters

Validates alias format (only a-zA-Z0-9@:\_-):

```go
Alias *string `validate:"invalidaliascharacters"`
```

#### prohibitedexternalaccountprefix

Prevents @external/ prefix:

```go
Alias *string `validate:"prohibitedexternalaccountprefix"`
```

#### invalidstrings

Prevents specific strings:

```go
Type string `validate:"invalidstrings=external,admin"`
```

#### nowhitespaces

Prevents whitespace:

```go
Key string `validate:"nowhitespaces"`
```

### Unknown Field Detection

The package automatically detects fields in JSON that aren't in the struct:

```go
// Request: {"name": "Account", "extraField": "value"}
// Response: 400 Bad Request
// {
//   "code": "0053",
//   "title": "Unexpected Fields in the Request",
//   "fields": {"extraField": "value"}
// }
```

### Null Byte Validation

All string fields are checked for null bytes (\x00) to prevent injection attacks:

```go
// Request: {"name": "Account\x00WithNull"}
// Response: 400 Bad Request
// {
//   "code": "0047",
//   "fields": {"name": "name cannot contain null byte (\\x00)"}
// }
```

## Query Parameter Handling

### Pagination

**Offset-based pagination:**

```go
// Request: GET /accounts?page=2&limit=50&sort_order=desc
query, _ := http.ValidateParameters(params)
pagination := query.ToOffsetPagination()
// Use pagination.Page and pagination.Limit
```

**Cursor-based pagination:**

```go
// Request: GET /accounts?cursor=eyJpZCI6MTIzfQ&limit=50
query, _ := http.ValidateParameters(params)
pagination := query.ToCursorPagination()
// Use pagination.Cursor and pagination.Limit
```

### Date Range Filtering

```go
// Request: GET /transactions?start_date=2024-01-01&end_date=2024-12-31
query, _ := http.ValidateParameters(params)
// Use query.StartDate and query.EndDate
```

**Default Behavior:**

- If no dates provided: last N months (from MAX_PAGINATION_MONTH_DATE_RANGE env var)
- If MAX_PAGINATION_MONTH_DATE_RANGE=0: all time (Unix epoch to now)

### Metadata Filtering

```go
// Request: GET /accounts?metadata.department=Sales
query, _ := http.ValidateParameters(params)
if query.UseMetadata {
    // Use query.Metadata for MongoDB filtering
}
```

### Entity Filtering

```go
// Request: GET /accounts?portfolio_id=123e4567-e89b-12d3-a456-426614174000
query, _ := http.ValidateParameters(params)
// Use query.PortfolioID

// Request: GET /operations?type=DEBIT
query, _ := http.ValidateParameters(params)
// Use query.OperationType

// Request: GET /rates?to=USD,EUR,BTC
query, _ := http.ValidateParameters(params)
// Use query.ToAssetCodes
```

## Middleware

### UUID Path Parameter Validation

Automatically validates UUID path parameters:

```go
app.Get("/accounts/:id",
    http.ParseUUIDPathParameters("account"),
    getAccountHandler)
```

**Benefits:**

- Validates UUIDs before reaching handler
- Stores parsed UUIDs in context.Locals
- Adds parameters to OpenTelemetry spans
- Returns 400 Bad Request for invalid UUIDs

### Body Decoding and Validation

Automatically decodes and validates request bodies:

```go
app.Post("/accounts",
    http.WithBody(&mmodel.CreateAccountInput{}, createAccountHandler))
```

**Features:**

- JSON unmarshalling
- Unknown field detection
- Struct validation
- Null byte validation
- Metadata parsing (RFC 7396)

## Configuration

The package uses environment variables for configuration:

- **MAX_PAGINATION_LIMIT**: Maximum items per page (default: 100)
- **MAX_PAGINATION_MONTH_DATE_RANGE**: Maximum date range in months (default: 1)

Set in environment or .env file:

```bash
MAX_PAGINATION_LIMIT=100
MAX_PAGINATION_MONTH_DATE_RANGE=12
```

## Error Response Format

All errors follow a consistent JSON format:

```json
{
  "code": "0052",
  "title": "Account Not Found",
  "message": "The provided account ID does not exist in our records."
}
```

**With field-level validation:**

```json
{
  "code": "0009",
  "title": "Missing Fields in Request",
  "message": "Your request is missing required fields.",
  "fields": {
    "name": "name is a required field",
    "assetCode": "assetCode is a required field"
  }
}
```

**With unknown fields:**

```json
{
  "code": "0053",
  "title": "Unexpected Fields in the Request",
  "message": "The request contains unexpected fields.",
  "fields": {
    "extraField": "value",
    "anotherField": 123
  }
}
```

## Best Practices

### Always Use WithError

Convert domain errors to HTTP responses using WithError:

```go
// Good
if err != nil {
    return http.WithError(c, err)
}

// Bad - loses error type information
if err != nil {
    return http.InternalServerError(c, "0046", "Error", err.Error())
}
```

### Use Appropriate Response Functions

Choose the correct response function for the operation:

```go
// POST (create) → Created
return http.Created(c, newAccount)

// GET (read) → OK
return http.OK(c, account)

// PUT/PATCH (update) → OK
return http.OK(c, updatedAccount)

// DELETE → NoContent
return http.NoContent(c)
```

### Validate Path Parameters

Always use ParseUUIDPathParameters for UUID path parameters:

```go
app.Get("/accounts/:id",
    http.ParseUUIDPathParameters("account"),
    handler)
```

### Handle Idempotency

For non-idempotent operations (POST), support idempotency keys:

```go
ikey, ttl := http.GetIdempotencyKeyAndTTL(c)
if ikey != "" {
    // Check cache, process, store result
}
```

### Validate Query Parameters

Always validate query parameters for list endpoints:

```go
query, err := http.ValidateParameters(c.AllParams())
if err != nil {
    return http.WithError(c, err)
}
```

## Security Features

### Null Byte Validation

Automatically validates that no string fields contain null bytes (\x00):

- Prevents null byte injection attacks
- Protects database drivers
- Ensures data integrity

### Unknown Field Detection

Rejects requests with unexpected fields:

- Enforces strict API contracts
- Prevents accidental data leakage
- Helps catch client errors early

### Metadata Constraints

Validates metadata to prevent abuse:

- Key length: max 100 characters
- Value length: max 2000 characters
- No nested objects allowed
- Prevents DoS via large metadata

### UUID Validation

Validates all UUID path parameters:

- Prevents invalid UUID errors in business logic
- Provides early validation feedback
- Improves security posture

## Performance Considerations

### Reflection Overhead

WithBody uses reflection to create struct instances:

- Use WithDecode with a constructor for better performance
- Constructor avoids reflection on each request
- Minimal impact for most use cases

### Validation Caching

Validator instances are created per request:

- Consider caching validator instances for high-throughput scenarios
- Current implementation prioritizes correctness over performance

### Unknown Field Detection

Requires marshaling/unmarshaling twice:

- Necessary for strict API contracts
- Overhead is acceptable for API endpoints
- Can be disabled if needed (modify FiberHandlerFunc)

## Dependencies

This package depends on:

- `github.com/gofiber/fiber/v2`: Web framework
- `gopkg.in/go-playground/validator.v9`: Struct validation
- `github.com/google/uuid`: UUID parsing
- `github.com/shopspring/decimal`: Decimal comparison
- `github.com/LerianStudio/lib-commons/v2`: Shared utilities
- `go.mongodb.org/mongo-driver/bson`: MongoDB query building
- Standard library: `encoding/json`, `reflect`, `regexp`, `strings`, `time`

## Related Packages

- `pkg`: Error types used throughout
- `pkg/constant`: Error codes and constants
- `pkg/mmodel`: Request/response models
- `components/onboarding`: Uses HTTP utilities
- `components/transaction`: Uses HTTP utilities

## Testing

The package includes comprehensive tests:

- `withBody_test.go`: Request body handling tests

Run tests:

```bash
go test ./pkg/net/http/...
```

## Future Enhancements

Potential additions:

- Response caching middleware
- Rate limiting middleware
- Request ID middleware
- CORS configuration helpers
- Content negotiation utilities
- Streaming response support

## Version History

This package follows semantic versioning as part of the Midaz v3 module.
