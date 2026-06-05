// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libOtel "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

func TestHTTPServer_ServerAddress(t *testing.T) {
	t.Run("returns configured address", func(t *testing.T) {
		server := &HTTPServer{
			serverAddress: ":8080",
		}

		result := server.ServerAddress()

		assert.Equal(t, ":8080", result)
	})
}

func TestNewHTTPServer_Success(t *testing.T) {
	t.Run("creates HTTP server with valid config", func(t *testing.T) {
		// Arrange
		cfg := &Config{
			ServerAddress: ":8080",
		}
		app := fiber.New()
		logger := testutil.NewMockLogger()
		telemetry := &libOtel.Telemetry{}

		// Act
		server, err := NewHTTPServer(cfg, app, logger, telemetry)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, server)
		assert.Equal(t, ":8080", server.ServerAddress())
		assert.NotNil(t, server.app)
		assert.NotNil(t, server.logger)
		assert.NotNil(t, server.telemetry)
	})
}

func TestNewHTTPServer_Error_NilTelemetry(t *testing.T) {
	t.Run("returns error when telemetry is nil", func(t *testing.T) {
		// Arrange
		cfg := &Config{
			ServerAddress: ":8080",
		}
		app := fiber.New()
		logger := testutil.NewMockLogger()

		// Act
		server, err := NewHTTPServer(cfg, app, logger, nil)

		// Assert
		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "telemetry must not be nil")
	})
}

func TestNewHTTPServer_Error_NilApp(t *testing.T) {
	t.Run("returns error when app is nil", func(t *testing.T) {
		cfg := &Config{ServerAddress: ":8080"}
		logger := testutil.NewMockLogger()
		telemetry := &libOtel.Telemetry{}

		server, err := NewHTTPServer(cfg, nil, logger, telemetry)

		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "app must not be nil")
	})
}

func TestNewHTTPServer_Error_NilLogger(t *testing.T) {
	t.Run("returns error when logger is nil", func(t *testing.T) {
		cfg := &Config{ServerAddress: ":8080"}
		app := fiber.New()
		telemetry := &libOtel.Telemetry{}

		server, err := NewHTTPServer(cfg, app, nil, telemetry)

		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "logger must not be nil")
	})
}

func TestNewHTTPServer_Error_NilConfig(t *testing.T) {
	t.Run("returns error when config is nil", func(t *testing.T) {
		app := fiber.New()
		logger := testutil.NewMockLogger()
		telemetry := &libOtel.Telemetry{}

		server, err := NewHTTPServer(nil, app, logger, telemetry)

		require.Error(t, err)
		assert.Nil(t, server)
		assert.Contains(t, err.Error(), "config must not be nil")
	})
}
