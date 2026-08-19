// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"strconv"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// noopEmitterReference is a NoopEmitter constructed independently of the
// code under test. Its concrete type is what BuildStreamingEmitter must
// return on every disabled/fallback branch, so tests assert type identity
// against it rather than dialling a broker or inspecting private state.
var noopEmitterReference = libStreaming.NewNoopEmitter()

// expectedRuleEventKeys is the canonical set of event keys tracer registers
// for the Rule lifecycle (Phase 2).
var expectedRuleEventKeys = []string{
	"rule.created",
	"rule.updated",
	"rule.activated",
	"rule.deactivated",
	"rule.drafted",
	"rule.deleted",
}

// expectedLimitEventKeys is the canonical set of event keys tracer registers
// for the Limit lifecycle (Phase 3).
var expectedLimitEventKeys = []string{
	"limit.created",
	"limit.updated",
	"limit.activated",
	"limit.deactivated",
	"limit.drafted",
	"limit.deleted",
}

// expectedAllEventKeys is the full ordered set of the twelve lifecycle events
// tracer registers (six Rule, then six Limit). This is the drift lock the
// catalog/routes/bijection tests assert against.
var expectedAllEventKeys = append(append([]string{}, expectedRuleEventKeys...), expectedLimitEventKeys...)

// TestTracerEventDefinitions_CoversAllLifecycles locks the Phase-3 contract:
// tracerEventDefinitions() registers exactly the twelve lifecycle events (six
// Rule, then six Limit), in the fixed order, with no extra and none missing.
// This is the single source of truth that feeds both the catalog and the
// routes.
func TestTracerEventDefinitions_CoversAllLifecycles(t *testing.T) {
	t.Parallel()

	defs := tracerEventDefinitions()
	require.Len(t, defs, len(expectedAllEventKeys),
		"tracerEventDefinitions must register exactly the twelve lifecycle events")

	actualKeys := make([]string, 0, len(defs))
	for _, d := range defs {
		actualKeys = append(actualKeys, d.Key())
	}

	// Order is part of the contract: six Rule events (created, updated,
	// activated, deactivated, drafted, deleted) then six Limit events in the
	// same order.
	assert.Equal(t, expectedAllEventKeys, actualKeys,
		"tracerEventDefinitions must return the Rule then Limit events in the fixed order")
}

