# pkg/assert - Domain Invariant Assertions

**Location**: `pkg/assert/`
**Priority**: ⚠️ CRITICAL - Referenced in CLAUDE.md Critical Rules
**Status**: Production-safe, always-on assertions

Production-safe assertions for detecting programming bugs. Unlike test assertions, these remain enabled in production and panic when invariants are violated.

## Design Philosophy

**Use assertions for bugs** - Conditions that should NEVER be false if code is correct
**Use error returns for user errors** - Conditions that CAN legitimately fail (I/O, user input)
**Assertions panic** - Because a failed assertion indicates a bug that must be fixed

### When to Use assert vs error

```go
// ✅ GOOD - Use assert for programming bugs
func ProcessTransaction(tx *Transaction) error {
    assert.NotNil(tx, "transaction must not be nil")  // BUG if nil
    assert.That(tx.Amount > 0, "amount must be positive", "amount", tx.Amount)  // BUG if ≤0

    // ✅ GOOD - Use error return for user/system failures
    _, err := db.Query(...)
    if err != nil {
        return fmt.Errorf("database query failed: %w", err)  // Expected failure
    }
}

// ❌ BAD - Don't use assert for user input validation
func CreateAccount(name string) error {
    assert.NotEmpty(name, "name required")  // WRONG - user input can be empty
    // Instead: if name == "" { return ValidationError{...} }
}

// ❌ BAD - Don't use assert for I/O that can fail
func LoadConfig() {
    assert.NoError(os.ReadFile("config.json"), "config must exist")  // WRONG - file might not exist
    // Instead: if err != nil { return fmt.Errorf("failed to read config: %w", err) }
}
```

## Core Functions

### assert.That

General-purpose assertion - panic if condition is false.

```go
func That(ok bool, msg string, kv ...any)
```

**Usage:**
```go
assert.That(len(items) > 0, "items must not be empty", "count", len(items))
assert.That(balance >= 0, "balance must not be negative",
    "account_id", accountID,
    "balance", balance)
assert.That(assert.Positive(count), "count must be positive", "count", count)
```

### assert.NotNil

Panic if value is nil. Handles both untyped nil and typed nil (nil interface with concrete type).

```go
func NotNil(v any, msg string, kv ...any)
```

**Usage:**
```go
assert.NotNil(config, "config must be initialized")
assert.NotNil(handler, "handler must not be nil", "name", handlerName)
assert.NotNil(db, "database handle must not be nil", "repository", "AccountRepository")
```

**Why it handles typed nil:**
```go
var ptr *Account = nil
var iface any = ptr

// Standard Go nil check would MISS this:
if iface == nil {  // FALSE - iface is not nil, it's (*Account)(nil)
    panic("should detect but doesn't")
}

// assert.NotNil CATCHES this:
assert.NotNil(iface, "must catch typed nil")  // ✅ Panics correctly
```

### assert.NotEmpty

Panic if string is empty.

```go
func NotEmpty(s string, msg string, kv ...any)
```

**Usage:**
```go
assert.NotEmpty(userID, "userID must be provided")
assert.NotEmpty(tx.ID, "transaction must have ID", "tx", tx)
```

### assert.NoError

Panic if error is not nil. Automatically includes error message and type in context.

```go
func NoError(err error, msg string, kv ...any)
```

**Usage:**
```go
result, err := compute()
assert.NoError(err, "compute must succeed", "input", input)

db, err := r.connection.GetDB()
assert.NoError(err, "database connection required for repository",
    "repository", "AccountPostgreSQLRepository")
```

**Panic output includes:**
```
assertion failed: compute must succeed
    error=computation failed: division by zero
    error_type=*errors.errorString
    input=42
[stack trace]
```

### assert.Never

Always panic. Use for code paths that should be unreachable.

```go
func Never(msg string, kv ...any)
```

**Usage:**
```go
switch status {
case Active:
    return handleActive()
case Inactive:
    return handleInactive()
case Deleted:
    return handleDeleted()
default:
    assert.Never("unhandled status", "status", status)
    return nil  // unreachable, but satisfies compiler
}
```

## Domain Predicates

Predicates for common domain validations. Use with `assert.That()`.

### Numeric Predicates

```go
// Returns true if n > 0
func Positive(n int64) bool

// Returns true if n >= 0
func NonNegative(n int64) bool

// Returns true if n != 0
func NotZero(n int64) bool

// Returns true if min <= n <= max
// Note: Returns false if min > max (fail-safe behavior)
func InRange(n, minVal, maxVal int64) bool
```

**Usage:**
```go
assert.That(assert.Positive(count), "count must be positive", "count", count)
assert.That(assert.NonNegative(balance), "balance must not be negative", "balance", balance)
assert.That(assert.NotZero(divisor), "divisor must not be zero", "divisor", divisor)
assert.That(assert.InRange(page, 1, 1000), "page out of range", "page", page)
```

### String Predicates

```go
// Returns true if s is a valid UUID string (canonical or non-canonical format)
// Returns false if s is empty
func ValidUUID(s string) bool
```

