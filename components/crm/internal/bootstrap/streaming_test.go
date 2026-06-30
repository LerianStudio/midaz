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
// brokers still running without auth.
func TestResolveSASLMechanism_Disabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "empty", cfg: &Config{}},
		{name: "whitespace only", cfg: &Config{StreamingSASLMechanism: "   \t  "}},
		{
			// USERNAME/PASSWORD set without a MECHANISM is also treated as
			// disabled — operators who half-configure SASL from the bottom up
			// shouldn't accidentally activate auth.
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

// TestResolveSASLMechanism_Supported covers every accepted mechanism string,
// including case-insensitive matching, and asserts that the returned
// canonical name matches the upper-case form used in the bootstrap log line.
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
				StreamingSASLUsername:  "crm-prod",
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

// TestResolveSASLMechanism_MissingCredentials guarantees the resolver fails
// closed when MECHANISM is set but USERNAME or PASSWORD is empty.
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

// TestResolveSASLMechanism_Unsupported guards against typos and unsupported
// mechanisms. The error message must enumerate the accepted values.
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

// TestBuildStreamingEmitter_NilConfig documents the nil-guard contract:
// BuildStreamingEmitter must never panic on a nil config and always returns a
// non-nil no-op closer the caller can invoke unconditionally.
func TestBuildStreamingEmitter_NilConfig(t *testing.T) {
	t.Parallel()

	emitter, closer, err := BuildStreamingEmitter(context.Background(), nil, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_DisabledReturnsNoop covers the default pilot path:
// STREAMING_ENABLED is false, so the emitter is the no-op, a non-nil no-op
// closer is returned, and no broker connection is attempted regardless of the
// SASL config. This is the contract that keeps CRM behaving exactly as today.
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
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EmptyBrokersFailsClosed locks the
// enabled-but-misconfigured contract: STREAMING_ENABLED=true with a
// STREAMING_BROKERS value that resolves to an empty broker list fails the
// build. In lib-streaming v1.4.0 LoadConfig itself rejects empty brokers when
// ENABLED=true (ErrMissingBrokers), so the error surfaces from LoadConfig
// before reaching the local broker-fallback guard. Either way BuildStreamingEmitter
// must return an error, a nil emitter, and a non-nil no-op closer the caller
// can invoke unconditionally on the failure path.
func TestBuildStreamingEmitter_EmptyBrokersFailsClosed(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "  ,  ")
	// LoadConfig validates ce-source too; set it so the broker check is the
	// only thing under test.
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.Error(t, err)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EnabledEmptyCatalogReturnsNoop locks the
// empty-catalog footgun fix: STREAMING_ENABLED=true with valid brokers but no
// registered events must fall back to a NoopEmitter rather than crashing
// bootstrap on the Builder's empty-routes/empty-catalog rejection. The guard
// runs AFTER LoadConfig + broker validation, so this only fires once brokers
// are present. It returns immediately because the emitter is a no-op — no
// broker dial is attempted. Guarded on the empty catalog so it auto-disables
// once Epic 1.2 registers the first event.
func TestBuildStreamingEmitter_EnabledEmptyCatalogReturnsNoop(t *testing.T) {
	if len(crmEventDefinitions()) != 0 {
		t.Skip("crmEventDefinitions() is no longer empty; empty-catalog fallback is unreachable")
	}

	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	// LoadConfig validates ce-source; set it so LoadConfig does NOT error and the
	// empty-catalog guard is the path under test.
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestCRMCatalogRoutesAssembly locks the catalog/routes assembly path: it must
// build without error from whatever crmEventDefinitions() currently returns.
// Epic 1.1 ships an empty definition slice (holder/alias events land in a later
// epic), so the assembled catalog/routes are empty but well-formed. buildCatalog
// is exercised directly because the full Builder.Build path legitimately rejects
// empty input — NewRouteTable rejects the empty ROUTES first (no-routes-configured)
// before the catalog check is reached — and that path is reached only once events
// are registered.
func TestCRMCatalogRoutesAssembly(t *testing.T) {
	t.Parallel()

	defs := crmEventDefinitions()

	catalog, err := buildCatalog()
	require.NoError(t, err)
	assert.Equal(t, len(defs), catalog.Len())

	routes := buildRoutes(streamingPrimaryTargetName)
	assert.Len(t, routes, len(defs))

	// Every assembled route must carry the canonical topic prefix and the
	// shared primary target so the wiring stays consistent once events land.
	for _, r := range routes {
		assert.Equal(t, streamingPrimaryTargetName, r.Target)
		assert.True(t, strings.HasPrefix(r.Destination.Name, streamingTopicPrefix),
			"route destination %q must start with %q", r.Destination.Name, streamingTopicPrefix)
	}
}

// TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed is the integration guard:
// when SASL is enabled but neither TLS nor AllowPlaintextSASL is set,
// lib-streaming must reject construction with ErrPlaintextSASLNotAllowed. This
// locks the contract that CRM never silently downgrades a SASL config to
// plaintext without explicit opt-in.
//
// NOTE: this path is only reachable once crmEventDefinitions() registers at
// least one event. With no events the empty-catalog guard short-circuits to a
// NoopEmitter before Build, and even reaching Build the Builder rejects the
// empty ROUTES first (NewRouteTable: no-routes-configured) ahead of the SASL/TLS
// check. It is skipped while the catalog is empty so Epic 1.1 stays green; it
// activates automatically when Epic 1.2 lands the first holder event.
func TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed(t *testing.T) {
	if len(crmEventDefinitions()) == 0 {
		t.Skip("crmEventDefinitions() is empty; SASL/TLS validation is unreachable until events are registered")
	}

	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")

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
