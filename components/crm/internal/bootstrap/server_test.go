// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// TestNewServer_WiresFieldsFromConfig verifies NewServer copies the configured
// server address and stores the provided Fiber app, logger, and telemetry so
// downstream components (Run, ServerAddress) observe consistent state. We use
// a disabled telemetry configuration so no OTLP exporter is initialised.
func TestNewServer_WiresFieldsFromConfig(t *testing.T) {
	t.Parallel()

	logger := libLog.Logger(&libLog.GoLogger{Level: libLog.InfoLevel})

	tl, err := libOpenTelemetry.InitializeTelemetryWithError(&libOpenTelemetry.TelemetryConfig{
		LibraryName:     "test",
		ServiceName:     "crm-bootstrap-test",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	cfg := &Config{ServerAddress: ":0"}
	app := fiber.New()

	s := NewServer(cfg, app, logger, tl)

	require.NotNil(t, s)
	assert.Equal(t, ":0", s.ServerAddress(), "ServerAddress must round-trip the value from Config")
	assert.Same(t, app, s.app, "Fiber app must be stored verbatim for Run to use")
	assert.Equal(t, tl.LibraryName, s.telemetry.LibraryName, "telemetry must be copied into the server")
}

// TestServer_ServerAddressReflectsConfig exercises the ServerAddress getter
// against multiple inputs. This is a trivial accessor but the test guards
// against accidental formatting (e.g. trimming a leading colon) that would
// break the Fiber listener.
func TestServer_ServerAddressReflectsConfig(t *testing.T) {
	t.Parallel()

	logger := libLog.Logger(&libLog.GoLogger{Level: libLog.InfoLevel})

	tl, err := libOpenTelemetry.InitializeTelemetryWithError(&libOpenTelemetry.TelemetryConfig{
		LibraryName:     "test",
		ServiceName:     "crm-bootstrap-test",
		EnableTelemetry: false,
		Logger:          logger,
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		addr string
	}{
		{name: "localhost with port", addr: "127.0.0.1:4003"},
		{name: "wildcard with port", addr: ":4003"},
		{name: "empty address", addr: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := NewServer(&Config{ServerAddress: tt.addr}, fiber.New(), logger, tl)

			assert.Equal(t, tt.addr, s.ServerAddress())
		})
	}
}
