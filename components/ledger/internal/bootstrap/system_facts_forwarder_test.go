// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSystemFactsForwarder is the forwarder seam of the Launcher app. Like the
// kit's forwarder, Run blocks until its context ends or Shutdown is called
// (unless runErr is set), and Shutdown records the context it was handed.
type fakeSystemFactsForwarder struct {
	runErr      error
	shutdownErr error

	started  chan struct{}
	returned chan struct{}
	stop     chan struct{}
	stopOnce sync.Once

	mu                  sync.Mutex
	shutdownCalls       int
	shutdownHasDeadline bool
}

func newFakeSystemFactsForwarder() *fakeSystemFactsForwarder {
	return &fakeSystemFactsForwarder{
		started:  make(chan struct{}),
		returned: make(chan struct{}),
		stop:     make(chan struct{}),
	}
}

func (f *fakeSystemFactsForwarder) Run(ctx context.Context) error {
	close(f.started)
	defer close(f.returned)

	if f.runErr != nil {
		return f.runErr
	}

	select {
	case <-ctx.Done():
	case <-f.stop:
	}

	return nil
}

func (f *fakeSystemFactsForwarder) Shutdown(ctx context.Context) error {
	f.stopOnce.Do(func() { close(f.stop) })

	f.mu.Lock()
	defer f.mu.Unlock()

	f.shutdownCalls++
	_, f.shutdownHasDeadline = ctx.Deadline()

	return f.shutdownErr
}

func (f *fakeSystemFactsForwarder) shutdowns() (int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.shutdownCalls, f.shutdownHasDeadline
}

// recordingLogger keeps every entry so a test can assert on the level a
// runnable logged at and on the fields it attached.
type recordingLogger struct {
	mu      sync.Mutex
	entries []recordedLogEntry
}

type recordedLogEntry struct {
	level  int
	msg    string
	fields []any
}

func (l *recordingLogger) Log(_ context.Context, level int, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, recordedLogEntry{level: level, msg: msg, fields: fields})
}

func (l *recordingLogger) With(...any) libLog.Logger      { return l }
func (l *recordingLogger) WithGroup(string) libLog.Logger { return l }
func (l *recordingLogger) Enabled(int) bool               { return true }
func (l *recordingLogger) Sync(context.Context) error     { return nil }

// find returns the first entry at level carrying an error field, if any.
func (l *recordingLogger) find(level int) (recordedLogEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, entry := range l.entries {
		if entry.level != level {
			continue
		}

		for _, field := range entry.fields {
			if f, ok := field.(libLog.Field); ok && f.Key == "error" {
				return entry, true
			}
		}
	}

	return recordedLogEntry{}, false
}

// cancelOnStart is the signal seam: the context ends once the forwarder's Run
// has been entered, standing in for SIGTERM.
func cancelOnStart(forwarder *fakeSystemFactsForwarder) func() (context.Context, context.CancelFunc) {
	return func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			<-forwarder.started
			cancel()
		}()

		return ctx, cancel
	}
}

func awaitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not happen within 5s", what)
	}
}

// TestService_launcherApps_SystemFactsForwarderGuard asserts the forwarder app
// is present IFF the producer built a forwarder, independent of whether the
// preflight turned emission on: a backlog recorded before a failed preflight
// still drains.
func TestService_launcherApps_SystemFactsForwarderGuard(t *testing.T) {
	t.Parallel()

	assert.NotContains(t, launcherAppNames(&Service{}), "System Facts Forwarder",
		"no System Facts wiring must register no forwarder app")
	assert.NotContains(t, launcherAppNames(&Service{SystemFacts: &systemFactsComponents{}}), "System Facts Forwarder",
		"SYSTEM_FACTS_ENABLED=false builds no producer, so no forwarder app")

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeRejected}, err: reconciler.ErrRejected}

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient(), preflight: seam.preflight})
	require.NoError(t, err)
	require.False(t, components.emitting, "the rejected preflight keeps emission off")

	assert.Contains(t, launcherAppNames(&Service{SystemFacts: components, Logger: libLog.NewNop()}), "System Facts Forwarder",
		"an enabled producer registers the forwarder app even with emission off")
}

func TestSystemFactsForwarderRunnable_Run_ShutsDownAfterTheSignal(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	runnable := &systemFactsForwarderRunnable{
		forwarder:     forwarder,
		logger:        libLog.NewNop(),
		signalContext: cancelOnStart(forwarder),
	}

	require.NoError(t, runnable.Run(nil))

	calls, hasDeadline := forwarder.shutdowns()
	assert.Equal(t, 1, calls, "Shutdown runs exactly once after the signal")
	assert.True(t, hasDeadline, "Shutdown is bounded by a deadline")
	awaitClosed(t, forwarder.returned, "the forwarder goroutine returning")
}

func TestSystemFactsForwarderRunnable_Run_LogsAForwarderErrorAtError(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	forwarder.runErr = errors.New("outbox: forwarder run: dispatcher refused")
	logger := &recordingLogger{}
	runnable := &systemFactsForwarderRunnable{
		forwarder:     forwarder,
		logger:        logger,
		signalContext: cancelOnStart(forwarder),
	}

	require.NoError(t, runnable.Run(nil), "a forwarder error is logged, never propagated")

	entry, found := logger.find(libLog.LevelError)
	require.True(t, found, "the forwarder's Run error is logged at Error with an error field")
	assert.Contains(t, entry.msg, "forwarder")
}

func TestSystemFactsForwarderRunnable_Run_LogsAShutdownErrorAtWarn(t *testing.T) {
	t.Parallel()

	forwarder := newFakeSystemFactsForwarder()
	forwarder.shutdownErr = errors.New("outbox: forwarder shutdown: wait for run: context deadline exceeded")
	logger := &recordingLogger{}
	runnable := &systemFactsForwarderRunnable{
		forwarder:     forwarder,
		logger:        logger,
		signalContext: cancelOnStart(forwarder),
	}

	require.NoError(t, runnable.Run(nil))

	_, found := logger.find(libLog.LevelWarn)
	assert.True(t, found, "a Shutdown error is logged at Warn with an error field")
}

func TestSystemFactsForwarderRunnable_Run_NilForwarderReturnsAtOnce(t *testing.T) {
	t.Parallel()

	var runnable *systemFactsForwarderRunnable

	require.NoError(t, runnable.Run(nil))
	require.NoError(t, (&systemFactsForwarderRunnable{logger: libLog.NewNop()}).Run(nil))
}
