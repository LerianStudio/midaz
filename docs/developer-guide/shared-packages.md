# Shared Packages

**Navigation:** [Home](../) > [Developer Guide](../) > Shared Packages

This document describes the shared package architecture in Midaz, explaining how common functionality is organized, reused across different components, and maintained for consistency.

## Table of Contents

- [Overview](#overview)
- [Package Organization](#package-organization)
- [Core Shared Packages](#core-shared-packages)
  - [Constants Package](#constants-package)
  - [Error Handling Package](#error-handling-package)
  - [Models Package](#models-package)
  - [Network Utilities Package](#network-utilities-package)
  - [Gold DSL Package](#gold-dsl-package)
- [Usage Guidelines](#usage-guidelines)
- [Dependency Management](#dependency-management)
- [Testing Shared Code](#testing-shared-code)

## Overview

Midaz employs a shared package architecture to minimize code duplication, ensure consistency across services, and centralize critical functionality. These packages establish common patterns, provide reusable utilities, and define standards that all components follow.

Key benefits of the shared package approach include:

- **Consistency**: Common abstractions provide consistent patterns across services
- **Reusability**: Core functionality like error handling, models, and HTTP utilities can be shared
- **Maintainability**: Updates to shared code propagate to all dependent services
- **Documentation**: Centralized code makes it easier to understand system-wide patterns
- **Testing**: Shared code receives more comprehensive testing by being used in multiple contexts

## Package Organization

Midaz shared packages are organized under the `/pkg` directory in the project root. The shared code is structured into focused, domain-specific packages:

```
/pkg
├── constant/          # System-wide constants and enumerations
├── errors.go          # Centralized error handling framework
├── gold/              # Transaction DSL parsing and processing
│   ├── Transaction.g4 # ANTLR grammar for transaction language
│   ├── parser/        # Generated parser code
│   └── transaction/   # Transaction processing utilities
├── mmodel/            # Shared domain models
│   ├── account.go     # Account domain model
│   ├── asset.go       # Asset domain model
│   └── ...            # Other domain models
└── net/               # Network-related utilities
    └── http/          # HTTP utilities, middleware, and response handling
```

## Core Shared Packages

### Constants Package

The `/pkg/constant` package contains system-wide constants, enumerations, and static values used across different services. These include:

- **Status codes**: Common status values for entities
- **Error codes**: Standardized error codes for consistent error reporting
- **Transaction states**: Valid states in transaction processing
- **HTTP constants**: Common HTTP headers and response codes
- **Pagination constants**: Parameters for pagination handling
- **Account types**: Valid account types and prefixes

Example of constants usage:

```go
// From pkg/constant/transaction.go
const (
    CREATED  = "CREATED"
    APPROVED = "APPROVED"
    PENDING  = "PENDING"
    SENT     = "SENT"
    CANCELED = "CANCELED"
    DECLINED = "DECLINED"
)
```

### Error Handling Package

The `/pkg/errors.go` file defines a comprehensive error handling framework that provides:

- **Structured errors**: Rich error types with contextual information
- **Error categorization**: Business, validation, and system error types
- **HTTP mapping**: Automatic mapping to appropriate HTTP status codes
- **Client-friendly messages**: User-friendly error messages for API responses
- **Localization support**: Infrastructure for localized error messages

The error system includes specialized error types:

- `EntityNotFoundError`: For when requested resources don't exist
- `ValidationError`: For invalid input data
- `EntityConflictError`: For uniqueness constraint violations
- `UnauthorizedError`: For authentication issues
- `ForbiddenError`: For permission issues
- `ValidationKnownFieldsError`: For field-specific validation errors

Example of error handling:

```go
// Creating a business error with proper context
func GetAccount(id string) (*Account, error) {
    account, err := repo.FindByID(id)
    if err != nil {
        return nil, pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, "account")
    }
    return account, nil
}
```

### Models Package

The `/pkg/mmodel` package contains domain models shared across services:

- **Entity definitions**: Core business entities like accounts, assets, etc.
- **Input/output models**: Standard request/response structures
- **Validation rules**: Built-in validation logic and annotations
- **Documentation**: Swagger annotations for API documentation

Example model structure:

```go
// From pkg/mmodel/account.go
type Account struct {
    // Unique identifier for the account (UUID format)
    ID string `json:"id" example:"00000000-0000-0000-0000-000000000000" format:"uuid"`
    
    // Name of the account (max length 256 characters)
    Name string `json:"name" example:"My Account" maxLength:"256"`
    
    // Asset code associated with this account
    AssetCode string `json:"assetCode" example:"BRL" maxLength:"100"`
    
    // Status of the account (active, inactive, pending)
    Status Status `json:"status"`
    
    // Additional custom attributes for the account
    Metadata map[string]any `json:"metadata,omitempty"`
    
    // ... other fields
}
```

### Network Utilities Package

The `/pkg/net/http` package provides HTTP-related utilities:

- **Request parsing**: Utilities for parsing and validating requests
- **Response formatting**: Standardized response structures
- **Middleware**: Common middleware for authentication, logging, etc.
- **Pagination**: Helper functions for handling pagination parameters
- **Error handling**: HTTP-specific error handling and status code mapping

Example of HTTP utilities:

```go
// ValidateParameters handles common query parameter validation
func ValidateParameters(params map[string]string) (*QueryHeader, error) {
    // Validates and normalizes pagination, filtering, sorting parameters
    // Returns structured query parameters for repository layers
}
```

### Gold DSL Package

The `/pkg/gold` package implements the domain-specific language (DSL) for transaction processing:

- **Grammar definition**: ANTLR4 grammar file (`Transaction.g4`)
- **Parser components**: Generated lexer and parser code
- **Visitor implementation**: Custom visitor for DSL processing
- **Transaction validation**: Rules for validating transaction scripts
- **Error handling**: DSL-specific error reporting

Example of Gold DSL:

```
(transaction V1
  (chart-of-accounts-group-name ledger-123)
  (description "Transfer between accounts")
  (send USD 1000|2
    (source
      (from account-123 :amount USD 1000|2))
    (distribute
      (to account-456 :amount USD 1000|2))))
```

## Usage Guidelines

When working with shared packages, follow these guidelines:

1. **Minimize dependencies**: Shared packages should have minimal external dependencies to avoid circular dependencies
2. **Backward compatibility**: Changes to shared packages should maintain backward compatibility
3. **Version management**: Use semantic versioning to signal breaking changes
4. **Clear documentation**: Document public functions, types, and interfaces thoroughly
5. **Comprehensive testing**: Ensure high test coverage for shared code
6. **Centralized definitions**: Don't duplicate definitions that should be shared

## Dependency Management

The Midaz codebase manages dependencies using Go modules. The `go.mod` file at the project root defines all dependencies, including any external libraries used by shared packages.

Shared packages generally avoid external dependencies where possible, but some are necessary:

- **UUID generation**: Uses `github.com/google/uuid`
- **HTTP handling**: Uses `github.com/gofiber/fiber/v2`
- **DSL parsing**: Uses `github.com/antlr4-go/antlr/v4`
- **MongoDB integration**: Uses `go.mongodb.org/mongo-driver/bson`

## Testing Shared Code

Shared packages have their own test files to ensure reliability. For example:

- `errors_test.go` validates the error handling framework
- `mmodel/account_test.go` tests account model functionality
- `mmodel/status_test.go` tests status enumeration handling

When adding or modifying shared code, always add corresponding tests to ensure correctness and avoid regressions.