// TestBuildStreamingEmitter_DisabledReturnsNoop covers the master-flag-off
// branch: BuildStreamingEmitter returns the concrete NoopEmitter without loading
// libStreaming.LoadConfig or touching a broker. This is the ONE noop path that
// survives — and readiness reports it as "skipped", never as a healthy enabled
// producer. Broker-security env is set here to prove it is ignored entirely on
// this path: a disabled producer must not fail closed on SASL-without-TLS.
func TestBuildStreamingEmitter_DisabledReturnsNoop(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "false")
	t.Setenv("STREAMING_SASL_MECHANISM", "PLAIN")
	t.Setenv("STREAMING_SASL_USERNAME", "u")
	t.Setenv("STREAMING_SASL_PASSWORD", "p")
	t.Setenv("STREAMING_TLS_ENABLED", "false")

	cfg := &Config{StreamingEnabled: false}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, emitter)
	require.NotNil(t, closer)

	assert.IsType(t, noopEmitterReference, emitter,
		"disabled streaming must return the NoopEmitter")
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_EnabledMissingBrokersRefusesBoot locks the
// fail-closed contract for an enabled producer with nowhere to publish.
//
// STREAMING_ENABLED=true with an empty STREAMING_BROKERS is not a degraded
// mode, it is total event loss: every emit lands on a NoopEmitter and is
// discarded, the IMPORTANT posture swallows nothing because nothing fails, and
// readiness reports the streaming dependency healthy because the noop emitter
// answers healthy. That is the same invisible-total-loss failure the roster
// source gate exists to kill, so it gets the same posture — refuse boot.
func TestBuildStreamingEmitter_EnabledMissingBrokersRefusesBoot(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")

	cfg := &Config{StreamingEnabled: true}

	emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, pkgStreaming.ErrMissingBrokers)
	assert.Nil(t, emitter, "an enabled producer with no brokers must not yield an emitter")
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed proves the tracer wires
// lib-streaming's canonical SASLFromConfig rather than parsing STREAMING_SASL_*
// itself: with SASL configured, TLS off and no plaintext opt-in, Build must fail
// closed instead of dialling the broker with credentials in cleartext. Drop the
// SASLFromConfig call and the build succeeds unauthenticated, which this catches.
func TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")
	t.Setenv("STREAMING_SASL_MECHANISM", "PLAIN")
	t.Setenv("STREAMING_SASL_USERNAME", "u")
	t.Setenv("STREAMING_SASL_PASSWORD", "p")
	t.Setenv("STREAMING_TLS_ENABLED", "false")
	t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "false")

	emitter, closer, err := BuildStreamingEmitter(context.Background(), &Config{StreamingEnabled: true}, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, libStreaming.ErrPlaintextSASLNotAllowed)
	assert.Nil(t, emitter)
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_TLSFromConfigWired proves STREAMING_TLS_ENABLED
// reaches the broker dial. The tracer previously hand-rolled SASL and never
// touched the TLS knobs at all, so a TLS-only broker was unreachable and the
// setting was silently inert.
//
// The discriminator is lib-streaming's fail-closed SASL-requires-TLS gate, which
// runs at Build with no network I/O: SASL configured and plaintext NOT permitted
// builds successfully only when a *tls.Config was also wired. Unwire
// TLSFromConfig and this same config fails with ErrPlaintextSASLNotAllowed,
// which is exactly the pre-fix behaviour. Asserting on a malformed CA would NOT
// discriminate — LoadConfig rejects that before the builder is ever touched.
func TestBuildStreamingEmitter_TLSFromConfigWired(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")
	t.Setenv("STREAMING_TLS_ENABLED", "true")
	t.Setenv("STREAMING_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("STREAMING_SASL_USERNAME", "u")
	t.Setenv("STREAMING_SASL_PASSWORD", "p")
	t.Setenv("STREAMING_SASL_ALLOW_PLAINTEXT", "false")

	emitter, closer, err := BuildStreamingEmitter(context.Background(), &Config{StreamingEnabled: true}, nil, nil)
	require.NoError(t, err, "STREAMING_TLS_ENABLED must satisfy the SASL-requires-TLS gate")
	require.NotNil(t, emitter)
	require.NotNil(t, closer)
	t.Cleanup(func() { _ = closer() })
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
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")

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
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "tracer")

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

// TestBuildCatalog_CoversAllLifecycles exercises buildCatalog against the
// populated Rule + Limit definition set: it must succeed and register exactly
// one entry per event, each looked up by its canonical key.
func TestBuildCatalog_CoversAllLifecycles(t *testing.T) {
	t.Parallel()

	catalog, err := buildCatalog()
	require.NoError(t, err)
	require.NotNil(t, catalog)
	assert.Equal(t, len(expectedAllEventKeys), catalog.Len(),
		"catalog must hold one entry per lifecycle event")

	for _, key := range expectedAllEventKeys {
		_, ok := catalog.Lookup(key)
		assert.Truef(t, ok, "catalog must register key %q", key)
	}
}

// TestBuildRoutes_SingleCatchAllRoute exercises buildRoutes under one topic per
// producing application: ONE required catch-all route carries every lifecycle
// event to "lerian.streaming.tracer". A route count that tracked the event count
// again would mean the per-event fan-out came back, and with it twelve topics no
// consumer subscribes to.
func TestBuildRoutes_SingleCatchAllRoute(t *testing.T) {
	t.Parallel()

	routes, err := buildRoutes(streamingPrimaryTargetName, streamingServiceName)
	require.NoError(t, err)
	require.Len(t, routes, 1, "tracer publishes through exactly one catch-all route")

	route := routes[0]
	assert.Empty(t, route.DefinitionKey,
		"an empty DefinitionKey is what makes the route a catch-all serving the whole catalog")
	assert.Equal(t, streamingPrimaryTargetName, route.Target)
	assert.Equal(t, libStreaming.RouteRequired, route.Requirement,
		"the only route carrying every fact must be required, or a lost publish would report success")

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)
	assert.Equal(t, libStreaming.KafkaTopic(wantTopic), route.Destination)
	assert.Equal(t, "lerian.streaming.tracer", wantTopic)

	// Every lifecycle event resolves through the catch-all bucket.
	table, err := libStreaming.NewRouteTable(routes...)
	require.NoError(t, err, "tracer's catch-all route must satisfy lib-streaming's route validation")

	for _, key := range expectedAllEventKeys {
		resolved := table.Routes(key)
		require.Lenf(t, resolved, 1, "%q must resolve to exactly one route", key)
		assert.Equalf(t, wantTopic, resolved[0].Destination.Name,
			"%q must ride the tracer application topic", key)
	}
}

// TestBuildRoutes_RejectsIllegalSource proves the route table is never built from
// a malformed ce-source. lib-streaming REJECTS an illegal source rather than
// normalizing it: the v2 normalization could fold two distinct applications onto
// one topic namespace and one ACL scope with neither owner noticing, so a bad
// value must fail startup instead of quietly publishing into somebody else's
// stream.
func TestBuildRoutes_RejectsIllegalSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"", "Tracer", "lerian.midaz.tracer", "tracer service"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			routes, err := buildRoutes(streamingPrimaryTargetName, source)
			require.Error(t, err)
			assert.Nil(t, routes)
		})
	}
}

