// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

// TestNewServer_UsesConfiguredAddress asserts that NewServer reflects the
// user-supplied address on the returned Server.
func TestNewServer_UsesConfiguredAddress(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	telemetry := &libOpentelemetry.Telemetry{}
	app := fiber.New()

	cfg := &Config{ServerAddress: ":9001"}

	srv := NewServer(cfg, app, logger, telemetry)
	require.NotNil(t, srv)

	assert.Equal(t, ":9001", srv.ServerAddress())
}

// TestNewServer_DefaultsAddressWhenEmpty asserts that NewServer falls back
// to the documented default ":3002" when cfg.ServerAddress is empty.
// This guards the contract advertised in the Config struct literal default.
func TestNewServer_DefaultsAddressWhenEmpty(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	telemetry := &libOpentelemetry.Telemetry{}
	app := fiber.New()

	cfg := &Config{ServerAddress: ""}

	srv := NewServer(cfg, app, logger, telemetry)
	require.NotNil(t, srv)

	assert.Equal(t, ":3002", srv.ServerAddress())
}
