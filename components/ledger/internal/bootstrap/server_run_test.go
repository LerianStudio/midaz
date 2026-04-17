// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

// TestServer_Run_GracefulShutdown exercises the Server.Run path. Because
// StartWithGracefulShutdown blocks on SIGTERM/SIGINT, we send SIGTERM to
// our own process once the server goroutine is running. Go's signal.Notify
// traps the signal so the test binary is not killed.
//
// No t.Parallel(): this test sends a process-wide signal that would disturb
// other concurrent tests relying on signal handling.
func TestServer_Run_GracefulShutdown(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Use a telemetry pointer that can be safely shutdown. Building
	// lib-commons telemetry with EnableTelemetry=false produces a
	// no-op provider that handles Shutdown without panicking.
	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:     "test-server-run",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	cfg := &Config{ServerAddress: "127.0.0.1:0"}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/ping", func(c *fiber.Ctx) error { return c.SendStatus(200) })

	srv := NewServer(cfg, app, logger, telemetry)
	require.NotNil(t, srv)

	done := make(chan error, 1)

	go func() {
		// Pass nil launcher — Run ignores it and delegates to ServerManager.
		done <- srv.Run((*libCommons.Launcher)(nil))
	}()

	// Give the goroutine a moment to invoke StartWithGracefulShutdown so
	// the signal handler is installed before we fire SIGTERM.
	time.Sleep(150 * time.Millisecond)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case runErr := <-done:
		assert.NoError(t, runErr, "Server.Run should return nil after graceful shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("Server.Run did not return within 10s of SIGTERM")
	}
}

// TestUnifiedServer_Run_GracefulShutdown exercises the UnifiedServer.Run
// path using the same SIGTERM strategy. No t.Parallel() for the same
// reason as Server_Run above.
func TestUnifiedServer_Run_GracefulShutdown(t *testing.T) {
	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:     "test-unified-run",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	srv := NewUnifiedServer("127.0.0.1:0", logger, telemetry)
	require.NotNil(t, srv)

	done := make(chan error, 1)

	go func() {
		done <- srv.Run((*libCommons.Launcher)(nil))
	}()

	time.Sleep(150 * time.Millisecond)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case runErr := <-done:
		assert.NoError(t, runErr, "UnifiedServer.Run should return nil after graceful shutdown")
	case <-time.After(10 * time.Second):
		t.Fatal("UnifiedServer.Run did not return within 10s of SIGTERM")
	}
}
