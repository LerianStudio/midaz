//go:build chaos

package chaos

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertRecoveryWithin asserts that a check function succeeds within the given timeout.
// This is useful for verifying that a service recovers after chaos injection.
func AssertRecoveryWithin(t *testing.T, check func() error, timeout time.Duration, msgAndArgs ...interface{}) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			msg := fmt.Sprintf("service did not recover within %v", timeout)
			if lastErr != nil {
				msg = fmt.Sprintf("%s: last error: %v", msg, lastErr)
			}
			require.Fail(t, msg, msgAndArgs...)
			return
		case <-ticker.C:
			if err := check(); err == nil {
				return // Success
			} else {
				lastErr = err
			}
		}
	}
}

// AssertNoDataLoss asserts that data is preserved after chaos.
// It compares the result of a query function before and after chaos.
func AssertNoDataLoss[T comparable](t *testing.T, before, after T, msgAndArgs ...interface{}) {
	t.Helper()
	assert.Equal(t, before, after, msgAndArgs...)
}

// AssertDataIntegrity runs a custom integrity check function and fails if it returns an error.
func AssertDataIntegrity(t *testing.T, check func() error, msgAndArgs ...interface{}) {
	t.Helper()
	err := check()
	require.NoError(t, err, msgAndArgs...)
}

// AssertEventuallyConsistent asserts that a condition becomes true within the timeout.
// Unlike AssertRecoveryWithin, this accepts a boolean function.
func AssertEventuallyConsistent(t *testing.T, condition func() bool, timeout time.Duration, msgAndArgs ...interface{}) {
	t.Helper()
	require.Eventually(t, condition, timeout, 100*time.Millisecond, msgAndArgs...)
}

// AssertGracefulDegradation asserts that operations fail gracefully during chaos.
// It expects the operation to return an error (not panic) and optionally validates the error.
func AssertGracefulDegradation(t *testing.T, operation func() error, validateError func(error) bool, msgAndArgs ...interface{}) {
	t.Helper()

	err := operation()
	require.Error(t, err, msgAndArgs...)

	if validateError != nil {
		assert.True(t, validateError(err), "error should match expected type: %v", err)
	}
}

// AssertNoNegativeBalance is a domain-specific assertion for financial systems.
// It checks that a balance value is non-negative.
func AssertNoNegativeBalance(t *testing.T, balance int64, accountID string) {
	t.Helper()
	assert.GreaterOrEqual(t, balance, int64(0),
		"account %s should not have negative balance, got: %d", accountID, balance)
}

// AssertIdempotency asserts that running an operation multiple times produces the same result.
func AssertIdempotency[T comparable](t *testing.T, operation func() (T, error), times int) {
	t.Helper()

	var firstResult T
	var firstErr error

	for i := 0; i < times; i++ {
		result, err := operation()
		if i == 0 {
			firstResult = result
			firstErr = err
		} else {
			if firstErr != nil {
				assert.Error(t, err, "subsequent calls should also error")
			} else {
				require.NoError(t, err, "subsequent calls should also succeed")
				assert.Equal(t, firstResult, result, "result should be idempotent (call %d)", i+1)
			}
		}
	}
}

// ChaosTestResult holds the result of a chaos test for reporting.
type ChaosTestResult struct {
	TestName      string
	ChaosType     string
	Duration      time.Duration
	RecoveryTime  time.Duration
	DataIntegrity bool
	Errors        []error
}

// RecordChaosResult creates a ChaosTestResult for reporting.
func RecordChaosResult(testName, chaosType string, duration, recoveryTime time.Duration, dataIntegrity bool, errs ...error) *ChaosTestResult {
	return &ChaosTestResult{
		TestName:      testName,
		ChaosType:     chaosType,
		Duration:      duration,
		RecoveryTime:  recoveryTime,
		DataIntegrity: dataIntegrity,
		Errors:        errs,
	}
}

// Log outputs the chaos test result to the test log.
func (r *ChaosTestResult) Log(t *testing.T) {
	t.Helper()
	t.Logf("Chaos Test Result: %s", r.TestName)
	t.Logf("  Chaos Type: %s", r.ChaosType)
	t.Logf("  Duration: %v", r.Duration)
	t.Logf("  Recovery Time: %v", r.RecoveryTime)
	t.Logf("  Data Integrity: %v", r.DataIntegrity)
	if len(r.Errors) > 0 {
		t.Logf("  Errors: %d", len(r.Errors))
		for i, err := range r.Errors {
			t.Logf("    [%d] %v", i+1, err)
		}
	}
}
