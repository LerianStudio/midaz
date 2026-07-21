// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingEnvVars is the exact set of STREAMING_* keys the tracer Config binds.
// Tests clear these to a known baseline so a leaked env var from the surrounding
// shell/CI cannot make a default-path assertion flaky.
var streamingEnvVars = []string{
	"STREAMING_ENABLED",
	"STREAMING_SASL_MECHANISM",
	"STREAMING_SASL_USERNAME",
	"STREAMING_SASL_PASSWORD",
	"STREAMING_ALLOW_PLAINTEXT_SASL",
}

// TestConfig_StreamingDefaults verifies the safe-by-default posture: with every
// STREAMING_* env var unset (empty), a loaded Config has streaming disabled and
// all SASL fields empty, so a deployment that never sets these vars is not
// broken by the new dependency.
func TestConfig_StreamingDefaults(t *testing.T) {
	// Setenv every streaming key to empty to exercise the default path
	// deterministically. t.Setenv restores prior values on cleanup.
	for _, ev := range streamingEnvVars {
		t.Setenv(ev, "")
	}

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	assert.False(t, cfg.StreamingEnabled, "STREAMING_ENABLED must default to false")
	assert.Empty(t, cfg.StreamingSASLMechanism, "STREAMING_SASL_MECHANISM must default to empty")
	assert.Empty(t, cfg.StreamingSASLUsername, "STREAMING_SASL_USERNAME must default to empty")
	assert.Empty(t, cfg.StreamingSASLPassword, "STREAMING_SASL_PASSWORD must default to empty")
	assert.False(t, cfg.StreamingAllowPlaintextSASL, "STREAMING_ALLOW_PLAINTEXT_SASL must default to false")
}

// TestConfig_StreamingParsesEnvVars verifies the five streaming fields are bound
// from their environment variables via SetConfigFromEnvVars.
func TestConfig_StreamingParsesEnvVars(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("STREAMING_SASL_USERNAME", "tracer-user")
	t.Setenv("STREAMING_SASL_PASSWORD", "tracer-secret")
	t.Setenv("STREAMING_ALLOW_PLAINTEXT_SASL", "true")

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	assert.True(t, cfg.StreamingEnabled, "STREAMING_ENABLED should parse to true")
	assert.Equal(t, "SCRAM-SHA-512", cfg.StreamingSASLMechanism)
	assert.Equal(t, "tracer-user", cfg.StreamingSASLUsername)
	assert.Equal(t, "tracer-secret", cfg.StreamingSASLPassword)
	assert.True(t, cfg.StreamingAllowPlaintextSASL, "STREAMING_ALLOW_PLAINTEXT_SASL should parse to true")
}
