// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// manifestEnvelope captures only the fields the ledger manifest contract asserts.
// Under one topic per producing application the topic pair is a DOCUMENT-level
// fact; each event carries only its dispatch selector. The full lib-streaming
// document carries more, but drift on the rest is locked by lib-streaming's own
// tests.
type manifestEnvelope struct {
	Publisher struct {
		Source string `json:"source"`
	} `json:"publisher"`
	Topic         string `json:"topic"`
	DLQTopic      string `json:"dlqTopic"`
	CommandsTopic string `json:"commandsTopic"`
	Events        []struct {
		Key      string `json:"key"`
		EventKey string `json:"eventKey"`
	} `json:"events"`
}

// fetchLedgerManifest builds the manifest handler for cfg and returns the decoded
// envelope served at the manifest path.
func fetchLedgerManifest(t *testing.T, cfg *Config) manifestEnvelope {
	t.Helper()

	handler, err := BuildStreamingManifestHandler(cfg)
	require.NoError(t, err, "manifest handler must build")
	require.NotNil(t, handler, "manifest handler must be non-nil")

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, nethttp.StatusOK, rec.Code, "manifest GET must return 200")

	var doc manifestEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc), "manifest body must be valid JSON")

	return doc
}

// TestBuildStreamingManifestHandler_AdvertisesApplicationTopic locks the
// one-topic-per-application manifest: the ledger advertises ONE topic for its
// whole catalog plus its DLQ, no commands queue (it emits facts only), and one
// entry per midaz Definition whose eventKey is the dispatch selector a consumer
// registers a handler under.
func TestBuildStreamingManifestHandler_AdvertisesApplicationTopic(t *testing.T) {
	t.Parallel()

	doc := fetchLedgerManifest(t, &Config{})

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	wantDLQ, err := libStreaming.AppDLQTopic(streamingServiceName)
	require.NoError(t, err)

	require.Equal(t, streamingServiceName, doc.Publisher.Source)
	require.Equal(t, wantTopic, doc.Topic, "manifest must advertise the ledger application topic")
	require.Equal(t, wantDLQ, doc.DLQTopic, "manifest must advertise the ledger DLQ")
	require.Empty(t, doc.CommandsTopic,
		"the ledger emits facts only; advertising a commands queue would point provisioning at a stream nothing writes")

	defs := midazEventDefinitions()

	// The manifest advertises exactly the midaz definitions — no billing entry,
	// which rides a fixed literal topic owned by lib-streaming's billing package
	// rather than the ledger's application topic.
	require.Len(t, doc.Events, len(defs),
		"manifest must advertise exactly the midaz definitions (billing excluded)")

	byKey := make(map[string]string, len(doc.Events))
	for _, ev := range doc.Events {
		byKey[ev.Key] = ev.EventKey
	}

	for _, def := range defs {
		key := def.Key()
		eventKey, ok := byKey[key]
		require.Truef(t, ok, "manifest must advertise event %q", key)
		require.Equalf(t, key, eventKey,
			"manifest eventKey for %q must be the <resource>.<event> dispatch selector", key)
	}
}

// TestBuildStreamingManifestHandler_TopicFollowsConfiguredSource locks the
// coherence invariant the v3 contract rests on: the topic the manifest advertises
// is derived from the SAME ce-source the emitter publishes under, so the streaming
// hub and topic provisioning are pointed at the stream the ledger actually writes.
//
// This inverts the pre-v3 behaviour deliberately. Before the collapse, topics were
// derived from a compile-time service segment and the configured ce-source only
// reached the header, so pinning the manifest against the config was the safe
// choice. Now the topic IS the source: pinning them apart would advertise a topic
// nothing writes, and a source-verifying consumer subscribing by application name
// would quarantine every record it received.
func TestBuildStreamingManifestHandler_TopicFollowsConfiguredSource(t *testing.T) {
	t.Parallel()

	const configuredSource = "midaz-ledger"

	doc := fetchLedgerManifest(t, &Config{StreamingCloudEventsSource: configuredSource})

	wantTopic, err := libStreaming.AppTopic(configuredSource)
	require.NoError(t, err)

	require.Equal(t, configuredSource, doc.Publisher.Source)
	require.Equal(t, wantTopic, doc.Topic,
		"manifest topic must follow the configured ce-source, which is what the emitter publishes to")
	require.Equal(t, "lerian.streaming.midaz-ledger", doc.Topic)
}

// TestBuildStreamingManifestHandler_RejectsIllegalConfiguredSource proves a
// malformed STREAMING_CLOUDEVENTS_SOURCE leaves the manifest route UNMOUNTED
// rather than advertising a topic name derived from garbage. lib-streaming rejects
// an illegal source instead of normalizing it — the v2 normalization could fold
// two distinct services onto one topic namespace with neither owner noticing — and
// the composition root treats a manifest build failure as degraded-safe.
func TestBuildStreamingManifestHandler_RejectsIllegalConfiguredSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"lerian.midaz.ledger", "//lerian.midaz/ledger", "Ledger"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			handler, err := BuildStreamingManifestHandler(&Config{StreamingCloudEventsSource: source})
			require.Error(t, err, "an illegal ce-source must not produce a manifest handler")
			require.Nil(t, handler)
		})
	}
}

// TestBuildStreamingManifestHandler_IndependentOfStreamingEnabled asserts the
// manifest is built regardless of STREAMING_ENABLED and regardless of a nil
// config (the descriptor's Source then falls back to the roster name).
func TestBuildStreamingManifestHandler_IndependentOfStreamingEnabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "streaming disabled", cfg: &Config{StreamingEnabled: false}},
		{name: "streaming enabled", cfg: &Config{StreamingEnabled: true}},
		{name: "nil config", cfg: nil},
	}

	defs := midazEventDefinitions()

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := fetchLedgerManifest(t, tc.cfg)
			require.Len(t, doc.Events, len(defs),
				"manifest must advertise exactly the midaz definitions regardless of STREAMING_ENABLED / nil config")
			require.Equal(t, wantTopic, doc.Topic,
				"an unset ce-source must fall back to the roster name, never to an empty topic")
		})
	}
}
