// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopEmitterReference is a NoopEmitter constructed independently of the
// code under test. Its concrete type is what BuildStreamingEmitter must
// return on every disabled/fallback branch, so tests assert type identity
// against it rather than dialling a broker or inspecting private state.
var noopEmitterReference = libStreaming.NewNoopEmitter()

// TestTracerEventDefinitions_EmptyInPhase1 locks the Phase-1 contract:
// Tracer emits no events yet, so the catalog source is empty. This is the
// guard that forces BuildStreamingEmitter onto the NoopEmitter path even
// when streaming is enabled with valid brokers — an empty catalog can
// never build a live producer.
func TestTracerEventDefinitions_EmptyInPhase1(t *testing.T) {
	t.Parallel()

	assert.Empty(t, tracerEventDefinitions(),
		"tracerEventDefinitions must be empty in Phase 1; Tracer events land in Phases 2/3")
}

// TestBuildStreamingEmitter covers the three NoopEmitter fallback branches.
// None of them opens a broker connection, so the test runs without any
// local Kafka/Redpanda dependency.
func TestBuildStreamingEmitter(t *testing.T) {
	cases := []struct {
		name    string
		envVars map[string]string
		cfg     *Config
	}{
		{
			// Branch 1: master flag off. Returns the noop without loading
			// libStreaming.LoadConfig or touching env.
			name: "disabled returns noop",
			envVars: map[string]string{
				"STREAMING_ENABLED": "false",
			},
			cfg: &Config{
				StreamingEnabled: false,
				// SASL fields ignored when disabled.
				StreamingSASLMechanism: "PLAIN",
				StreamingSASLUsername:  "u",
				StreamingSASLPassword:  "p",
			},
		},
		{
			// Branch 2: enabled but STREAMING_BROKERS empty after
			// LoadConfig — the Builder would reject construction, so we
			// fall back to the noop.
			name: "enabled with empty brokers returns noop",
			envVars: map[string]string{
				"STREAMING_ENABLED":            "true",
				"STREAMING_BROKERS":            "",
				"STREAMING_CLOUDEVENTS_SOURCE": "lerian.midaz.tracer.test",
			},
			cfg: &Config{
				StreamingEnabled: true,
			},
		},
		{
			// Branch 3: enabled with valid brokers, but the empty
			// tracerEventDefinitions() catalog guard short-circuits to the
			// noop. This is the live Phase-1 path.
			name: "enabled with brokers but empty catalog returns noop",
			envVars: map[string]string{
				"STREAMING_ENABLED":            "true",
				"STREAMING_BROKERS":            "127.0.0.1:0",
				"STREAMING_CLOUDEVENTS_SOURCE": "lerian.midaz.tracer.test",
			},
			cfg: &Config{
				StreamingEnabled: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv forbids t.Parallel — these mutate process env.
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}

			emitter, closer, err := BuildStreamingEmitter(context.Background(), tc.cfg, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, emitter)
			require.NotNil(t, closer)

			// Every fallback path returns the concrete NoopEmitter type.
			assert.IsType(t, noopEmitterReference, emitter,
				"expected NoopEmitter on the fallback path")

			// The noop closer never errors and never blocks.
			assert.NoError(t, closer())
		})
	}
}

// TestBuildStreamingEmitter_NilConfig documents the nil-guard contract:
// BuildStreamingEmitter must return an error (never panic) and a non-nil
// closer that is safe to invoke.
func TestBuildStreamingEmitter_NilConfig(t *testing.T) {
	t.Parallel()

	emitter, closer, err := BuildStreamingEmitter(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildLiveStreamingEmitter_EmptyCatalogFailsClosed proves why the
// empty-catalog guard in BuildStreamingEmitter is load-bearing: if the
// live-producer path is ever reached with the Phase-1 (empty) catalog,
// lib-streaming's Builder rejects construction rather than silently
// producing a dead emitter. The test calls the helper directly (the public
// entry point short-circuits to Noop before reaching it) and asserts a
// wrapped error plus a safe closer — never a panic.
func TestBuildLiveStreamingEmitter_EmptyCatalogFailsClosed(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.tracer.test")

	streamingCfg, _, err := libStreaming.LoadConfig()
	require.NoError(t, err)
	require.NotEmpty(t, streamingCfg.Brokers)

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := buildLiveStreamingEmitter(context.Background(), cfg, nil, streamingCfg)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildCatalog_EmptyInPhase1 exercises buildCatalog against the empty
// Phase-1 definition set: it must succeed and produce a zero-length
// catalog. This is the helper the live path uses once tracer events land,
// so keeping it green now guards against Catalog construction regressions.
func TestBuildCatalog_EmptyInPhase1(t *testing.T) {
	t.Parallel()

	catalog, err := buildCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	assert.Equal(t, 0, catalog.Len(), "Phase-1 catalog must be empty")
}

// TestBuildRoutes_EmptyInPhase1 exercises buildRoutes against the empty
// Phase-1 definition set: one route per event, so zero events yields zero
// routes. Locks the route-derivation shape ahead of Phase 2/3 events.
func TestBuildRoutes_EmptyInPhase1(t *testing.T) {
	t.Parallel()

	routes := buildRoutes(streamingPrimaryTargetName)
	assert.Empty(t, routes, "Phase-1 route table must be empty")
}

// TestResolveStreamingSource locks the CloudEvents source resolution
// contract: a trimmed, non-empty STREAMING_CLOUDEVENTS_SOURCE value wins;
// an empty, whitespace-only, or nil config falls back to the in-code
// streamingSource default so an unset var never changes historical
// behaviour.
func TestResolveStreamingSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cfg      *Config
		expected string
	}{
		{
			name:     "nil config falls back to default",
			cfg:      nil,
			expected: streamingSource,
		},
		{
			name:     "empty value falls back to default",
			cfg:      &Config{StreamingCloudEventsSource: ""},
			expected: streamingSource,
		},
		{
			name:     "whitespace-only value falls back to default",
			cfg:      &Config{StreamingCloudEventsSource: "  \t  "},
			expected: streamingSource,
		},
		{
			name:     "configured value wins",
			cfg:      &Config{StreamingCloudEventsSource: "lerian.midaz.tracer.staging"},
			expected: "lerian.midaz.tracer.staging",
		},
		{
			name:     "configured value is trimmed",
			cfg:      &Config{StreamingCloudEventsSource: "  lerian.midaz.tracer.shadow  "},
			expected: "lerian.midaz.tracer.shadow",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, resolveStreamingSource(tc.cfg))
		})
	}
}

