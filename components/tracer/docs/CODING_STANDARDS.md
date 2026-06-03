# Coding Standards - Tracer Project

**Language Policy:** All code, comments, documentation, commit messages, and PR descriptions MUST be written in English.

This document consolidates best practices identified through code reviews to ensure consistency and quality.

---

## Golden Rules

1. **Validate Before Mutate** - Methods returning `error` MUST validate before mutating
2. **Domain Logic in Domain** - Business logic belongs in domain models, not services
3. **Deterministic Tests** - Use `testutil.FixedTime()`, never `time.Now()` in tests
4. **Error Chain Preservation** - Always use `%w`, never `%v` for errors
5. **Normalize-Validate-Store** - In that order, store the normalized value

---

## 1. Domain Model Invariants

### Always-Valid Objects

Domain objects MUST be in valid state after construction.

```go
// ✅ CORRECT
func NewRule(name, expression string, action Decision, scopes []Scope, description *string, createdAt time.Time) (*Rule, error) {
    // 1. Normalize
    normalizedName := strings.TrimSpace(name)
    
    // 2. Validate EVERYTHING
    if normalizedName == "" {
        return nil, ErrRuleNameRequired
    }
    if !action.IsValid() {
        return nil, ErrInvalidDecision
    }
    
    // 3. Create valid object
    return &Rule{
        Name:      normalizedName,
        Action:    action,
        CreatedAt: createdAt,
    }, nil
}
```

### Validate Before Mutate (Atomicity)

```go
// ✅ CORRECT - Atomic validation
func (r *Rule) Update(name *string, scopes *[]Scope) error {
    // 1. VALIDATE first
    if scopes != nil {
        for _, scope := range *scopes {
            if scope.IsEmpty() {
                return ErrRuleInvalidScope  // Returns WITHOUT mutating
            }
        }
    }
    
    // 2. MUTATE only after validation passes
    if name != nil {
        r.Name = *name
    }
    if scopes != nil {
        r.Scopes = append([]Scope{}, *scopes...)
    }
    
    return nil
}

// ❌ INCORRECT - Mutates before validating
func (r *Request) BadUpdate() error {
    r.Field = newValue      // ← ALREADY MUTATED
    return r.Validate()     // ← If fails, state is inconsistent!
}
```

### Defensive Copy Consistency

**CRITICAL RULE:** Constructor and Update methods MUST have consistent defensive copy semantics.

```go
// ✅ CORRECT - NewRule deep copies UUID pointers
func NewRule(..., scopes []Scope) (*Rule, error) {
    scopesCopy := make([]Scope, 0, len(scopes))
    for _, scope := range scopes {
        scopeCopy := scope
        if scope.AccountID != nil {
            accountIDCopy := *scope.AccountID
            scopeCopy.AccountID = &accountIDCopy
        }
        // ... same for SegmentID, PortfolioID
        scopesCopy = append(scopesCopy, scopeCopy)
    }
    return &Rule{Scopes: scopesCopy}, nil
}

// ✅ CORRECT - Update() MUST use same deep copy pattern
func (r *Rule) Update(..., scopes *[]Scope, now time.Time) error {
    if scopes != nil {
        // Same defensive deep copy as NewRule!
        scopesCopy := make([]Scope, 0, len(*scopes))
        for _, scope := range *scopes {
            scopeCopy := scope
            if scope.AccountID != nil {
                accountIDCopy := *scope.AccountID
                scopeCopy.AccountID = &accountIDCopy
            }
            // ... same for SegmentID, PortfolioID
            scopesCopy = append(scopesCopy, scopeCopy)
        }
        r.Scopes = scopesCopy
    }
    return nil
}

// ❌ INCORRECT - Update() shallow copy (inconsistent with NewRule!)
func (r *Rule) Update(..., scopes *[]Scope) error {
    if scopes != nil {
        r.Scopes = append([]Scope{}, *scopes...)  // SHALLOW - UUID pointers shared!
    }
    return nil
}
```

**Why:** External mutations to pointer fields corrupt rule state if only shallow copied.

### Clock Injection in Domain Methods

**CRITICAL RULE:** Domain methods that set timestamps MUST accept `now time.Time` parameter.

```go
// ✅ CORRECT - Inject time for testability
func (r *Rule) SetAction(action Decision, now time.Time) error {
    r.Action = action
    r.UpdatedAt = now  // ← Injected
    return nil
}

func (r *Rule) Update(..., now time.Time) error {
    if updated {
        r.UpdatedAt = now  // ← Consistent with SetAction
    }
    return nil
}

// ❌ INCORRECT - Using time.Now() directly (non-testable!)
func (r *Rule) Update(...) error {
    if updated {
        r.UpdatedAt = time.Now().UTC()  // ← Cannot control in tests!
    }
    return nil
}
```

