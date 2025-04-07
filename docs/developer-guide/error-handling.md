# Error Handling

**Navigation:** [Home](../) > [Developer Guide](./) > Error Handling

This document describes the error handling approach in the Midaz system, covering error types, error propagation, and best practices for managing errors.

## Overview

Midaz implements a robust error handling system with these key features:

- **Domain-Specific Error Types**: Clearly defined error types that represent different error categories
- **Consistent Error Structure**: Standardized error format across the system
- **Error Translation**: Conversion between different error representations
- **Client-Friendly Errors**: User-friendly error messages with clear guidance
- **Developer-Friendly Debugging**: Detailed error information for troubleshooting

This approach allows for precise error handling while maintaining good user experience and debuggability.

## Error Model

### Core Error Types

The system defines a hierarchy of error types in `pkg/errors.go`:

```go
// EntityNotFoundError records an error indicating an entity was not found
type EntityNotFoundError struct {
    EntityType string `json:"entityType,omitempty"`
    Title      string `json:"title,omitempty"`
    Message    string `json:"message,omitempty"`
    Code       string `json:"code,omitempty"`
    Err        error  `json:"err,omitempty"`
}

// ValidationError records validation errors
type ValidationError struct {
    EntityType string `json:"entityType,omitempty"`
    Title      string `json:"title,omitempty"`
    Message    string `json:"message,omitempty"`
    Code       string `json:"code,omitempty"`
    Err        error  `json:"err,omitempty"`
}

// EntityConflictError records when an entity already exists
type EntityConflictError struct {
    EntityType string `json:"entityType,omitempty"`
    Title      string `json:"title,omitempty"`
    Message    string `json:"message,omitempty"`
    Code       string `json:"code,omitempty"`
    Err        error  `json:"err,omitempty"`
}

// Additional error types:
// - UnauthorizedError
// - ForbiddenError
// - UnprocessableOperationError
// - HTTPError
// - InternalServerError
// - ValidationKnownFieldsError
// - ValidationUnknownFieldsError
// - ResponseError
```

Each error type follows a consistent structure with:
- **EntityType**: The type of entity involved in the error
- **Title**: A short title describing the error
- **Message**: A human-readable message explaining the error
- **Code**: A unique error code for identification
- **Err**: The original underlying error (for error wrapping)

### Error Codes

Error codes are defined as constants in `pkg/constant/errors.go` and provide unique identifiers for each error type:

```go
var (
    ErrEntityNotFound = errors.New("ENTITY_NOT_FOUND")
    ErrDuplicateLedger = errors.New("DUPLICATE_LEDGER")
    ErrInsufficientFunds = errors.New("INSUFFICIENT_FUNDS")
    // ... many more error codes
)
```

These codes are used to identify errors across services and in API responses.

## Error Handling Patterns

### Error Creation

Errors are created using factory functions that ensure consistent error structure:

```go
// ValidateBusinessError translates error codes to domain error types
func ValidateBusinessError(err error, entityType string, args ...any) error {
    // Maps error codes to appropriate error types
    errorMap := map[error]error{
        constant.ErrEntityNotFound: EntityNotFoundError{
            EntityType: entityType,
            Code:       constant.ErrEntityNotFound.Error(),
            Title:      "Entity Not Found",
            Message:    "No entity was found for the given ID...",
        },
        // ... many more mappings
    }

    if mappedError, found := errorMap[err]; found {
        return mappedError
    }

    return err
}
```

### Error Propagation

Errors flow through the system via:

1. **Return-based propagation**:
   ```go
   func (uc *UseCase) GetEntityByID(ctx context.Context, id string) (*Entity, error) {
       entity, err := uc.repo.FindByID(ctx, id)
       if err != nil {
           return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")
       }
       return entity, nil
   }
   ```

2. **Error wrapping**:
   ```go
   if err != nil {
       return EntityNotFoundError{
           EntityType: entityType,
           Message:    "Entity not found",
           Err:        err, // Original error is preserved
       }
   }
   ```

3. **Error typing**:
   ```go
   // Type switches for handling different error types
   switch e := err.(type) {
   case pkg.EntityNotFoundError:
       // Handle not found
   case pkg.ValidationError:
       // Handle validation error
   }
   ```

## API Error Handling

### HTTP Error Translation

The system maps domain errors to HTTP status codes and response formats:

```go
// WithError translates domain errors to HTTP responses
func WithError(c *fiber.Ctx, err error) error {
    switch e := err.(type) {
    case pkg.EntityNotFoundError:
        return NotFound(c, e.Code, e.Title, e.Message)
    case pkg.EntityConflictError:
        return Conflict(c, e.Code, e.Title, e.Message)
    case pkg.ValidationError:
        return BadRequest(c, pkg.ValidationKnownFieldsError{
            Code:    e.Code,
            Title:   e.Title,
            Message: e.Message,
            Fields:  nil,
        })
    // ... other error types
    default:
        var iErr pkg.InternalServerError
        _ = errors.As(pkg.ValidateInternalError(err, ""), &iErr)
        return InternalServerError(c, iErr.Code, iErr.Title, iErr.Message)
    }
}
```

### HTTP Response Formats

HTTP errors are returned in a consistent JSON format:

```json
{
  "code": "ENTITY_NOT_FOUND",
  "title": "Entity Not Found",
  "message": "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage."
}
```

For validation errors with field-specific issues:

```json
{
  "code": "BAD_REQUEST",
  "title": "Bad Request",
  "message": "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
  "fields": {
    "name": "Name is required",
    "amount": "Amount must be a positive number"
  }
}
```

## Error Categories

### Resource Errors

- **EntityNotFoundError**: When a requested resource doesn't exist
- **EntityConflictError**: When a resource already exists (e.g., duplicate name)

