// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"regexp"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// and no broker connection is attempted.
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
	t.Cleanup(func() { _ = closer() })
}

// TestMidazEventDefinitions_IncludesBalanceChanged asserts the generic
// balance.changed event is registered in the single-source-of-truth
// definition list, so it flows into both the Catalog and the Routes.
func TestMidazEventDefinitions_IncludesBalanceChanged(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	found := false
	for _, d := range defs {
		if d.Key() == "balance.changed" {
			found = true
			break
		}
	}
	assert.True(t, found, "balance.changed must be registered in midazEventDefinitions")
}

// TestBuildRoutes_BalanceChangedTopic asserts the balance.changed route
// resolves to the canonical ledger.balance.changed Kafka topic.
func TestBuildRoutes_BalanceChangedTopic(t *testing.T) {
	t.Parallel()

	routes := buildRoutes(streamingPrimaryTargetName)

	var dest string
	for _, r := range routes {
		if r.DefinitionKey == "balance.changed" {
			// KafkaTopic stores the topic string in Destination.Name
			// (Destination is a struct, not a string).
			dest = r.Destination.Name
		}
	}
	assert.Equal(t, "ledger.balance.changed", dest)
}

// TestBuildRoutes_HyphenatedTopics pins the wire topic names for the ledger
// event keys whose <resource> or <event> segment carries an underscore — the
// keys where the underscore-canonical DefinitionKey mapping onto the topic name
// (while the route Key / ce-type stay hyphenated) is easiest to get wrong.
func TestBuildRoutes_HyphenatedTopics(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"operation_route.created": "ledger.operation_route.created",
		"balance.config_changed":  "ledger.balance.config_changed",
		"balance.overdraft_drawn": "ledger.balance.overdraft_drawn",
	}

	got := make(map[string]string, len(want))
	for _, r := range buildRoutes(streamingPrimaryTargetName) {
		if _, ok := want[r.DefinitionKey]; ok {
			got[r.DefinitionKey] = r.Destination.Name
		}
	}

	for key, topic := range want {
		assert.Equal(t, topic, got[key], "route for %q must target topic %q", key, topic)
	}
}

// TestBuildRoutes_FoldedRouteKey pins the behavioral heart of the topic-prefix
// change: RouteDefinition.Key is the HYPHENATED RouteKey() (plus the target
// suffix), while DefinitionKey stays UNDERSCORE-canonical. A revert to using the
// underscore Key() for RouteDefinition.Key would flip Key to the underscore form
// and fail these assertions. The wire topic (Destination) stays
// underscore-canonical, asserted alongside so both halves of the split are
// locked in the default (non-integration) unit run.
func TestBuildRoutes_FoldedRouteKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		definitionKey string
		wantKey       string
		wantTopic     string
	}{
		{
			name:          "operation route created folds underscore in Key only",
			definitionKey: "operation_route.created",
			wantKey:       "operation-route.created.primary",
			wantTopic:     "ledger.operation_route.created",
		},
		{
			name:          "balance overdraft drawn folds underscore in event segment",
			definitionKey: "balance.overdraft_drawn",
			wantKey:       "balance.overdraft-drawn.primary",
			wantTopic:     "ledger.balance.overdraft_drawn",
		},
	}

	routes := buildRoutes(streamingPrimaryTargetName)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var found *libStreaming.RouteDefinition
			for i := range routes {
				if routes[i].DefinitionKey == tt.definitionKey {
					found = &routes[i]

					break
				}
			}

			require.NotNil(t, found, "route for DefinitionKey %q must exist", tt.definitionKey)
			assert.Equal(t, tt.wantKey, found.Key,
				"RouteDefinition.Key must be the hyphen-folded RouteKey()+target suffix")
			assert.Equal(t, tt.definitionKey, found.DefinitionKey,
				"DefinitionKey must stay underscore-canonical")
			assert.Equal(t, tt.wantTopic, found.Destination.Name,
				"wire topic must stay underscore-canonical")
		})
	}
}

// TestBuildRoutes_TopicsMatchConsumerRegex asserts every ledger route
// destination stays inside the streaming-hub ingest consumer's subscription
// grammar (^<service>.<resource>.<event>(\.vN)?$ over [a-z0-9_]) and carries no
// hyphen — a hyphen on the wire topic would silently fall outside the consumer
// regex.
func TestBuildRoutes_TopicsMatchConsumerRegex(t *testing.T) {
	t.Parallel()

	consumerRegex := regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+\.[a-z0-9_]+(\.v[0-9]+)?$`)

	for _, r := range buildRoutes(streamingPrimaryTargetName) {
		assert.Regexp(t, consumerRegex, r.Destination.Name,
			"topic %q must match the streaming-hub consumer regex", r.Destination.Name)
		assert.NotContains(t, r.Destination.Name, "-",
			"topic %q must not contain a hyphen (folded to underscore on the wire)", r.Destination.Name)
	}

	// Negative cases: the 3-segment underscore grammar must REJECT a hyphenated
	// topic (a regression that folds a hyphen onto the wire) and a 2-segment
	// topic (a missing service or resource segment).
	assert.NotRegexp(t, consumerRegex, "ledger.operation-route.created",
		"hyphenated topic must fall outside the consumer regex")
	assert.NotRegexp(t, consumerRegex, "ledger.balance",
		"2-segment topic must fall outside the consumer regex")
}

// TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed locks the security
// contract at midaz's wiring seam: with SASL configured, TLS disabled, and no
// plaintext opt-in, BuildStreamingEmitter must fail closed rather than dial the
// broker with credentials in cleartext. This guards that the
// builder.SASLFromConfig call stays wired — drop it and the build would succeed
// unauthenticated, which this test would catch.
func TestBuildStreamingEmitter_SASLWithoutTLSFailsClosed(t *testing.T) {
	// t.Setenv prevents t.Parallel — lib-streaming's LoadConfig reads process env.
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")
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
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "lerian.midaz.ledger.test")
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
