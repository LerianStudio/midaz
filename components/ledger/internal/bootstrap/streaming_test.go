// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveSASLMechanism_Disabled covers the default code path: when
// STREAMING_SASL_MECHANISM is empty (or whitespace) the resolver returns a
// nil mechanism and an empty name, signalling that the Builder should be
// left unauthenticated. This is the back-compat contract for local/dev
// brokers that are still running without auth.
func TestResolveSASLMechanism_Disabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "empty", cfg: &Config{}},
		{name: "whitespace only", cfg: &Config{StreamingSASLMechanism: "   \t  "}},
		{
			// Ensure that USERNAME/PASSWORD set without a MECHANISM is also
			// treated as disabled — operators who half-configure SASL from
			// the bottom up shouldn't accidentally activate auth.
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
// string, including case-insensitive matching, and asserts that the
// returned canonical name matches the upper-case form used in the
// bootstrap log line.
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
				StreamingSASLUsername:  "ledger-prod",
				StreamingSASLPassword:  "s3cret",
			}

			mech, name, err := resolveSASLMechanism(cfg)
			require.NoError(t, err)
			require.NotNil(t, mech)
			assert.Equal(t, tc.expectedName, name)
			// franz-go's Mechanism.Name() returns the wire-format mechanism
			// string, which should match the canonical name 1:1.
			assert.Equal(t, tc.expectedName, mech.Name())
		})
	}
}

// TestResolveSASLMechanism_MissingCredentials guarantees the resolver
// fails closed when MECHANISM is set but USERNAME or PASSWORD is empty.
// SASL with empty credentials would either be rejected by the broker
// after I/O (PLAIN) or panic deep inside franz-go's SCRAM handshake;
// failing at bootstrap is the safer contract.
func TestResolveSASLMechanism_MissingCredentials(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "missing both",
			cfg:  &Config{StreamingSASLMechanism: "PLAIN"},
		},
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
// unsupported mechanisms (OAUTHBEARER, GSSAPI, etc). The error message
// must enumerate the accepted values so an operator can fix it without
// reading the source.
func TestResolveSASLMechanism_Unsupported(t *testing.T) {
	t.Parallel()

	cases := []string{
		"OAUTHBEARER",
		"GSSAPI",
		"SCRAM",         // missing -SHA-xxx suffix
		"SCRAM-SHA-1",   // not supported by franz-go
		"plain-sha-256", // mangled
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

// TestBuildStreamingEmitter_NilConfig keeps the existing nil-guard
// contract documented as a unit test: BuildStreamingEmitter must never
// panic on a nil config.
func TestBuildStreamingEmitter_NilConfig(t *testing.T) {
	t.Parallel()

	emitter, closer, err := BuildStreamingEmitter(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_DisabledReturnsNoop covers the default
// pilot path: STREAMING_ENABLED is false, the emitter is the no-op,
// and no broker connection is attempted regardless of the SASL config.
func TestBuildStreamingEmitter_DisabledReturnsNoop(t *testing.T) {
	// t.Setenv prevents t.Parallel — we mutate process env.
	t.Setenv("STREAMING_ENABLED", "false")

	cfg := &Config{
		StreamingEnabled:       false,
		StreamingSASLMechanism: "PLAIN", // ignored when disabled
		StreamingSASLUsername:  "u",
		StreamingSASLPassword:  "p",
	}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	t.Cleanup(func() { _ = closer() })
}

// TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed is the integration
// guard: when SASL is enabled but neither TLS nor AllowPlaintextSASL is
// set, lib-streaming must reject construction with
// ErrPlaintextSASLNotAllowed. This locks the contract that midaz never
// silently downgrades a SASL config to plaintext without explicit opt-in.
//
// The test does NOT dial the broker — Build() validates the option
// combination before any network I/O, so STREAMING_BROKERS can point at
// a non-routable host.
func TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")

	cfg := &Config{
		StreamingEnabled:       true,
		StreamingSASLMechanism: "PLAIN",
		StreamingSASLUsername:  "u",
		StreamingSASLPassword:  "p",
		// StreamingAllowPlaintextSASL intentionally false.
	}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
	assert.True(t, errors.Is(err, libStreaming.ErrPlaintextSASLNotAllowed),
		"expected ErrPlaintextSASLNotAllowed, got %v", err)
}

// TestBuildStreamingEmitter_SASLWithAllowPlaintextSucceeds is the
// dev-broker happy path: SASL enabled + AllowPlaintextSASL=true + no TLS
// builds an emitter without dialling the broker. The emitter is closed
// by t.Cleanup so franz-go's background goroutines do not leak into
// other tests in this package.
func TestBuildStreamingEmitter_SASLWithAllowPlaintextSucceeds(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")

	cfg := &Config{
		StreamingEnabled:            true,
		StreamingSASLMechanism:      "SCRAM-SHA-256",
		StreamingSASLUsername:       "u",
		StreamingSASLPassword:       "p",
		StreamingAllowPlaintextSASL: true,
	}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	t.Cleanup(func() { _ = closer() })
}

// TestBuildStreamingEmitter_UnsupportedMechanismFailsClosed verifies
// that resolveSASLMechanism's unsupported-mechanism error propagates
// out of BuildStreamingEmitter rather than getting masked by a downstream
// builder error.
func TestBuildStreamingEmitter_UnsupportedMechanismFailsClosed(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")

	cfg := &Config{
		StreamingEnabled:            true,
		StreamingSASLMechanism:      "OAUTHBEARER", // not on the allow-list
		StreamingSASLUsername:       "u",
		StreamingSASLPassword:       "p",
		StreamingAllowPlaintextSASL: true,
	}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
	assert.True(t,
		strings.Contains(err.Error(), "OAUTHBEARER") ||
			strings.Contains(err.Error(), "not supported"),
		"expected unsupported-mechanism error, got %v", err)
}
