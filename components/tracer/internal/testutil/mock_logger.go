// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package testutil provides shared test utilities.
package testutil

import (
	"context"
	"sync"

	libLog "github.com/LerianStudio/lib-observability/log"
)

// LogCall represents a single logging call with its level, message, and fields.
type LogCall struct {
	Level   string
	Message string
	Fields  []libLog.Field
}

// MockLogger tracks logging calls for verification in tests.
// It is safe for concurrent use.
type MockLogger struct {
	mu    sync.Mutex
	Calls []LogCall
}

// NewMockLogger creates a new MockLogger instance.
func NewMockLogger() *MockLogger {
	return &MockLogger{
		Calls: []LogCall{},
	}
}

func (m *MockLogger) Log(_ context.Context, level libLog.Level, msg string, fields ...libLog.Field) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, LogCall{Level: levelToString(level), Message: msg, Fields: fields})
}

func (m *MockLogger) With(fields ...libLog.Field) libLog.Logger {
	return &mockLoggerFieldsRecorder{parent: m, fields: fields}
}

func (m *MockLogger) WithGroup(_ string) libLog.Logger { return m }

func (m *MockLogger) Enabled(_ libLog.Level) bool { return true }

func (m *MockLogger) Sync(_ context.Context) error { return nil }

// mockLoggerFieldsRecorder records With and subsequent log calls.
type mockLoggerFieldsRecorder struct {
	parent *MockLogger
	fields []libLog.Field
}

func (m *mockLoggerFieldsRecorder) Log(_ context.Context, level libLog.Level, msg string, fields ...libLog.Field) {
	allFields := make([]libLog.Field, 0, len(m.fields)+len(fields))
	allFields = append(allFields, m.fields...)
	allFields = append(allFields, fields...)

	m.parent.mu.Lock()
	defer m.parent.mu.Unlock()

	m.parent.Calls = append(m.parent.Calls, LogCall{Level: levelToString(level), Message: msg, Fields: allFields})
}

func (m *mockLoggerFieldsRecorder) With(fields ...libLog.Field) libLog.Logger {
	allFields := make([]libLog.Field, 0, len(m.fields)+len(fields))
	allFields = append(allFields, m.fields...)
	allFields = append(allFields, fields...)

	return &mockLoggerFieldsRecorder{parent: m.parent, fields: allFields}
}

func (m *mockLoggerFieldsRecorder) WithGroup(_ string) libLog.Logger { return m }

func (m *mockLoggerFieldsRecorder) Enabled(_ libLog.Level) bool { return true }

func (m *mockLoggerFieldsRecorder) Sync(_ context.Context) error { return nil }

// levelToString converts a libLog.Level to its string representation.
func levelToString(level libLog.Level) string {
	switch level {
	case libLog.LevelDebug:
		return "debug"
	case libLog.LevelInfo:
		return "info"
	case libLog.LevelWarn:
		return "warn"
	case libLog.LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// FieldsToMap converts a slice of libLog.Field to a map for easier assertions.
func FieldsToMap(fields []libLog.Field) map[string]any {
	result := make(map[string]any)

	for _, f := range fields {
		result[f.Key] = f.Value
	}

	return result
}
