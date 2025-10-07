# Package pkg

## Overview

The `pkg` package is the shared library layer of the Midaz ledger system. It contains reusable components, utilities, models, and helpers that are used across all Midaz services (onboarding, transaction, and CLI).

## Purpose

This package provides the foundational building blocks for the Midaz platform:

- **Constants**: System-wide constant values and error codes
- **Error handling**: Structured error types and validation functions
- **Domain models**: Data structures for all business entities
- **HTTP utilities**: Request/response handling and validation
- **DSL parsing**: Gold DSL parser for transaction definitions
- **Utilities**: Retry logic, backoff strategies, and helpers

## Package Structure

```
pkg/
├── constant/            # System-wide constants and error codes
│   ├── account.go
│   ├── balance.go
│   ├── errors.go       # 124 error code definitions
│   ├── http.go
│   ├── operation.go
│   ├── operation-route.go
│   ├── pagination.go
│   ├── transaction.go
│   └── README.md
├── errors.go            # Error type definitions and validation
├── errors_test.go
├── gold/                # Gold DSL parser and validator
│   ├── Transaction.g4  # ANTLR4 grammar definition
│   ├── parser/         # ANTLR-generated parser code
│   ├── transaction/    # Custom parsing logic
│   └── README.md
├── mmodel/              # Domain model definitions
│   ├── account.go
│   ├── account-type.go
│   ├── asset.go
│   ├── balance.go
│   ├── error.go
│   ├── ledger.go
│   ├── operation-route.go
│   ├── organization.go
│   ├── portfolio.go
│   ├── queue.go
│   ├── segment.go
│   ├── status.go
│   ├── transaction-route.go
│   └── README.md
├── net/                 # Network utilities
│   └── http/           # HTTP utilities and helpers
│       ├── errors.go
│       ├── httputils.go
│       ├── response.go
│       ├── withBody.go
│       └── README.md
├── utils/               # Utility functions
│   ├── jitter.go       # Retry and backoff utilities
│   └── README.md
└── README.md           # This file
```

## Package Hierarchy

The packages are organized in layers from innermost (most foundational) to outermost (most specific):

```
Layer 1 (Innermost - Pure Data/Logic)
├── constant/           # Constants and error codes
└── utils/              # Pure utility functions

Layer 2 (Data Structures)
├── errors.go           # Error type definitions
└── mmodel/             # Domain models

Layer 3 (Parsing and Transformation)
└── gold/               # DSL parsing

Layer 4 (Outermost - Framework Integration)
└── net/http/           # HTTP utilities (Fiber framework)
```

## Key Components

### 1. Constants (pkg/constant)

System-wide constants including:

- **124 error codes** (0001-0124) organized by category
- **Operation types** (DEBIT, CREDIT, ON_HOLD, RELEASE)
- **Transaction status** (CREATED, APPROVED, PENDING, CANCELED, NOTED)
- **HTTP constants** (header names, path parameters)
- **Account constants** (external prefix, alias patterns)
- **Pagination constants** (sort order types)

**Usage:**

```go
import "github.com/LerianStudio/midaz/v3/pkg/constant"

if balance < amount {
    return constant.ErrInsufficientFunds
}
```

**See:** [constant/README.md](constant/README.md)

### 2. Error Handling (pkg/errors.go)

Structured error types that map to HTTP status codes:

- `EntityNotFoundError` (404)
- `ValidationError` (400)
- `EntityConflictError` (409)
- `UnauthorizedError` (401)
- `ForbiddenError` (403)
- `UnprocessableOperationError` (422)
- `InternalServerError` (500)

**Functions:**

- `ValidateBusinessError()`: Maps error codes to structured errors
- `ValidateInternalError()`: Wraps unexpected errors
- `ValidateUnmarshallingError()`: Handles JSON parsing errors
- `ValidateBadRequestFieldsError()`: Creates field validation errors

**Usage:**

```go
import "github.com/LerianStudio/midaz/v3/pkg"

if account == nil {
    return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "Account")
}
```

