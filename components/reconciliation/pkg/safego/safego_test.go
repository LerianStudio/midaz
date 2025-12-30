package safego

import (
	"sync"
	"testing"
	"time"
)

// mockLogger implements Logger for testing
type mockLogger struct {
	mu       sync.Mutex
	messages []string
	onLog    func() // optional callback when Errorf is called
}

func (m *mockLogger) Errorf(format string, args ...interface{}) {
	m.mu.Lock()
	m.messages = append(m.messages, format)
	onLog := m.onLog
	m.mu.Unlock()

	if onLog != nil {
		onLog()
	}
}

func (m *mockLogger) getMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.messages...)
}

func TestGo_NoPanic(t *testing.T) {
	logger := &mockLogger{}
	var executed bool
	var wg sync.WaitGroup
	wg.Add(1)

	Go(logger, "test", func() {
		defer wg.Done()
		executed = true
	})

	wg.Wait()

	if !executed {
		t.Error("expected function to be executed")
	}
	if len(logger.getMessages()) != 0 {
		t.Errorf("expected no error logs, got %v", logger.getMessages())
	}
}

func TestGo_WithPanic(t *testing.T) {
	logDone := make(chan struct{})
	logger := &mockLogger{
		onLog: func() { close(logDone) },
	}
	var wg sync.WaitGroup
	wg.Add(1)

	Go(logger, "panic-test", func() {
		defer wg.Done()
		panic("test panic")
	})

	wg.Wait()

	// Wait for panic recovery logging to complete
	select {
	case <-logDone:
		// Logging completed
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for panic recovery logging")
	}

	messages := logger.getMessages()
	if len(messages) != 1 {
		t.Errorf("expected 1 error log, got %d", len(messages))
	}
}

func TestGoWithCallback_CallsOnPanic(t *testing.T) {
	logDone := make(chan struct{})
	logger := &mockLogger{
		onLog: func() { close(logDone) },
	}
	var panicValue interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	GoWithCallback(logger, "callback-test", func() {
		defer wg.Done()
		panic("callback panic")
	}, func(r interface{}) {
		panicValue = r
	})

	wg.Wait()

	// Wait for panic recovery logging to complete
	select {
	case <-logDone:
		// Logging completed
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for panic recovery logging")
	}

	if panicValue != "callback panic" {
		t.Errorf("expected panic value 'callback panic', got %v", panicValue)
	}
}

func TestRunWithRecovery_NoPanic(t *testing.T) {
	err := RunWithRecovery(func() {
		// No panic
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunWithRecovery_WithPanic(t *testing.T) {
	err := RunWithRecovery(func() {
		panic("sync panic")
	})

	if err == nil {
		t.Error("expected error from panic")
	}

	recoveredErr, ok := err.(*RecoveredError)
	if !ok {
		t.Errorf("expected *RecoveredError, got %T", err)
	}
	if recoveredErr.Recovered != "sync panic" {
		t.Errorf("expected recovered value 'sync panic', got %v", recoveredErr.Recovered)
	}
	if recoveredErr.Stack == "" {
		t.Error("expected stack trace, got empty string")
	}
}

func TestRecoveredError_Error(t *testing.T) {
	err := &RecoveredError{Recovered: "test panic"}
	expected := "panic: test panic"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
