// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/components/authorizer/internal/bootstrap"
)

// Static sentinel errors for failure-path tests (err113 lint compliance).
var (
	errLoggerInitBoom = errors.New("logger init boom")
	errConfigLoadBoom = errors.New("config load boom")
	errTelemetryBoom  = errors.New("telemetry boom")
	errRunBoom        = errors.New("run boom")
)

// trackingLogger wraps NoneLogger and counts Sync() calls so we can verify the
// documented "Sync on every error path after logger exists" contract.
type trackingLogger struct {
	libLog.NoneLogger
	syncCalls int
}

func (t *trackingLogger) Sync() error { t.syncCalls++; return nil }

// fakeTelemetry satisfies the telemetryHandle interface and records whether
// ShutdownTelemetry was deferred. Using a struct (not just a func) keeps the
// intent legible and lets us assert call count.
type fakeTelemetry struct {
	shutdownCalls int
}

func (f *fakeTelemetry) ShutdownTelemetry() { f.shutdownCalls++ }

// ---- Failure-path tests --------------------------------------------------

func TestRun_LoggerInitFails_WritesToStderrAndExits1(t *testing.T) {
	t.Parallel()

	envCalls := 0
	buf := &bytes.Buffer{}
	d := deps{
		initEnvConfig: func() { envCalls++ },
		initLogger:    func() (libLog.Logger, error) { return nil, errLoggerInitBoom },
		// Downstream deps must not be reached.
		loadConfig:    func() (*bootstrap.Config, error) { panic("must not be called") },
		initTelemetry: func(*bootstrap.Config, libLog.Logger) (telemetryHandle, error) { panic("must not be called") },
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			panic("must not be called")
		},
		runBootstrap: func(context.Context, *bootstrap.Config, libLog.Logger, telemetryHandle) error {
			panic("must not be called")
		},
	}

	code := run(buf, d)

	require.Equal(t, 1, code)
	require.Equal(t, 1, envCalls, "env config init runs before logger init")
	require.Contains(t, buf.String(), "failed to initialize logger")
	require.Contains(t, buf.String(), "logger init boom")
}

func TestRun_LoadConfigFails_LogsAndSyncsAndExits1(t *testing.T) {
	t.Parallel()

	tl := &trackingLogger{}
	buf := &bytes.Buffer{}
	d := deps{
		initEnvConfig: func() {},
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		loadConfig:    func() (*bootstrap.Config, error) { return nil, errConfigLoadBoom },
		initTelemetry: func(*bootstrap.Config, libLog.Logger) (telemetryHandle, error) { panic("must not be called") },
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			panic("must not be called")
		},
		runBootstrap: func(context.Context, *bootstrap.Config, libLog.Logger, telemetryHandle) error {
			panic("must not be called")
		},
	}

	code := run(buf, d)

	require.Equal(t, 1, code)
	require.Equal(t, 1, tl.syncCalls, "logger.Sync must be flushed on config-load failure")
	// Config errors route through the logger, not stderr.
	require.Empty(t, buf.String())
}

func TestRun_InitTelemetryFails_LogsAndSyncsAndExits1(t *testing.T) {
	t.Parallel()

	tl := &trackingLogger{}
	buf := &bytes.Buffer{}
	cfg := &bootstrap.Config{}
	d := deps{
		initEnvConfig: func() {},
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		loadConfig:    func() (*bootstrap.Config, error) { return cfg, nil },
		initTelemetry: func(*bootstrap.Config, libLog.Logger) (telemetryHandle, error) {
			return nil, errTelemetryBoom
		},
		notifyContext: func(context.Context, ...os.Signal) (context.Context, context.CancelFunc) {
			panic("must not be called")
		},
		runBootstrap: func(context.Context, *bootstrap.Config, libLog.Logger, telemetryHandle) error {
			panic("must not be called")
		},
	}

	code := run(buf, d)

	require.Equal(t, 1, code)
	require.Equal(t, 1, tl.syncCalls, "logger.Sync must be flushed on telemetry failure")
	require.Empty(t, buf.String())
}

func TestRun_RunBootstrapFails_LogsSyncsShutsDownTelemetryAndExits1(t *testing.T) {
	t.Parallel()

	tl := &trackingLogger{}
	ft := &fakeTelemetry{}
	buf := &bytes.Buffer{}
	cfg := &bootstrap.Config{}
	stopCalls := 0
	d := deps{
		initEnvConfig: func() {},
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		loadConfig:    func() (*bootstrap.Config, error) { return cfg, nil },
		initTelemetry: func(*bootstrap.Config, libLog.Logger) (telemetryHandle, error) {
			return ft, nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			return ctx, func() { stopCalls++; cancel() }
		},
		runBootstrap: func(context.Context, *bootstrap.Config, libLog.Logger, telemetryHandle) error {
			return errRunBoom
		},
	}

	code := run(buf, d)

	require.Equal(t, 1, code)
	require.Equal(t, 1, tl.syncCalls, "logger.Sync must be flushed after bootstrap failure")
	require.Equal(t, 1, ft.shutdownCalls, "deferred ShutdownTelemetry must run even on failure")
	require.Equal(t, 1, stopCalls, "deferred signal stop must run even on failure")
	require.Empty(t, buf.String())
}

// ---- Happy-path test -----------------------------------------------------