### 3. Domain Models (pkg/mmodel)

Data structures for all business entities:

- **Organization**: Top-level entities
- **Ledger**: Financial record-keeping systems
- **Asset**: Currencies, cryptocurrencies, commodities
- **Account**: Financial buckets (bank accounts, cards, etc.)
- **Balance**: Asset holdings with available/on-hold amounts
- **Portfolio**: Account groupings
- **Segment**: Logical divisions
- **AccountType**: Account classifications
- **OperationRoute**: Transaction routing rules
- **TransactionRoute**: Transaction flow definitions

**Features:**

- JSON serialization with validation tags
- Swagger annotations for API documentation
- Pagination support
- Metadata extensibility
- Soft deletion support
- UUID conversion methods

**Usage:**

```go
import "github.com/LerianStudio/midaz/v3/pkg/mmodel"

input := mmodel.CreateAccountInput{
    Name:      "Corporate Checking",
    AssetCode: "USD",
    Type:      "deposit",
    Status:    mmodel.Status{Code: "ACTIVE"},
}
```

**See:** [mmodel/README.md](mmodel/README.md)

### 4. Gold DSL (pkg/gold)

Domain-Specific Language for defining transactions:

- **Lisp-like syntax** for transaction definitions
- **ANTLR4-based parser** for robust parsing
- **Support for n:n transactions** (multiple sources to multiple destinations)
- **Flexible distribution** (amounts, percentages, remaining)
- **Asset rate conversion** for cross-asset transactions
- **Variables and templates** for reusable patterns

**Functions:**

- `transaction.Parse()`: Parse DSL to transaction struct
- `transaction.Validate()`: Validate DSL syntax

**Usage:**

```go
import "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"

dsl := `(transaction V1
  (chart-of-accounts-group-name 123e4567-e89b-12d3-a456-426614174000)
  (send USD 1000 | 100
    (source (from @customer :amount USD 1000 | 100))
    (distribute (to @revenue :amount USD 1000 | 100))))`

if err := transaction.Validate(dsl); err != nil {
    return err
}
tx := transaction.Parse(dsl)
```

**See:** [gold/README.md](gold/README.md)

### 5. HTTP Utilities (pkg/net/http)

HTTP helpers for the Fiber web framework:

- **Error conversion**: Domain errors to HTTP responses
- **Response helpers**: Standardized response functions
- **Request validation**: Body decoding and validation
- **Query parsing**: Parameter parsing and validation
- **Middleware**: UUID validation, body decoding
- **Idempotency**: Support for idempotent requests

**Functions:**

- `WithError()`: Convert domain errors to HTTP responses
- `WithBody()`: Decode and validate request bodies
- `ParseUUIDPathParameters()`: Validate UUID path parameters
- `ValidateParameters()`: Parse and validate query parameters
- Response helpers: `OK()`, `Created()`, `NotFound()`, etc.

**Usage:**

```go
import httputil "github.com/LerianStudio/midaz/v3/pkg/net/http"

func handler(p any, c *fiber.Ctx) error {
    input := p.(*mmodel.CreateAccountInput)
    account, err := service.CreateAccount(input)
    if err != nil {
        return httputil.WithError(c, err)
    }
    return httputil.Created(c, account)
}

app.Post("/accounts",
    httputil.WithBody(&mmodel.CreateAccountInput{}, handler))
```

**See:** [net/http/README.md](net/http/README.md)

### 6. Utilities (pkg/utils)

General-purpose utility functions:

- **Exponential backoff**: Gradually increasing retry delays
- **Full jitter**: Randomized delays to prevent thundering herd
- **Retry configuration**: Standard retry behavior constants

**Constants:**

- `MaxRetries` (5): Maximum retry attempts
- `InitialBackoff` (500ms): Starting delay
- `MaxBackoff` (10s): Maximum delay cap
- `BackoffFactor` (2.0): Exponential growth multiplier

**Functions:**

