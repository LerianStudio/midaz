// Package testsupport provides test utilities for the transaction service.
package testsupport

import (
	"fmt"
	"sync"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// MockLogger is a lightweight lib-commons logger implementation intended for tests.
//
// It records formatted log messages so tests can assert warnings/errors were emitted.
// Production code MUST NOT depend on this package.
type MockLogger struct {
	mu sync.Mutex

	DebugfMessages []string
	InfofMessages  []string
	WarnfMessages  []string
	ErrorfMessages []string
}

// Debug logs debug level messages.
func (m *MockLogger) Debug(args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprint(args...))
}

// Debugf logs formatted debug level messages.
func (m *MockLogger) Debugf(format string, args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprintf(format, args...))
}

// Debugln logs debug level messages with newline.
func (m *MockLogger) Debugln(args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprintln(args...))
}

// Info logs info level messages.
func (m *MockLogger) Info(args ...any) {
	m.append(&m.InfofMessages, fmt.Sprint(args...))
}

// Infof logs formatted info level messages.
func (m *MockLogger) Infof(format string, args ...any) {
	m.append(&m.InfofMessages, fmt.Sprintf(format, args...))
}

// Infoln logs info level messages with newline.
func (m *MockLogger) Infoln(args ...any) {
	m.append(&m.InfofMessages, fmt.Sprintln(args...))
}

// Warn logs warn level messages.
func (m *MockLogger) Warn(args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprint(args...))
}

// Warnf logs formatted warn level messages.
func (m *MockLogger) Warnf(format string, args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprintf(format, args...))
}

// Warnln logs warn level messages with newline.
func (m *MockLogger) Warnln(args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprintln(args...))
}

// Error logs error level messages.
func (m *MockLogger) Error(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprint(args...))
}

// Errorf logs formatted error level messages.
func (m *MockLogger) Errorf(format string, args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintf(format, args...))
}

// Errorln logs error level messages with newline.
func (m *MockLogger) Errorln(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintln(args...))
}

// Fatal logs fatal level messages.
func (m *MockLogger) Fatal(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprint(args...))
}

// Fatalf logs formatted fatal level messages.
func (m *MockLogger) Fatalf(format string, args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintf(format, args...))
}

// Fatalln logs fatal level messages with newline.
func (m *MockLogger) Fatalln(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintln(args...))
}

// Sync flushes any buffered log entries.
func (m *MockLogger) Sync() error { return nil }

// WithDefaultMessageTemplate sets a default message template for the logger.
func (m *MockLogger) WithDefaultMessageTemplate(string) libLog.Logger { return m }

// WithFields returns a logger with additional fields.
func (m *MockLogger) WithFields(...any) libLog.Logger { return m }

func (m *MockLogger) append(dst *[]string, msg string) {
	if m == nil {
		return
	}

	m.mu.Lock()

	*dst = append(*dst, msg)

	m.mu.Unlock()
}