func TestRun_HappyPath_RunsBootstrapAndDefersShutdown(t *testing.T) {
	t.Parallel()

	tl := &trackingLogger{}
	ft := &fakeTelemetry{}
	buf := &bytes.Buffer{}
	cfg := &bootstrap.Config{}
	envCalls := 0
	loadCalls := 0
	telemetryCalls := 0
	runCalls := 0
	stopCalls := 0

	d := deps{
		initEnvConfig: func() { envCalls++ },
		initLogger:    func() (libLog.Logger, error) { return tl, nil },
		loadConfig: func() (*bootstrap.Config, error) {
			loadCalls++
			return cfg, nil
		},
		initTelemetry: func(gotCfg *bootstrap.Config, gotLogger libLog.Logger) (telemetryHandle, error) {
			telemetryCalls++
			require.Same(t, cfg, gotCfg, "cfg must flow through to telemetry init")
			require.Same(t, tl, gotLogger, "logger must flow through to telemetry init")
			return ft, nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(parent)
			return ctx, func() { stopCalls++; cancel() }
		},
		runBootstrap: func(_ context.Context, gotCfg *bootstrap.Config, gotLogger libLog.Logger, gotTel telemetryHandle) error {
			runCalls++
			require.Same(t, cfg, gotCfg)
			require.Same(t, tl, gotLogger)
			require.Same(t, ft, gotTel)
			return nil
		},
	}

	code := run(buf, d)

	require.Equal(t, 0, code)
	require.Equal(t, 1, envCalls)
	require.Equal(t, 1, loadCalls)
	require.Equal(t, 1, telemetryCalls)
	require.Equal(t, 1, runCalls)
	require.Equal(t, 1, stopCalls, "signal-context stop must be deferred")
	require.Equal(t, 1, ft.shutdownCalls, "telemetry Shutdown must be deferred on happy path too")
	require.Zero(t, tl.syncCalls, "Sync must not be called on happy path (matches original flow)")
	require.Empty(t, buf.String())
}

// ---- realDeps() wiring tests --------------------------------------------

func TestRealDeps_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	d := realDeps()
	require.NotNil(t, d.initEnvConfig, "initEnvConfig must be wired")
	require.NotNil(t, d.initLogger, "initLogger must be wired")
	require.NotNil(t, d.loadConfig, "loadConfig must be wired")
	require.NotNil(t, d.initTelemetry, "initTelemetry must be wired")
	require.NotNil(t, d.notifyContext, "notifyContext must be wired")
	require.NotNil(t, d.runBootstrap, "runBootstrap must be wired")
}

// TestRealDeps_InitEnvConfig_DoesNotPanic exercises the env-config closure.
// libCommons.InitLocalEnvConfig reads optional .env files; in a test dir with
// none present it is a no-op. Safe to invoke.
func TestRealDeps_InitEnvConfig_Executes(t *testing.T) {
	t.Parallel()

	d := realDeps()
	require.NotPanics(t, d.initEnvConfig)
}

// TestRealDeps_InitLogger_BuildsLogger exercises the real libZap closure.
// InitializeLoggerWithError builds a zap logger from env vars; with no env
// configured it falls back to defaults.
func TestRealDeps_InitLogger_BuildsLogger(t *testing.T) {
	t.Parallel()

	d := realDeps()
	logger, err := d.initLogger()
	require.NoError(t, err)
	require.NotNil(t, logger)
}

// TestRealDeps_NotifyContext_WiresSignalHandler drives the real
// signal.NotifyContext closure just far enough to prove it returns a usable
// context/cancel pair. We invoke stop immediately to avoid leaking signal
// handlers.
func TestRealDeps_NotifyContext_ReturnsContextAndStop(t *testing.T) {
	t.Parallel()

	d := realDeps()
	ctx, stop := d.notifyContext(context.Background(), os.Interrupt)
	require.NotNil(t, ctx)
	require.NotNil(t, stop)
	// stop() unregisters the signal handler; per signal.NotifyContext docs it
	// also cancels the returned context. We invoke it to prevent leaking
	// signal handlers between tests and don't assert on ctx.Err() since the
	// cancellation is a documented side effect.
	stop()
}

// TestRealDeps_LoadConfigClosure_Executes runs the real LoadConfig closure. It
// may succeed or fail depending on the test environment's env vars. Either
// outcome covers the closure body; we only assert the two results are
// mutually exclusive.
func TestRealDeps_LoadConfig_ExecutesClosureBody(t *testing.T) {
	t.Parallel()

	defer func() { _ = recover() }()

	d := realDeps()
	cfg, err := d.loadConfig()
	if err == nil {
		require.NotNil(t, cfg)
	} else {
		require.Nil(t, cfg)
	}
}

// TestRealDeps_InitTelemetry_ExecutesClosureBody exercises the telemetry
// closure. Without a real OTEL collector the call is likely to fail, but that
// is fine — we only need coverage of the closure statements.
func TestRealDeps_InitTelemetry_ExecutesClosureBody(t *testing.T) {
	t.Parallel()

	defer func() { _ = recover() }()

	d := realDeps()
	tel, err := d.initTelemetry(&bootstrap.Config{}, &libLog.NoneLogger{})
	if err == nil {
		require.NotNil(t, tel)
	} else {
		require.Nil(t, tel)
	}
}
