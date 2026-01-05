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

func (m *MockLogger) Debug(args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprint(args...))
}

func (m *MockLogger) Debugf(format string, args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprintf(format, args...))
}

func (m *MockLogger) Debugln(args ...any) {
	m.append(&m.DebugfMessages, fmt.Sprintln(args...))
}

func (m *MockLogger) Info(args ...any) {
	m.append(&m.InfofMessages, fmt.Sprint(args...))
}

func (m *MockLogger) Infof(format string, args ...any) {
	m.append(&m.InfofMessages, fmt.Sprintf(format, args...))
}

func (m *MockLogger) Infoln(args ...any) {
	m.append(&m.InfofMessages, fmt.Sprintln(args...))
}

func (m *MockLogger) Warn(args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprint(args...))
}

func (m *MockLogger) Warnf(format string, args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprintf(format, args...))
}

func (m *MockLogger) Warnln(args ...any) {
	m.append(&m.WarnfMessages, fmt.Sprintln(args...))
}

func (m *MockLogger) Error(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprint(args...))
}

func (m *MockLogger) Errorf(format string, args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintf(format, args...))
}

func (m *MockLogger) Errorln(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintln(args...))
}

func (m *MockLogger) Fatal(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprint(args...))
}

func (m *MockLogger) Fatalf(format string, args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintf(format, args...))
}

func (m *MockLogger) Fatalln(args ...any) {
	m.append(&m.ErrorfMessages, fmt.Sprintln(args...))
}

func (m *MockLogger) Sync() error { return nil }

func (m *MockLogger) WithDefaultMessageTemplate(string) libLog.Logger { return m }
func (m *MockLogger) WithFields(...any) libLog.Logger                 { return m }

func (m *MockLogger) append(dst *[]string, msg string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	*dst = append(*dst, msg)
	m.mu.Unlock()
}