// TestResolveSASLMechanism_Disabled covers the default code path: an empty
// (or whitespace) STREAMING_SASL_MECHANISM yields a nil mechanism and
// empty name so the Builder is left unauthenticated — the back-compat
// contract for local/dev brokers running without auth.
func TestResolveSASLMechanism_Disabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "empty", cfg: &Config{}},
		{name: "whitespace only", cfg: &Config{StreamingSASLMechanism: "   \t  "}},
		{
			name: "credentials without mechanism",
			cfg: &Config{
				StreamingSASLUsername: "u",
				StreamingSASLPassword: "p",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mech, name, err := resolveSASLMechanism(tc.cfg)
			require.NoError(t, err)
			assert.Nil(t, mech)
			assert.Empty(t, name)
		})
	}
}

// TestResolveSASLMechanism_Supported covers every accepted mechanism
// string, including case-insensitive matching, and asserts the returned
// canonical name matches the upper-case form used in the bootstrap log.
func TestResolveSASLMechanism_Supported(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input        string
		expectedName string
	}{
		{input: "PLAIN", expectedName: saslMechanismPlain},
		{input: "plain", expectedName: saslMechanismPlain},
		{input: "  PLAIN  ", expectedName: saslMechanismPlain},
		{input: "SCRAM-SHA-256", expectedName: saslMechanismScram256},
		{input: "scram-sha-256", expectedName: saslMechanismScram256},
		{input: "SCRAM-SHA-512", expectedName: saslMechanismScram512},
		{input: "scram-sha-512", expectedName: saslMechanismScram512},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				StreamingSASLMechanism: tc.input,
				StreamingSASLUsername:  "tracer-prod",
				StreamingSASLPassword:  "s3cret",
			}

			mech, name, err := resolveSASLMechanism(cfg)
			require.NoError(t, err)
			require.NotNil(t, mech)
			assert.Equal(t, tc.expectedName, name)
			assert.Equal(t, tc.expectedName, mech.Name())
		})
	}
}

// TestResolveSASLMechanism_MissingCredentials guarantees the resolver
// fails closed when MECHANISM is set but USERNAME or PASSWORD is empty.
func TestResolveSASLMechanism_MissingCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "missing both", cfg: &Config{StreamingSASLMechanism: "PLAIN"}},
		{
			name: "missing username",
			cfg: &Config{
				StreamingSASLMechanism: "SCRAM-SHA-256",
				StreamingSASLPassword:  "p",
			},
		},
		{
			name: "missing password",
			cfg: &Config{
				StreamingSASLMechanism: "SCRAM-SHA-512",
				StreamingSASLUsername:  "u",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mech, name, err := resolveSASLMechanism(tc.cfg)
			require.Error(t, err)
			assert.Nil(t, mech)
			assert.Empty(t, name)
			assert.Contains(t, err.Error(), "STREAMING_SASL_USERNAME")
			assert.Contains(t, err.Error(), "STREAMING_SASL_PASSWORD")
		})
	}
}

// TestResolveSASLMechanism_Unsupported guards against typos and
// unsupported mechanisms; the error must enumerate the accepted values.
func TestResolveSASLMechanism_Unsupported(t *testing.T) {
	t.Parallel()

	cases := []string{
		"OAUTHBEARER",
		"GSSAPI",
		"SCRAM",
		"SCRAM-SHA-1",
		"plain-sha-256",
		"unknown",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				StreamingSASLMechanism: input,
				StreamingSASLUsername:  "u",
				StreamingSASLPassword:  "p",
			}

			mech, name, err := resolveSASLMechanism(cfg)
			require.Error(t, err)
			assert.Nil(t, mech)
			assert.Empty(t, name)
			assert.Contains(t, err.Error(), saslMechanismPlain)
			assert.Contains(t, err.Error(), saslMechanismScram256)
			assert.Contains(t, err.Error(), saslMechanismScram512)
		})
	}
}
