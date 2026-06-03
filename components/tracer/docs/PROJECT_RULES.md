# Project Rules - Tracer MVP v1.2 (Last Updated: April 2026)

This document defines the architecture patterns, code conventions, testing requirements, and DevOps standards for the Tracer transaction validation service.

## Table of Contents

- [Coding Standards (NEW!)](#coding-standards)
- [Architecture](#architecture)
- [Code Conventions](#code-conventions)
- [Error Handling](#error-handling)
- [Distributed Tracing](#distributed-tracing)
- [Authentication](#authentication)
- [Performance Requirements](#performance-requirements)
- [Expression Language (CEL)](#expression-language-cel)
- [Resilience Patterns](#resilience-patterns)
- [Testing](#testing)
- [DevOps](#devops)
- [Structured Logging](#structured-logging)
- [Observability Patterns](#observability-patterns)
- [Forbidden Practices](#forbidden-practices)
- [AI Assistant Rules](#ai-assistant-rules)

---

## Coding Standards

**🌐 Language Policy:** All code, comments, and docs MUST be in English.

This document consolidates best practices identified through code reviews to ensure consistency. Main topics:

1. **Domain Model Invariants** - Always-Valid Objects, Validate Before Mutate
2. **Error Handling** - %w vs %v, Context Propagation, Typed Errors
3. **Testing Standards** - Deterministic Tests, Build Tags, Parallelization
4. **Encapsulation & DDD** - Domain Logic Location, Tell Don't Ask
5. **Normalization & Validation** - Normalize-Validate-Store Pattern
6. **Code Organization** - Error Constants Order, Test Helpers Centralization
7. **Review Checklist** - For authors and reviewers
8. **Language Policy** - English only for all artifacts

**Golden Rules:**
- ✅ **Validate Before Mutate** - Atomicity in error-returning methods
- ✅ **Domain Logic in Domain** - Not in service layer
- ✅ **Deterministic Tests** - testutil.FixedTime(), never time.Now()
- ✅ **Error Chain Preservation** - Always `%w`, never `%v`
- ✅ **Normalize-Validate-Store** - In that order, store normalized value
- ✅ **English Only** - All code, comments, docs, commits in English

---

## Architecture

### Hexagonal Architecture (Ports & Adapters) with CQRS

This project follows Hexagonal Architecture (Ports & Adapters) principles with Command Query Responsibility Segregation (CQRS), organized into three bounded contexts: Validation, Rules, and Limits.

**Design Principles:**
- **Single Responsibility:** Each bounded context owns its data and operations
- **Single-Tenant MVP:** One client per instance; simplified authentication
- **Fail-Open with Alerting:** System defaults to ALLOW under failure conditions
- **Payload-Complete Pattern:** All context required for validation included in request
- **Performance by Design:** Sub-100ms latency is architectural constraint

#### Directory Structure

The directory structure follows the **Lerian/Ring pattern** - a simplified hexagonal architecture.

```text
├── cmd/app/                    # Application entry point
├── internal/
│   ├── adapters/               # Infrastructure implementations
│   │   ├── http/in/            # Inbound HTTP handlers + routes + middleware
│   │   ├── postgres/           # PostgreSQL repositories
│   │   └── cel/                # CEL expression engine adapter
│   ├── bootstrap/              # Application bootstrap and DI
│   │   ├── config.go           # Config struct + InitServers() + all DI wiring
│   │   ├── http_server.go      # HTTP server with graceful shutdown
│   │   └── service.go          # Service struct + Run()/Shutdown()
│   └── services/               # Business logic (CQRS)
│       ├── command/            # Write operations (use cases)
│       ├── query/              # Read operations (use cases)
│       ├── cache/              # In-memory rule cache (RuleCache, CacheAdapter, WarmUp)
│       └── workers/            # Background workers (RuleSyncWorker, UsageCleanupWorker)
├── pkg/                        # Public packages
│   ├── constant/               # Error constants
│   └── model/                  # Domain entities and DTOs
├── migrations/                 # Database migrations
└── api/                        # API documentation (Swagger)
```

**Key principles (Lerian/Ring pattern):**
- **No `/internal/domain` folder** - Business entities live in `/pkg/model`
- **Services are the core** - `/internal/services` contains all business logic
- **Adapters are flat** - Organized by technology, not by domain
- **Interfaces where used** - Define interfaces in the package that USES them, not in separate `/port` folders

#### Layer Responsibilities

| Layer | Responsibility | Dependencies |
|-------|----------------|--------------|
| **Handlers** (`internal/adapters/http/in`) | HTTP request handling, validation, response formatting | Services |
| **Services** (`internal/services/command`, `internal/services/query`) | Business logic, orchestration | Repositories |
| **Repositories** (`internal/adapters/postgres`) | Data persistence, external system integration | Database drivers |
| **Models** (`pkg/model`) | Domain entities, DTOs | None |

#### Dependency Direction

```text
Handlers → Services → Repositories → Database
    ↓           ↓           ↓
  Models      Models      Models
```

Dependencies must flow inward. Inner layers must not depend on outer layers.

#### Bounded Contexts

| Context | Responsibility | Owns | Provides |
|---------|----------------|------|----------|
| **Validation** | Orchestrate validation requests, coordinate rule/limit evaluation, record audit trail | TransactionValidation, Validation history | Decisions (ALLOW/DENY/REVIEW), Audit records |
| **Rules** | Manage rule lifecycle, evaluate expressions | Rule definitions, Expression compilation, Rule sync | Rule evaluation results, Matched rule details |
| **Limits** | Manage spending limits, track usage, enforce thresholds | Limit configurations, Usage counters, Period reset | Limit check results (OK/EXCEEDED), Usage statistics |

### CQRS Pattern

- **Commands** (`internal/services/command/`): Handle write operations (Create, Update, Delete)
- **Queries** (`internal/services/query/`): Handle read operations (Get, List, Find)

Each command/query file should contain a single operation:
- `create_example.go` - CreateExample
- `update_example.go` - UpdateExampleByID
- `delete_example.go` - DeleteExampleByID
- `get_example_by_id.go` - GetExampleByID
- `get_all_examples.go` - GetAllExample

### Repository Pattern

Repositories must be defined as interfaces for testability.
Mocks are generated via gomock (using `go generate` with `mockgen`):

```go
// Interface definition - add //go:generate mockgen directive for mock generation
type Repository interface {
    Create(ctx context.Context, input *model.Example) (*model.ExampleOutput, error)
    Find(ctx context.Context, id uuid.UUID) (*model.ExampleOutput, error)
    FindAll(ctx context.Context, filter http.Pagination) ([]*model.ExampleOutput, error)
    Update(ctx context.Context, id uuid.UUID, example *model.Example) (*model.ExampleOutput, error)
    Delete(ctx context.Context, id uuid.UUID) error
}
```

### Cursor-Based Pagination Validation

When building cursors for pagination, validate `sortBy` against an allowlist and normalize `sortOrder` BEFORE encoding the cursor:

```go
// WRONG - cursor encoded without validation
func (r *Repository) buildNextCursor(item *Item, sortBy, sortOrder string) (string, error) {
    cursor := Cursor{
        ID:        item.ID,
        SortBy:    sortBy,      // Not validated - could be SQL injection vector
        SortOrder: sortOrder,   // Not normalized - "asc" vs "ASC" inconsistency
    }
    return encodeBase64(cursor), nil
}

// CORRECT - validate and normalize before encoding
func (r *Repository) buildNextCursor(item *Item, sortBy, sortOrder string) (string, error) {
    // Validate sortBy against allowlist (define allowlist per repository)
    validFields := map[string]bool{
        "createdAt": true,
        "updatedAt": true,
        "name":      true,
    }
    if !validFields[sortBy] {
        return "", ErrInvalidSortColumn
    }

    // Normalize sortOrder to uppercase
    normalizedOrder := strings.ToUpper(sortOrder)
    if normalizedOrder != "ASC" && normalizedOrder != "DESC" {
        normalizedOrder = "DESC" // Safe default
    }

    cursor := Cursor{
        ID:        item.ID,
        SortBy:    sortBy,
        SortOrder: normalizedOrder,
    }
    return encodeBase64(cursor), nil
}
```

**Why:** Invalid sort fields in cursors cause confusing errors on subsequent page requests. Normalization ensures consistency regardless of client input casing.

### Domain Model

**Validation Context:**

```go
// ValidationRequest - Input for transaction validation
// Amount uses decimal.Decimal (shopspring/decimal) for precise monetary arithmetic.
// Example: $10.50 is sent as 10.50 (decimal, not cents).
type ValidationRequest struct {
    RequestID       uuid.UUID              `json:"requestId"`
    TransactionType TransactionType        `json:"transactionType"` // CARD, WIRE, PIX, CRYPTO
    SubType         *string                `json:"subType"`         // "debit", "credit", "instant", etc.
    Amount          decimal.Decimal        `json:"amount"`          // Precise decimal (shopspring/decimal)
    Currency        string                 `json:"currency"`        // ISO 4217
    Timestamp       time.Time              `json:"timestamp"`
    Account         AccountContext         `json:"account"`
    Merchant        *MerchantContext       `json:"merchant"`        // Optional
    Scopes          []Scope                `json:"scopes"`
    Metadata        map[string]interface{} `json:"metadata"`
}

// ValidationResponse - Output from transaction validation
type ValidationResponse struct {
    RequestID         uuid.UUID      `json:"requestId"`
    Decision          Decision       `json:"decision"`          // ALLOW, DENY, REVIEW
    Reason            string         `json:"reason"`
    MatchedRuleIDs    []uuid.UUID    `json:"matchedRuleIds"`
    EvaluatedRuleIDs  []uuid.UUID    `json:"evaluatedRuleIds"`
    LimitUsageDetails []LimitUsage   `json:"limitUsageDetails"`
    ProcessingTimeMs  int            `json:"processingTimeMs"`
}

// TransactionType - Product types (payment methods)
type TransactionType string

const (
    TransactionTypeCard   TransactionType = "CARD"
    TransactionTypeWire   TransactionType = "WIRE"
    TransactionTypePix    TransactionType = "PIX"
    TransactionTypeCrypto TransactionType = "CRYPTO"
)

// Decision - Validation decision
type Decision string

const (
    DecisionAllow  Decision = "ALLOW"
    DecisionDeny   Decision = "DENY"
    DecisionReview Decision = "REVIEW"
)
```

**Rules Context:**

```go
// Rule - Fraud prevention rule with expression evaluation
type Rule struct {
    ID          uuid.UUID   `json:"id"`
    Name        string      `json:"name"`
    Description *string     `json:"description"`
    Expression  string      `json:"expression"`  // CEL expression
    Action      Decision    `json:"action"`      // ALLOW, DENY, REVIEW
    Scopes      []Scope     `json:"scopes"`
    Status      RuleStatus  `json:"status"`
    CreatedAt   time.Time   `json:"createdAt"`
    UpdatedAt   time.Time   `json:"updatedAt"`
    DeletedAt   *time.Time  `json:"deletedAt"`   // Soft delete
}

// RuleStatus - Rule lifecycle states
type RuleStatus string

const (
    RuleStatusDraft    RuleStatus = "DRAFT"    // Not evaluated; can modify
    RuleStatusActive   RuleStatus = "ACTIVE"   // Evaluated in validations
    RuleStatusInactive RuleStatus = "INACTIVE" // Not evaluated; preserved
    RuleStatusDeleted  RuleStatus = "DELETED"  // Permanently removed
)

// Scope - Application context for rules/limits
// ID fields are uuid.UUID type (not free-form strings)
type Scope struct {
    SegmentID       *uuid.UUID       `json:"segmentId,omitempty"`
    PortfolioID     *uuid.UUID       `json:"portfolioId,omitempty"`
    AccountID       *uuid.UUID       `json:"accountId,omitempty"`
    MerchantID      *uuid.UUID       `json:"merchantId,omitempty"`
    TransactionType *TransactionType `json:"transactionType,omitempty"`
    SubType         *string          `json:"subType,omitempty"`
}
```

**Limits Context:**

```go
// Limit - Spending limit configuration
// MaxAmount uses decimal.Decimal for precise monetary values.
type Limit struct {
    ID        uuid.UUID       `json:"id"`
    Name      string          `json:"name"`
    LimitType LimitType       `json:"limitType"`
    MaxAmount decimal.Decimal `json:"maxAmount"`   // Precise decimal (shopspring/decimal)
    Currency  string          `json:"currency"`    // ISO 4217
    Scopes    []Scope         `json:"scopes"`
    Status    LimitStatus     `json:"status"`
    ResetAt   *time.Time      `json:"resetAt"`
    CreatedAt time.Time       `json:"createdAt"`
    UpdatedAt time.Time       `json:"updatedAt"`
    DeletedAt *time.Time      `json:"deletedAt"`   // Soft delete
}

// LimitType - Period types for limits
type LimitType string

const (
    LimitTypeDaily          LimitType = "DAILY"
    LimitTypeMonthly        LimitType = "MONTHLY"
    LimitTypePerTransaction LimitType = "PER_TRANSACTION"
)

// LimitUsage - Current usage for response
// All monetary amounts use decimal.Decimal for precision.
type LimitUsage struct {
    LimitID      uuid.UUID       `json:"limitId"`
    LimitAmount  decimal.Decimal `json:"limitAmount"`  // Precise decimal
    CurrentUsage decimal.Decimal `json:"currentUsage"` // Precise decimal
    Exceeded     bool            `json:"exceeded"`
}
```

### Always Valid Domain Model

Domain entities must maintain their invariants at all times. An object should never exist in an invalid state - validation and invariant enforcement happen at construction and mutation, not as a separate step.

**Core Principles:**

1. **Validate in constructors** - Objects are born valid
2. **Validate before mutation** - State changes preserve validity
3. **Private setters** - No external code can break invariants
4. **Defensive copies** - Slices and maps cannot be mutated externally
5. **No invalid state** - Impossible to represent invalid combinations

#### Constructor Validation

All domain entity constructors must validate inputs and return errors for invalid data:

```go
// WRONG - entity can be created in invalid state
func NewRule(name, expression string, action Decision) *Rule {
    return &Rule{
        ID:         uuid.New(),
        Name:       name,  // Not validated - could be empty or too long
        Expression: expression,  // Not validated - could be malformed
        Action:     action,
        Status:     RuleStatusDraft,
        CreatedAt:  time.Now(),
    }
}

// Usage creates invalid entity with no error feedback
rule := NewRule("", "invalid CEL", "INVALID_ACTION")  // Silent failure!

// CORRECT - validation at construction prevents invalid state
func NewRule(name, expression string, action Decision) (*Rule, error) {
    // Normalize input
    name = strings.TrimSpace(name)
    expression = strings.TrimSpace(expression)

    // Validate all invariants
    if name == "" {
        return nil, ErrRuleNameRequired
    }
    if len(name) > MaxRuleNameLength {
        return nil, ErrRuleNameTooLong
    }
    if expression == "" {
        return nil, ErrExpressionRequired
    }
    if !isValidDecision(action) {
        return nil, ErrInvalidAction
    }

    // Object is guaranteed valid
    return &Rule{
        ID:         uuid.New(),
        Name:       name,
        Expression: expression,
        Action:     action,
        Status:     RuleStatusDraft,
        CreatedAt:  time.Now(),
        UpdatedAt:  time.Now(),
    }, nil
}

// Usage forces error handling
rule, err := NewRule(input.Name, input.Expression, input.Action)
if err != nil {
    return nil, err  // Cannot proceed with invalid entity
}
```

#### Mutation Validation

Methods that change state must validate before applying changes. Failed validations must leave the object unchanged (no partial mutations):

```go
// WRONG - partial mutation on validation failure
func (l *Limit) Update(name string, maxAmount int64) error {
    l.Name = strings.TrimSpace(name)  // Mutated!
    
    if l.Name == "" {
        return ErrNameRequired  // BUG: Name was mutated even though validation failed
    }
    
    l.MaxAmount = maxAmount  // Mutated!
    
    if maxAmount <= 0 {
        return ErrInvalidAmount  // BUG: Both fields mutated before validation completed
    }
    
    l.UpdatedAt = time.Now()
    return nil
}

// CORRECT - validate first, then mutate atomically
func (l *Limit) Update(name string, maxAmount int64) error {
    // Normalize inputs (does not mutate state)
    normalizedName := strings.TrimSpace(name)
    
    // Validate ALL invariants before ANY mutation
    if normalizedName == "" {
        return ErrNameRequired
    }
    if len(normalizedName) > MaxLimitNameLength {
        return ErrNameTooLong
    }
    if maxAmount <= 0 {
        return ErrInvalidAmount
    }
    if maxAmount > MaxAllowedAmount {
        return ErrAmountExceedsMax
    }
    
    // All validations passed - now mutate atomically
    l.Name = normalizedName
    l.MaxAmount = maxAmount
    l.UpdatedAt = time.Now()
    
    return nil
}
```

#### Private Fields with Validated Setters

Expose fields through methods that enforce invariants, not public fields:

```go
// WRONG - public fields allow invalid mutations
type Rule struct {
    Status     RuleStatus  // Anyone can set invalid status!
    DeletedAt  *time.Time  // Can be inconsistent with Status!
}

// External code can break invariants
rule.Status = "INVALID_STATUS"  // Compiles but violates domain rules
rule.DeletedAt = &now  // Inconsistent: DeletedAt set but Status != DELETED

// CORRECT - private fields with validated setters
type Rule struct {
    status    RuleStatus   // Private - cannot be set externally
    deletedAt *time.Time   // Private - managed by SetStatus
}

// Getter (read-only access)
func (r *Rule) Status() RuleStatus {
    return r.status
}

// Setter enforces invariants
func (r *Rule) SetStatus(status RuleStatus) error {
    // Validate state transition
    if !r.isValidTransition(r.status, status) {
        return ErrInvalidStatusTransition
    }
    
    // Update status
    r.status = status
    r.updatedAt = time.Now()
    
    // Maintain invariant: DeletedAt set iff status is DELETED
    if status == RuleStatusDeleted {
        now := time.Now()
        r.deletedAt = &now
    } else {
        r.deletedAt = nil
    }
    
    return nil
}

// External code cannot break invariants
err := rule.SetStatus(RuleStatusActive)  // Validated transition
if err != nil {
    // Handle invalid transition
}
```

#### Defensive Copies for Collections

Always create defensive copies of slices and maps to prevent external mutation:

```go
// WRONG - stores reference to external slice
func NewLimit(name string, scopes []Scope) *Limit {
    return &Limit{
        Name:   name,
        Scopes: scopes,  // DANGER: External code can modify this slice!
    }
}

// External code breaks encapsulation
scopes := []Scope{{AccountID: &accountID}}
limit := NewLimit("Daily Limit", scopes)
scopes[0].AccountID = nil  // BUG: Mutates limit.Scopes!

// CORRECT - defensive copy prevents external mutation
func NewLimit(name string, scopes []Scope) (*Limit, error) {
    // Validate inputs
    name = strings.TrimSpace(name)
    if name == "" {
        return nil, ErrNameRequired
    }
    
    // Defensive copy of slice
    scopesCopy := make([]Scope, len(scopes))
    copy(scopesCopy, scopes)
    
    return &Limit{
        ID:        uuid.New(),
        Name:      name,
        Scopes:    scopesCopy,  // Safe: external changes don't affect this
        CreatedAt: time.Now(),
    }, nil
}

// Also apply in getters that return slices
func (l *Limit) Scopes() []Scope {
    // Return defensive copy, not internal slice
    scopesCopy := make([]Scope, len(l.scopes))
    copy(scopesCopy, l.scopes)
    return scopesCopy
}
```

#### Validation Method

Every domain entity should have a `Validate()` method that checks all invariants. This serves as documentation and enables defensive validation at persistence boundaries:

```go
// Validation errors for Limit entity
var (
    ErrLimitNameRequired     = errors.New("limit name is required")
    ErrLimitNameTooLong      = errors.New("limit name exceeds maximum length")
    ErrInvalidMaxAmount      = errors.New("max amount must be positive")
    ErrMaxAmountExceedsLimit = errors.New("max amount exceeds maximum allowed value")
    ErrInvalidCurrency       = errors.New("currency must be valid ISO 4217 code")
    ErrDeletedAtInconsistent = errors.New("deletedAt must be set iff status is DELETED")
)

func (l *Limit) Validate() error {
    // Name validation
    if strings.TrimSpace(l.Name) == "" {
        return ErrLimitNameRequired
    }
    if len(l.Name) > MaxLimitNameLength {
        return ErrLimitNameTooLong
    }
    
    // Amount validation
    if l.MaxAmount <= 0 {
        return ErrInvalidMaxAmount
    }
    if l.MaxAmount > MaxAllowedAmount {
        return ErrMaxAmountExceedsLimit
    }
    
    // Currency validation
    if !isValidCurrency(l.Currency) {
        return ErrInvalidCurrency
    }
    
    // Invariant: DeletedAt set iff status is DELETED
    if l.Status == LimitStatusDeleted && l.DeletedAt == nil {
        return ErrDeletedAtInconsistent
    }
    if l.Status != LimitStatusDeleted && l.DeletedAt != nil {
        return ErrDeletedAtInconsistent
    }
    
    return nil
}

// Use at persistence boundaries
func (r *Repository) Save(ctx context.Context, limit *Limit) error {
    // Defensive validation before persistence
    if err := limit.Validate(); err != nil {
        return fmt.Errorf("invalid limit: %w", err)
    }
    
    // Proceed with persistence
    return r.db.Insert(ctx, limit)
}
```

#### Benefits

**Type Safety:** Invalid states are unrepresentable - the type system prevents bugs at compile time.

**No Validation Drift:** Validation logic lives with the entity, not scattered across handlers/services.

**Testability:** Constructor and mutation validation can be unit tested independently.

**Clarity:** Reading a constructor shows all business rules and constraints.

**Fail Fast:** Invalid data is caught at creation/mutation, not deep in the call stack.

#### Anti-Patterns to Avoid

```go
// ❌ Don't: Separate validation from construction
limit := &Limit{Name: input.Name, MaxAmount: input.Amount}
if err := limit.Validate(); err != nil {
    return err  // Too late - already constructed invalid object
}

// ✅ Do: Validate during construction
limit, err := NewLimit(input.Name, input.Amount)
if err != nil {
    return err  // Cannot create invalid object
}

// ❌ Don't: Allow mutation via exported fields
limit.MaxAmount = -1000  // Compiles but violates domain rules

// ✅ Do: Mutation through validated methods
err := limit.SetMaxAmount(newAmount)
if err != nil {
    return err  // Validation failed
}

// ❌ Don't: Return internal slices directly
func (l *Limit) Scopes() []Scope {
    return l.scopes  // Caller can mutate internal state!
}

// ✅ Do: Return defensive copies
func (l *Limit) Scopes() []Scope {
    result := make([]Scope, len(l.scopes))
    copy(result, l.scopes)
    return result
}
```

**Apply to:** All domain entities in `pkg/model` - Rule, Limit, Scope, and any future domain types.

### State Invariant Enforcement (Soft-Delete)

Models using soft-delete patterns must enforce invariants that ensure data consistency. The `DeletedAt` field must be set if and only if the status is `DELETED`:

```go
// WRONG - DeletedAt only set on delete, not cleared on restore
func (l *Limit) SetStatus(status LimitStatus) error {
    l.Status = status
    if status == LimitStatusDeleted {
        l.DeletedAt = &now
    }
    // BUG: DeletedAt not cleared if transitioning from DELETED to ACTIVE
    return nil
}

// CORRECT - maintain invariant: DeletedAt set iff status is DELETED
func (l *Limit) SetStatus(status LimitStatus) error {
    l.Status = status
    l.UpdatedAt = now

    if status == LimitStatusDeleted {
        l.DeletedAt = &now
    } else {
        l.DeletedAt = nil  // Clear on any other transition
    }
    return nil
}

// Also validate in Validate() method
func (l *Limit) Validate() error {
    // Enforce invariant: DeletedAt must be set iff status is DELETED
    if l.Status == LimitStatusDeleted && l.DeletedAt == nil {
        return ErrDeletedAtRequired
    }
    if l.Status != LimitStatusDeleted && l.DeletedAt != nil {
        return ErrDeletedAtMustBeNil
    }
    return nil
}
```

**Apply to:** Any model with soft-delete (DeletedAt field) and status transitions.

---

## Code Conventions

### Go Version and Modern Syntax (Go 1.25+)

This project requires Go 1.25 or later. Use modern Go syntax:

#### Use `any` Instead of `interface{}`

```go
// WRONG - legacy syntax
func ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

// CORRECT - modern syntax (Go 1.18+)
func ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
```

#### Use Generics for Reusable Utilities

```go
// CORRECT - generic pointer helper
func Ptr[T any](v T) *T {
    return &v
}

// Usage
testutil.Ptr(model.RuleStatusActive)
testutil.Ptr("some string")
testutil.Ptr(uuid.New())
```

#### Remove Dead Code

- Remove unused structs, functions, and variables
- Remove redundant tests (e.g., testing struct field assignment)
- Remove no-op assignments like `_ = ctx`

```go
// WRONG - no-op assignment
func (e *Evaluator) Evaluate(ctx context.Context, ...) {
    _ = ctx  // Remove this
    // ...
}

// CORRECT - use ctx or remove if unused
func (e *Evaluator) Evaluate(ctx context.Context, ...) {
    ctx, span := tracer.Start(ctx, "evaluator.evaluate")
    defer span.End()
    // ...
}
```

### Overflow-Safe Range Validation

When validating that a computed value doesn't exceed a maximum, rearrange arithmetic to avoid integer overflow:

```go
// WRONG - risks int64 overflow when startBase + count is large
maxBase := int64(999_999_999_999)
lastBase := startBase + int64(count) - 1
if lastBase > maxBase {
    return fmt.Errorf("exceeds max")
}

// CORRECT - rearranged to avoid overflow
// Original check: startBase + count - 1 <= maxBase
// Rearranged:     startBase <= maxBase - count + 1
if startBase > maxBase-int64(count)+1 {
    return fmt.Errorf("exceeds max")
}
```

**When to apply:**
- Range validation with large max values
- Any arithmetic that could overflow before comparison
- Loop bounds and slice capacity calculations

### Encapsulation via Accessor Functions

When exposing validation maps or internal collections, provide typed accessor functions instead of exporting the raw collection. This prevents runtime mutation and improves testability:

```go
// WRONG - exported collection can be mutated at runtime
var ValidLimitSortFields = map[string]bool{
    "createdAt": true,
    "updatedAt": true,
    "name":      true,
}

// Usage allows accidental mutation (bug risk)
ValidLimitSortFields["malicious"] = true  // Compiles!

// CORRECT - unexported with accessor function
var validLimitSortFields = map[string]bool{
    "createdAt": true,
    "updatedAt": true,
    "name":      true,
}

func IsValidLimitSortField(field string) bool {
    return validLimitSortFields[field]
}

// Usage is safe - no mutation possible
if model.IsValidLimitSortField("createdAt") { ... }
```

**Apply to:** All validation maps, enum allowlists, and allowed-values collections.

### Struct Field Order Consistency

When related structs represent the same data (e.g., input/output, request/response DTOs), maintain consistent field order across both structs for readability:

```go
// Input struct
type ScopeInput struct {
    AccountID       *string
    SegmentID       *string
    PortfolioID     *string
    MerchantID      *string
    TransactionType *string
    SubType         *string
}

// WRONG - different field order in response (confusing)
type ScopeResponse struct {
    SegmentID       *string  // Different order!
    PortfolioID     *string
    AccountID       *string  // Moved down
    MerchantID      *string
    TransactionType *string
    SubType         *string
}

// CORRECT - same field order as input
type ScopeResponse struct {
    AccountID       *string  // Matches input order
    SegmentID       *string
    PortfolioID     *string
    MerchantID      *string
    TransactionType *string
    SubType         *string
}
```

**Apply to:** Input/output pairs, Request/Response DTOs, model/DTO mappings.

### Interface Design

#### Define Minimal Interfaces

Interfaces should contain only the methods actually used:

```go
// CORRECT - minimal interface with only required methods
type DB interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

#### Interfaces Where Used

Define interfaces in the package that USES them, not where they're implemented:

```go
// internal/services/rule/command/repository.go
// Interface defined where it's used (in the service layer)
type RuleRepository interface {
    Create(ctx context.Context, rule *model.Rule) (*model.Rule, error)
    GetByID(ctx context.Context, id uuid.UUID) (*model.Rule, error)
    // ...
}
```

#### Use Adapter Pattern for Testability

When wrapping external concrete types, use adapters to satisfy interfaces:

```go
// postgresConnectionAdapter adapts *libPostgres.PostgresConnection to pgdb.Connection.
type postgresConnectionAdapter struct {
    conn *libPostgres.PostgresConnection
}

func (p *postgresConnectionAdapter) GetDB() (pgdb.DB, error) {
    return p.conn.GetDB()
}
```

### Naming Conventions

| Element | Convention | Example |
|---------|------------|---------|
| Files | snake_case (Go standard) | `create_example.go`, `example_handler.go` |
| Packages | lowercase, single word | `command`, `query`, `example` |
| Interfaces | PascalCase, noun | `Repository`, `ExampleHandler` |
| Structs | PascalCase | `ExampleCommand`, `ExampleQuery` |
| Methods | PascalCase, verb + noun | `CreateExample`, `GetExampleByID` |
| Test files | `*_test.go` | `create_example_test.go` |
| Mock files | `*_mock.go` | `example_repository_mock.go` |

### File Organization

Each file should have a single responsibility:
- One handler per entity
- One service operation per file
- One repository interface per entity

### Import Organization

Group imports in this order, separated by blank lines:

```go
import (
    // Standard library
    "context"
    "database/sql"

    // External dependencies
    libCommons "github.com/LerianStudio/lib-commons/v4/commons"
    libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
    libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
    "github.com/google/uuid"

    // Internal packages
    "github.com/LerianStudio/midaz/v3/components/tracer/internal/services"
    "github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)
```

### Context Propagation

Always pass `context.Context` as the first parameter:

```go
func (ex *ExampleCommand) CreateExample(ctx context.Context, ei *model.CreateExampleInput) (*model.ExampleOutput, error)
```

### Context Cancellation Checks

**CRITICAL:** Check for context cancellation **at the very start** of service methods, **before** any validation or processing. This prevents wasted CPU cycles when the client has already timed out or cancelled the request.

```go
// WRONG - validates before checking cancellation (wastes CPU)
func (s *Service) Execute(ctx context.Context, input *Input) (*Output, error) {
    // Expensive validation happens even if context is cancelled
    if err := input.Validate(); err != nil {
        return nil, err
    }

    // Only then check cancellation - too late!
    if err := ctx.Err(); err != nil {
        return nil, err
    }
    // ...
}

// CORRECT - check cancellation FIRST
func (s *Service) Execute(ctx context.Context, input *Input) (*Output, error) {
    // Check cancellation before ANY work
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    // Then proceed with validation and business logic
    if err := input.Validate(); err != nil {
        return nil, err
    }

    // ... rest of operation
}
```

**When to check:**
- **First line** of service methods (before validation)
- Before expensive operations (DB queries, external calls)
- In loops processing multiple items
- After long-running operations before continuing

**Testing tip:** Use `Times(0)` mock expectations to verify that downstream methods are NOT called when context is cancelled.

### Input Normalization Order

Always follow this order when processing inputs:

```go
func (s *Service) Create(ctx context.Context, input *CreateInput) (*Output, error) {
    // 1. Check context cancellation
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    // 2. Normalize input (trim whitespace, uppercase, etc.)
    input.Name = strings.TrimSpace(input.Name)
    input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))

    // 3. Apply defaults (for optional fields)
    input.ApplyDefaults()

    // 4. Validate (after normalization and defaults)
    if err := input.Validate(); err != nil {
        return nil, err
    }

    // 5. Business logic
    // ...
}
```

**Order matters:** Validation must happen AFTER normalization and defaults are applied.

### Whitespace-Only Validation

Reject strings that contain only whitespace characters:

```go
// WRONG - only checks empty string
if name == "" {
    return ErrNameRequired
}

// CORRECT - rejects whitespace-only strings
if strings.TrimSpace(name) == "" {
    return ErrNameRequired
}
```

**Apply to:** Name fields, description fields, any user-provided text that is required.

### Defensive Slice Copies

Create defensive copies of slices to prevent external mutation:

```go
// WRONG - stores reference, external code can modify
func NewLimit(scopes []Scope) *Limit {
    return &Limit{Scopes: scopes}  // Dangerous!
}

// CORRECT - defensive copy prevents mutation
func NewLimit(scopes []Scope) *Limit {
    scopesCopy := make([]Scope, len(scopes))
    copy(scopesCopy, scopes)

    return &Limit{Scopes: scopesCopy}
}

// Also apply in Update methods
func (l *Limit) Update(input UpdateInput) error {
    // ... validation ...

    if input.Scopes != nil {
        scopesCopy := make([]Scope, len(input.Scopes))
        copy(scopesCopy, input.Scopes)
        l.Scopes = scopesCopy
    }

    return nil
}
```

**Apply to:** Any slice field that comes from external input.

### Whitespace Style (wsl_v5)

This project uses the `wsl_v5` linter (configured in `.golangci.yml`) to enforce whitespace conventions. Follow these rules:

**Empty lines required before:**
- `return` statements (unless single-line block)
- `if`, `for`, `switch`, `select` blocks
- `defer` statements
- Assignments after different statement types

**Empty lines NOT allowed:**
- At the start of blocks (after `{`)
- At the end of blocks (before `}`)
- Multiple consecutive empty lines

```go
// WRONG - missing empty line before return
func example() error {
    result := doSomething()
    return result
}

// CORRECT - empty line before return
func example() error {
    result := doSomething()

    return result
}

// WRONG - empty line at start of block
func example() {

    doSomething()
}

// CORRECT - no empty line at start
func example() {
    doSomething()
}

// WRONG - cuddled if after assignment
func example() {
    result := getValue()
    if result > 0 {
        // ...
    }
}

// CORRECT - empty line before if
func example() {
    result := getValue()

    if result > 0 {
        // ...
    }
}
```

Run `make lint` to check for violations. Some issues can be auto-fixed with `golangci-lint run --fix`.

### HTTP Method Constants

Always use `net/http` package constants instead of string literals for HTTP methods:

```go
import "net/http"

// WRONG - string literals
req, _ := http.NewRequest("GET", url, nil)
req, _ := http.NewRequest("POST", url, body)
req, _ := http.NewRequest("DELETE", url, nil)

// CORRECT - use constants
req, _ := http.NewRequest(http.MethodGet, url, nil)
req, _ := http.NewRequest(http.MethodPost, url, body)
req, _ := http.NewRequest(http.MethodDelete, url, nil)
```

**Available constants:**
- `http.MethodGet`, `http.MethodPost`, `http.MethodPut`
- `http.MethodPatch`, `http.MethodDelete`, `http.MethodHead`
- `http.MethodOptions`, `http.MethodTrace`, `http.MethodConnect`

### UUID Fields in Models

ID fields representing UUIDs must use `uuid.UUID` type (not `string`):

```go
import "github.com/google/uuid"

type Scope struct {
    SegmentID   *uuid.UUID `json:"segmentId,omitempty" swaggertype:"string" format:"uuid"`
    PortfolioID *uuid.UUID `json:"portfolioId,omitempty" swaggertype:"string" format:"uuid"`
    AccountID   *uuid.UUID `json:"accountId,omitempty" swaggertype:"string" format:"uuid"`
    MerchantID  *uuid.UUID `json:"merchantId,omitempty" swaggertype:"string" format:"uuid"`
}
```

**Rules:**
- Use `uuid.UUID` for ID fields, not `string`
- Add `swaggertype:"string" format:"uuid"` tags for proper OpenAPI documentation
- Use pointer (`*uuid.UUID`) for optional fields
- JSON unmarshaling handles string-to-UUID conversion automatically

---

## Error Handling

### Error Types

| Type | When to Use | Example |
|------|-------------|---------|
| Business Error | Domain validation failures | `ErrEntityNotFound`, `ErrActionNotPermitted` |
| Technical Error | Infrastructure failures | Database connection errors, network errors |

### Error Constants

Define errors in `pkg/constant/errors.go`:

```go
var (
    ErrEntityNotFound     = errors.New("entity not found")
    ErrBadRequest         = errors.New("bad request")
    ErrActionNotPermitted = errors.New("action not permitted")
)
```

### Error Wrapping Rules

**Business errors**: Return directly without wrapping. These are expected errors with well-defined constants.

```go
if errors.Is(err, constant.ErrRuleNotFound) {
    libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rule not found", err)
    return nil, err  // Return directly
}
```

**Technical errors**: Wrap with context using `fmt.Errorf` and `%w` verb. This provides stack context for debugging.

```go
if err != nil {
    libOpentelemetry.HandleSpanError(&span, "Failed to update rule status", err)
    return nil, fmt.Errorf("failed to update rule status: %w", err)
}
```

### Error Handling Pattern (Complete Example)

```go
func (s *ActivateRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (*model.Rule, error) {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "service.rule.activate")
    defer span.End()

    rule, err := s.repository.GetByID(ctx, ruleID)
    if err != nil {
        if errors.Is(err, constant.ErrRuleNotFound) {
            // Business error - return directly
            libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rule not found", err)
            return nil, err
        }
        // Technical error - wrap with context
        libOpentelemetry.HandleSpanError(&span, "Failed to get rule", err)
        return nil, fmt.Errorf("failed to get rule: %w", err)
    }

    // ... business logic ...

    if err := s.repository.UpdateStatus(ctx, ruleID, model.RuleStatusActive, now); err != nil {
        // Technical error - wrap with context
        libOpentelemetry.HandleSpanError(&span, "Failed to update rule status", err)
        return nil, fmt.Errorf("failed to update rule status: %w", err)
    }

    return rule, nil
}
```

### Business Error Helper

Use `libCommons.ValidateBusinessError` when you need to add context to a business error:

```go
err := libCommons.ValidateBusinessError(constant.ErrRuleNotFound, "Rule")
return nil, err
```

### Sentinel Errors for Constructors

Constructors that receive dependencies must validate them and return errors instead of panicking:

```go
// Define sentinel errors for nil dependencies
var (
    ErrNilRepository = errors.New("repository cannot be nil")
    ErrNilEvaluator  = errors.New("evaluator cannot be nil")
)

// WRONG - panics on nil
func NewService(repo Repository) *Service {
    if repo == nil {
        panic("repository cannot be nil")  // Don't panic
    }
    return &Service{repo: repo}
}

// CORRECT - returns error on nil
func NewService(repo Repository) (*Service, error) {
    if repo == nil {
        return nil, ErrNilRepository
    }
    return &Service{repo: repo}, nil
}
```

**Rules:**
- Use sentinel errors (e.g., `ErrNilRepository`, `ErrNilEvaluator`) for nil dependency validation
- Constructor signature changes from `NewX(dep) *X` to `NewX(dep) (*X, error)`
- Test nil cases with `require.ErrorIs(t, err, ErrNilRepository)`

---

## HTTP Responses

All HTTP responses **MUST** use `libHTTP` wrappers for consistent response format across all Lerian services.

### Response Methods

| Method | HTTP Status | When to Use |
|--------|-------------|-------------|
| `libHTTP.OK(c, data)` | 200 | Successful GET, PUT, PATCH |
| `libHTTP.Created(c, data)` | 201 | Successful POST (resource created) |
| `libHTTP.NoContent(c)` | 204 | Successful DELETE |
| `libHTTP.WithError(c, err)` | 4xx/5xx | Error responses |

### Forbidden Patterns

```go
// FORBIDDEN - Direct Fiber responses
c.JSON(status, data)           // Don't use
c.Status(code).JSON(err)       // Don't use
c.SendString(text)             // Don't use

// CORRECT - Use libHTTP wrappers
libHTTP.OK(c, data)
libHTTP.Created(c, data)
libHTTP.WithError(c, err)
```

### Handler Example

```go
import (
    libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
    "github.com/gofiber/fiber/v2"
)

func (h *RuleHandler) Create(c *fiber.Ctx) error {
    ctx := c.UserContext()

    var input model.CreateRuleInput
    if err := c.BodyParser(&input); err != nil {
        return libHTTP.WithError(c, err)
    }

    result, err := h.ruleService.Create(ctx, &input)
    if err != nil {
        return libHTTP.WithError(c, err)
    }

    return libHTTP.Created(c, result)
}
```

---

## Pagination

### Cursor-Based Pagination (Required)

All list/search endpoints **MUST** use cursor-based pagination for consistent results during navigation.

**Why cursor-based?**
- Consistent results when data changes during navigation
- Efficient for large datasets (no offset scanning)
- Better performance for real-time data

### Pagination Model

```go
// Filter/Input for list operations
type ListRulesFilter struct {
    Status    *RuleStatus
    Action    *Decision
    Limit     int    // Max items per page (1-100, default: 10)
    Cursor    string // Base64 encoded cursor (empty for first page)
    SortBy    string // Field to sort by (e.g., "created_at", "name")
    SortOrder string // "ASC" or "DESC"
}

// Result/Output for list operations
type ListRulesResult struct {
    Rules      []Rule
    NextCursor string // Base64 encoded cursor for next page (empty if no more)
    HasMore    bool   // Indicates if there are more results
}
```

### Cursor Structure

The cursor contains all information needed to resume pagination consistently, even when data changes between requests:

```go
// pkg/net/http/cursor.go
type Cursor struct {
    ID         string `json:"id"`  // ID of the last item returned
    SortValue  string `json:"sv"`  // Value of the sort field for the last item
    SortBy     string `json:"sb"`  // Field used for sorting (e.g., "created_at", "name")
    SortOrder  string `json:"so"`  // Sort direction: "ASC" or "DESC"
    PointsNext bool   `json:"pn"`  // Direction indicator (true = next page)
}
```

**How it ensures consistency:**
1. The cursor stores the sort field value (`SortValue`) along with the ID
2. Repository queries use compound condition: `WHERE sort_field < :sv OR (sort_field = :sv AND id < :id)`
3. This ensures no items are skipped even if new items are inserted between requests

**Example:**
- Sorting by `created_at DESC`, last item: `{id: "rule-123", created_at: "2024-01-15T10:00:00Z"}`
- Cursor encoded: `{id: "rule-123", sv: "2024-01-15T10:00:00Z", sb: "created_at", so: "DESC", pn: true}`
- Next page query: `WHERE created_at < '2024-01-15T10:00:00Z' OR (created_at = '2024-01-15T10:00:00Z' AND id < 'rule-123') ORDER BY created_at DESC, id DESC`

### API Response Format

```json
{
    "rules": [...],
    "nextCursor": "eyJpZCI6InJ1bGUtMTIzIiwicG9pbnRzTmV4dCI6dHJ1ZX0=",
    "hasMore": true
}
```

### Usage Pattern

```go
// First page
GET /v1/rules?limit=10

// Next page (use nextCursor from previous response)
GET /v1/rules?limit=10&cursor=eyJpZCI6InJ1bGUtMTIzIiwicG9pbnRzTmV4dCI6dHJ1ZX0=
```

### Implementation Notes

1. **Do NOT use offset/page-based pagination** - it causes inconsistent results
2. **Limit range**: 1-100 items per page (default: 10)
3. **Empty cursor** = first page
4. **Empty nextCursor** = no more results
5. **Cursor is opaque** - clients should not decode/modify it

---

## Distributed Tracing

### OpenTelemetry Integration

This project uses OpenTelemetry via `lib-commons` for distributed tracing.

### Required Tracing Pattern

Every service method and repository operation must include tracing and structured logging:

```go
func (ex *ExampleCommand) CreateExample(ctx context.Context, ei *model.CreateExampleInput) (*model.ExampleOutput, error) {
    // 1. Extract logger and tracer from context (Ring Standards pattern)
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    // 2. Start span with proper naming: "service.{domain}.{operation}"
    ctx, span := tracer.Start(ctx, "service.example.create")
    defer span.End()

    // 3. Enrich logger with trace context (see Structured Logging section)
    logger = logging.WithTrace(ctx, logger)

    // 4. Log operation start with structured fields
    logger.WithFields(
        "operation", "service.example.create",
        "example.name", ei.Name,
    ).Info("Creating example")

    // 5. Set span attributes for debugging
    err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "example_repository_input", example)
    if err != nil {
        libOpentelemetry.HandleSpanError(&span, "Failed to convert example repository input to JSON string", err)
        return nil, err
    }

    // 6. Execute operation
    out, err := ex.ExampleRepo.Create(ctx, example)
    if err != nil {
        // 7. Handle errors with span context and structured logging
        libOpentelemetry.HandleSpanError(&span, "Failed to create example", err)
        logger.WithFields(
            "operation", "service.example.create",
            "error.message", err.Error(),
        ).Error("Failed to create example")
        return nil, err
    }

    // 8. Log success
    logger.WithFields(
        "operation", "service.example.create",
        "example.id", out.ID,
    ).Info("Example created successfully")

    return out, nil
}
```

### Span Naming Convention

| Layer | Pattern | Example |
|-------|---------|---------|
| Handler | `handler.{resource}.{action}` | `handler.rule.create` |
| Service | `service.{domain}.{operation}` | `service.rule.create` |
| Repository | `repository.{entity}.{operation}` | `repository.rule.find_by_id` |
| External Call | `external.{service}.{operation}` | `external.auth.validate` |
| Consumer | `consumer.{queue}.{operation}` | `consumer.validation.process` |

### Error Message Sanitization

Never expose internal implementation details in error messages returned to clients. Sanitize error messages to prevent information leakage:

```go
// WRONG - exposes internal details
return libHTTP.WithError(c, fmt.Errorf("database connection failed: %v", err))
return libHTTP.WithError(c, fmt.Errorf("field 'internal_score' validation failed"))

// CORRECT - generic client-facing message, detailed internal logging
logger.WithFields(
    "error.message", err.Error(),
    "operation", "handler.validation.create",
).Error("Database connection failed")
return libHTTP.WithError(c, constant.ErrInternalServer)

// CORRECT - sanitized validation error
return libHTTP.WithError(c, constant.ErrInvalidInput)
```

**Never expose:**
- Database connection strings or errors
- Internal field names not in API contract
- Stack traces or file paths
- Third-party service details

### Error Classification

| Error Type | Method | Effect on Span | When to Use |
|------------|--------|----------------|-------------|
| Technical errors | `libOpentelemetry.HandleSpanError(&span, message, err)` | Marks span as ERROR | DB failure, network timeout, unexpected panic |
| Business errors | `libOpentelemetry.HandleSpanBusinessErrorEvent(&span, message, err)` | Span stays OK (adds event) | Validation failed, not found, conflict, unauthorized |

**Why distinguish error types:**
- Business errors are expected and don't indicate system problems
- Technical errors indicate infrastructure issues requiring investigation
- Alerting systems typically trigger on ERROR status spans
- Using `HandleSpanBusinessErrorEvent` for validation errors prevents alert noise

**Example:**

    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    ctx, span := tracer.Start(ctx, "service.rule.get")
    defer span.End()

    rule, err := s.ruleRepo.FindByID(ctx, id)
    if err != nil {
        if errors.Is(err, constant.ErrEntityNotFound) {
            // Business error - expected, span stays OK, return directly
            libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rule not found", err)
            return nil, err
        }
        // Technical error - unexpected, span marked ERROR, wrap with context
        libOpentelemetry.HandleSpanError(&span, "Database query failed", err)
        return nil, fmt.Errorf("failed to get rule: %w", err)
    }

    return rule, nil
}
```

---

## Authentication

### Single-Tenant MVP Architecture

Tracer MVP uses single-tenant deployment with API Key authentication only.

| Aspect | Value |
|--------|-------|
| **Mechanism** | API Key (header) |
| **Header** | `X-API-Key` |
| **Validation** | Environment variable comparison |
| **Multi-tenant** | Phase 2 (not MVP) |

### Authentication Flow

```go
import (
    "crypto/subtle"
    
    libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
    "github.com/gofiber/fiber/v2"
)

func AuthMiddleware(apiKey string) fiber.Handler {
    return func(c *fiber.Ctx) error {
        key := c.Get("X-API-Key")
        if key == "" {
            return libHTTP.Unauthorized(c, "Unauthenticated", "Unauthorized", "API Key missing or invalid")
        }
        // Use constant-time comparison to prevent timing attacks
        if subtle.ConstantTimeCompare([]byte(key), []byte(apiKey)) != 1 {
            return libHTTP.Unauthorized(c, "Unauthenticated", "Unauthorized", "API Key missing or invalid")
        }
        return c.Next()
    }
}
```

### Security Rules

- All endpoints require authentication (except `/health`, `/readyz`, `/metrics`, `/version`, `/swagger/*`)
- `/metrics` is unauthenticated by design (Prometheus scrape) and MUST be network-restricted via Kubernetes Service `ClusterIP`, NetworkPolicy, or equivalent firewall — never expose via public ingress.
- API Key configured via `API_KEY` environment variable
- No JWT/JWK validation in MVP (simplification)
- Log all authentication failures for auditing

---

## Performance Requirements

### Latency Targets

| Metric | Target | Maximum |
|--------|--------|---------|
| **Validation Latency (p50)** | <35ms | - |
| **Validation Latency (p99)** | <80ms | 100ms |
| **Expression Evaluation** | <1ms | 5ms |
| **Rule Query (all active)** | <5ms | 10ms |

### Latency Budget

| Stage | Target | Max |
|-------|--------|-----|
| Request parse | 1ms | 2ms |
| Auth validation | 2ms | 5ms |
| Rule query | 5ms | 10ms |
| Scope filtering | 2ms | 5ms |
| Expression evaluation | 10ms | 20ms |
| Limit query | 3ms | 5ms |
| Limit check | 5ms | 10ms |
| Audit write | async | N/A |
| Response build | 1ms | 2ms |
| **Total** | **34ms** | **80ms** |

### Async Audit Write Pattern (Fire-and-Forget)

Audit writes **MUST** be asynchronous to not block validation response latency. Use goroutines with proper error logging:

```go
// Audit writes must be async to not block validation response
func (s *ValidationService) Validate(ctx context.Context, req *ValidationRequest) (*ValidationResponse, error) {
    // ... validation logic ...

    // Fire and forget - do not block response
    // Use background context since request context may be cancelled
    go func() {
        if err := s.auditWriter.QueueAudit(context.Background(), audit); err != nil {
            // Use structured logging with request context for correlation
            logger.WithFields(
                "request.id", req.RequestID.String(),
                "error.message", err.Error(),
            ).Error("Failed to queue audit")
        }
    }()

    return response, nil
}
```

**Key points:**
- Use `context.Background()` in goroutine (request context may be cancelled)
- Log errors with structured fields including `request.id` for correlation
- Never block the response waiting for audit completion
- Consider using a channel-based queue for backpressure in high-throughput scenarios

---

## Expression Language (CEL)

Tracer uses CEL (Common Expression Language) for rule expressions.

### Why CEL

- Type-safe with compile-time validation
- Cost limits prevent DoS attacks
- Google-backed, used in Kubernetes policies
- Evaluates in <1ms

### Expression Context

Rules have access to the complete transaction context:

```cel
// Available variables in expression evaluation
transactionType       // String: "CARD", "WIRE", "PIX", "CRYPTO"
subType               // String: "debit", "credit", "instant", etc.
amount                // dyn (decimal.Decimal as float64 — supports == with int and double literals)
currency              // String (ISO 4217)
transactionTimestamp  // int64 Unix timestamp in nanoseconds
account               // Map: account["id"], account["type"], account["status"]
segment               // Map: segment["id"] (optional)
portfolio             // Map: portfolio["id"] (optional)
merchant              // Map: merchant["id"], merchant["name"], merchant["category"] (optional)
metadata              // Map of custom fields
```

> **Note on `amount` precision:** The `amount` variable is internally converted from
> `decimal.Decimal` to `float64` (via `InexactFloat64()`). This means exact equality
> checks like `amount == 100.01` may behave unexpectedly due to binary floating-point
> representation. Prefer range comparisons (e.g., `amount >= 100.00 && amount <= 100.02`)
> or integer thresholds (e.g., `amount > 100`) for reliable results.

### Expression Examples

```cel
// Block high-value transactions (amount > $100.00)
amount > 100

// Review international wire transactions
transactionType == "WIRE" && subType == "international"

// Deny transactions from suspended accounts
account["status"] == "suspended"

// Allow small transactions from active accounts (amount < $10.00)
amount < 10 && account["status"] == "active"
```

### Compilation and Caching

- Expressions are compiled at rule creation/update
- Compiled programs cached in-memory (L1)
- Cache key: expression hash
- Invalidation: on expression change

---

## Resilience Patterns

### Circuit Breaker

```go
// Circuit breaker for database operations
type CircuitBreakerConfig struct {
    OpenAfterFailures   int           // 5 consecutive failures
    HalfOpenAfter       time.Duration // 30 seconds
    CloseAfterSuccesses int           // 3 successful calls
}
```

**Fallback Behavior:**
- On database circuit open: Return ALLOW with warning flag
- Log circuit breaker activation
- Alert operations team

### Timeout Strategy

```text
Client timeout: 100ms (configurable)
    |
    v
Tracer internal timeout: 80ms
    |
    +-- Database query: 30ms
    +-- Cache operation: 5ms
    +-- Expression eval: 10ms
    +-- Auth validation: 10ms
```

### Graceful Degradation

| Scenario | Fallback | Impact |
|----------|----------|--------|
| Cache unavailable | Query database directly | +50ms latency |
| Database slow | Use cached data (stale) | Decisions based on cached rules |
| High load | Shed oldest requests | Some requests timeout |

---

## Testing

### Testing Framework

- **Test Framework**: `testing` (stdlib) + `github.com/stretchr/testify`
- **Mocking**: `go.uber.org/mock/gomock` (always use gomock for interface mocks)
- **Mock Generation**: `mockgen` (gomock generator, using `//go:generate` directives)
- **SQL Mocking**: `github.com/DATA-DOG/go-sqlmock` (for database query testing)

### Test Code Quality Rules

#### Error Handling in Tests

Never ignore errors with `_` in test code. Use `require.NoError` for errors in helpers:

```go
// WRONG - error ignored
scopesJSON, _ := json.Marshal(rule.Scopes)

// CORRECT - error handled with descriptive message
scopesJSON, err := json.Marshal(rule.Scopes)
require.NoError(t, err, "failed to marshal scopes")
```

Test setup functions that can fail must accept `*testing.T`:

```go
// CORRECT - setup function receives *testing.T for error handling
func ruleRow(t *testing.T, rule *model.Rule) *sqlmock.Rows {
    t.Helper()
    scopesJSON, err := json.Marshal(rule.Scopes)
    require.NoError(t, err, "failed to marshal scopes")
    // ...
}
```

#### Test Naming

Test names should describe behavior, not success/failure status:

```go
// WRONG - redundant prefix
"Success - creates rule"
"Error - rule not found"

// CORRECT - descriptive behavior
"creates rule"
"creates rule with scopes"
"returns error when rule not found"
"returns error when database fails"
```

#### Test Assertions

Validate all relevant fields in assertions, not just status codes:

```go
// WRONG - only checks status code
assert.Equal(t, http.StatusOK, resp.StatusCode)

// CORRECT - validates all relevant response fields
assert.Equal(t, http.StatusOK, resp.StatusCode)
assert.Equal(t, expectedTitle, resp.Title)
assert.Equal(t, expectedMessage, resp.Message)
assert.Len(t, resp.Fields, expectedFieldCount)
```

Include edge cases in test tables (nil values, empty collections, boundary conditions).

#### Slice Indexing Safety

Always use `require.Len` before indexing slices to prevent index out-of-bounds panics:

```go
// WRONG - can panic if slice is empty
assert.Equal(t, expectedID, rules[0].ID)

// CORRECT - validate length first
require.Len(t, rules, 1, "expected exactly one rule")
assert.Equal(t, expectedID, rules[0].ID)

// CORRECT - for multiple items
require.Len(t, results, 3, "expected three results")
assert.Equal(t, expected1, results[0].Name)
assert.Equal(t, expected2, results[1].Name)
assert.Equal(t, expected3, results[2].Name)
```

#### Array/Slice Content Verification

Always verify the **content** of arrays/slices, not just their **length**. Length checks alone can miss bugs where size matches but elements are wrong.

```go
// WRONG - only checks length, not content
require.Len(t, result.MatchedRuleIDs, len(expected.MatchedRuleIDs))
require.Len(t, result.Items, 3)

// CORRECT - checks length AND content
require.Len(t, result.MatchedRuleIDs, len(expected.MatchedRuleIDs))
require.Equal(t, expected.MatchedRuleIDs, result.MatchedRuleIDs, "MatchedRuleIDs content mismatch")

// CORRECT - for unordered comparisons
require.ElementsMatch(t, expected.Tags, result.Tags, "Tags content mismatch")
```

**When to use each:**
- `require.Equal`: When **order matters** (IDs in evaluation order, sorted results)
- `require.ElementsMatch`: When **order doesn't matter** (tags, sets, unordered collections)

**Real Bug Example:**
```go
// Test passes even though UUID is wrong!
expected := []uuid.UUID{uuid.MustParse("...001")}
actual   := []uuid.UUID{uuid.MustParse("...999")} // BUG!
require.Len(t, actual, len(expected)) // ✅ Passes (both length 1)
// Missing: require.Equal(t, expected, actual) ❌
```

**Pattern to follow:**
```go
// Step 1: Validate length (prevents index panic)
require.Len(t, result.Items, len(expected.Items), "unexpected number of items")

// Step 2: Validate content (catches wrong values)
require.Equal(t, expected.Items, result.Items, "items content mismatch")
```

#### Error Comparison with errors.Is

Use `errors.Is` or `require.ErrorIs` instead of string matching for error assertions:

```go
// WRONG - fragile string matching
assert.Contains(t, err.Error(), "not found")
assert.True(t, strings.Contains(err.Error(), "invalid"))

// CORRECT - use sentinel errors with ErrorIs
require.ErrorIs(t, err, constant.ErrNotFound)
assert.ErrorIs(t, err, constant.ErrInvalidInput)

// CORRECT - for context cancellation
require.ErrorIs(t, err, context.Canceled)
```

#### Deterministic Test Data

**CRITICAL RULE:** Never use `uuid.New()`, `time.Now()`, or any non-deterministic values in tests or test helpers.

Use deterministic UUIDs and timestamps for reproducible tests:

```go
// WRONG - non-deterministic, hard to debug
rule := &model.Rule{
    ID:        uuid.New(),           // Random each run - FORBIDDEN
    CreatedAt: time.Now(),           // Different each run - FORBIDDEN
}

// WRONG - test helper with non-deterministic values
func createTestRequest() *ValidationRequest {
    return &ValidationRequest{
        RequestID:            uuid.New(),   // FORBIDDEN IN HELPERS
        TransactionTimestamp: time.Now(),  // FORBIDDEN IN HELPERS
    }
}

// CORRECT - deterministic, reproducible
rule := &model.Rule{
    ID:        testutil.DeterministicUUID(1),  // Always same UUID
    CreatedAt: testutil.FixedTime(),           // Consistent timestamp
}

// CORRECT - test helper with deterministic values
func createTestRequest() *ValidationRequest {
    return &ValidationRequest{
        RequestID:            testutil.MustDeterministicUUID(1),
        TransactionTimestamp: testutil.FixedTime(),
        Account: AccountContext{
            ID: testutil.MustDeterministicUUID(2),
        },
    }
}

// For multiple UUIDs
ids, err := testutil.DeterministicUUIDs(1, 5)
require.NoError(t, err)
```

**Benefits:**
- Tests are reproducible across runs
- Easier to debug failures (same values every time)
- Consistent expected values in assertions
- Prevents flaky tests from timing issues
- CI/CD builds are deterministic

**Available Deterministic Helpers:**
- `testutil.FixedTime()` - Returns 2024-01-01T00:00:00Z
- `testutil.MustDeterministicUUID(seed)` - Returns deterministic UUID based on seed
- `testutil.DeterministicUUIDs(start, count)` - Returns slice of deterministic UUIDs
- `testutil.NewDefaultMockClock()` - Returns mock clock with fixed time

**Common Violations:**
```go
// ❌ WRONG - uuid.New() in test helper
createValidRequest := func() *ValidationRequest {
    return &ValidationRequest{
        RequestID: uuid.New(),  // Will cause flaky tests
    }
}

// ❌ WRONG - time.Now() in test setup
beforeTest := time.Now()
// Test logic...
assert.True(t, createdAt.After(beforeTest))  // Timing-dependent

// ✅ CORRECT - Fixed time reference
fixedTime := testutil.FixedTime()
// Test logic...
assert.Equal(t, fixedTime, createdAt)  // Deterministic assertion
```

#### Boundary Value Tests

Always test boundary conditions for validation limits:

```go
func TestNewLimit(t *testing.T) {
    tests := []struct {
        name      string
        input     CreateLimitInput
        expectErr bool
    }{
        // Boundary tests for name length
        {
            name:      "name at max length is valid",
            input:     CreateLimitInput{Name: strings.Repeat("a", MaxNameLength)},
            expectErr: false,
        },
        {
            name:      "name exceeds max length fails",
            input:     CreateLimitInput{Name: strings.Repeat("a", MaxNameLength+1)},
            expectErr: true,
        },
        // Boundary tests for numeric values
        {
            name:      "amount at max value is valid",
            input:     CreateLimitInput{MaxAmount: MaxAllowedAmount},
            expectErr: false,
        },
        {
            name:      "amount exceeds max value fails",
            input:     CreateLimitInput{MaxAmount: MaxAllowedAmount + 1},
            expectErr: true,
        },
    }
    // ...
}
```

**Always test:**
- Exactly at the limit (should pass)
- One over the limit (should fail)
- Zero/empty values
- Negative values (if applicable)

#### No-Mutation Assertions

Verify that failed operations do not partially mutate state:

```go
func TestLimit_Update_InvalidInput(t *testing.T) {
    limit := &model.Limit{
        Name:   "Original Name",
        Status: model.LimitStatusActive,
    }

    // Capture original state
    originalName := limit.Name
    originalStatus := limit.Status

    // Attempt invalid update
    err := limit.Update(invalidInput)

    // Verify error occurred
    require.Error(t, err)

    // Verify NO partial mutation happened
    assert.Equal(t, originalName, limit.Name, "name should not mutate on error")
    assert.Equal(t, originalStatus, limit.Status, "status should not mutate on error")
}
```

**Apply to:**
- Update methods that can fail validation
- State transitions that can be rejected
- Any operation where partial mutation would be a bug

#### Test Helper Error Handling

Test helper functions that can fail should return errors instead of panicking, allowing tests to handle failures gracefully:

```go
// WRONG - panics on invalid input
func DeterministicUUID(base int) uuid.UUID {
    if base < 0 || base > maxBase {
        panic("base out of range")
    }
    return uuid.MustParse(...)
}

// CORRECT - returns error for graceful handling
func DeterministicUUID(base int64) (uuid.UUID, error) {
    if base < 0 || base > maxBase {
        return uuid.Nil, ErrBaseOutOfRange
    }
    return uuid.MustParse(...), nil
}

// CORRECT - convenience wrapper for test setup (panics on error)
func MustDeterministicUUID(base int64) uuid.UUID {
    id, err := DeterministicUUID(base)
    if err != nil {
        panic(fmt.Sprintf("MustDeterministicUUID: %v", err))
    }
    return id
}
```

**Benefits:**
- Tests can verify error conditions: `require.ErrorIs(t, err, ErrBaseOutOfRange)`
- Clearer error messages than stack traces from panics
- Distinguishes validation failures from logic bugs

**Apply to:** UUID generators, timestamp helpers, fixture builders, any helper that validates input.

### Decimal Values in Tests (shopspring/decimal)

- **Tests**: ALWAYS use `decimal.RequireFromString("value")` — this reproduces the production path where JSON unmarshaling calls `NewFromString` internally
- **Production**: values arrive automatically via JSON/DB unmarshaling, no manual constructors needed
- **Math constants** (e.g., ×100 for percentage): `decimal.NewFromInt` is acceptable in production code
- **NEVER** use `decimal.NewFromFloat()` — IEEE 754 floating-point imprecision

```go
// CORRECT — matches production behavior (JSON → NewFromString)
decimal.RequireFromString("100")
decimal.RequireFromString("99.99")
decimal.RequireFromString("0.01")

// WRONG — float64 imprecision
decimal.NewFromFloat(99.99)

// WRONG in tests — does not match production path
decimal.NewFromInt(100)
```

### Mocking Rules

#### Always Use gomock

Always use `go.uber.org/mock/gomock` for interface mocks. Never use manual mock implementations:

```go
// CORRECT - use gomock
ctrl := gomock.NewController(t)
mockRepo := mocks.NewMockRepository(ctrl)
mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(result, nil)
```

#### No Explicit ctrl.Finish() Required

With `go.uber.org/mock` v0.3.0+, `gomock.NewController(t)` automatically registers cleanup via `t.Cleanup()`. Do not add explicit `defer ctrl.Finish()` calls - they are redundant:

```go
// CORRECT - no explicit Finish needed (go.uber.org/mock v0.3.0+)
ctrl := gomock.NewController(t)
mockRepo := mocks.NewMockRepository(ctrl)

// WRONG - redundant cleanup call
ctrl := gomock.NewController(t)
defer ctrl.Finish()  // Remove this - cleanup is automatic
```

#### Mock Package Organization

To avoid import cycles, place interfaces in a separate package from their implementations:

```go
// internal/adapters/postgres/db/interfaces.go
package db

//go:generate mockgen -source=interfaces.go -destination=mocks/interfaces_mock.go -package=mocks

type DB interface {
    ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Connection interface {
    GetDB() (DB, error)
}
```

Generated mocks go in `mocks/` subpackage:
- `internal/adapters/postgres/db/mocks/interfaces_mock.go`

#### SQL Testing with sqlmock

For repository tests, combine gomock (for connection interface) with sqlmock (for query expectations):

```go
func setupMockDB(t *testing.T) (*Repository, sqlmock.Sqlmock, func()) {
    t.Helper()
    
    ctrl := gomock.NewController(t)
    db, sqlMock, err := sqlmock.New()
    require.NoError(t, err)
    
    mockConn := mocks.NewMockConnection(ctrl)
    mockConn.EXPECT().GetDB().Return(db, nil).AnyTimes()
    
    repo := NewRepositoryWithConnection(mockConn)
    
    cleanup := func() {
        db.Close()
        ctrl.Finish()
    }
    
    return repo, sqlMock, cleanup
}
```

### Test Helpers

#### Centralized Helpers

Shared test helpers must be placed in `internal/testutil/` package:

```go
// internal/testutil/uuid_helpers.go
package testutil

// Ptr returns a pointer to any value. Generic helper for tests.
func Ptr[T any](v T) *T {
    return &v
}

// UUIDPtr returns a pointer to the given UUID.
// Wrapper around Ptr for discoverability.
func UUIDPtr(u uuid.UUID) *uuid.UUID {
    return Ptr(u)
}
```

#### No Local Duplicate Helpers

Do not create local helper functions when equivalent exists in `testutil`:

```go
// WRONG - local helper duplicating testutil.Ptr
func ruleStatusPtr(s model.RuleStatus) *model.RuleStatus { return &s }

// CORRECT - use generic helper
testutil.Ptr(model.RuleStatusActive)
testutil.Ptr(model.TransactionTypeCard)
```

#### Integration Test Helpers

Integration test helpers are centralized in `internal/testutil/integration_helpers.go`:

```go
// Available helpers for integration tests
testutil.CreateTestRule(t, name, expression, action)  // Creates rule via API
testutil.CleanupRule(t, ruleID)                       // Deletes rule via API
testutil.GetAPIKey()                                  // Returns API key from env
testutil.GetBaseURL()                                 // Returns base URL from env
testutil.AssertErrorResponse(t, resp, expectedStatus) // Validates error response
```

#### HTTP Response Body Handling

Always close response bodies to prevent resource leaks:

```go
// WRONG - body leak when reassigning resp
resp, err := client.Do(req1)
require.NoError(t, err)
// ... use resp ...
resp, err = client.Do(req2)  // Previous body leaked!

// CORRECT - close body before reassigning
resp, err := client.Do(req1)
require.NoError(t, err)
defer resp.Body.Close()
body, err := io.ReadAll(resp.Body)
require.NoError(t, err)
resp.Body.Close()  // Explicit close before reassigning

resp, err = client.Do(req2)
require.NoError(t, err)
defer resp.Body.Close()
```

### Test File Location

Tests must be co-located with the code they test:
- `create_example.go` → `create_example_test.go`

### Table-Driven Tests

All tests must use table-driven pattern:

```go
func TestCreateExample(t *testing.T) {
    tests := []struct {
        name           string
        exampleInput   *model.CreateExampleInput
        mockSetup      func(ctrl *gomock.Controller) *MockRepository
        expectErr      bool
        expectedResult *model.ExampleOutput
    }{
        {
            name:         "Success - Create example",
            exampleInput: createExampleInput,
            mockSetup: func(ctrl *gomock.Controller) *MockRepository {
                mockRepo := NewMockRepository(ctrl)
                mockRepo.EXPECT().
                    Create(gomock.Any(), gomock.Any()).
                    Return(&model.ExampleOutput{ID: "valid-uuid", Name: "test", Age: 12}, nil)
                return mockRepo
            },
            expectErr:      false,
            expectedResult: &model.ExampleOutput{ID: "valid-uuid", Name: "test", Age: 12},
        },
        {
            name:         "Error - Create an example",
            exampleInput: createExampleInput,
            mockSetup: func(ctrl *gomock.Controller) *MockRepository {
                mockRepo := NewMockRepository(ctrl)
                mockRepo.EXPECT().
                    Create(gomock.Any(), gomock.Any()).
                    Return(nil, constant.ErrBadRequest)
                return mockRepo
            },
            expectErr:      true,
            expectedResult: nil,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // gomock.NewController(t) automatically registers cleanup via t.Cleanup()
            // so explicit defer ctrl.Finish() is not needed (go.uber.org/mock v0.3.0+)
            ctrl := gomock.NewController(t)

            mockRepo := tt.mockSetup(ctrl)
            exampleCase := &ExampleCommand{ExampleRepo: mockRepo}

            ctx := context.Background()
            result, err := exampleCase.CreateExample(ctx, tt.exampleInput)

            if tt.expectErr {
                assert.Error(t, err)
                assert.Nil(t, result)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
            }
        })
    }
}
```

### Test Naming Convention

- Test function: `Test{FunctionName}`
- Test cases: Descriptive names with prefix (`Success -`, `Error -`, `Validation -`)

### Shared Test Helpers

Test helper functions must be placed in `internal/testutil/` package, not duplicated in each test file:

```go
// internal/testutil/uuid_helpers.go
package testutil

import "github.com/google/uuid"

// UUIDPtr returns a pointer to the given UUID.
func UUIDPtr(u uuid.UUID) *uuid.UUID {
    return &u
}
```

Usage in tests:

```go
import "github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"

// In test:
{AccountID: testutil.UUIDPtr(uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"))}
```

### Benchmark Requirements

Benchmarks must cover realistic scenarios, not just happy paths:

- **Baseline:** Best-case scenario (all operations succeed)
- **Partial matches:** Simulated failure rates (e.g., 50% match rate)
- **Edge cases:** Early exits, error paths
- **Realistic latency:** Simulated I/O or computation costs

### Benchmark Best Practices

#### 1. Use `b.Loop()` (Go 1.24+)

Always use `b.Loop()` instead of `for i := 0; i < b.N; i++`. This modern pattern provides better accuracy and cleaner code:

```go
// CORRECT: Modern b.Loop() pattern (Go 1.24+)
for b.Loop() {
    result = expensiveOperation()
}

// INCORRECT: Legacy pattern (do not use)
for i := 0; i < b.N; i++ {
    result = expensiveOperation()
}
```

#### 2. Prevent Compiler Optimization

The Go compiler may optimize away operations whose results are unused. Always assign results to a **package-level variable**:

```go
// Package-level sink - REQUIRED to prevent optimization
var benchSink any

func BenchmarkOperation(b *testing.B) {
    for b.Loop() {
        result := expensiveOperation()
        benchSink = result // Prevents compiler from eliminating the call
    }
}
```

**Why package-level?** Local variables can still be optimized away. Package-level variables force the compiler to keep the computation.

#### 3. Never Ignore Errors

Always check errors in benchmarks. Silent failures produce misleading results:

```go
// CORRECT: Check errors
for b.Loop() {
    result, err := operation()
    if err != nil {
        b.Fatal(err) // Stops benchmark immediately on error
    }
    benchSink = result
}

// INCORRECT: Ignoring errors
for b.Loop() {
    _, _ = operation() // BAD: hides failures, misleading results
}
```

#### 4. Setup Outside the Loop

Move setup code outside the benchmark loop and use `b.ResetTimer()`:

```go
func BenchmarkWithSetup(b *testing.B) {
    // Setup (not measured)
    data := expensiveSetup()

    b.ResetTimer() // Reset timer after setup

    for b.Loop() {
        result, err := processData(data)
        if err != nil {
            b.Fatal(err)
        }
        benchSink = result
    }
}
```

#### 5. Use Subtests for Multiple Scenarios

```go
func BenchmarkEvaluateRules(b *testing.B) {
    ruleCounts := []int{10, 100, 1000, 10000}

    for _, count := range ruleCounts {
        b.Run(fmt.Sprintf("all_match_%d_rules", count), func(b *testing.B) {
            data := setupData(count)
            b.ResetTimer()

            for b.Loop() {
                result, err := processData(data)
                if err != nil {
                    b.Fatal(err)
                }
                benchSink = result
            }
        })
    }
}
```

#### Complete Example

```go
package mypackage

// Package-level sink to prevent compiler optimization
var benchSink any

func BenchmarkCompleteExample(b *testing.B) {
    scenarios := []struct {
        name  string
        count int
    }{
        {"small", 10},
        {"medium", 100},
        {"large", 1000},
    }

    for _, sc := range scenarios {
        b.Run(sc.name, func(b *testing.B) {
            // Setup outside loop
            data := setupTestData(sc.count)
            b.ResetTimer()

            // Modern b.Loop() pattern
            for b.Loop() {
                result, err := processData(data)
                if err != nil {
                    b.Fatal(err) // Never ignore errors
                }
                benchSink = result // Prevent optimization
            }
        })
    }
}
```

### Concurrency Safety in Test Mocks

When test mocks have shared state accessed from multiple goroutines, use `sync.Mutex` or `sync.Once` for thread safety:

```go
// WRONG - race condition when mock accessed from multiple goroutines
type MockLogger struct {
    messages []string
}

func (m *MockLogger) Info(msg string) {
    m.messages = append(m.messages, msg)  // Race condition!
}

// CORRECT - mutex protects shared state
type MockLogger struct {
    mu       sync.Mutex
    messages []string
}

func (m *MockLogger) Info(msg string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.messages = append(m.messages, msg)
}

// CORRECT - sync.Once for idempotent operations
type MockTicker struct {
    stopOnce sync.Once
    stopped  bool
}

func (m *MockTicker) Stop() {
    m.stopOnce.Do(func() {
        m.stopped = true
    })
}
```

**Apply to:** MockLogger, MockTicker, any test mock with mutable state.

### Parallel Benchmark Error Handling

In benchmarks using `RunParallel`, use `b.Error` instead of `b.Fatal`. `b.Fatal` calls `runtime.Goexit()` which only terminates the current goroutine, not the entire benchmark:

```go
// WRONG - b.Fatal only terminates one goroutine
b.RunParallel(func(pb *testing.PB) {
    for pb.Next() {
        result, err := operation()
        if err != nil {
            b.Fatal(err)  // Only stops THIS goroutine
        }
    }
})

// CORRECT - b.Error marks benchmark as failed
b.RunParallel(func(pb *testing.PB) {
    for pb.Next() {
        result, err := operation()
        if err != nil {
            b.Error(err)  // Marks benchmark failed, continues others
            return
        }
        benchSink = result
    }
})
```

### t.Parallel() Convention

Follow the existing convention of each test file. Do **not** mix `t.Parallel()` and serial tests in the same file:

- **If the file already uses `t.Parallel()`** — new tests should also use it.
- **If the file runs tests serially** (no `t.Parallel()`) — new tests must **not** add it.
- **Never nest `t.Parallel()` in subtests** when the parent test uses `t.Run` with table-driven patterns and a shared `sqlmock`. The `sqlmock` instance is not goroutine-safe, so concurrent subtests race on its internal state, causing data races and deadlocks. Safe alternatives: either avoid `t.Parallel()` in subtests (preferred), or create an independent `sqlmock` per subtest before calling `t.Parallel()`.

```go
// WRONG - nested t.Parallel() races on shared sqlmock (not goroutine-safe)
func TestFoo(t *testing.T) {
    t.Parallel()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // DATA RACE: subtests share one sqlmock instance
            // ...
        })
    }
}

// CORRECT - only top-level t.Parallel() if file convention allows
func TestFoo(t *testing.T) {
    t.Parallel()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // No t.Parallel() here
            repo, mock, cleanup := setupMockDB(t)
            defer cleanup()
            // ...
        })
    }
}
```

### Mock Generation

Add `//go:generate mockgen` directive to interfaces and run `go generate` to generate mocks:

```go
// In the file that defines the interface (e.g., internal/services/example/repository.go)
//go:generate mockgen -source=repository.go -destination=repository_mock.go -package=example

type Repository interface {
    Create(ctx context.Context, input *model.Example) (*model.ExampleOutput, error)
    Find(ctx context.Context, id uuid.UUID) (*model.ExampleOutput, error)
    // ...
}
```

Run mock generation:

```bash
go generate ./...  # Generates mocks based on //go:generate directives
```

### Integration Test Infrastructure

#### Testcontainers as Default

Integration tests **MUST** use testcontainers by default for reproducible, isolated test environments:

```go
// internal/testutil/testcontainer_suite.go
type TestContainerSuite struct {
    PostgresContainer testcontainers.Container
    PostgresDSN       string
    // ...
}

func (s *TestContainerSuite) SetupSuite() {
    // Start containers automatically
    s.PostgresContainer = startPostgresContainer()
    s.PostgresDSN = getPostgresDSN(s.PostgresContainer)
}
```

**Benefits:**
- No external dependencies required to run tests
- Consistent environment across CI and local development
- Automatic cleanup after tests complete

#### Localhost Binding for Test Servers

Test servers **MUST** bind to loopback address only, never to all interfaces:

```go
// WRONG - exposes test server to network
listener, err := net.Listen("tcp", ":0")
os.Setenv("SERVER_ADDRESS", fmt.Sprintf(":%d", port))

// CORRECT - binds only to localhost
listener, err := net.Listen("tcp", "127.0.0.1:0")
port := listener.Addr().(*net.TCPAddr).Port
os.Setenv("SERVER_ADDRESS", fmt.Sprintf("127.0.0.1:%d", port))
```

**Why:** Binding to `:0` exposes the test server to all network interfaces, which is a security risk in shared environments.

#### Graceful Shutdown Order

When using testcontainers, services **MUST** be shut down before containers are terminated:

```go
func (s *TestContainerSuite) TearDownSuite() {
    // 1. First shutdown the service gracefully
    if s.Service != nil {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        s.Service.Shutdown(ctx)
    }

    // 2. Then terminate containers
    if s.PostgresContainer != nil {
        s.PostgresContainer.Terminate(context.Background())
    }
}
```

**Why:** Terminating containers before services can cause connection errors and test flakiness.

### Test Commands

```bash
# Test Commands
make test                # Run all tests
make test-unit           # Run unit tests only
make test-integration    # Run integration tests (with testcontainers)
make test-e2e            # Run E2E BDD tests (resets DB, runs Godog scenarios)
make test-all            # Run all tests (unit + integration)
make test-bench          # Run benchmark tests

# Coverage Commands
make coverage-unit       # Unit test coverage (uses .ignorecoverunit)
make coverage-integration # Integration test coverage
make coverage            # All coverage targets

# Security Commands
make sec                 # Run security checks (gosec + govulncheck)
make sec SARIF=1         # Generate SARIF output for GitHub Security
```

---

## DevOps

### Build System

- **Build Tool**: Make
- **Container**: Docker + Docker Compose

### Key Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the application |
| `make test` | Run tests |
| `make lint` | Run golangci-lint |
| `make format` | Format code |
| `make up` | Start services with Docker Compose |
| `make down` | Stop services |
| `make generate-docs` | Generate Swagger documentation |

### Docker Configuration

- `Dockerfile`: Multi-stage build for production
- `docker-compose.yml`: Local development environment

### Environment Configuration

- `.env.example`: Template for environment variables
- Use `make set-env` to initialize `.env` from template

### Code Quality

#### Linting

Uses `golangci-lint` with configuration in `.golangci.yml`.

Key enabled linters:
- `bodyclose` - Check HTTP response body is closed
- `errchkjson` - Check errors from JSON encoding
- `gocognit` - Cognitive complexity
- `gocyclo` - Cyclomatic complexity (max: 20)
- `misspell` - Spelling checker
- `staticcheck` - Static analysis
- `revive` - Code style

#### Pre-commit Hooks

Git hooks are managed via `.githooks/`:

```bash
make setup-git-hooks  # Install hooks
make check-hooks      # Verify installation
```

### API Documentation

This project uses **swaggo/swag** for OpenAPI/Swagger documentation generation.

**IMPORTANT:** Always use the Makefile to generate documentation:

```bash
make generate-docs
```

**Rules:**
- Documentation is generated in `api/` directory (not `docs/`)
- Never run `swag init` directly - always use `make generate-docs`
- Add swagger annotations to handler functions (see existing handlers for examples)
- Use `swaggertype` and `format` tags for proper type mapping (e.g., `swaggertype:"string" format:"uuid"` for uuid.UUID fields)

**Generated files:**
- `api/docs.go` - Go embed file
- `api/swagger.json` - OpenAPI 3.0 JSON spec
- `api/swagger.yaml` - OpenAPI 3.0 YAML spec
- `api/openapi/openapi.yaml` - Converted OpenAPI YAML

Access documentation at: `http://localhost:4020/swagger/index.html`

### Pre-Dev Documentation

#### Subtask Files

Large subtask files should be split into multiple parts for easier review:

- **Maximum recommended size:** ~800-900 lines per file
- **Naming convention:** `T-XXX-subtasks-partN-description.md`
- **Example:** `T-008-subtasks-part1-types.md`, `T-008-subtasks-part2-services.md`

Each part should include:
- Cross-references to other parts
- Completion checklist
- Estimated time for that part

#### Definition of Done Requirements

Definition of Done (DoD) must align with Success Criteria and include:

- **Explicit acceptance criteria** for each item
- **Test type specification** (unit/integration) for verification items
- **Idempotency verification** for deterministic operations
- **Telemetry verification** with specific span names and attributes

Example DoD item format:
```markdown
- [ ] **Feature flag: defaultDecisionWhenNoMatch** - Environment variable loaded via 
      libCommons.SetConfigFromEnvVars; accepts ALLOW (default) or DENY; unit tests 
      verify both values return correct decision when no rules match; integration 
      test verifies environment override works
```

### Markdown Formatting

Follow these markdown formatting rules (enforced by markdownlint):

- **Indentation:** Use spaces, not tabs (4 spaces per indent level)
- **Headings:** Use proper heading syntax (`##`), not bold text (`**text**`)
- **Code blocks:** Add blank line before and after fenced code blocks
- **Code fence language:** Always specify language (`go`, `bash`, `yaml`)

Example:

    ## Correct Heading

    Some text here.

    ```go
    // Code with language specified
    func example() {}
    ```

    More text after blank line.

### Database Migrations

Migrations are stored in `migrations/` directory as a single numbered sequence (currently 000001 through 000016). There is no function/schema split — all SQL (functions, triggers, tables) lives in the same numbered `.up.sql` / `.down.sql` pairs and is applied in order.

- Naming: `000NNN_descriptive_name.up.sql` / `000NNN_descriptive_name.down.sql`
- Applied via `lib-commons/v4/commons/postgres.Migrator` (wraps `golang-migrate/migrate/v4`)
- Seed data in `migrations/seeds/` (loaded via `make seed`)
- Validate with: `make migrate-version`

**Rollback note.** Rolling back past migration 000016 permanently drops the `schema_migrations_functions` tracker table (its `down.sql` is intentionally empty). Operators who need the legacy tracker back — for any reason — must recreate it manually:

```sql
CREATE TABLE schema_migrations_functions (
    version    BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dirty      BOOLEAN NOT NULL DEFAULT false
);
```

This is documented here rather than restored in the down migration because recreating an empty table would lie about the prior state — worse than leaving it absent.

#### Migration Renumbering Invariant (MANDATORY)

Any migration file that is renamed or renumbered MUST contain SQL that is strictly idempotent:
`CREATE ... IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`, `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`,
`DROP ... IF EXISTS`, or a deterministic `UPDATE` / `INSERT ... ON CONFLICT` pattern.

**Rationale.** golang-migrate tracks applied versions by number in the `schema_migrations` table. Consider a
production database at `schema_migrations.version = N` where the files on disk have been renumbered so that
old versions ≤ N now have different content. When the boot migrator replays the gap between the recorded
version and `max_disk_version`, it will execute the renumbered files it never saw before. A non-idempotent
renumbered migration will either fail (breaking boot) or corrupt state (breaking compliance) in that
upgrade path — precisely the scenario that silently masks tamper evidence on an SOX/GLBA-regulated service
like Tracer.

**Required checklist when a PR renumbers any migration:**

- [ ] Every renumbered `.up.sql` uses `IF NOT EXISTS`, `CREATE OR REPLACE`, `ALTER ... IF NOT EXISTS`,
      `DROP ... IF EXISTS`, or equivalently idempotent constructs.
- [ ] Every renumbered `.down.sql` is also idempotent (rollback must survive replay).
- [ ] The upgrade-path integration test
      (`TestBootstrapAppliesAllMigrations` and, when applicable, an explicit
      `git show origin/develop`-based replay test) passes against a database
      primed with the previous migration sequence.

**Escape hatch (EXCEPTIONAL ONLY — not a default path).** The idempotency requirement above is the
default, and it is the expectation on every PR. The escape hatch applies only when a renumbered
migration is blocked by an irreducibly non-idempotent operation — i.e., one that cannot be expressed
with `IF NOT EXISTS`, `CREATE OR REPLACE`, `DROP ... IF EXISTS`, a `DO $$ ... EXCEPTION WHEN
duplicate_object THEN NULL; END $$` guard, or a deterministic `UPDATE` / `INSERT ... ON CONFLICT`
pattern. Using the escape hatch in place of a mechanically available idempotency guard is a
standards violation, not a trade-off.

When the escape hatch genuinely applies, the PR must include **both**:

1. **Either** (a) a bridge migration (new top-numbered file) that reconciles version state for
   previously-applied databases, **or** (b) a documented per-environment upgrade runbook with
   manual operator steps captured in the PR body; AND
2. Explicit written sign-off on the PR from **both** SRE and the compliance reviewer before merge.

"We ran out of time" and "the existing migration is harder to rewrite than to waive" are **not**
acceptable justifications. Default expectation: idempotency via the mechanisms listed above.

> **Note on salvageable patterns from the previous custom runner.** Directory-traversal safety
> (`os.OpenRoot`) and advisory locking (`pg_advisory_lock`) for migration files are now handled
> by `lib-commons/v4/commons/postgres.Migrator`, which is the single source of truth for the
> PostgreSQL migration path. Any future non-PostgreSQL migrator (e.g. a MongoDB or ClickHouse
> migrator) should reproduce those patterns from the deleted `pkg/migration` runner — see
> the git history on `origin/develop` for the canonical implementation.

### Rule Cache and Background Workers

#### In-Memory Rule Cache (`internal/services/cache/`)

- `RuleCache` stores compiled CEL rules in memory (thread-safe via `sync.RWMutex`)
- `CacheAdapter` wraps `RuleCache` to satisfy `query.ActiveRulesRepository` interface
- `WarmUp()` loads all active rules from DB at startup (30s timeout)
- Command services (activate/deactivate) update cache synchronously via `RuleCacheWriter`
- Health: readiness probe includes cache staleness check

#### RuleSyncWorker (`internal/services/workers/`)

Polls PostgreSQL for rule changes and updates in-memory cache:
- Configurable via `RULE_SYNC_POLL_INTERVAL_SECONDS` (default: 10)
- Staleness threshold: `RULE_SYNC_STALENESS_THRESHOLD_SECONDS` (default: 50)
- Overlap buffer for delta queries: `RULE_SYNC_OVERLAP_BUFFER_SECONDS` (default: 2)
- Circuit breaker (sony/gobreaker) protects against DB failures

#### UsageCleanupWorker (`internal/services/workers/`)

Periodically removes expired usage counters:
- Disabled by default (`CLEANUP_WORKER_ENABLED=false`)
- Configurable interval: `CLEANUP_INTERVAL_HOURS` (default: 24)
- Both workers managed by `libCommons.NewLauncher` for graceful lifecycle

### Clock Abstraction (`pkg/clock/`)

- `clock.Clock` interface with `Now()` method
- `clock.New()` returns `RealClock` (production)
- `clock.NewFixedClock(t)` returns `FixedClock` (testing)
- `MOCK_TIME` env var: read once at server boot for integration tests
  - Allows testing time-dependent features (nighttime PIX limits, Black Friday periods)
  - Cannot be modified via HTTP (prevents timestamp injection)
  - Invalid format falls back to real clock with warning

### Circuit Breaker (`pkg/resilience/`)

- Wrapper around `sony/gobreaker`
- Used by `RuleSyncWorker` for DB poll resilience
- Config: failure threshold, timeout, success threshold
- Fallback: fail-open (ALLOW) with warning flag when circuit open

### Docker Compose Services

| Service | Image | Port |
|---------|-------|------|
| tracer | `tracer:dev` (Dockerfile.dev, Alpine + air live reload) | 4020 |
| tracer-postgres | `postgres:16-alpine` | 5432 |

Both services have healthchecks. Tracer depends on postgres being healthy.

---

## Middleware Order

The order of middleware registration is critical for proper telemetry and logging. Follow this exact order:

```go
func NewRouter(lg libLog.Logger, tl *libOpentelemetry.Telemetry, ...) *fiber.App {
    f := fiber.New(fiber.Config{
        DisableStartupMessage: true,
        ErrorHandler: func(ctx *fiber.Ctx, err error) error {
            return libHTTP.HandleFiberError(ctx, err)
        },
    })

    tlMid := libHTTP.NewTelemetryMiddleware(tl)

    // Middleware order - CRITICAL
    f.Use(tlMid.WithTelemetry(tl))                     // 1. FIRST - injects tracer/logger into context
    f.Use(recover.New())                               // 2. Panic recovery
    f.Use(cors.New())                                  // 3. CORS
    f.Use(otelfiber.Middleware(...))                   // 4. OpenTelemetry metrics
    f.Use(libHTTP.WithHTTPLogging(...))                // 5. HTTP request logging

    // ... define routes ...

    f.Use(tlMid.EndTracingSpans)                       // LAST - closes root spans

    return f
}
```

**Why order matters:**
- `WithTelemetry` must be first to inject tracer/logger into context for all subsequent middleware
- `EndTracingSpans` must be last to properly close spans after response is sent
- Recovery middleware should be early to catch panics from any middleware

---

## Dependencies

### Core Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/LerianStudio/lib-commons/v4` | Common utilities, logging, tracing, PostgreSQL client |
| `github.com/gofiber/fiber/v2` | HTTP framework |
| `google.golang.org/grpc` | gRPC framework (used for OTLP telemetry export) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/Masterminds/squirrel` | SQL query builder |
| `github.com/google/uuid` | UUID generation |

### Testing Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/stretchr/testify` | Test assertions |
| `go.uber.org/mock` | Mock generation tool (gomock) |

### Observability Dependencies

| Dependency | Purpose |
|------------|---------|
| `go.opentelemetry.io/otel` | Distributed tracing (use via lib-commons only) |
| `go.uber.org/zap` | Structured logging (use via lib-commons only) |

---

## Structured Logging

### Overview

All logging in this project **MUST** use structured logging with `WithFields` instead of string interpolation. This ensures logs are searchable, indexable, and consistent across all observability tools.

### Log Field Naming Convention (OpenTelemetry Semantic Conventions)

Field names **MUST** follow [OpenTelemetry Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/general/naming/):

- **Lowercase** names
- **Dot notation** for namespacing (e.g., `rule.id`, `trace.id`)
- **snake_case** for multi-word components within a namespace (e.g., `http.status_code`)

| Field | Correct | Incorrect |
|-------|---------|-----------|
| Rule ID | `rule.id` | `ruleId`, `rule_id`, `id` |
| Rule name | `rule.name` | `ruleName`, `name` |
| Rule status | `rule.status` | `ruleStatus`, `status` |
| Trace ID | `trace.id` | `traceId`, `trace_id` |
| Span ID | `span.id` | `spanId`, `span_id` |
| Error message | `error.message` | `error`, `err`, `errorMessage` |
| Operation | `operation` | `op`, `action` |

### Trace Context Helper

Since `lib-commons` does not automatically inject trace context into loggers, use the helper function in `pkg/logging/trace.go`:

```go
package logging

import (
    "context"

    libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
    "go.opentelemetry.io/otel/trace"
)

// WithTrace enriches a logger with trace context (trace.id and span.id).
// Returns the original logger if no valid span is found in context.
func WithTrace(ctx context.Context, logger libLog.Logger) libLog.Logger {
    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        return logger.WithFields(
            "trace.id", span.SpanContext().TraceID().String(),
            "span.id", span.SpanContext().SpanID().String(),
        )
    }

    return logger
}
```

### Required Logging Pattern

Every handler, service, and repository method **MUST** follow this pattern:

```go
func (h *Handler) CreateRule(c *fiber.Ctx) error {
    ctx := c.UserContext()
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

    ctx, span := tracer.Start(ctx, "handler.rule.create")
    defer span.End()

    // Enrich logger with trace context
    logger = logging.WithTrace(ctx, logger)

    // Log operation start with structured fields
    logger.WithFields(
        "operation", "handler.rule.create",
        "rule.name", input.Name,
    ).Info("Creating rule")

    // ... business logic ...

    // Log success with structured fields
    logger.WithFields(
        "operation", "handler.rule.create",
        "rule.id", result.ID.String(),
        "rule.name", result.Name,
    ).Info("Rule created successfully")

    return libHTTP.Created(c, result)
}
```

### Log Message Guidelines

Log messages **MUST** be descriptive and follow these patterns:

| Event | Message Pattern | Example |
|-------|-----------------|---------|
| Operation start | Present participle | "Creating rule", "Activating rule" |
| Operation success | Past tense + "successfully" | "Rule created successfully", "Rule activated successfully" |
| Operation failure | "Failed to" + verb | "Failed to create rule", "Failed to activate rule" |
| Warning/Skip | Descriptive reason | "Rule already active (idempotent no-op)", "Invalid state transition" |

### Operation Field Convention

The `operation` field **MUST** match the span name for correlation between logs and traces:

| Layer | Operation Value | Span Name |
|-------|-----------------|-----------|
| Handler | `handler.rule.create` | `handler.rule.create` |
| Service | `service.rule.create` | `service.rule.create` |
| Repository | `repository.rule.create` | `repository.rule.create` |

### Error Logging

For errors, include the error message as a structured field:

```go
// Technical error
logger.WithFields(
    "operation", "service.rule.create",
    "error.message", err.Error(),
).Error("Failed to create rule")

// Business error (warning level)
logger.WithFields(
    "operation", "service.rule.activate",
    "rule.id", ruleID.String(),
    "rule.status", rule.Status,
).Warn("Invalid state transition")
```

### Forbidden Logging Patterns

```go
// FORBIDDEN - String interpolation
logger.Infof("Creating rule: name=%s", input.Name)
logger.Errorf("Failed to create rule: %v", err)

// FORBIDDEN - Inconsistent field names
logger.WithFields("ruleId", id).Info("...")      // Use "rule.id"
logger.WithFields("rule_name", name).Info("...")  // Use "rule.name"

// FORBIDDEN - Missing operation field
logger.WithFields("rule.id", id).Info("Rule created")  // Missing "operation"

// FORBIDDEN - Missing trace context
logger.WithFields("operation", "handler.rule.create").Info("...")  // Missing WithTrace()
```

---

## Observability Patterns

### Tracing with lib-commons (Required)

**Always use lib-commons wrappers** for OpenTelemetry operations. Direct imports of `go.opentelemetry.io/otel/*` packages are only allowed for types (e.g., `trace.Span`) when required by lib-commons function signatures.

**Allowed patterns:**

```go
import (
    libCommons "github.com/LerianStudio/lib-commons/v4/commons"
    libOtel "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
    "go.opentelemetry.io/otel/trace"  // OK: Only for trace.Span type
)

// Creating spans
_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
ctx, span := tracer.Start(ctx, "operation.name")
defer span.End()

// Setting span attributes (use SetSpanAttributesFromStruct)
_ = libOtel.SetSpanAttributesFromStruct(&span, "input_data", map[string]any{
    "id":     id,
    "amount": amount,
})

// Handling errors
libOtel.HandleSpanError(&span, "operation failed", err)
libOtel.HandleSpanBusinessErrorEvent(&span, "validation failed", err)
```

**Forbidden in application code:**

```go
// FORBIDDEN - Direct attribute imports
import "go.opentelemetry.io/otel/attribute"
span.SetAttributes(attribute.String("key", value))  // Don't use

// FORBIDDEN - Direct codes imports
import "go.opentelemetry.io/otel/codes"
span.SetStatus(codes.Error, message)  // Don't use
```

**Allowed in test code:**

```go
// OK in tests - SDK imports for setting up test tracer
import (
    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    "go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Setup test tracer
exporter := tracetest.NewInMemoryExporter()
tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
otel.SetTracerProvider(tp)  // Set global provider for lib-commons
```

---

## Forbidden Practices

1. **Direct database access from handlers** - Always go through services
2. **Business logic in repositories** - Repositories are for data access only
3. **Hardcoded configuration** - Use environment variables
4. **Ignoring errors** - All errors must be handled and logged. Never use blank identifier `_` to discard errors:

   ```go
   // WRONG - error ignored
   result, _ := someFunction()
   _ = anotherFunction()
   
   // CORRECT - error handled
   result, err := someFunction()
   if err != nil {
       return fmt.Errorf("someFunction failed: %w", err)
   }
   
   if err := anotherFunction(); err != nil {
       logger.Errorf("anotherFunction failed: %v", err)
       // handle appropriately
   }
   ```

5. **Missing context propagation** - Always pass context through layers
6. **Missing tracing** - All operations must have tracing spans
7. **Tests without mocks** - Service tests must mock dependencies
8. **Cyclomatic complexity > 20** - Refactor complex functions (configured in .golangci.yml)
9. **Synchronous audit writes** - Audit must be async (performance)
10. **External calls during validation** - Use Payload-Complete pattern
11. **Missing circuit breaker** - Database operations need circuit breaker
12. **Blocking on failure** - Fail-open for availability (configurable)
13. **Priority-based rule evaluation** - All rules evaluated, DENY precedence
14. **Direct OTel attribute/codes imports** - Use lib-commons wrappers (SetSpanAttributesFromStruct, HandleSpanError)
15. **Unstructured logging** - Use `WithFields` instead of `Infof`/`Errorf` with string interpolation (see [Structured Logging](#structured-logging))
16. **Task/ticket IDs in code** - Never reference task IDs (T-001, JIRA-123, etc.) in source code, comments, or test names. Code must be self-explanatory without project management context. Use descriptive names instead.

---

## AI Assistant Rules

**CRITICAL - These rules are mandatory for any AI assistant (Droid, Claude, etc.):**

1. **NEVER run `git commit` without explicit user approval** - Always show changes and ask "Posso fazer commit?" before committing
2. **NEVER run `git push` without explicit user approval**
3. **NEVER modify files outside the scope of the requested task**
4. **Always ask for confirmation before destructive operations**
5. **NEVER include test execution results in commit messages** - Do not mention test counts, pass/fail status, or coverage numbers. Commit messages describe *what* changed and *why*, not verification results.
