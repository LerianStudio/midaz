package mruntime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger is a test logger that captures log calls.
type mockLogger struct {
	mu          sync.Mutex
	errorCalls  []string
	lastMessage string
	panicLogged atomic.Bool
	logged      chan struct{} // Signals when a panic was logged
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		logged: make(chan struct{}, 1), // Buffered to avoid blocking
	}
}

func (m *mockLogger) Errorf(format string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorCalls = append(m.errorCalls, format)
	// Format the message for inspection
	if len(args) > 0 {
		m.lastMessage = format // Store the format string for assertions
	}
	m.panicLogged.Store(true)
	// Signal that logging occurred (non-blocking)
	select {
	case m.logged <- struct{}{}:
	default:
	}
}

func (m *mockLogger) wasPanicLogged() bool {
	return m.panicLogged.Load()
}

func (m *mockLogger) waitForPanicLog(timeout time.Duration) bool {
	select {
	case <-m.logged:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestPanicPolicyString tests the String method of PanicPolicy.
func TestPanicPolicyString(t *testing.T) {
	tests := []struct {
		name     string
		policy   PanicPolicy
		expected string
	}{
		{
			name:     "KeepRunning",
			policy:   KeepRunning,
			expected: "KeepRunning",
		},
		{
			name:     "CrashProcess",
			policy:   CrashProcess,
			expected: "CrashProcess",
		},
		{
			name:     "Unknown",
			policy:   PanicPolicy(99),
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRecoverAndLog_NoPanic tests that RecoverAndLog does nothing when no panic occurs.
func TestRecoverAndLog_NoPanic(t *testing.T) {
	logger := newMockLogger()

	func() {
		defer RecoverAndLog(logger, "test-no-panic")
		// No panic here
	}()

	assert.False(t, logger.wasPanicLogged(), "Should not log when no panic occurs")
}

// TestRecoverAndLog_WithPanic tests that RecoverAndLog catches and logs panics.
func TestRecoverAndLog_WithPanic(t *testing.T) {
	logger := newMockLogger()

	func() {
		defer RecoverAndLog(logger, "test-with-panic")
		panic("test panic value")
	}()

	assert.True(t, logger.wasPanicLogged(), "Should log when panic occurs")
	// The log message should contain the panic info and stack trace
	assert.NotEmpty(t, logger.errorCalls, "Should have logged error")
}

// TestRecoverAndCrash_NoPanic tests that RecoverAndCrash does nothing when no panic occurs.
func TestRecoverAndCrash_NoPanic(t *testing.T) {
	logger := newMockLogger()

	func() {
		defer RecoverAndCrash(logger, "test-no-panic")
		// No panic here
	}()

	assert.False(t, logger.wasPanicLogged(), "Should not log when no panic occurs")
}

// TestRecoverAndCrash_WithPanic tests that RecoverAndCrash catches, logs, and re-panics.
func TestRecoverAndCrash_WithPanic(t *testing.T) {
	logger := newMockLogger()

	defer func() {
		r := recover()
		require.NotNil(t, r, "Should re-panic after logging")
		assert.Equal(t, "test panic value", r)
	}()

	func() {
		defer RecoverAndCrash(logger, "test-with-panic")
		panic("test panic value")
	}()

	t.Fatal("Should not reach here - panic should propagate")
}

// TestRecoverWithPolicy_KeepRunning tests policy-based recovery with KeepRunning.
func TestRecoverWithPolicy_KeepRunning(t *testing.T) {
	logger := newMockLogger()

	func() {
		defer RecoverWithPolicy(logger, "test-keep-running", KeepRunning)
		panic("test panic")
	}()

	assert.True(t, logger.wasPanicLogged(), "Should log the panic")
	// If we get here, the panic was swallowed (KeepRunning behavior)
}

// TestRecoverWithPolicy_CrashProcess tests policy-based recovery with CrashProcess.
func TestRecoverWithPolicy_CrashProcess(t *testing.T) {
	logger := newMockLogger()

	defer func() {
		r := recover()
		require.NotNil(t, r, "Should re-panic with CrashProcess policy")
	}()

	func() {
		defer RecoverWithPolicy(logger, "test-crash", CrashProcess)
		panic("test panic")
	}()

	t.Fatal("Should not reach here")
}

// TestSafeGo_NoPanic tests SafeGo with a function that doesn't panic.
func TestSafeGo_NoPanic(t *testing.T) {
	logger := newMockLogger()
	done := make(chan struct{})

	SafeGo(logger, "test-no-panic", KeepRunning, func() {
		close(done)
	})

	select {
	case <-done:
		// Success - goroutine completed
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not complete in time")
	}

	// No sleep needed - if no panic occurred, logger won't be called
	assert.False(t, logger.wasPanicLogged(), "Should not log when no panic occurs")
}

// TestSafeGo_WithPanic_KeepRunning tests SafeGo catching panics with KeepRunning policy.
func TestSafeGo_WithPanic_KeepRunning(t *testing.T) {
	logger := newMockLogger()
	done := make(chan struct{})

	SafeGo(logger, "test-panic-keep-running", KeepRunning, func() {
		defer close(done)
		panic("goroutine panic")
	})

	select {
	case <-done:
		// Success - goroutine completed (panic was caught)
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not complete in time")
	}

	// Wait for logging via channel instead of arbitrary sleep
	require.True(t, logger.waitForPanicLog(time.Second), "Should log the panic")
}

// TestSafeGoWithContext_NoPanic tests SafeGoWithContext with no panic.
func TestSafeGoWithContext_NoPanic(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()
	done := make(chan struct{})

	SafeGoWithContext(ctx, logger, "test-ctx-no-panic", KeepRunning, func(ctx context.Context) {
		close(done)
	})

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not complete in time")
	}
}

// TestSafeGoWithContext_WithCancellation tests context cancellation propagation.
func TestSafeGoWithContext_WithCancellation(t *testing.T) {
	logger := newMockLogger()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	SafeGoWithContext(ctx, logger, "test-ctx-cancel", KeepRunning, func(ctx context.Context) {
		<-ctx.Done()
		close(done)
	})

	cancel()

	select {
	case <-done:
		// Success - context cancellation was received
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not receive cancellation in time")
	}
}

// TestSafeGoWithContext_WithPanic tests SafeGoWithContext catching panics.
func TestSafeGoWithContext_WithPanic(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()
	done := make(chan struct{})

	SafeGoWithContext(ctx, logger, "test-ctx-panic", KeepRunning, func(ctx context.Context) {
		defer close(done)
		panic("context goroutine panic")
	})

	select {
	case <-done:
		// Success - panic was caught
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not complete in time")
	}

	// Wait for logging via channel instead of arbitrary sleep
	require.True(t, logger.waitForPanicLog(time.Second), "Should log the panic")
}

// Note: SafeGo with CrashProcess policy is not directly tested because the re-panic
// would crash the test process. The underlying RecoverWithPolicy is tested with
// CrashProcess policy in TestRecoverWithPolicy_CrashProcess, which verifies the
// re-panic behavior. In production, CrashProcess is intended to terminate the
// process, which is the expected and correct behavior.

// TestSafeGoWithContext_WithComponent tests SafeGoWithContextAndComponent.
func TestSafeGoWithContext_WithComponent(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()
	done := make(chan struct{})

	SafeGoWithContextAndComponent(ctx, logger, "transaction", "test-component", KeepRunning, func(ctx context.Context) {
		defer close(done)
		panic("component panic")
	})

	select {
	case <-done:
		// Success - panic was caught
	case <-time.After(time.Second):
		t.Fatal("Goroutine did not complete in time")
	}

	// Wait for logging via channel
	require.True(t, logger.waitForPanicLog(time.Second), "Should log the panic")
}

// TestRecoverWithPolicyAndContext_KeepRunning tests context-aware recovery with KeepRunning.
func TestRecoverWithPolicyAndContext_KeepRunning(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()

	func() {
		defer RecoverWithPolicyAndContext(ctx, logger, "test-component", "test-handler", KeepRunning)
		panic("context panic")
	}()

	assert.True(t, logger.wasPanicLogged(), "Should log the panic")
}

// TestRecoverWithPolicyAndContext_CrashProcess tests context-aware recovery with CrashProcess.
func TestRecoverWithPolicyAndContext_CrashProcess(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()

	defer func() {
		r := recover()
		require.NotNil(t, r, "Should re-panic with CrashProcess policy")
	}()

	func() {
		defer RecoverWithPolicyAndContext(ctx, logger, "test-component", "test-crash", CrashProcess)
		panic("crash panic")
	}()

	t.Fatal("Should not reach here")
}

// TestRecoverAndLogWithContext tests RecoverAndLogWithContext.
func TestRecoverAndLogWithContext(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()

	func() {
		defer RecoverAndLogWithContext(ctx, logger, "test-component", "test-handler")
		panic("log context panic")
	}()

	assert.True(t, logger.wasPanicLogged(), "Should log the panic")
}

// TestRecoverAndCrashWithContext tests RecoverAndCrashWithContext.
func TestRecoverAndCrashWithContext(t *testing.T) {
	logger := newMockLogger()
	ctx := context.Background()

	defer func() {
		r := recover()
		require.NotNil(t, r, "Should re-panic after logging")
		assert.Equal(t, "crash context panic", r)
	}()

	func() {
		defer RecoverAndCrashWithContext(ctx, logger, "test-component", "test-crash")
		panic("crash context panic")
	}()

	t.Fatal("Should not reach here - panic should propagate")
}

// TestPanicMetrics_NilFactory tests that nil factory doesn't cause panic.
func TestPanicMetrics_NilFactory(t *testing.T) {
	ctx := context.Background()

	// Should not panic even with nil metrics
	var pm *PanicMetrics
	pm.RecordPanicRecovered(ctx, "test", "test")

	// Also test the package-level function with no initialization
	recordPanicMetric(ctx, "test", "test")
}

// TestErrorReporter_NilReporter tests that nil reporter doesn't cause panic.
func TestErrorReporter_NilReporter(t *testing.T) {
	ctx := context.Background()

	// Ensure no reporter is set
	SetErrorReporter(nil)

	// Should not panic
	reportPanicToErrorService(ctx, "test panic", nil, "test", "test")

	assert.Nil(t, GetErrorReporter())
}

// TestErrorReporter_CustomReporter tests custom error reporter integration.
func TestErrorReporter_CustomReporter(t *testing.T) {
	ctx := context.Background()
	var capturedErr error
	var capturedTags map[string]string

	// Create a mock reporter
	mockReporter := &mockErrorReporter{
		captureFunc: func(ctx context.Context, err error, tags map[string]string) {
			capturedErr = err
			capturedTags = tags
		},
	}

	// Set the reporter with proper cleanup
	t.Cleanup(func() { SetErrorReporter(nil) })
	SetErrorReporter(mockReporter)

	// Report a panic
	reportPanicToErrorService(ctx, "test panic", []byte("test stack trace"), "transaction", "worker")

	require.NotNil(t, capturedErr)
	assert.Contains(t, capturedErr.Error(), "test panic")
	assert.Equal(t, "transaction", capturedTags["component"])
	assert.Equal(t, "worker", capturedTags["goroutine_name"])
	assert.Equal(t, "test stack trace", capturedTags["stack_trace"])
}

// mockErrorReporter is a test implementation of ErrorReporter.
type mockErrorReporter struct {
	captureFunc func(ctx context.Context, err error, tags map[string]string)
}

func (m *mockErrorReporter) CaptureException(ctx context.Context, err error, tags map[string]string) {
	if m.captureFunc != nil {
		m.captureFunc(ctx, err, tags)
	}
}