// TestTracerCatalog_CoversAllEmittedEvents is the drift lock: every registered
// event definition has a catalog entry, and every catalog entry resolves to
// exactly one deliverable route. It locks all twelve events (six Rule, six Limit).
//
// The route half of the old bijection is gone with the topic collapse — there is
// one route, not one per event — so what matters now is RESOLUTION: a definition
// that resolved to zero required routes could lose every copy and still return a
// nil Emit error, which is why lib-streaming fails construction on it and why this
// test walks every key through the route table.
func TestTracerCatalog_CoversAllEmittedEvents(t *testing.T) {
	t.Parallel()

	defs := tracerEventDefinitions()
	require.Len(t, defs, len(expectedAllEventKeys),
		"tracer must register all twelve lifecycle events")

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes, err := buildRoutes(streamingPrimaryTargetName, streamingServiceName)
	require.NoError(t, err)

	assert.Equal(t, len(defs), catalog.Len(),
		"catalog entry count must equal definition count")

	// Definition key set (the source of truth).
	defKeys := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		defKeys[d.Key()] = struct{}{}
	}

	require.Lenf(t, defKeys, len(defs),
		"definition keys must be unique (found %d unique of %d defs)", len(defKeys), len(defs))

	// Every definition resolves to a catalog entry.
	for key := range defKeys {
		_, ok := catalog.Lookup(key)
		assert.Truef(t, ok, "catalog is missing a definition-registered key %q", key)
	}

	// Every catalog entry resolves to exactly one required route on the
	// application topic — no unroutable event, no double publish.
	table, err := libStreaming.NewRouteTable(routes...)
	require.NoError(t, err)

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	for _, entry := range catalog.Definitions() {
		resolved := table.Routes(entry.Key)
		require.Lenf(t, resolved, 1, "catalog entry %q must resolve to exactly one route", entry.Key)
		assert.Equalf(t, libStreaming.RouteRequired, resolved[0].Requirement,
			"catalog entry %q must resolve to a REQUIRED route, or a lost publish reports success", entry.Key)
		assert.Equalf(t, wantTopic, resolved[0].Destination.Name,
			"catalog entry %q must ride the tracer application topic", entry.Key)
	}

	// Guard against a stale reference to the events package import.
	assert.Equal(t, "rule.created", events.RuleCreatedDefinition.Key())
	assert.Equal(t, "limit.created", events.LimitCreatedDefinition.Key())
}

// TestResolveStreamingSource locks the HELPER-level CloudEvents source
// resolution contract: a trimmed, non-empty STREAMING_CLOUDEVENTS_SOURCE value
// wins verbatim; a nil, empty, or whitespace-only config value normalizes to the
// roster name streamingServiceName ("tracer").
//
// This is a helper-level fallback only, NOT an end-to-end unset-env default: a
// genuinely-unset STREAMING_CLOUDEVENTS_SOURCE fail-closes at
// libStreaming.LoadConfig (ErrMissingSource) before resolveStreamingSource ever
// runs, so a live enabled deployment never converges here — it MUST set the var
// (.env.example recommends the roster name).
//
// The helper does not validate the grammar; lib-streaming does, at LoadConfig,
// Builder.Build, and AppTopic. The configured-value cases below therefore use
// legal single-segment sources — a dotted or upper-case value is rejected
// downstream rather than normalized, so it never reaches a topic name.
func TestResolveStreamingSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cfg      *Config
		expected string
	}{
		{
			// Helper-level fallback; a genuinely-unset env fail-closes at
			// LoadConfig (ErrMissingSource) before this helper runs.
			name:     "nil config normalizes to default",
			cfg:      nil,
			expected: streamingServiceName,
		},
		{
			// Helper-level fallback; a genuinely-unset env fail-closes at
			// LoadConfig (ErrMissingSource) before this helper runs.
			name:     "empty config value normalizes to default",
			cfg:      &Config{StreamingCloudEventsSource: ""},
			expected: streamingServiceName,
		},
		{
			// Whitespace-only slips past LoadConfig's == "" check, so the
			// helper's trim-based fallback to the default applies.
			name:     "whitespace-only config value normalizes to default",
			cfg:      &Config{StreamingCloudEventsSource: "  \t  "},
			expected: streamingServiceName,
		},
		{
			name:     "configured value wins",
			cfg:      &Config{StreamingCloudEventsSource: "midaz-tracer-staging"},
			expected: "midaz-tracer-staging",
		},
		{
			// Padded comes back RAW, not trimmed: the raw value is what the roster
			// gate compares and what lib-streaming's ValidateSource rejects, so a
			// padded source must refuse boot NOW rather than the day the flag flips.
			name:     "padded configured value is returned raw",
			cfg:      &Config{StreamingCloudEventsSource: "  midaz-tracer-shadow  "},
			expected: "  midaz-tracer-shadow  ",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, resolveStreamingSource(tc.cfg))
		})
	}
}

