// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopEmitterReference is a NoopEmitter constructed independently of the
// code under test. Its concrete type is what BuildStreamingEmitter must
// return on every disabled/fallback branch, so tests assert type identity
// against it rather than dialling a broker or inspecting private state.
var noopEmitterReference = libStreaming.NewNoopEmitter()

// expectedRuleEventKeys is the canonical set of event keys tracer registers
// for the Rule lifecycle (Phase 2). Limit events (Phase 3) extend this set.
var expectedRuleEventKeys = []string{
	"rule.created",
	"rule.updated",
	"rule.activated",
	"rule.deactivated",
	"rule.drafted",
	"rule.deleted",
}

// TestTracerEventDefinitions_CoversRuleLifecycle locks the Phase-2 contract:
// tracerEventDefinitions() registers exactly the six Rule lifecycle events,
// in the fixed order, with no extra and none missing. This is the single
// source of truth that feeds both the catalog and the routes.
func TestTracerEventDefinitions_CoversRuleLifecycle(t *testing.T) {
	t.Parallel()

	defs := tracerEventDefinitions()
	require.Len(t, defs, len(expectedRuleEventKeys),
		"tracerEventDefinitions must register exactly the six Rule lifecycle events")

	actualKeys := make([]string, 0, len(defs))
	for _, d := range defs {
		actualKeys = append(actualKeys, d.Key())
	}

	// Order is part of the contract (created, updated, activated,
	// deactivated, drafted, deleted).
	assert.Equal(t, expectedRuleEventKeys, actualKeys,
		"tracerEventDefinitions must return the Rule events in the fixed order")
}

// TestBuildStreamingEmitter_DisabledReturnsNoop covers the master-flag-off
// branch: BuildStreamingEmitter returns the concrete NoopEmitter without
// loading libStreaming.LoadConfig or touching a broker. SASL fields are
// ignored on this path.
func TestBuildStreamingEmitter_DisabledReturnsNoop(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "false")

	cfg := &Config{
		StreamingEnabled: false,
		// SASL fields ignored when disabled.
		StreamingSASLMechanism: "PLAIN",
		StreamingSASLUsername:  "u",
		StreamingSASLPassword:  "p",
	}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)

	assert.IsType(t, noopEmitterReference, emitter,
		"disabled streaming must return the NoopEmitter")
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EnabledMissingBrokersDegradesToNoop asserts the
// graceful-degradation contract: with STREAMING_ENABLED=true and no
// STREAMING_BROKERS, libStreaming.LoadConfig returns ErrMissingBrokers, which
// BuildStreamingEmitter recognises as a caller-correctable misconfiguration
// and degrades to a NoopEmitter (no error, safe closer) rather than aborting
// bootstrap. A missing broker list is an operator-fixable condition, not a
// reason to prevent the service from starting; the Warn log records the
// degradation. Any OTHER LoadConfig failure still propagates as a wrapped
// error.
func TestBuildStreamingEmitter_EnabledMissingBrokersDegradesToNoop(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.tracer.test")

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)

	assert.IsType(t, noopEmitterReference, emitter,
		"missing brokers must degrade to the NoopEmitter, not fail closed")
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EnabledWithRuleCatalogBuildsLive proves that once
// the Rule catalog is populated, an enabled + brokered config no longer
// short-circuits to the noop: BuildStreamingEmitter constructs a real
// producer (not a NoopEmitter) and returns a working closer. The broker
// address is intentionally unresolvable (127.0.0.1:0); lib-streaming dials
// asynchronously, so Build succeeds without a live broker and no real
// network I/O occurs during the test.
func TestBuildStreamingEmitter_EnabledWithRuleCatalogBuildsLive(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.tracer.test")

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)

	// With the Rule catalog populated the emitter must be a real producer,
	// never the NoopEmitter fallback.
	assert.NotEqual(t, noopEmitterReference, emitter,
		"expected a live emitter once the Rule catalog is populated")

	assert.NoError(t, closer())
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

// TestBuildLiveStreamingEmitter_BuildsWithRuleCatalog proves the live path
// constructs a real producer once the Rule catalog is populated. The helper
// derives its catalog from tracerEventDefinitions() internally, so this
// asserts the six-event catalog builds a non-nil emitter without a panic and
// with a safe closer. The unresolvable broker address (127.0.0.1:0) keeps
// the dial asynchronous so no real network I/O occurs.
func TestBuildLiveStreamingEmitter_BuildsWithRuleCatalog(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:0")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.tracer.test")

	streamingCfg, _, err := libStreaming.LoadConfig()
	require.NoError(t, err)
	require.NotEmpty(t, streamingCfg.Brokers)

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := buildLiveStreamingEmitter(context.Background(), cfg, nil, streamingCfg)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	assert.NotEqual(t, noopEmitterReference, emitter,
		"live path must return a real producer, not the NoopEmitter")
	assert.NoError(t, closer())
}