**Service layer pattern:**
```go
// Service passes clock
if err := rule.Update(name, expr, desc, scopes, c.clock.Now()); err != nil {
    return err
}
```

**Why:** Deterministic timestamps in tests, consistent dependency injection pattern.

---

## 2. Error Handling

### Always Use %w for Error Wrapping

```go
// ✅ CORRECT - Preserves error chain
return fmt.Errorf("failed to build activation: %w", err)
return fmt.Errorf("%w: description: %w", sentinel, err)

// ❌ INCORRECT - Loses error chain (breaks errors.Is/errors.As)
return fmt.Errorf("failed to build activation: %v", err)
return fmt.Errorf("%w: description: %v", sentinel, err)
```

### Error Detection: errors.Is vs errors.As

```go
// ✅ CORRECT - Use errors.Is for sentinel errors
if errors.Is(err, constant.ErrRuleInvalidStatus) {
    // Handle sentinel error
}

// ✅ CORRECT - Use errors.As for typed errors
var transitionErr *model.InvalidTransitionError
if errors.As(err, &transitionErr) {
    // Access transitionErr.From and transitionErr.To
    log.Warn("Invalid transition from %s to %s", transitionErr.From, transitionErr.To)
}

// ❌ INCORRECT - Using errors.Is for typed errors (doesn't work!)
if errors.Is(err, constant.ErrRuleInvalidStatus) {
    // This won't catch InvalidTransitionError which wraps different sentinel
    businessErr := model.NewInvalidTransitionError(...)  // Won't be reached!
}
```

**Rule:** Use `errors.Is` for sentinel errors, `errors.As` for typed errors with fields.

### Context Propagation

```go
// ✅ CORRECT - Captures context for child spans
func (a *Adapter) Method(ctx context.Context) error {
    ctx, span := tracer.Start(ctx, "operation")  // ← Capture ctx
    defer span.End()
    return a.dependency.Call(ctx)
}

// ❌ INCORRECT - Discards context
func (a *Adapter) Method(ctx context.Context) error {
    _, span := tracer.Start(ctx, "operation")  // ← Lost!
    defer span.End()
    return a.dependency.Call(ctx)
}
```

### Trace Context Initialization Order

**CRITICAL RULE:** Create span BEFORE enriching logger with trace context.

```go
// ✅ CORRECT - Span created before logger enrichment
func (a *Adapter) Method(ctx context.Context) error {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    
    // 1. Create span first and capture updated context
    ctx, span := tracer.Start(ctx, "operation")
    defer span.End()
    
    // 2. Enrich logger with trace context (now span exists!)
    logger = logging.WithTrace(ctx, logger)
    
    logger.Info("operation started")  // ← Includes trace_id and span_id
    return nil
}

// ❌ INCORRECT - Logger enrichment before span creation
func (a *Adapter) Method(ctx context.Context) error {
    logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
    
    // WithTrace called BEFORE span exists
    logger = logging.WithTrace(ctx, logger)  // ← trace.SpanFromContext(ctx) returns nil!
    
    ctx, span := tracer.Start(ctx, "operation")
    defer span.End()
    
    logger.Info("operation started")  // ← Missing trace_id and span_id!
    return nil
}
```

**Why this matters:**
- `logging.WithTrace(ctx, logger)` calls `trace.SpanFromContext(ctx)` internally
- If span doesn't exist in context yet, returns nil
- Logger entries miss trace_id/span_id → broken log-trace correlation
- Always: `tracer.Start()` → capture ctx → `logging.WithTrace(updatedCtx, logger)`

**Pattern for methods using both tracing and logging:**
1. Get logger and tracer from context
2. Start span and capture updated context
3. Defer span.End()
4. Enrich logger with updated context containing span
5. Use enriched logger for all log entries

---

## 3. Testing Standards

### Deterministic Tests

```go
// ✅ CORRECT - Deterministic
import "tracer/internal/testutil"

fixedTime := testutil.FixedTime()
validID := testutil.MustDeterministicUUID(1)
rule := NewRule(..., fixedTime)
assert.Equal(t, fixedTime, rule.CreatedAt)

// ❌ INCORRECT - Non-deterministic (flaky!)
rule := NewRule(...)
assert.WithinDuration(t, time.Now(), rule.CreatedAt, 1*time.Second)
```

**NEVER use in tests:**
- `time.Now()` - Use `testutil.FixedTime()` instead
- `uuid.New()` - Use `testutil.MustDeterministicUUID(seed)` instead
- `rand.Intn()` - Use fixed seeds with `rand.New(rand.NewSource(42))`
- `time.Sleep()` - Use mock clocks or polling with timeouts

