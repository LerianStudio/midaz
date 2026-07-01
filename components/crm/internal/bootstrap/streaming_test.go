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
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
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

// TestCRMCatalog_CoversAllEmittedEvents locks the bijection between the event
// keys the CRM use-case helpers actually emit and the Definitions registered in
// both the Catalog and the route table. A Go test cannot introspect which keys
// the emit<Event>Event helpers call at runtime, so the canonical set is pinned
// here as an explicit literal — this list IS the human-maintained source of
// truth the test guards the wiring against.
//
// TRIPWIRE — when a new CRM streaming event is added (a new emit<Event>Event
// helper wired to a new events.*Definition), you MUST add its Key() to
// expectedKeys below. Forgetting to register the Definition in
// crmEventDefinitions() (emitted key with no route → silent ghost-topic drop) OR
// registering a Definition no helper emits (dead route) BOTH fail this test.
func TestCRMCatalog_CoversAllEmittedEvents(t *testing.T) {
	t.Parallel()

	// The 7 canonical keys emitted by the CRM use-case helpers. Pinned as
	// literals (not derived from events.*Definition) so a mis-keyed Definition
	// var is caught here rather than silently agreeing with itself.
	expectedKeys := map[string]struct{}{
		"holder.created":              {},
		"holder.updated":              {},
		"holder.deleted":              {},
		"alias.created":               {},
		"alias.updated":               {},
		"alias.deleted":               {},
		"alias.related-party-deleted": {},
	}

	require.Len(t, expectedKeys, 7, "CRM emits exactly 7 streaming events")

	// (1) crmEventDefinitions() must contain EXACTLY the canonical key set — no
	// missing registration (forgotten event) and no extra/ghost key.
	catalogKeys := make(map[string]struct{}, len(crmEventDefinitions()))
	for _, d := range crmEventDefinitions() {
		key := d.Key()
		_, ok := catalogKeys[key]
		require.False(t, ok, "duplicate Definition key registered: %q", key)
		catalogKeys[key] = struct{}{}
	}

	assert.Equal(t, expectedKeys, catalogKeys,
		"crmEventDefinitions() must register exactly the canonical emitted keys (no missing, no ghost)")
	assert.Len(t, catalogKeys, 7)

	// (2) Every route produced by buildRoutes must correspond 1:1 to a Definition
	// key: DefinitionKey in the canonical set, destination topic == the prefixed
	// key, and no key routed twice / no key without a route.
	routes := buildRoutes(streamingPrimaryTargetName)
	assert.Len(t, routes, 7, "one route per emitted event")

	routeKeys := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		_, expected := expectedKeys[r.DefinitionKey]
		assert.True(t, expected,
			"route DefinitionKey %q is not an emitted event (dead/ghost route)", r.DefinitionKey)

		_, dup := routeKeys[r.DefinitionKey]
		assert.False(t, dup, "duplicate route for DefinitionKey %q", r.DefinitionKey)
		routeKeys[r.DefinitionKey] = struct{}{}

		assert.Equal(t, streamingTopicPrefix+r.DefinitionKey, r.Destination.Name,
			"route for %q must target topic %q", r.DefinitionKey, streamingTopicPrefix+r.DefinitionKey)
	}

	// Bijection both directions: every emitted key has a route, every route has
	// an emitted key.
	assert.Equal(t, expectedKeys, routeKeys,
		"routes must map 1:1 to emitted keys (no route without emitter, no emitter without route)")

	// (3) The hyphenated key is the easiest to typo (related-party-deleted vs
	// related_party_deleted); pin it and its topic explicitly.
	const hyphenatedKey = "alias.related-party-deleted"
	assert.Contains(t, catalogKeys, hyphenatedKey, "hyphenated alias key must be registered")

	var hyphenatedRoute *libStreaming.RouteDefinition
	for i := range routes {
		if routes[i].DefinitionKey == hyphenatedKey {
			hyphenatedRoute = &routes[i]

			break
		}
	}

	require.NotNil(t, hyphenatedRoute, "route for %q must exist", hyphenatedKey)
	assert.Equal(t, "lerian.streaming.alias.related-party-deleted", hyphenatedRoute.Destination.Name)

	// The canonical set must match the events.*Definition vars the helpers use,
	// so a Definition var mis-keyed away from the literal above is also caught.
	assert.Equal(t, "alias.related-party-deleted", events.AliasRelatedPartyDeletedDefinition.Key())
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