### Validation Errors

- **ValidationError**: General validation errors
- **ValidationKnownFieldsError**: Validation errors for specific fields
- **ValidationUnknownFieldsError**: When unexpected fields are provided

### Authorization Errors

- **UnauthorizedError**: When authentication is required
  - This error is returned when the `Authorization` header is missing or invalid
  - Only occurs when the Authentication Plugin is enabled (`PLUGIN_AUTH_ENABLED=true`)
- **ForbiddenError**: When the user lacks necessary permissions
  - This error is returned when the user is authenticated but lacks permissions for the requested resource or action

> **Note:** Authentication in Midaz is handled by a separate plugin. When this plugin is disabled (`PLUGIN_AUTH_ENABLED=false`), authentication errors will not occur. See the [API Reference](../api-reference/README.md#authentication) for more details on authentication.

### Business Logic Errors

- **UnprocessableOperationError**: When an operation fails due to business rules
- **FailedPreconditionError**: When prerequisites for an operation aren't met

### System Errors

- **InternalServerError**: For unexpected server errors
- **HTTPError**: For HTTP-related errors

## Database Error Handling

Midaz maps database-specific errors to domain errors for consistent error handling:

```go
// Mapping of PostgreSQL errors to domain errors
if errors.Is(err, sql.ErrNoRows) {
    return pkg.ValidateBusinessError(constant.ErrEntityNotFound, entityType)
}

var pgErr *pgconn.PgError
if errors.As(err, &pgErr) {
    switch pgErr.Code {
    case "23505": // Unique violation
        return pkg.ValidateBusinessError(constant.ErrEntityConflictError, entityType)
    // ... other database error codes
    }
}
```

## Error Handling in Services

In the service layer, errors are mapped to domain-specific errors:

```go
func (uc *UseCase) CreateAccount(ctx context.Context, input *mmodel.CreateAccountInput) (*mmodel.Account, error) {
    // Validate the input
    if err := uc.validate.Struct(input); err != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrInvalidInput, "Account")
    }

    // Check if account exists
    existing, err := uc.repo.FindByAlias(ctx, input.Alias)
    if err == nil && existing != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrAliasUnavailability, "Account", input.Alias)
    }

    // Create the account
    account, err := uc.repo.Create(ctx, account)
    if err != nil {
        return nil, err // Database errors are mapped at the repository level
    }

    return account, nil
}
```

## Validation Error Handling

The system uses structured validation with the go-playground/validator package:

```go
// Example validation tags on struct fields
type CreateAccountInput struct {
    Name     string         `json:"name" validate:"required,max=256"`
    Alias    *string        `json:"alias" validate:"omitempty,max=100,prohibitedexternalaccountprefix"`
    Type     string         `json:"type" validate:"required"`
    Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}
```

Validation errors are converted to user-friendly messages:

```go
func translateValidationError(err error) map[string]string {
    result := make(map[string]string)
    
    if validationErrs, ok := err.(validator.ValidationErrors); ok {
        for _, e := range validationErrs {
            switch e.Tag() {
            case "required":
                result[e.Field()] = "This field is required"
            case "max":
                result[e.Field()] = fmt.Sprintf("Exceeds maximum length of %s", e.Param())
            // ... more validation error translations
            }
        }
    }
    
    return result
}
```

## Error Logging

Errors are logged with appropriate context:

```go
if err != nil {
    logger.WithFields(
        "entity_id", id,
        "error", err.Error(),
    ).Error("Failed to retrieve entity")
    
    return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")
}
```

## Observable Errors

Errors are integrated with the observability system:

```go
if err != nil {
    libOpentelemetry.HandleSpanError(&span, "Failed to create entity", err)
    logger.Errorf("Error creating entity: %v", err)
    return nil, err
}
```

## Common Error Patterns

### Entity Not Found Pattern

```go
entity, err := uc.repo.FindByID(ctx, id)
if err != nil {
    if errors.Is(err, sql.ErrNoRows) {
        return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, "Entity")
    }
    return nil, err
}
```

### Validation Error Pattern

```go
if err := uc.validate.Struct(input); err != nil {
    validationErrors := translateValidationError(err)
    return nil, pkg.ValidateBadRequestFieldsError(nil, validationErrors, "Entity", nil)
}
```

### Business Rule Violation Pattern

```go
if account.Balance < transferAmount {
    return nil, pkg.ValidateBusinessError(constant.ErrInsufficientFunds, "Account")
}
```

### Permission Error Pattern

```go
if !hasPermission(user, resource) {
    return nil, pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, "User")
}
```

## Best Practices

1. **Use Domain-Specific Error Types**:
   Always use the appropriate error type for the situation (e.g., `EntityNotFoundError` for missing resources).

2. **Include Useful Context**:
   Include the entity type, field names, and other relevant information in error messages.

3. **User-Friendly Messages**:
   Write error messages that explain what went wrong and how to fix it.

4. **Consistent Error Codes**:
   Use the predefined error constants from `pkg/constant/errors.go`.

5. **Preserve Original Errors**:
   Use error wrapping to preserve the original error for debugging.

6. **Type-Based Error Handling**:
   Use type switches and type assertions to handle different error categories.

7. **Map Low-Level Errors**:
   Map database and third-party errors to domain-specific errors.

8. **Validate Early**:
   Perform validation early to avoid unnecessary processing.

9. **Centralized Error Translation**:
   Use the error factory functions for consistent error creation.

10. **Log Appropriately**:
    Log errors with sufficient context but avoid logging sensitive information.

## Related Documentation

- [API Error Responses](../api-reference/README.md)
- [Validation Rules](../developer-guide/validation.md)
- [Transaction Error Handling](../architecture/data-flow/transaction-lifecycle.md)
