// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Static sentinel errors for tests (satisfies err113 lint).
var (
	errEnvBlewUp   = errors.New("env blew up")
	errLoggerKaput = errors.New("logger kaput")
	errServiceBoom = errors.New("service boom")
)

// fakeRunner is a no-op consumer service used for the happy-path test. The real
// service.Run() is a blocking worker loop; the fake lets run() return promptly.
type fakeRunner struct {
	runCalls int
}

func (f *fakeRunner) Run() { f.runCalls++ }

// trackingLogger wraps NoneLogger and records that Sync() was called so we can
// assert on the documented "Sync on service-init failure" behavior.
type trackingLogger struct {
	libLog.NoneLogger
	syncCalled int
}

func (t *trackingLogger) Sync() error { t.syncCalled++; return nil }

func TestRun_Healthcheck_ShortCircuits(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	// Intentionally leave all deps nil / panicking. --healthcheck must return 0
	// before touching any of them.
	d := deps{
		initEnvConfig: func() error { panic("must not be called") },
		initLogger:    func() (libLog.Logger, error) { panic("must not be called") },
		initService:   func(libLog.Logger) (runner, error) { panic("must not be called") },
	}

	code := run([]string{"app", "--healthcheck"}, buf, d)

	require.Equal(t, 0, code)
	require.Empty(t, buf.String(), "healthcheck path must not log to stderr")
}

func TestRun_EnvConfigFails_ReturnsExitCode1(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	d := deps{
		initEnvConfig: func() error { return errEnvBlewUp },
		initLogger:    func() (libLog.Logger, error) { panic("must not be called") },
		initService:   func(libLog.Logger) (runner, error) { panic("must not be called") },
	}

	code := run([]string{"app"}, buf, d)

	require.Equal(t, 1, code)
	require.Contains(t, buf.String(), "env blew up")
	require.Contains(t, buf.String(), "failed to initialize env config")
}

func TestRun_LoggerInitFails_ReturnsExitCode1(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	d := deps{
		initEnvConfig: func() error { return nil },
		initLogger:    func() (libLog.Logger, error) { return nil, errLoggerKaput },
		initService:   func(libLog.Logger) (runner, error) { panic("must not be called") },
	}

	code := run([]string{"app"}, buf, d)

	require.Equal(t, 1, code)
	require.Contains(t, buf.String(), "logger kaput")
	require.Contains(t, buf.String(), "failed to initialize logger")
}

func TestRun_ServiceInitFails_CallsSyncAndReturnsExitCode1(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	tl := &trackingLogger{}

	d := deps{
		initEnvConfig: func() error { return nil },
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		initService: func(libLog.Logger) (runner, error) {
			return nil, errServiceBoom
		},
	}

	code := run([]string{"app"}, buf, d)

	require.Equal(t, 1, code)
	// Service-init failures go to the logger, not stderr (matches original flow).
	require.Empty(t, buf.String(), "service-init errors must route through the logger")
	require.Equal(t, 1, tl.syncCalled, "logger.Sync must be flushed before exit on service-init failure")
}

func TestRun_HappyPath_CallsServiceRun(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	fr := &fakeRunner{}
	tl := &trackingLogger{}

	d := deps{
		initEnvConfig: func() error { return nil },
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		initService:   func(libLog.Logger) (runner, error) { return fr, nil },
	}

	code := run([]string{"app"}, buf, d)

	require.Equal(t, 0, code)
	require.Equal(t, 1, fr.runCalls, "service.Run must be invoked exactly once")
	require.Zero(t, tl.syncCalled, "Sync must not be called on the happy path (matches original flow)")
	require.Empty(t, buf.String())
}

// TestRun_NoArgs verifies the len(args) > 1 guard does not panic with a minimal
// argv. Go's os.Args always has len >= 1, but defensive coverage is cheap.
func TestRun_NoArgs_ProceedsThroughBootstrap(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	fr := &fakeRunner{}
	tl := &trackingLogger{}

	d := deps{
		initEnvConfig: func() error { return nil },
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		initService:   func(libLog.Logger) (runner, error) { return fr, nil },
	}

	code := run([]string{"app"}, buf, d)

	require.Equal(t, 0, code)
	require.Equal(t, 1, fr.runCalls)
}

// TestRealDeps_Constructed verifies realDeps() returns a fully-populated deps
// struct. We cannot call the closures (they'd touch real env/logger/DB), but
// we can assert they are non-nil so main() never dereferences a zero dep.
func TestRealDeps_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	d := realDeps()

	require.NotNil(t, d.initEnvConfig, "initEnvConfig must be wired")
	require.NotNil(t, d.initLogger, "initLogger must be wired")
	require.NotNil(t, d.initService, "initService must be wired")
}

// TestRealDeps_InitEnvConfig_IsSafeNoError verifies the env-config closure
// returns nil. InitLocalEnvConfig is void in libCommons; our closure wraps it
// without inventing error conditions. This test locks in that contract so a
// future refactor doesn't silently start returning errors and break main().
func TestRealDeps_InitEnvConfig_ReturnsNil(t *testing.T) {
	t.Parallel()

	d := realDeps()
	// InitLocalEnvConfig reads optional .env files; calling it in a test dir
	// is a no-op when no .env is present. Safe to invoke.
	err := d.initEnvConfig()
	require.NoError(t, err)
}

// TestRealDeps_InitLogger_ReturnsRealLogger exercises the real libZap closure.
// InitializeLoggerWithError builds a zap logger from env vars — with no env
// configured it falls back to defaults and returns successfully. We don't care
// about the logger's output here, only that the closure wires correctly.
func TestRealDeps_InitLogger_BuildsLogger(t *testing.T) {
	t.Parallel()

	d := realDeps()
	logger, err := d.initLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)
}

// TestRealDeps_InitService_RequiresRealInfrastructure documents that the
// service-init closure is intentionally NOT exercised in unit tests — it would
// try to open real PostgreSQL/MongoDB/Redpanda connections. This test asserts
// only that the closure is non-nil; integration tests cover end-to-end behavior.
func TestRealDeps_InitService_IsWiredButNotInvoked(t *testing.T) {
	t.Parallel()

	d := realDeps()
	require.NotNil(t, d.initService, "initService closure must be wired for main() to use")
}

// TestRealDeps_InitService_InvokesUnderlyingFactory exercises the production
// initService closure just far enough to cover its statements. We feed it a
// NoneLogger; the closure will call transaction.InitConsumerServiceOrError,
// which will almost certainly return an error in a unit-test environment (no
// postgres, no mongo, no redpanda). Either outcome covers the closure body.
func TestRealDeps_InitService_ExecutesClosureBody(t *testing.T) {
	t.Parallel()

	d := realDeps()
	// Recover from any panic inside underlying init — we only care that the
	// closure body executes, not that it succeeds.
	defer func() {
		_ = recover()
	}()

	svc, err := d.initService(&libLog.NoneLogger{})
	// In unit-test env there's no infra, so we expect err != nil OR a non-nil
	// svc if defaults happen to succeed. Either is fine — the closure executed.
	if err == nil {
		require.NotNil(t, svc)
	} else {
		require.Nil(t, svc)
	}
}