- `FullJitter()`: Random delay with jitter
- `NextBackoff()`: Calculate next exponential backoff

**Usage:**

```go
import "github.com/LerianStudio/midaz/v3/pkg/utils"

backoff := utils.InitialBackoff
for attempt := 0; attempt < utils.MaxRetries; attempt++ {
    if err := operation(); err != nil {
        time.Sleep(utils.FullJitter(backoff))
        backoff = utils.NextBackoff(backoff)
        continue
    }
    break
}
```

**See:** [utils/README.md](utils/README.md)

## Design Principles

### 1. Separation of Concerns

Each package has a single, well-defined responsibility:

- `constant`: Define constants
- `errors.go`: Handle errors
- `mmodel`: Define data structures
- `gold`: Parse DSL
- `net/http`: Handle HTTP
- `utils`: Provide utilities

### 2. Dependency Direction

Dependencies flow from outer to inner layers:

- `net/http` depends on `mmodel`, `errors.go`, `constant`
- `mmodel` depends on `constant`
- `errors.go` depends on `constant`
- `constant` has no dependencies (except standard library)
- `utils` has no dependencies (except standard library)

### 3. Reusability

All packages are designed for reuse:

- No service-specific logic
- Generic, composable functions
- Clear interfaces and contracts

### 4. Type Safety

Strong typing throughout:

- UUID types for identifiers
- Decimal types for monetary values
- Custom types for enums (Status, Order)
- Struct tags for validation

### 5. Documentation

Comprehensive documentation:

- Package-level comments
- Function documentation with examples
- README files for each package
- Swagger annotations for API models

## Common Usage Patterns

### Error Handling Pattern

```go
import (
    "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
)

func GetAccount(id string) (*Account, error) {
    account, err := repository.FindByID(id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, pkg.ValidateBusinessError(
                constant.ErrAccountIDNotFound, "Account")
        }
        return nil, pkg.ValidateInternalError(err, "Account")
    }
    return account, nil
}
```

### HTTP Handler Pattern

```go
import (
    httputil "github.com/LerianStudio/midaz/v3/pkg/net/http"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func createAccountHandler(p any, c *fiber.Ctx) error {
    input := p.(*mmodel.CreateAccountInput)

    account, err := service.CreateAccount(input)
    if err != nil {
        return httputil.WithError(c, err)
    }

    return httputil.Created(c, account)
}

app.Post("/accounts",
    httputil.WithBody(&mmodel.CreateAccountInput{}, createAccountHandler))
```

### Retry Pattern

```go
import "github.com/LerianStudio/midaz/v3/pkg/utils"

func connectWithRetry() error {
    backoff := utils.InitialBackoff
    for attempt := 0; attempt < utils.MaxRetries; attempt++ {
        if err := connect(); err != nil {
            if attempt < utils.MaxRetries-1 {
                time.Sleep(utils.FullJitter(backoff))
                backoff = utils.NextBackoff(backoff)
                continue
            }
            return err
        }
        return nil
    }
    return errors.New("max retries exceeded")
}
```

### DSL Transaction Pattern

```go
import "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"

func processTransactionDSL(dslContent string) error {
    // Validate syntax
    if err := transaction.Validate(dslContent); err != nil {
        return constant.ErrInvalidScriptFormat
    }

    // Parse to struct
    result := transaction.Parse(dslContent)
    tx, ok := result.(libTransaction.Transaction)
    if !ok {
        // Handle parse failure - returned unexpected type
        return constant.ErrInvalidScriptFormat
    }

    // Process transaction
    return service.CreateTransaction(tx)
}
```

## Testing

Each package includes comprehensive tests:

```bash
# Test all packages
go test ./pkg/...

# Test specific package
go test ./pkg/constant/...
go test ./pkg/mmodel/...
go test ./pkg/gold/transaction/...
go test ./pkg/net/http/...

# With coverage
go test -cover ./pkg/...

# Verbose output
go test -v ./pkg/...
```

## Dependencies

### External Dependencies

