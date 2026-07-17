// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"regexp"
	"strings"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	pkgStreaming "github.com/LerianStudio/midaz/v3/pkg/streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// closer is returned, and no broker connection is attempted. This is the
// contract that keeps CRM behaving exactly as today.
func TestBuildStreamingEmitter_DisabledReturnsNoop(t *testing.T) {
	// t.Setenv prevents t.Parallel — we mutate process env.
	t.Setenv("STREAMING_ENABLED", "false")

	cfg := &Config{
		StreamingEnabled: false,
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

// TestCRMCatalogRoutesAssembly locks the catalog/routes assembly path: it must
// build without error from whatever crmEventDefinitions() currently returns,
// with the assembled catalog and route table both sized to the definition
// count. buildCatalog is exercised directly because the full Builder.Build path
// legitimately rejects empty input — NewRouteTable rejects an empty ROUTES table
// first (no-routes-configured) before the catalog check is reached.
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
		assert.True(t, strings.HasPrefix(r.Destination.Name, pkgStreaming.TopicPrefix),
			"route destination %q must start with %q", r.Destination.Name, pkgStreaming.TopicPrefix)
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

		expectedTopic := pkgStreaming.TopicName("crm", r.DefinitionKey)
		assert.Equal(t, expectedTopic, r.Destination.Name,
			"route for %q must target topic %q", r.DefinitionKey, expectedTopic)
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
	assert.Equal(t, "lerian.streaming.crm_alias.related_party_deleted", hyphenatedRoute.Destination.Name)

	// The canonical set must match the events.*Definition vars the helpers use,
	// so a Definition var mis-keyed away from the literal above is also caught.
	assert.Equal(t, "alias.related-party-deleted", events.AliasRelatedPartyDeletedDefinition.Key())
}

// TestBuildRoutes_TopicsMatchConsumerRegex asserts every CRM route destination
// stays inside the streaming-hub ingest consumer's subscription grammar
// (^lerian.streaming.<seg>.<seg>(\.vN)?$ over [a-z0-9_]) and carries no hyphen —
// a hyphen on the wire topic would silently fall outside the consumer regex.
func TestBuildRoutes_TopicsMatchConsumerRegex(t *testing.T) {
	t.Parallel()

	consumerRegex := regexp.MustCompile(`^lerian\.streaming\.[a-z0-9_]+\.[a-z0-9_]+(\.v[0-9]+)?$`)

	for _, r := range buildRoutes(streamingPrimaryTargetName) {
		assert.Regexp(t, consumerRegex, r.Destination.Name,
			"topic %q must match the streaming-hub consumer regex", r.Destination.Name)
		assert.NotContains(t, r.Destination.Name, "-",
			"topic %q must not contain a hyphen (folded to underscore on the wire)", r.Destination.Name)
	}
}

// TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed locks the security
// contract at CRM's wiring seam: with SASL configured, TLS disabled, and no
// plaintext opt-in, BuildStreamingEmitter must fail closed rather than dial the
// broker with credentials in cleartext. This guards that the
// builder.SASLFromConfig call stays wired — drop it and the build would succeed
// unauthenticated, which this test would catch.
func TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed(t *testing.T) {
	// t.Setenv prevents t.Parallel — lib-streaming's LoadConfig reads process env.
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")
	t.Setenv("STREAMING_SASL_MECHANISM", "PLAIN")
	t.Setenv("STREAMING_SASL_USERNAME", "u")
	t.Setenv("STREAMING_SASL_PASSWORD", "p")
	// Pin TLS off and plaintext-SASL not permitted, so the fail-closed assertion
	// does not depend on ambient STREAMING_* env leaking into the test.
	t.Setenv("STREAMING_TLS_ENABLED", "false")
	t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "false")

	emitter, closer, err := BuildStreamingEmitter(context.Background(), &Config{StreamingEnabled: true}, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, libStreaming.ErrPlaintextSASLNotAllowed)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EnabledBuildsAndCloses exercises the enabled happy
// path through the builder — catalog + routes + target + SASL-over-plaintext
// (dev opt-in) + Build — guarding the otherwise-untested assembly and proving
// the TLS/SASL delegation produces a working, closeable emitter.
func TestBuildStreamingEmitter_EnabledBuildsAndCloses(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.crm.test")
	t.Setenv("STREAMING_SASL_MECHANISM", "PLAIN")
	t.Setenv("STREAMING_SASL_USERNAME", "u")
	t.Setenv("STREAMING_SASL_PASSWORD", "p")
	t.Setenv("STREAMING_TLS_ENABLED", "false")
	t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "true")

	emitter, closer, err := BuildStreamingEmitter(context.Background(), &Config{StreamingEnabled: true}, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	t.Cleanup(func() { _ = closer() })
}
