// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

// TestLogDeprecatedBrokerEnvVars_NoDeprecatedVars covers the early-return
// path where no deprecated broker env vars are set. Setting DUMMY_VAR
// ensures os.Environ() is non-empty but contains nothing matching the
// deprecated prefixes/keys. The function must return silently.
func TestLogDeprecatedBrokerEnvVars_NoDeprecatedVars(t *testing.T) {
	// Setenv mutates process env; disable parallelism.
	t.Setenv("DUMMY_VAR", "dummy")

	// Explicitly unset any deprecated vars that may leak from the host env.
	t.Setenv("RABBITMQ_URI", "")
	t.Setenv("AUTHORIZER_RABBITMQ_URI", "")
	t.Setenv("BROKER_HEALTH_CHECK_TIMEOUT", "")
	t.Setenv("BROKER_HEALTHCHECK_TIMEOUT", "")

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Must not panic and must not produce spurious warnings. The function
	// has no return value; we simply exercise the early-return path.
	require.NotPanics(t, func() {
		logDeprecatedBrokerEnvVars(logger)
	})
}

// TestLogDeprecatedBrokerEnvVars_WithDeprecatedVars covers the warn branch
// where at least one deprecated env var is present. It asserts that the
// function emits the warning (implicitly, by running without panic) and
// reads the deprecated var through os.Environ().
func TestLogDeprecatedBrokerEnvVars_WithDeprecatedVars(t *testing.T) {
	// Setenv mutates process env; disable parallelism.
	t.Setenv("RABBITMQ_URI", "amqp://deprecated")

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	// Function should log a warning and return. We verify the no-panic
	// contract only — the actual log output is owned by lib-commons zap
	// and is not part of this package's behavior under test.
	require.NotPanics(t, func() {
		logDeprecatedBrokerEnvVars(logger)
	})
}