// TestBuildCatalog_CoversRuleLifecycle exercises buildCatalog against the
// populated Rule definition set: it must succeed and register exactly one
// entry per Rule event, each looked up by its canonical key.
func TestBuildCatalog_CoversRuleLifecycle(t *testing.T) {
	t.Parallel()

	catalog, err := buildCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	assert.Equal(t, len(expectedRuleEventKeys), catalog.Len(),
		"catalog must hold one entry per Rule event")

	for _, key := range expectedRuleEventKeys {
		_, ok := catalog.Lookup(key)
		assert.Truef(t, ok, "catalog must register key %q", key)
	}
}

// TestBuildRoutes_CoversRuleLifecycle exercises buildRoutes against the
// populated Rule definition set: one required route per event, each keyed
// "<event>.<target>" and pointing at the canonical
// "lerian.streaming.<event>" Kafka topic.
func TestBuildRoutes_CoversRuleLifecycle(t *testing.T) {
	t.Parallel()

	routes := buildRoutes(streamingPrimaryTargetName)
	require.Len(t, routes, len(expectedRuleEventKeys),
		"one route per Rule event")

	byDefKey := make(map[string]libStreaming.RouteDefinition, len(routes))
	for _, r := range routes {
		byDefKey[r.DefinitionKey] = r
	}

	for _, key := range expectedRuleEventKeys {
		r, ok := byDefKey[key]
		require.Truef(t, ok, "missing route for %q", key)
		assert.Equal(t, key+"."+streamingPrimaryTargetName, r.Key,
			"route Key must be <event>.<target>")
		assert.Equal(t, streamingPrimaryTargetName, r.Target)
		assert.Equal(t, libStreaming.RouteRequired, r.Requirement)
		assert.Equal(t, libStreaming.KafkaTopic(streamingTopicPrefix+key), r.Destination,
			"route Destination must be the canonical Kafka topic")
	}
}

// TestTracerCatalog_CoversAllEmittedEvents is the drift lock: it asserts an
// exact 1:1:1 bijection between the registered event definitions, the
// catalog entries, and the route table — no event registered without a
// route, no route pointing at an unregistered event (ghost topic), and no
// count drift between the three. Phase 3 extends this to all 12 events.
func TestTracerCatalog_CoversAllEmittedEvents(t *testing.T) {
	t.Parallel()

	defs := tracerEventDefinitions()
	require.NotEmpty(t, defs, "tracer must register at least the Rule events")

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes := buildRoutes(streamingPrimaryTargetName)

	// (a) count parity across all three views.
	assert.Equal(t, len(defs), catalog.Len(),
		"catalog entry count must equal definition count")
	assert.Equal(t, len(defs), len(routes),
		"route count must equal definition count")

	// Definition key set (the source of truth).
	defKeys := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		defKeys[d.Key()] = struct{}{}
	}

	require.Lenf(t, defKeys, len(defs),
		"definition keys must be unique (found %d unique of %d defs)", len(defKeys), len(defs))

	// (b) every definition resolves to a catalog entry.
	for key := range defKeys {
		_, ok := catalog.Lookup(key)
		assert.Truef(t, ok, "catalog is missing a definition-registered key %q", key)
	}

	// (c) route set is a bijection with the definition set: every route
	// targets a registered definition (no ghost topics) and every
	// definition has exactly one route.
	routeKeys := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		_, dup := routeKeys[r.DefinitionKey]
		require.Falsef(t, dup, "duplicate route for definition key %q", r.DefinitionKey)

		routeKeys[r.DefinitionKey] = struct{}{}

		_, registered := defKeys[r.DefinitionKey]
		assert.Truef(t, registered,
			"route %q points at an unregistered event (ghost topic)", r.DefinitionKey)

		// (d) destination topic derives from the definition key.
		assert.Equal(t, libStreaming.KafkaTopic(streamingTopicPrefix+r.DefinitionKey), r.Destination,
			"route Destination must be lerian.streaming.<key>")
	}

	assert.Equal(t, defKeys, routeKeys,
		"route definition-key set must exactly equal the registered definition-key set")

	// Guard against a stale reference to the events package import.
	assert.Equal(t, "rule.created", events.RuleCreatedDefinition.Key())
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
