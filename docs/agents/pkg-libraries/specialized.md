# Specialized pkg/ Libraries

**Location**: Various `pkg/` subdirectories
**Priority**: 📦 Specialized - Domain-specific packages
**Status**: Production-ready specialized utilities

This document covers specialized packages for specific domains: transaction validation, gRPC, MongoDB, DSL parsing, and custom linters.

## pkg/transaction - Transaction Validation

**Location**: `pkg/transaction/`

Business rule validation for financial transactions.

**Key Functions:**
- Transaction structure validation
- Send/distribute validation
- Balance requirement checks
- Account eligibility validation

**Usage:** Used by transaction use cases

**References:**
- Source: `pkg/transaction/transaction.go:1`, `pkg/transaction/validations.go:1`
- Tests: `pkg/transaction/*_test.go`

## pkg/mgrpc - gRPC Utilities

**Location**: `pkg/mgrpc/`

gRPC-specific utilities and error mapping.

### Key Functions

```go
// Map authentication errors to gRPC status codes
func MapAuthGRPCError(ctx context.Context, err error) error
```

**Usage:**
```go
if err := authenticate(ctx, token); err != nil {
    return mgrpc.MapAuthGRPCError(ctx, err)
}
```

**References:**
- Source: `pkg/mgrpc/grpc.go:1`, `pkg/mgrpc/errors.go:1`
- Protobuf: `pkg/mgrpc/balance/`

## pkg/mongo - MongoDB Utilities

**Location**: `pkg/mongo/`

MongoDB connection and helper utilities for metadata storage.

**Purpose:** MongoDB is used for flexible metadata storage alongside PostgreSQL for structured data.

**References:**
- Source: `pkg/mongo/mongo.go:1`

## pkg/gold - Transaction DSL Parser

**Location**: `pkg/gold/`

ANTLR-based parser for the Gold transaction language (DSL for expressing complex financial transactions).

### Structure

- `Transaction.g4` - ANTLR grammar definition
- `parser/` - Generated ANTLR parser code
- `transaction/` - Transaction AST and interpreter

### Usage

The Gold DSL allows expressing complex n:n transactions in a declarative format.

**References:**
- Grammar: `pkg/gold/Transaction.g4`
- Documentation: See `docs/agents/transaction-dsl.md` (when created)

## pkg/mlint - Custom Linters

**Location**: `pkg/mlint/`

Custom golangci-lint plugins for Midaz-specific rules.

### panicguard

Enforces safe goroutine usage by blocking bare `go` keyword in production code.

**Rules:**
- ❌ Blocks: `go func() { ... }()`
- ✅ Requires: `mruntime.SafeGo*()` functions

**Configuration:** Integrated into `.golangci.yml`

**Example Violation:**
```go
// ❌ COMPILATION ERROR - caught by panicguard linter
go func() {
    doWork()
}()

// ✅ ALLOWED
mruntime.SafeGoWithContextAndComponent(ctx, logger, "component", "name",
    mruntime.KeepRunning, func(ctx context.Context) {
        doWork()
    })
```

### panicguardwarn

Warning-only version of panicguard for gradual adoption in existing codebases.

**References:**
- Source: `pkg/mlint/panicguard/`, `pkg/mlint/panicguard warn/`
- Related: [`mruntime.md`](./mruntime.md) for safe goroutine functions

## pkg/constant - Error Codes & Constants

**Location**: `pkg/constant/`

Standardized error codes and domain constants.

### Error Codes (100+ codes)

```go
// Location: pkg/constant/errors.go
var (
    ErrDuplicateLedger                   = errors.New("0001")
    ErrLedgerNameConflict                = errors.New("0002")
    ErrAssetNameOrCodeDuplicate          = errors.New("0003")
    // ... 100+ more error codes
)
```

See [API Error List](https://docs.midaz.io/midaz/api-reference/resources/errors-list) for complete documentation.

### Other Constants

- Account constants: `pkg/constant/account.go`
- Balance constants: `pkg/constant/balance.go`
- HTTP constants: `pkg/constant/http.go`
- Operation constants: `pkg/constant/operation.go`
- Operation route constants: `pkg/constant/operation-route.go`
- Pagination constants: `pkg/constant/pagination.go`
- Transaction constants: `pkg/constant/transaction.go`

**Usage:**
```go
// Use with ValidateBusinessError
return pkg.ValidateBusinessError(constant.ErrDuplicateLedger, "ledger", name, divisionID)

// Direct comparison
if err.Error() == constant.ErrEntityNotFound.Error() {
    // handle not found
}
```

**References:**
- Source: `pkg/constant/*.go`
- API Docs: https://docs.midaz.io/midaz/api-reference/resources/errors-list
- Related: [`errors.md`](./errors.md) for typed error conversion

## Summary

Specialized packages serve specific domains:

1. **pkg/transaction** - Transaction business rules
2. **pkg/mgrpc** - gRPC error mapping
3. **pkg/mongo** - MongoDB utilities
4. **pkg/gold** - Transaction DSL parser (ANTLR)
5. **pkg/mlint** - Custom linters (panicguard)
6. **pkg/constant** - Error codes and constants

Use these packages when working in their specific domains.
