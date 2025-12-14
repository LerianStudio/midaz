// Package assert provides always-on runtime assertions for detecting programming bugs.
//
// Unlike test assertions, these assertions are intended to remain enabled in production
// code. They are designed for detecting invariant violations, programming errors, and
// impossible states - NOT for input validation or expected error conditions.
//
// # Design Philosophy
//
// Assertions are for catching bugs, not for handling user input:
//
//   - Use assertions for conditions that should NEVER be false if the code is correct
//   - Use error returns for conditions that CAN legitimately fail (I/O, user input, etc.)
//   - Assertions panic because a failed assertion indicates a bug that must be fixed
//
// Good assertion usage:
//
//	assert.NotNil(config, "config must be loaded before server starts")
//	assert.That(len(items) > 0, "processItems called with empty slice")
//	assert.Never("unreachable: switch should handle all enum values")
//
// Bad assertion usage (use error returns instead):
//
//	// DON'T: User input validation
//	assert.That(email != "", "email is required")  // Use validation errors
//
//	// DON'T: I/O that can fail
//	assert.NoError(file.Read())  // Use proper error handling
//
// # Core Assertion Functions
//
// The package provides five core assertion functions:
//
//	assert.That(ok bool, msg string, kv ...any)
//	    Panic if ok is false. General-purpose assertion.
//
//	assert.NotNil(v any, msg string, kv ...any)
//	    Panic if v is nil. Handles both untyped nil and typed nil (nil interface
//	    values with concrete types).
//
//	assert.NotEmpty(s string, msg string, kv ...any)
//	    Panic if s is an empty string.
//
//	assert.NoError(err error, msg string, kv ...any)
//	    Panic if err is not nil. Automatically includes the error in context.
//
//	assert.Never(msg string, kv ...any)
//	    Always panic. Use for unreachable code paths.
//
// # Key-Value Context
//
// All assertion functions accept optional key-value pairs to provide context
// in panic messages:
//
//	assert.That(balance >= 0, "balance must not be negative",
//	    "account_id", accountID,
//	    "balance", balance,
//	)
//
// The panic message will include:
//
//	assertion failed: balance must not be negative
//	    account_id=550e8400-e29b-41d4-a716-446655440000
//	    balance=-100
//	[stack trace]
//
// Odd numbers of key-value arguments are handled gracefully with a "MISSING_VALUE" marker.
//
// # Domain Predicates
//
// The package includes predicate functions for common domain validations:
//
//	// Numeric predicates
//	assert.Positive(n int64) bool        // n > 0
//	assert.NonNegative(n int64) bool     // n >= 0
//	assert.NotZero(n int64) bool         // n != 0
//	assert.InRange(n, min, max int64) bool // min <= n <= max
//
//	// String predicates
//	assert.ValidUUID(s string) bool      // valid UUID format
//
//	// Financial predicates (using shopspring/decimal)
//	assert.ValidAmount(d decimal.Decimal) bool    // exponent in [-18, 18]
//	assert.ValidScale(scale int) bool             // scale in [0, 18]
//	assert.PositiveDecimal(d decimal.Decimal) bool    // d > 0
//	assert.NonNegativeDecimal(d decimal.Decimal) bool // d >= 0
//
// Use predicates with assert.That:
//
//	assert.That(assert.Positive(count), "count must be positive", "count", count)
//	assert.That(assert.ValidUUID(id), "invalid UUID", "id", id)
//
// # Usage Examples
//
// Pre-conditions (validate inputs at function entry):
//
//	func ProcessTransaction(tx *Transaction) error {
//	    assert.NotNil(tx, "transaction must not be nil")
//	    assert.NotEmpty(tx.ID, "transaction must have ID", "tx", tx)
//	    // ... rest of function
//	}
//
// Post-conditions (validate outputs before return):
//
//	func CreateAccount(name string) *Account {
//	    acc := &Account{ID: uuid.New(), Name: name}
//	    // ... creation logic
//	    assert.NotNil(acc.ID, "created account must have ID")
//	    return acc
//	}
//
// Unreachable code:
//
//	switch status {
//	case Active:
//	    return handleActive()
//	case Inactive:
//	    return handleInactive()
//	case Deleted:
//	    return handleDeleted()
//	default:
//	    assert.Never("unhandled status", "status", status)
//	    return nil // unreachable, but satisfies compiler
//	}
//
// # Integration with mruntime
//
// Assertion panics are designed to be caught by mruntime recovery boundaries.
// In production, mruntime.SafeGo and HTTP middleware will:
//
//   - Log the full panic message including context and stack trace
//   - Apply the configured panic policy (KeepRunning or CrashProcess)
//   - Prevent process crashes while preserving debuggability
//
// This means assertion failures will be visible in logs and monitoring,
// allowing bugs to be identified and fixed without bringing down the service.
package assert