// TestEventKeyConvergesWithEventDefinition proves midaz's Definition.Key() is the
// SAME string lib-streaming publishes as the dispatch selector
// (EventDefinition.EventKey) for every registered tracer event.
//
// This replaced a topic-convergence test. Under one topic per application a
// definition has no topic of its own; what a consumer selects on inside the stream
// is this key, carried on the wire as ce-resourcetype / ce-eventtype and as the
// trailing two segments of ce-type. Underscores now survive end to end — the
// hyphen-folding both sides used to apply is gone — so the two forms can no longer
// drift apart.
func TestEventKeyConvergesWithEventDefinition(t *testing.T) {
	t.Parallel()

	for _, def := range tracerEventDefinitions() {
		entry := libStreaming.EventDefinition{
			Key:           def.Key(),
			ResourceType:  def.ResourceType,
			EventType:     def.EventType,
			SchemaVersion: def.SchemaVersion,
		}

		assert.Equalf(t, def.Key(), entry.EventKey(),
			"Definition.Key and EventDefinition.EventKey must converge for %q", def.Key())
	}

	// The shared mapper is the single place a Definition becomes a catalog entry;
	// lock that it carries the key through unchanged.
	defs := tracerEventDefinitions()
	entries := pkgStreaming.CatalogEntriesFromDefinitions(defs)
	require.Len(t, entries, len(defs))

	for i, def := range defs {
		assert.Equal(t, def.Key(), entries[i].Key)
		assert.Equal(t, def.Key(), entries[i].EventKey())
	}
}

// TestBuildStreamingEmitter_RefusesNonRosterSource locks the fail-closed gate.
//
// A grammar-legal ce-source is not a usable one: the tenant-manager grants a
// producer WRITE+DESCRIBE on LITERAL topic names derived from the roster name alone
// — literal precisely so a grant cannot reach a neighbouring application — and the
// roster name is what gates association admission, so no other name is ever
// provisioned. Publishing under one would hit a topic that does not exist, is not
// auto-created, and carries no grant, with the derived DLQ equally ungranted. The
// IMPORTANT posture swallows all of that as a single Warn and pods stay Ready on the strength of the rest of the service, so the
// deployment would lose every event while reporting healthy.
//
// The gate therefore bites regardless of STREAMING_ENABLED: a source left over from
// the pre-v3 dotted or URI shape must fail startup, not sit in an env file until
// someone flips the flag.
func TestBuildStreamingEmitter_RefusesNonRosterSource(t *testing.T) {
	t.Parallel()

	sources := []string{
		"midaz-tracer",          // grammar-legal, unprovisionable
		"tracerx",               // a PREFIXED grant would have reached this; a literal one does not
		"lerian.midaz.tracer",   // stale pre-v3 dotted shape
		"//lerian.midaz/tracer", // stale pre-v3 URI shape
		" tracer ",              // roster name with padding: ValidateSource rejects the space
	}

	for _, source := range sources {
		for _, enabled := range []bool{false, true} {
			t.Run(source+"/enabled="+strconv.FormatBool(enabled), func(t *testing.T) {
				t.Parallel()

				cfg := &Config{StreamingEnabled: enabled, StreamingCloudEventsSource: source}

				emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
				require.Error(t, err, "a non-roster ce-source must refuse boot")
				require.ErrorIs(t, err, pkgStreaming.ErrSourceNotRoster)
				assert.Nil(t, emitter, "no emitter may be handed back on a refused source")
				require.NotNil(t, closer)
				assert.NoError(t, closer())
			})
		}
	}
}

// TestBuildStreamingEmitter_AcceptsRosterSource is the other half of the gate: the
// roster name passes, both spelled out and left to the resolver's fallback, so the
// gate cannot be satisfied only by accident of an unset variable.
func TestBuildStreamingEmitter_AcceptsRosterSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{streamingServiceName, "", "  \t "} {
		t.Run("source="+strconv.Quote(source), func(t *testing.T) {
			t.Parallel()

			cfg := &Config{StreamingEnabled: false, StreamingCloudEventsSource: source}

			emitter, closer, err := BuildStreamingEmitter(context.Background(), cfg, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, emitter)
			require.NotNil(t, closer)
			assert.NoError(t, closer())
		})
	}
}
