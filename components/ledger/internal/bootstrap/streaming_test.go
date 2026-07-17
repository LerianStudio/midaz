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
// resolves to the canonical midaz.balance.changed Kafka topic.
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
	assert.Equal(t, "midaz.balance.changed", dest)
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
