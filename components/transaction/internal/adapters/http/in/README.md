# HTTP Handlers - Transaction

This package contains HTTP request handlers for the transaction service API endpoints.

## Structure

- `assetrate.go` - Asset exchange rate management endpoints
- `balance.go` - Balance query and management endpoints
- `operation.go` - Operation management within transactions
- `operation-route.go` - Operation routing rule endpoints
- `transaction.go` - Transaction creation and management endpoints
- `transaction-route.go` - Transaction routing configuration endpoints
- `routes.go` - HTTP routing configuration
- `swagger.go` - API documentation endpoints

## Handler Pattern

Each handler struct encapsulates:

- Command use case for write operations
- Query use case for read operations

Example:

```go
type TransactionHandler struct {
    Command *command.UseCase
    Query   *query.UseCase
}
```

## Request Flow

1. **Path Parameter Validation** - UUIDs extracted and validated
2. **Request Body Decoding** - Using `http.WithBody` middleware
3. **Business Logic Execution** - Delegated to command/query use cases
4. **Response Formatting** - Consistent JSON responses with proper HTTP status codes
5. **Error Handling** - Business errors mapped to appropriate HTTP responses

## Special Features

### Transaction Processing

- Support for both synchronous and asynchronous processing
- Idempotency key handling for duplicate prevention
- Gold DSL parsing for complex transaction definitions

### Balance Operations

- Real-time balance queries with cache integration
- Support for pending transaction holds
- Atomic balance updates via Redis/Lua scripts

## Swagger Documentation

Each handler method includes comprehensive Swagger annotations:

- Summary and description
- Request/response schemas
- Status codes and error responses
- Security requirements

## Pagination

List endpoints support cursor-based pagination for scalability:

- `cursor` - Continuation token from previous response
- `limit` - Page size (default: 10, max: 100)
- Date range and metadata filtering
