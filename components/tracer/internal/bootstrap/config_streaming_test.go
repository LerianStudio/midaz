// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamingEnvVars is the exact set of STREAMING_* keys these tests touch. Tests
// clear these to a known baseline so a leaked env var from the surrounding
// shell/CI cannot make a default-path assertion flaky.
var streamingEnvVars = []string{
	"STREAMING_ENABLED",
	"STREAMING_BROKERS",
	"STREAMING_CLOUDEVENTS_SOURCE",
	"STREAMING_TLS_ENABLED",
	"STREAMING_TLS_CA_CERT",
	"STREAMING_SASL_MECHANISM",
	"STREAMING_SASL_USERNAME",
	"STREAMING_SASL_PASSWORD",
	"STREAMING_SASL_ALLOW_PLAINTEXT",
	"STREAMING_ALLOW_PLAINTEXT_SASL",
}

// clearStreamingEnv resets every tracked STREAMING_* key to empty so a leaked
// value from the surrounding shell or CI cannot influence an assertion.
// t.Setenv restores prior values on cleanup.
func clearStreamingEnv(t *testing.T) {
	t.Helper()

	for _, ev := range streamingEnvVars {
		t.Setenv(ev, "")
	}
}

// TestConfig_StreamingDefaults verifies the safe-by-default posture: with every
// STREAMING_* env var unset, a loaded Config has streaming disabled, so a
// deployment that never sets these vars is not broken by the new dependency.
func TestConfig_StreamingDefaults(t *testing.T) {
	clearStreamingEnv(t)

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	assert.False(t, cfg.StreamingEnabled, "STREAMING_ENABLED must default to false")
	assert.Empty(t, cfg.StreamingCloudEventsSource, "STREAMING_CLOUDEVENTS_SOURCE must default to empty")
}

// TestConfig_StreamingParsesEnvVars verifies the two streaming fields the tracer
// Config binds are read from their environment variables. Transport, TLS and SASL
// knobs are deliberately absent from this struct — they belong to
// libStreaming.LoadConfig, which TestStreamingLoadConfig_ReadsTLSAndSASL covers.
func TestConfig_StreamingParsesEnvVars(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")

	cfg := &Config{}
	require.NoError(t, libCommons.SetConfigFromEnvVars(cfg))

	assert.True(t, cfg.StreamingEnabled, "STREAMING_ENABLED should parse to true")
	assert.Equal(t, "tracer", cfg.StreamingCloudEventsSource)
}

// TestStreamingLoadConfig_ReadsTLSAndSASL proves the canonical reader picks up
// every broker-security knob the tracer relies on, with no broker needed.
//
// This is the regression lock for the defect where the tracer bound its own
// STREAMING_SASL_* struct fields and hand-rolled the franz-go mechanism: that
// struct had no TLS field at all, so STREAMING_TLS_ENABLED had no reader and a
// TLS broker was unreachable. Reading them through LoadConfig — the value the
// Builder's TLSFromConfig / SASLFromConfig consume — is what makes the knobs real.
func TestStreamingLoadConfig_ReadsTLSAndSASL(t *testing.T) {
	clearStreamingEnv(t)
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "broker-a:9092,broker-b:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")
	t.Setenv("STREAMING_TLS_ENABLED", "true")
	t.Setenv("STREAMING_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("STREAMING_SASL_USERNAME", "tracer-user")
	t.Setenv("STREAMING_SASL_PASSWORD", "tracer-secret")
	t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "false")

	streamingCfg, _, err := libStreaming.LoadConfig()
	require.NoError(t, err)

	assert.Equal(t, []string{"broker-a:9092", "broker-b:9092"}, streamingCfg.Brokers)
	assert.True(t, streamingCfg.TLSEnabled, "STREAMING_TLS_ENABLED must reach the broker dial config")
	assert.Equal(t, "SCRAM-SHA-512", streamingCfg.SASLMechanism)
	assert.Equal(t, "tracer-user", streamingCfg.SASLUsername)
	assert.Equal(t, "tracer-secret", streamingCfg.SASLPassword)
	assert.False(t, streamingCfg.SASLAllowPlaintext,
		"the unsafe plaintext-SASL opt-in must stay closed unless explicitly enabled")

	// A TLS-enabled config must yield a real *tls.Config, which is what satisfies
	// lib-streaming's fail-closed SASL-requires-TLS gate at Build.
	tlsCfg, err := streamingCfg.BuildTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, tlsCfg, "STREAMING_TLS_ENABLED=true must build a TLS dial config")
}

// TestStreamingLoadConfig_PlaintextSASLCanonicalAndDeprecatedNames proves the
// canonical STREAMING_SASL_ALLOW_PLAINTEXT is honoured and the deprecated
// STREAMING_ALLOW_PLAINTEXT_SASL alias still works with a migration warning. The
// tracer previously bound ONLY the deprecated name in its own struct, so an
// operator setting the canonical one got silence.
func TestStreamingLoadConfig_PlaintextSASLCanonicalAndDeprecatedNames(t *testing.T) {
	baseEnv := func(t *testing.T) {
		t.Helper()
		clearStreamingEnv(t)
		t.Setenv("STREAMING_ENABLED", "true")
		t.Setenv("STREAMING_BROKERS", "broker-a:9092")
		t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")
	}

	t.Run("canonical name", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "true")

		streamingCfg, warnings, err := libStreaming.LoadConfig()
		require.NoError(t, err)
		assert.True(t, streamingCfg.SASLAllowPlaintext)
		assert.NotContains(t, warnings, "STREAMING_ALLOW_PLAINTEXT_SASL is deprecated; use STREAMING_SASL_ALLOW_PLAINTEXT")
	})

	t.Run("deprecated alias warns", func(t *testing.T) {
		baseEnv(t)
		t.Setenv("STREAMING_ALLOW_PLAINTEXT_SASL", "true")

		streamingCfg, warnings, err := libStreaming.LoadConfig()
		require.NoError(t, err)
		assert.True(t, streamingCfg.SASLAllowPlaintext)
		assert.Contains(t, warnings, "STREAMING_ALLOW_PLAINTEXT_SASL is deprecated; use STREAMING_SASL_ALLOW_PLAINTEXT")
	})
}