- `github.com/google/uuid`: UUID generation and parsing
- `github.com/shopspring/decimal`: Precise decimal arithmetic
- `github.com/gofiber/fiber/v2`: Web framework (net/http only)
- `gopkg.in/go-playground/validator.v9`: Struct validation (net/http only)
- `github.com/antlr4-go/antlr/v4`: ANTLR4 runtime (gold only)
- `github.com/vmihailenco/msgpack/v5`: Binary serialization (mmodel only)
- `go.mongodb.org/mongo-driver/bson`: MongoDB queries (net/http only)
- `github.com/LerianStudio/lib-commons/v2`: Shared Lerian utilities

### Internal Dependencies

```
constant ←─── errors.go
    ↑           ↑
    └───────────┴──── mmodel
                       ↑
                       └──── net/http

gold (independent)
utils (independent)
```

## Usage by Services

### Onboarding Service

Uses:

- `pkg/constant`: Error codes, constants
- `pkg/errors.go`: Error handling
- `pkg/mmodel`: Organization, Ledger, Asset, Account, Portfolio, Segment models
- `pkg/net/http`: HTTP utilities
- `pkg/utils`: Retry logic for database/message broker

### Transaction Service

Uses:

- `pkg/constant`: Error codes, operation types, transaction status
- `pkg/errors.go`: Error handling
- `pkg/mmodel`: Account, Balance, Transaction models
- `pkg/gold`: DSL parsing for transaction definitions
- `pkg/net/http`: HTTP utilities
- `pkg/utils`: Retry logic

### MDZ CLI

Uses:

- `pkg/constant`: Error codes
- `pkg/mmodel`: All models for display
- `pkg/gold`: DSL parsing for transaction files

## Best Practices

### Import Aliases

Use aliases to avoid naming conflicts:

```go
import (
    "github.com/LerianStudio/midaz/v3/pkg"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
    "github.com/LerianStudio/midaz/v3/pkg/mmodel"
    httputil "github.com/LerianStudio/midaz/v3/pkg/net/http"
    "github.com/LerianStudio/midaz/v3/pkg/utils"
    "github.com/LerianStudio/midaz/v3/pkg/gold/transaction"
)
```

### Error Handling

Always use structured errors:

```go
// Good - structured error with code
return pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "Account")

// Bad - generic error
return errors.New("account not found")
```

### Decimal Arithmetic

Use decimal.Decimal for all monetary values:

```go
// Good - precise decimal
amount := decimal.NewFromInt(1000)

// Bad - floating point errors
amount := 10.00
```

### UUID Handling

Convert string IDs to UUIDs when needed:

```go
account := &mmodel.Account{ID: "123e4567-e89b-12d3-a456-426614174000"}
accountUUID := account.IDtoUUID()
```

### Validation

Validate inputs using struct tags and validation functions:

```go
input := &mmodel.CreateAccountInput{...}
if err := httputil.ValidateStruct(input); err != nil {
    return err
}
```

## Documentation Standard

All packages follow a consistent documentation standard:

### Package Comments

Every package has a comprehensive package comment explaining:

- Purpose and responsibilities
- Key concepts and terminology
- Usage examples
- Design principles

### Type Comments

Every exported type includes:

- Description of purpose
- Usage examples
- Field descriptions
- HTTP status code mappings (for errors)

### Function Comments

Every exported function includes:

- Brief description
- Detailed explanation
- Parameters with types and descriptions
- Returns with types and descriptions
- Example usage
- Error conditions

### README Files

Every package has a README.md with:

- Overview and purpose
- Package structure
- Key components
- Usage examples
- Design principles
- Best practices
- Dependencies

## Error Code Reference

The package defines 124 standardized error codes (0001-0124):

