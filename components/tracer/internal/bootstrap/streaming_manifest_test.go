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

// manifestEnvelope captures only the fields the tracer manifest contract asserts.
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

// fetchTracerManifest builds the manifest handler for cfg and returns the
// decoded envelope served at the manifest path.
func fetchTracerManifest(t *testing.T, cfg *Config) manifestEnvelope {
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
// one-topic-per-application manifest: tracer advertises ONE topic for its whole
// catalog plus its DLQ, no commands queue (it emits facts only), and one entry per
// tracer Definition whose eventKey is the dispatch selector a consumer registers a
// handler under.
func TestBuildStreamingManifestHandler_AdvertisesApplicationTopic(t *testing.T) {
	t.Parallel()

	doc := fetchTracerManifest(t, &Config{})

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	wantDLQ, err := libStreaming.AppDLQTopic(streamingServiceName)
	require.NoError(t, err)

	require.Equal(t, streamingServiceName, doc.Publisher.Source)
	require.Equal(t, wantTopic, doc.Topic, "manifest must advertise the tracer application topic")
	require.Equal(t, wantDLQ, doc.DLQTopic, "manifest must advertise the tracer DLQ")
	require.Empty(t, doc.CommandsTopic,
		"tracer emits facts only; advertising a commands queue would point provisioning at a stream nothing writes")

	defs := tracerEventDefinitions()

	require.Len(t, doc.Events, len(defs),
		"manifest must advertise exactly the tracer definitions")

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

// TestStreamingRouteAndManifestAgreeOnTopic locks the coherence invariant: ONE
// resolved ce-source feeds both the emitter's route table and the manifest
// handler, so the topic tracer publishes to and the topic its manifest
// advertises must be byte-identical.
//
// The table varies only how the resolver is reached (explicit value, unset,
// whitespace, nil config), not the source.
func TestStreamingRouteAndManifestAgreeOnTopic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  *Config
	}{
		{name: "source configured explicitly", cfg: &Config{StreamingCloudEventsSource: streamingServiceName}},
		{name: "source unset falls back to roster", cfg: &Config{}},
		{name: "whitespace-only source falls back to roster", cfg: &Config{StreamingCloudEventsSource: "  \t "}},
		{name: "nil config falls back to roster", cfg: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source := resolveStreamingSource(tc.cfg)

			routes, err := buildRoutes(streamingPrimaryTargetName, source)
			require.NoError(t, err)
			require.Len(t, routes, 1)

			emitted := routes[0].Destination.Name
			advertised := fetchTracerManifest(t, tc.cfg).Topic

			require.Equal(t, emitted, advertised,
				"the topic tracer publishes to and the topic its manifest advertises must be the same string")

			// Pin the shared value too, so a change that moves BOTH sides together
			// still has to be deliberate.
			require.Equal(t, "lerian.streaming.tracer", emitted)
		})
	}
}

// TestBuildStreamingManifestHandler_RejectsIllegalConfiguredSource locks the
// invalid-source invariant: a malformed STREAMING_CLOUDEVENTS_SOURCE must fail
// the manifest handler build instead of producing a handler.
func TestBuildStreamingManifestHandler_RejectsIllegalConfiguredSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"lerian.midaz.tracer", "//lerian.midaz/tracer", "Tracer"} {
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

	defs := tracerEventDefinitions()

	wantTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := fetchTracerManifest(t, tc.cfg)
			require.Len(t, doc.Events, len(defs),
				"manifest must advertise exactly the tracer definitions regardless of STREAMING_ENABLED / nil config")
			require.Equal(t, wantTopic, doc.Topic,
				"an unset ce-source must fall back to the roster name, never to an empty topic")
		})
	}
}
