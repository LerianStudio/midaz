// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"regexp"
	"strings"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v2"
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

// TestResolveStreamingSource locks the HELPER-level CloudEvents source
// resolution contract: a trimmed, non-empty STREAMING_CLOUDEVENTS_SOURCE value
// wins verbatim; a nil, empty, or whitespace-only config value normalizes to the
// bare service name streamingServiceName ("ledger").
//
// This is a helper-level fallback only, NOT an end-to-end unset-env default: a
// genuinely-unset STREAMING_CLOUDEVENTS_SOURCE fail-closes at
// libStreaming.LoadConfig (ErrMissingSource) before resolveStreamingSource ever
// runs, so a live enabled deployment never converges here — it MUST set the var
// (.env.example recommends the bare service name).
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
			name:     "nil config normalizes to bare service name",
			cfg:      nil,
			expected: streamingServiceName,
		},
		{
			// Helper-level fallback; a genuinely-unset env fail-closes at
			// LoadConfig (ErrMissingSource) before this helper runs.
			name:     "empty config value normalizes to bare service name",
			cfg:      &Config{StreamingCloudEventsSource: ""},
			expected: streamingServiceName,
		},
		{
			// Whitespace-only slips past LoadConfig's == "" check, so the
			// helper's trim-based fallback to the bare service name applies.
			name:     "whitespace-only config value normalizes to bare service name",
			cfg:      &Config{StreamingCloudEventsSource: "  \t  "},
			expected: streamingServiceName,
		},
		{
			name:     "configured value wins",
			cfg:      &Config{StreamingCloudEventsSource: "lerian.midaz.ledger.staging"},
			expected: "lerian.midaz.ledger.staging",
		},
		{
			name:     "configured value is trimmed",
			cfg:      &Config{StreamingCloudEventsSource: "  lerian.midaz.ledger.shadow  "},
			expected: "lerian.midaz.ledger.shadow",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, resolveStreamingSource(tc.cfg))
		})
	}
}

// TestMidazEventDefinitions_IncludesBalanceChanged asserts the generic
// balance.changed event is registered in the single-source-of-truth
// definition list, so it flows into both the Catalog and the Routes.
func TestMidazEventDefinitions_IncludesBalanceChanged(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	found := false
	for _, def := range defs {
		if def.Key() == "balance.changed" {
			found = true
			break
		}
	}
	assert.True(t, found, "balance.changed must be registered in midazEventDefinitions")
}

// TestBuildRoutes_BalanceChangedTopic asserts the balance.changed route
// resolves to the canonical ledger.balance.changed Kafka topic under the
// ACL-prefix grammar.
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

// TestBuildRoutes_UnderscoreTopics pins the wire topic names for the ledger
// events whose <resource> or <event> segment carries an underscore in the
// canonical Definition.Key() — the segments feed straight through TopicName to
// the wire topic, so this guards that no accidental fold reappears.
func TestBuildRoutes_UnderscoreTopics(t *testing.T) {
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

// TestBuildRoutes_TopicsMatchConsumerRegex asserts every ledger route
// destination stays inside the streaming-hub ingest consumer's subscription
// grammar: a leading service segment [a-z0-9][a-z0-9-]* then the two trailing
// segments over [a-z0-9_] (no hyphen) with an optional ".vN" suffix. The two
// trailing (resource, event) segments must carry no hyphen — a hyphen there
// would silently fall outside the consumer regex.
func TestBuildRoutes_TopicsMatchConsumerRegex(t *testing.T) {
	t.Parallel()

	consumerRegex := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9_]+\.[a-z0-9_]+(\.v[0-9]+)?$`)

	for _, r := range buildRoutes(streamingPrimaryTargetName) {
		assert.Regexp(t, consumerRegex, r.Destination.Name,
			"topic %q must match the streaming-hub consumer regex", r.Destination.Name)

		// The resource.event tail (everything after the leading service segment)
		// must contain no hyphen: those two segments come from the underscore-
		// canonical Key() and a hyphen there would break the consumer subscription.
		_, tail, found := strings.Cut(r.Destination.Name, ".")
		require.True(t, found, "topic %q must have a service segment then a resource.event tail", r.Destination.Name)
		assert.NotContains(t, tail, "-",
			"topic tail %q must not contain a hyphen (resource/event are underscore-canonical)", tail)
	}
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
