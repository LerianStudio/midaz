# HTTP Handlers - Onboarding

This package contains HTTP request handlers for the onboarding service API endpoints.

## Structure

- `account.go` - Account management endpoints
- `accounttype.go` - Account type configuration endpoints
- `asset.go` - Asset and currency management endpoints
- `ledger.go` - Ledger lifecycle endpoints
- `organization.go` - Organization management endpoints
- `portfolio.go` - Portfolio grouping endpoints
- `segment.go` - Segment management endpoints
- `routes.go` - HTTP routing configuration
- `swagger.go` - API documentation endpoints

## Handler Pattern

Each handler struct encapsulates:

- Command use case for write operations
- Query use case for read operations

Example:

```go
type AccountHandler struct {
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

## Swagger Documentation

Each handler method includes comprehensive Swagger annotations:

- Summary and description
- Request/response schemas
- Status codes and error responses
- Security requirements

## Pagination

List endpoints support standard pagination parameters:

- `limit` - Page size (default: 10, max: 100)
- `page` - Page number (1-based)
- `sort_order` - Sort direction (asc/desc)
- `start_date`/`end_date` - Date range filtering