**ALWAYS use:**
- `testutil.FixedTime()` - Returns consistent 2024-01-01T00:00:00Z
- `testutil.MustDeterministicUUID(seed)` - Returns same UUID for same seed
- `testutil.NewDefaultMockClock()` - Mock clock for time-dependent logic

**CRITICAL: Test Helpers Must Be Deterministic**

```go
// ❌ INCORRECT - Test helper with non-deterministic values
func createTestRequest() *ValidationRequest {
    return &ValidationRequest{
        RequestID:            uuid.New(),   // FORBIDDEN
        TransactionTimestamp: time.Now(),  // FORBIDDEN
        Account: AccountContext{
            ID: uuid.New(),  // FORBIDDEN
        },
    }
}

// ✅ CORRECT - Deterministic test helper
func createTestRequest() *ValidationRequest {
    return &ValidationRequest{
        RequestID:            testutil.MustDeterministicUUID(1),
        TransactionTimestamp: testutil.FixedTime(),
        Account: AccountContext{
            ID: testutil.MustDeterministicUUID(2),
        },
    }
}
```

**Why This Matters:**
- Flaky tests waste time and erode trust
- Non-deterministic failures are impossible to reproduce
- CI/CD builds should be deterministic
- Test failures should always be investigable

### Test Parallelization

```go
package model

func TestNewRule(t *testing.T) {
    t.Parallel()  // ← Top-level
    
    tests := []struct {
        name string
        // ...
    }{
        // test cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // ← Subtest
            // test code
        })
    }
}
```

---

## 4. Encapsulation & DDD

### Domain Logic Location

```text
Command/Service Layer:
  ✓ Orchestration between aggregates
  ✓ Transaction management
  ✗ Business validation
  ✗ Direct field mutation

Domain Model:
  ✓ Business validation
  ✓ State transition rules
  ✓ Encapsulated mutation
  ✗ Repository access
  ✗ Transactions
```

### Tell, Don't Ask

```go
// ✅ CORRECT - Tell (domain knows its rules)
if err := rule.SetAction(input.Action, c.clock.Now()); err != nil {
    return err
}

// ❌ INCORRECT - Ask (service knows domain rules)
if !input.Action.IsValid() {
    return ErrInvalidDecision
}
rule.Action = input.Action  // Direct mutation
rule.UpdatedAt = c.clock.Now()
```

### Complete Constructors

```go
// ✅ CORRECT - Complete constructor
func NewRule(name, expression string, action Decision, createdAt time.Time) (*Rule, error) {
    return &Rule{
        Name:      name,
        CreatedAt: createdAt,  // ← Timestamp in constructor
        UpdatedAt: createdAt,
    }, nil
}

// Command passes timestamp
now := c.clock.Now()
rule, err := model.NewRule(name, expr, action, now)

// ❌ INCORRECT - Incomplete + external mutation
func NewRule(name, expression string) (*Rule, error) {
    return &Rule{Name: name}, nil  // Missing timestamps
}
rule.CreatedAt = c.clock.Now()  // ← Breaks encapsulation
```

---

## 5. Normalization & Validation

### Normalize-Validate-Store Pattern

```go
// ✅ CORRECT - Complete pattern
func NewAuditEvent(resourceID string, actor Actor) (*AuditEvent, error) {
    // 1. Normalize
    normalizedResourceID := strings.TrimSpace(resourceID)
    normalizedActorID := strings.TrimSpace(actor.ID)
    
    // 2. Validate normalized
    if normalizedResourceID == "" {
        return nil, ErrRequired
    }
    
    // 3. Store normalized
    return &AuditEvent{
        ResourceID: normalizedResourceID,  // ← Normalized
        Actor:      Actor{ID: normalizedActorID},
    }, nil
}

// ❌ INCORRECT - Validates normalized but stores original
func NewAuditEvent_BAD(resourceID string) (*AuditEvent, error) {
    if strings.TrimSpace(resourceID) == "" {  // Validates trimmed
        return nil, ErrRequired
    }
    return &AuditEvent{
        ResourceID: resourceID,  // ← Stores original (with spaces!)
    }, nil
}
```

### Documentation Clarity

```go
// ✅ CORRECT - Clear documentation
// NewValidationRequest creates a ValidationRequest with normalization.
// Currency is converted to uppercase ISO 4217.
// SubType is trimmed of whitespace.
// Metadata is shallow-copied (top-level keys only).
func NewValidationRequest(...) (*ValidationRequest, error)

// Validate checks that ValidationRequest fields are valid.
// Does NOT normalize - validates fields as-is.
// Currency must already be uppercase ISO 4217.
func (r *ValidationRequest) Validate() error
```

---

## 6. Code Organization

### Error Constants - Numerical Order