| Range     | Category            | Examples                                    |
| --------- | ------------------- | ------------------------------------------- |
| 0001-0002 | Ledger              | Duplicate ledger, name conflict             |
| 0003-0005 | Asset               | Duplicate asset, code format                |
| 0006-0009 | General             | Unmodifiable field, entity not found        |
| 0010-0012 | Account Type        | Immutable type, inactive type               |
| 0014-0016 | Segment             | Inactive segment, duplicate name            |
| 0017-0049 | Transaction DSL     | Invalid script, empty file                  |
| 0050-0051 | Metadata            | Key/value length exceeded                   |
| 0052-0064 | Account             | Account not found, invalid type             |
| 0065-0094 | HTTP/Validation     | Invalid parameter, bad request              |
| 0095      | Message Broker      | Broker unavailable                          |
| 0096-0099 | Account/Transaction | Invalid alias, not pending                  |
| 0100-0124 | Routes/Types        | Operation route errors, account type errors |

For complete error descriptions, see: [constant/README.md](constant/README.md)

## Performance Considerations

### Decimal Operations

Decimal arithmetic is slower than native float64:

- Use for all monetary calculations (accuracy > speed)
- Cache decimal values when possible
- Avoid repeated string parsing

### Reflection Usage

Some packages use reflection (net/http):

- Minimal performance impact for API endpoints
- Consider caching for high-throughput scenarios
- Use constructors to avoid reflection when possible

### ANTLR Parsing

DSL parsing has overhead:

- Cache parsed transactions when possible
- Use JSON API for high-frequency operations
- Consider transaction templates for common patterns

### Validation

Struct validation happens on every request:

- Validation is fast for most cases
- Custom validators are optimized
- Unknown field detection requires double marshaling

## Security Features

### Error Code Abstraction

Error codes prevent information leakage:

- Internal errors mapped to generic messages
- Detailed errors logged server-side
- Client receives user-friendly messages

### Input Validation

Comprehensive validation prevents attacks:

- Null byte detection
- Metadata size limits
- Unknown field rejection
- UUID format validation
- Regex pattern validation

### Type Safety

Strong typing prevents common errors:

- UUID types prevent string injection
- Decimal types prevent precision errors
- Enum types prevent invalid values

## Extending the Package

### Adding New Error Codes

1. Add error to `constant/errors.go`:

```go
ErrNewError = errors.New("0125")
```

2. Map error in `errors.go`:

```go
constant.ErrNewError: ValidationError{
    Code:    constant.ErrNewError.Error(),
    Title:   "New Error",
    Message: "Description of the error.",
}
```

3. Update documentation in `constant/README.md`

### Adding New Models

1. Create model in `mmodel/`:

```go
type NewEntity struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"createdAt"`
}
```

2. Add input/update models
3. Add response wrappers
4. Add to `mmodel/README.md`

### Adding New HTTP Utilities

1. Add function to appropriate file in `net/http/`
2. Follow existing patterns
3. Add comprehensive documentation
4. Add tests
5. Update `net/http/README.md`

### Adding New Constants

1. Add to appropriate file in `constant/`
2. Add comprehensive comment
3. Update `constant/README.md`

## Migration Guide

### From v2 to v3

Key changes in v3:

- Module path: `github.com/LerianStudio/midaz/v3`
- Enhanced error handling with more error types
- Additional validation rules
- Improved documentation

Update imports:

```go
// v2
import "github.com/LerianStudio/midaz/v2/pkg/constant"

// v3
import "github.com/LerianStudio/midaz/v3/pkg/constant"
```

## Contributing

When contributing to this package:

1. **Follow the standard**: Match existing documentation style
2. **Add tests**: All new code must have tests
3. **Update README**: Update package README when adding features
4. **Use examples**: Include usage examples in comments
5. **Validate**: Ensure code passes linters and tests

## Related Documentation

- [Main README](../README.md): Project overview
- [Contributing Guide](../CONTRIBUTING.md): Contribution guidelines
- [API Documentation](https://docs.lerian.studio): Complete API reference

## Version History

This package follows semantic versioning as part of the Midaz v3 module.

**Current Version:** v3.x.x
**Go Version:** 1.24.0+
**License:** Apache 2.0
