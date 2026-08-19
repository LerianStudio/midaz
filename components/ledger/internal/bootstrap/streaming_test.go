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
// roster name streamingServiceName ("ledger").
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
			name:     "nil config normalizes to roster name",
			cfg:      nil,
			expected: streamingServiceName,
		},
		{
			// Helper-level fallback; a genuinely-unset env fail-closes at
			// LoadConfig (ErrMissingSource) before this helper runs.
			name:     "empty config value normalizes to roster name",
			cfg:      &Config{StreamingCloudEventsSource: ""},
			expected: streamingServiceName,
		},
		{
			// Whitespace-only slips past LoadConfig's == "" check, so the
			// helper's trim-based fallback to the roster name applies.
			name:     "whitespace-only config value normalizes to roster name",
			cfg:      &Config{StreamingCloudEventsSource: "  \t  "},
			expected: streamingServiceName,
		},
		{
			name:     "configured value wins",
			cfg:      &Config{StreamingCloudEventsSource: "midaz-ledger-staging"},
			expected: "midaz-ledger-staging",
		},
		{
			// Padded comes back RAW, not trimmed: the raw value is what the roster
			// gate compares and what lib-streaming's ValidateSource rejects, so a
			// padded source must refuse boot NOW rather than the day the flag flips.
			name:     "padded configured value is returned raw",
			cfg:      &Config{StreamingCloudEventsSource: "  midaz-ledger-shadow  "},
			expected: "  midaz-ledger-shadow  ",
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

// TestBuildRoutes_RejectsIllegalSource proves the route table is never built from
// a malformed ce-source. lib-streaming REJECTS an illegal source rather than
// normalizing it: the v2 normalization could fold two distinct applications onto
// one topic namespace and one ACL scope with neither owner noticing, so a bad
// value must fail startup instead of quietly publishing into somebody else's
// stream.
func TestBuildRoutes_RejectsIllegalSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"", "Ledger", "lerian.midaz.ledger", "ledger service"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			routes, err := buildRoutes(streamingPrimaryTargetName, source)
			require.Error(t, err)
			assert.Nil(t, routes)
		})
	}
}

// TestBuildRoutes_RouteTableAccepted proves the catch-all route midaz builds is
// accepted by lib-streaming's own validation — the route-key grammar, the empty
// DefinitionKey (catch-all), the destination shape, and the no-duplicate rule are
// all enforced there, so building the table is the real assertion.
func TestBuildRoutes_RouteTableAccepted(t *testing.T) {
	t.Parallel()

	routes, err := buildRoutes(streamingPrimaryTargetName, streamingServiceName)
	require.NoError(t, err)

	table, err := libStreaming.NewRouteTable(routes...)
	require.NoError(t, err, "midaz's catch-all route must satisfy lib-streaming's route validation")

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	// Every catalog definition resolves through the catch-all bucket, including
	// keys with underscores — v3 route keys accept them, so the hyphen-folding the
	// old table needed is gone and the two forms can no longer drift.
	for _, key := range []string{"balance.changed", "operation_route.created", "balance.config_changed", "balance.overdraft_drawn"} {
		resolved := table.Routes(key)
		require.Lenf(t, resolved, 1, "%q must resolve to exactly one route", key)
		assert.Equalf(t, wantTopic, resolved[0].Destination.Name,
			"%q must ride the ledger application topic", key)
	}
}

// TestBuildStreamingEmitter_EnabledMissingBrokersRefusesBoot locks the
// fail-closed contract for an enabled producer with nowhere to publish.
//
// STREAMING_ENABLED=true with an empty STREAMING_BROKERS is not a degraded
// mode, it is total event loss: every emit lands on a NoopEmitter and is
// discarded, and the ledger has no streaming readiness prober, so the pod stays
// Ready throughout. That is the same invisible-total-loss failure the roster
// source gate exists to kill, so it gets the same posture and the same error
// identity — refuse boot with pkgStreaming.ErrMissingBrokers, so the two
// components are indistinguishable to an operator reading the log.
func TestBuildStreamingEmitter_EnabledMissingBrokersRefusesBoot(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "ledger")

	emitter, closer, err := BuildStreamingEmitter(context.Background(), &Config{StreamingEnabled: true}, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, pkgStreaming.ErrMissingBrokers)
	assert.Nil(t, emitter, "an enabled producer with no brokers must not yield an emitter")
	require.NotNil(t, closer)
	assert.NoError(t, closer())
}

// TestBuildStreamingEmitter_TLSFromConfigWired proves STREAMING_TLS_ENABLED
// reaches the broker dial, so a TLS-only broker is reachable.
//
// The discriminator is lib-streaming's fail-closed SASL-requires-TLS gate, which
// runs at Build with no network I/O: SASL configured and plaintext NOT permitted
// builds successfully only when a *tls.Config was also wired. Unwire
// TLSFromConfig and this same config fails with ErrPlaintextSASLNotAllowed.
// Asserting on a malformed CA would NOT discriminate — LoadConfig rejects that
// before the builder is ever touched.
func TestBuildStreamingEmitter_TLSFromConfigWired(t *testing.T) {
	t.Setenv("STREAMING_ENABLED", "true")
	t.Setenv("STREAMING_BROKERS", "127.0.0.1:9092")
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "ledger")
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
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "ledger")
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
	t.Setenv("STREAMING_CLOUDEVENTS_SOURCE", "ledger")
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

// TestBuildStreamingEmitter_RefusesNonRosterSource locks the fail-closed gate.
//
// A grammar-legal ce-source is not a usable one: the tenant-manager grants a
// producer WRITE+DESCRIBE on LITERAL topic names derived from the roster name alone
// — literal precisely so a grant cannot reach a neighbouring application — and the
// roster name is what gates association admission, so no other name is ever
// provisioned. Publishing under one would hit a topic that does not exist, is not
// auto-created, and carries no grant, with the derived DLQ equally ungranted. The
// IMPORTANT posture swallows all of that as a single Warn and the ledger has no streaming readiness prober, so pods stay Ready, so the
// deployment would lose every event while reporting healthy.
//
// The gate therefore bites regardless of STREAMING_ENABLED: a source left over from
// the pre-v3 dotted or URI shape must fail startup, not sit in an env file until
// someone flips the flag.
func TestBuildStreamingEmitter_RefusesNonRosterSource(t *testing.T) {
	t.Parallel()

	sources := []string{
		"midaz-ledger",          // grammar-legal, unprovisionable
		"ledgerx",               // a PREFIXED grant would have reached this; a literal one does not
		"lerian.midaz.ledger",   // stale pre-v3 dotted shape
		"//lerian.midaz/ledger", // stale pre-v3 URI shape
		" ledger ",              // roster name with padding: ValidateSource rejects the space
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