```go
// ✅ CORRECT - Numerical order
const (
    // TRC-0120 to TRC-0139: Limit Errors
    ErrLimitNotFound = errors.New("TRC-0120")
    
    // TRC-0140 to TRC-0159: Audit Event Errors
    ErrAuditEventNotFound = errors.New("TRC-0140")
    
    // TRC-0160 to TRC-0179: UsageCounter Errors
    ErrUsageCounterNotFound = errors.New("TRC-0160")
)
```

### Test Helpers - Centralization

```go
// ✅ CORRECT - Centralized in internal/testutil/
import "tracer/internal/testutil"

validID := testutil.MustDeterministicUUID(1)
ptrID := testutil.UUIDPtr(validID)

// ❌ INCORRECT - Duplicated in each package
// pkg/model/helpers_test.go
func UUIDPtr(id uuid.UUID) *uuid.UUID { ... }

// internal/services/helpers_test.go
func UUIDPtr(id uuid.UUID) *uuid.UUID { ... }  // Duplicate!
```

---

## 7. Review Checklist

### For Code Authors (Before Creating PR)

- [ ] Methods validate before mutating?
- [ ] Tests use `testutil.FixedTime()` not `time.Now()`?
- [ ] Error wrapping uses `%w` not `%v`?
- [ ] `tracer.Start()` returns captured `ctx`?
- [ ] Business logic in domain (not service)?
- [ ] Normalized values are stored?
- [ ] Comments reflect actual behavior?

### For Code Reviewers

- [ ] Objects are always valid after construction?
- [ ] No partial mutation on failures?
- [ ] Tests pass 3x consecutively (determinism)?
- [ ] Service layer doesn't mutate fields directly?
- [ ] `errors.Is()` works with wrapped errors?
- [ ] Validation order prevents inconsistent state?
- [ ] Tests use `t.Parallel()`?
- [ ] Test data generation ensures uniqueness?

---

## 8. Automated Enforcement

### golangci-lint Configuration

See `.golangci.yml` for automated checks:
- `errorlint`: Enforces `%w` usage
- `contextcheck`: Verifies context propagation
- `govet`: Shadow detection
- `revive`: Best practices

### CI/CD Checks

- **Determinism:** Run tests 3x consecutively to verify reproducibility
- **Error wrapping:** Check for `%v` with errors (should use `%w`)

---

## 9. Language Policy

**CRITICAL:** All project artifacts MUST be in English:

### MUST be in English:
- ✅ Code (variables, functions, types, constants)
- ✅ Comments (inline, block, doc comments)
- ✅ Commit messages
- ✅ PR titles and descriptions
- ✅ Documentation (README, guides, standards)
- ✅ Error messages
- ✅ Log messages
- ✅ Test names and descriptions
- ✅ TODOs and FIXMEs

### Examples:

```go
// ✅ CORRECT - English
// NewRule creates a new validation rule with normalization.
// Returns error if name is empty or action is invalid.
func NewRule(name string, action Decision) (*Rule, error) {
    if name == "" {
        return nil, errors.New("name is required")
    }
    return &Rule{Name: name, Action: action}, nil
}

// ❌ INCORRECT - Portuguese
// NewRule cria uma nova regra com normalização.
// Retorna erro se nome está vazio ou ação é inválida.
func NewRule(nome string, acao Decision) (*Rule, error) {
    if nome == "" {
        return nil, errors.New("nome é obrigatório")
    }
    return &Rule{Name: nome, Action: acao}, nil
}
```

```bash
# ✅ CORRECT - English commit
git commit -m "feat: add validation for empty scopes in Rule.Update

Validate that scopes are not empty before mutating rule state.
Returns ErrRuleInvalidScope if any scope has all nil fields.

Tests: 7 new test cases covering edge cases and atomicity."

# ❌ INCORRECT - Portuguese commit
git commit -m "feat: adiciona validação de scopes vazios

Valida que scopes não estão vazios antes de mutar o estado.
Retorna ErrRuleInvalidScope se algum scope tem todos campos nil."
```

### Enforcement:

**Pre-commit hook** (optional but recommended):
```bash
#!/bin/bash
# Check for non-ASCII characters in staged files
if git diff --cached | grep -P '[^\x00-\x7F]'; then
    echo "❌ Non-ASCII characters found. Use English only."
    exit 1
fi
```

**Code review:** Reviewers MUST reject PRs with non-English text.

---

## References

- Go Error Handling: https://go.dev/blog/go1.13-errors
- Go Testing: https://go.dev/doc/tutorial/add-a-test
- Domain-Driven Design: https://martinfowler.com/bliki/DomainDrivenDesign.html
- OpenTelemetry Go: https://opentelemetry.io/docs/instrumentation/go/

---

**Last Updated:** 2026-02-04  
**Version:** 1.0  
**Maintained by:** Engineering Team
