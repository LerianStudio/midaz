// Package stubs provides test doubles for tests.
//
// This file contains LoggerStub which can be used in both unit and integration tests.
// Unlike ports.go, this file has no build tag to ensure availability in all test contexts.
package stubs

import (
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Compile-time check that LoggerStub implements libLog.Logger
var _ libLog.Logger = (*LoggerStub)(nil)

// LoggerStub captures log messages for verification in tests.
//
// Use when you need to verify specific log messages were emitted (e.g., deprecation warnings).
// For tests that don't need to verify log output, prefer using the real logger:
//
//	logger := libZap.InitializeLogger()
//
// Example usage:
//
//	func TestFunction_LogsDeprecationWarning(t *testing.T) {
//	    logger := &stubs.LoggerStub{}
//	    result := FunctionUnderTest(logger)
//
//	    assert.True(t, logger.HasWarning("DEPRECATED"))
//	    assert.Equal(t, 1, logger.WarningCount())
//	}
type LoggerStub struct {
	Infos    []string
	Warnings []string
	Errors   []string
	Debugs   []string
	Fatals   []string
}

// Info captures info messages.
func (l *LoggerStub) Info(args ...any) {
	l.Infos = append(l.Infos, fmt.Sprint(args...))
}

// Infof captures formatted info messages.
func (l *LoggerStub) Infof(format string, args ...any) {
	l.Infos = append(l.Infos, fmt.Sprintf(format, args...))
}

// Infoln captures info messages with newline.
func (l *LoggerStub) Infoln(args ...any) {
	l.Infos = append(l.Infos, fmt.Sprintln(args...))
}

// Error captures error messages.
func (l *LoggerStub) Error(args ...any) {
	l.Errors = append(l.Errors, fmt.Sprint(args...))
}

// Errorf captures formatted error messages.
func (l *LoggerStub) Errorf(format string, args ...any) {
	l.Errors = append(l.Errors, fmt.Sprintf(format, args...))
}

// Errorln captures error messages with newline.
func (l *LoggerStub) Errorln(args ...any) {
	l.Errors = append(l.Errors, fmt.Sprintln(args...))
}

// Warn captures warning messages.
func (l *LoggerStub) Warn(args ...any) {
	l.Warnings = append(l.Warnings, fmt.Sprint(args...))
}

// Warnf captures formatted warning messages.
func (l *LoggerStub) Warnf(format string, args ...any) {
	l.Warnings = append(l.Warnings, fmt.Sprintf(format, args...))
}

// Warnln captures warning messages with newline.
func (l *LoggerStub) Warnln(args ...any) {
	l.Warnings = append(l.Warnings, fmt.Sprintln(args...))
}

// Debug captures debug messages.
func (l *LoggerStub) Debug(args ...any) {
	l.Debugs = append(l.Debugs, fmt.Sprint(args...))
}

// Debugf captures formatted debug messages.
func (l *LoggerStub) Debugf(format string, args ...any) {
	l.Debugs = append(l.Debugs, fmt.Sprintf(format, args...))
}

// Debugln captures debug messages with newline.
func (l *LoggerStub) Debugln(args ...any) {
	l.Debugs = append(l.Debugs, fmt.Sprintln(args...))
}

// Fatal captures fatal messages (does NOT exit - safe for tests).
func (l *LoggerStub) Fatal(args ...any) {
	l.Fatals = append(l.Fatals, fmt.Sprint(args...))
}

// Fatalf captures formatted fatal messages (does NOT exit - safe for tests).
func (l *LoggerStub) Fatalf(format string, args ...any) {
	l.Fatals = append(l.Fatals, fmt.Sprintf(format, args...))
}

// Fatalln captures fatal messages with newline (does NOT exit - safe for tests).
func (l *LoggerStub) Fatalln(args ...any) {
	l.Fatals = append(l.Fatals, fmt.Sprintln(args...))
}

// WithFields returns self (no-op for stub).
func (l *LoggerStub) WithFields(fields ...any) libLog.Logger {
	return l
}

// WithDefaultMessageTemplate returns self (no-op for stub).
func (l *LoggerStub) WithDefaultMessageTemplate(message string) libLog.Logger {
	return l
}

// Sync is a no-op for stub.
func (l *LoggerStub) Sync() error {
	return nil
}

// HasWarning checks if any captured warning contains the given substring.
func (l *LoggerStub) HasWarning(substring string) bool {
	for _, w := range l.Warnings {
		if strings.Contains(w, substring) {
			return true
		}
	}

	return false
}

// HasError checks if any captured error contains the given substring.
func (l *LoggerStub) HasError(substring string) bool {
	for _, e := range l.Errors {
		if strings.Contains(e, substring) {
			return true
		}
	}

	return false
}

// HasInfo checks if any captured info contains the given substring.
func (l *LoggerStub) HasInfo(substring string) bool {
	for _, i := range l.Infos {
		if strings.Contains(i, substring) {
			return true
		}
	}

	return false
}

// WarningCount returns the number of captured warnings.
func (l *LoggerStub) WarningCount() int {
	return len(l.Warnings)
}

// ErrorCount returns the number of captured errors.
func (l *LoggerStub) ErrorCount() int {
	return len(l.Errors)
}

// InfoCount returns the number of captured info messages.
func (l *LoggerStub) InfoCount() int {
	return len(l.Infos)
}

// Reset clears all captured messages.
func (l *LoggerStub) Reset() {
	l.Infos = nil
	l.Warnings = nil
	l.Errors = nil
	l.Debugs = nil
	l.Fatals = nil
}