**Usage:**
```go
assert.That(assert.ValidUUID(id), "invalid UUID format", "id", id)
```

### Financial Predicates (shopspring/decimal)

```go
// Returns true if decimal's exponent is in range [-18, 18]
func ValidAmount(amount decimal.Decimal) bool

// Returns true if scale is in range [0, 18]
func ValidScale(scale int) bool

// Returns true if amount > 0
func PositiveDecimal(amount decimal.Decimal) bool

// Returns true if amount >= 0
func NonNegativeDecimal(amount decimal.Decimal) bool
```

**Usage:**
```go
assert.That(assert.ValidAmount(amount), "amount has invalid precision", "amount", amount)
assert.That(assert.ValidScale(scale), "invalid scale", "scale", scale)
assert.That(assert.PositiveDecimal(price), "price must be positive", "price", price)
assert.That(assert.NonNegativeDecimal(balance), "balance must not be negative", "balance", balance)
```

## Usage Patterns

### Pre-conditions (validate inputs at function entry)

```go
func ProcessTransaction(tx *Transaction) error {
    // Validate inputs - these are programming bugs if violated
    assert.NotNil(tx, "transaction must not be nil")
    assert.NotEmpty(tx.ID, "transaction must have ID", "tx", tx)
    assert.That(assert.PositiveDecimal(tx.Amount), "amount must be positive",
        "amount", tx.Amount)

    // ... rest of function
}
```

### Post-conditions (validate outputs before return)

```go
func CreateAccount(name string) *Account {
    acc := &Account{ID: uuid.New(), Name: name}

    // ... creation logic

    // Validate result - BUG if these fail
    assert.NotNil(acc, "created account must not be nil")
    assert.NotEmpty(acc.ID.String(), "created account must have ID")
    assert.NotEmpty(acc.Name, "created account must have name")

    return acc
}
```

### Unreachable Code

```go
switch status {
case Active:
    return handleActive()
case Inactive:
    return handleInactive()
case Deleted:
    return handleDeleted()
default:
    assert.Never("unhandled status", "status", status)
    return nil  // unreachable, but satisfies compiler
}
```

### Repository Constructor Validation

```go
func NewAccountRepository(pc *libPostgres.PostgresConnection) *AccountRepository {
    // Validate constructor arguments
    assert.NotNil(pc, "PostgreSQL connection must not be nil",
        "repository", "AccountRepository")

    r := &AccountRepository{connection: pc, tableName: "account"}

    // Verify connection is working
    db, err := r.connection.GetDB()
    assert.NoError(err, "database connection required for AccountRepository",
        "repository", "AccountRepository")
    assert.NotNil(db, "database handle must not be nil",
        "repository", "AccountRepository")

    return r
}
```

### Complex Validation with Multiple Conditions

```go
func ValidateBalanceUpdate(acc *Account, amount decimal.Decimal) {
    assert.NotNil(acc, "account must not be nil")
    assert.NotEmpty(acc.ID.String(), "account must have ID")
    assert.That(assert.ValidAmount(amount), "invalid amount precision", "amount", amount)

    newBalance := acc.Balance.Add(amount)
    assert.That(assert.NonNegativeDecimal(newBalance),
        "balance update would result in negative balance",
        "account_id", acc.ID,
        "current_balance", acc.Balance,
        "amount", amount,
        "new_balance", newBalance)
}
```

## Key Features

### Value Truncation

Values longer than 200 characters are automatically truncated in panic messages to prevent log bloat:

```go
assert.That(false, "failed", "long_value", strings.Repeat("x", 500))

// Output:
// assertion failed: failed
//     long_value=xxxxxxxxxxxx... (truncated 300 chars)
// [stack trace]
```

### Rich Context with Key-Value Pairs

All functions accept optional key-value pairs for debugging context:

```go
assert.That(balance >= 0, "balance must not be negative",
    "account_id", accountID,           // Include account
    "balance", balance,                 // Include actual value
    "operation", "debit",              // Include operation type
    "amount", amount)                  // Include transaction amount

// Panic output:
// assertion failed: balance must not be negative
//     account_id=550e8400-e29b-41d4-a716-446655440000
//     balance=-100
//     operation=debit
//     amount=150
// [stack trace]
```

### Stack Traces

All panics include full stack trace using `runtime/debug.Stack()`:

```
assertion failed: transaction must not be nil
    handler=CreateTransaction
    request_id=abc-123

stack trace:
goroutine 1 [running]:
runtime/debug.Stack()
    /usr/local/go/src/runtime/debug/stack.go:24 +0x64
github.com/LerianStudio/midaz/v3/pkg/assert.panicWithContext(...)
    /Users/.../pkg/assert/assert.go:97
github.com/LerianStudio/midaz/v3/pkg/assert.NotNil(...)
    /Users/.../pkg/assert/assert.go:28
...
```

### Integration with mruntime

Assertion panics are designed to be caught by `mruntime` recovery boundaries:

```go
func (h *TransactionHandler) CreateTransaction(c *fiber.Ctx) error {
    ctx := c.UserContext()
    defer mruntime.RecoverAndLogWithContext(ctx, logger, "transaction", "handler.create")

    // If assertion panics here, mruntime catches it:
    // - Logs full panic message with context and stack trace
    // - Applies configured panic policy (KeepRunning or CrashProcess)
    // - Converts to 500 Internal Server Error for HTTP
    assert.NotNil(tx, "transaction must not be nil")

    // ... handler logic
}
```

See [`mruntime.md`](./mruntime.md) for panic recovery patterns.

## Anti-Patterns

### ❌ Don't Use for User Input Validation

```go
// ❌ BAD - User input can be invalid
func CreateAccount(name string) error {
    assert.NotEmpty(name, "name required")  // WRONG
    // User might not provide name - this is an expected error, not a bug
}

// ✅ GOOD - Return validation error
func CreateAccount(name string) error {
    if name == "" {
        return pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "account")
    }
}
```

### ❌ Don't Use for I/O Operations

```go
// ❌ BAD - Files/network can fail
func LoadConfig() {
    data, err := os.ReadFile("config.json")
    assert.NoError(err, "config must exist")  // WRONG
    // File might not exist - this is an expected failure
}

// ✅ GOOD - Return error
func LoadConfig() error {
    data, err := os.ReadFile("config.json")
    if err != nil {
        return fmt.Errorf("failed to read config: %w", err)
    }
}
```

### ❌ Don't Use for Business Logic Validation

```go
// ❌ BAD - Business rules can be violated by users
func Transfer(from, to *Account, amount decimal.Decimal) {
    assert.That(from.Balance >= amount, "sufficient balance required")  // WRONG
    // User might not have sufficient balance - this is a business error
}

// ✅ GOOD - Return business error
func Transfer(from, to *Account, amount decimal.Decimal) error {
    if from.Balance.LessThan(amount) {
        return pkg.ValidateBusinessError(constant.ErrInsufficientAccountBalance,
            "transfer", from.ID)
    }
}
```

## Performance Considerations

- **Zero overhead when conditions pass** - Only evaluates message/context on panic
- **No runtime flag** - Always enabled in production (by design)
- **Value truncation** - Prevents log bloat from large values
- **Stack trace capture** - Minimal overhead, only on panic

### Additional Predicates

#### Network & Configuration
```go
func ValidPort(port string) bool      // Validates network port (1-65535)
func ValidSSLMode(mode string) bool   // Validates PostgreSQL SSL modes
```

#### Integer Variants
```go
func PositiveInt(n int) bool                      // int variant of Positive
func InRangeInt(n, minVal, maxVal int) bool       // int variant of InRange
```

#### Financial Transaction Predicates
```go
func DebitsEqualCredits(debits, credits decimal.Decimal) bool  // Double-entry validation
func NonZeroTotals(debits, credits decimal.Decimal) bool       // Transaction totals non-zero
func BalanceSufficientForRelease(onHold, releaseAmount decimal.Decimal) bool
func BalanceIsZero(available, onHold decimal.Decimal) bool
```

#### Transaction State Predicates
```go
func ValidTransactionStatus(status string) bool
func TransactionCanTransitionTo(current, target string) bool
func TransactionCanBeReverted(status string, hasParent bool) bool
func TransactionHasOperations[T any](operations []*T) bool  // Generic
```

#### Date/Time Predicates
```go
func DateNotInFuture(t time.Time) bool
func DateAfter(date, reference time.Time) bool
```

#### String Predicates
```go
func NotEmptyString(s string) bool  // Non-whitespace validation
```

## Testing with Assertions

In tests, assertion panics can be caught and verified:

```go
func TestCreateAccount_NilInput(t *testing.T) {
    // Expect panic on nil input
    defer func() {
        r := recover()
        assert.NotNil(t, r, "should panic on nil input")

        panicMsg := fmt.Sprintf("%v", r)
        assert.Contains(t, panicMsg, "account must not be nil")
    }()

    // This should panic
    CreateAccount(nil)
}
```

## References

- **Source**: `pkg/assert/assert.go:1`, `pkg/assert/predicates.go:1`
- **Documentation**: `pkg/assert/doc.go:1`
- **Tests**: `pkg/assert/assert_test.go:1`
- **Benchmarks**: `pkg/assert/benchmark_test.go:1`
- **Related**: [`mruntime.md`](./mruntime.md) for panic recovery
- **Related**: [`errors.md`](./errors.md) for business error handling

## Summary

`pkg/assert` is your first line of defense against programming bugs:

1. **Use liberally** in pre-conditions, post-conditions, and invariants
2. **Never use** for user input or expected failures
3. **Always provide context** with key-value pairs
4. **Trust the system** - panics are caught by mruntime in production
5. **Think fail-fast** - Better to panic early than corrupt data later

The combination of always-on assertions + panic recovery (`mruntime`) = observable, debuggable bugs without process crashes.
