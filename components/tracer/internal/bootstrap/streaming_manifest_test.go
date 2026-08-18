// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// manifestEnvelope captures only the fields the tracer manifest contract
// asserts: the per-event key and the advertised topic. The full lib-streaming
// document carries more, but drift on the rest is locked by lib-streaming's own
// tests.
type manifestEnvelope struct {
	Events []struct {
		Key   string `json:"key"`
		Topic string `json:"topic"`
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

// TestBuildStreamingManifestHandler_TopicsConvergeWithTopicName locks the
// contract invariant: for every one of the 12 tracer event Definitions, the
// topic the manifest advertises equals pkgStreaming.TopicName("tracer",
// def.Key()). If the manifest's ce-source (SourceBase) ever drifts from the
// bare "tracer" service segment, or TopicName changes grammar, this convergence
// breaks — the hub would subscribe to a topic the producer never emits on.
func TestBuildStreamingManifestHandler_TopicsConvergeWithTopicName(t *testing.T) {
	t.Parallel()

	doc := fetchTracerManifest(t, &Config{})

	byKey := make(map[string]string, len(doc.Events))
	for _, ev := range doc.Events {
		byKey[ev.Key] = ev.Topic
	}

	defs := tracerEventDefinitions()

	require.Len(t, doc.Events, len(defs),
		"manifest must advertise exactly the tracer definitions")

	for _, def := range defs {
		key := def.Key()
		topic, ok := byKey[key]
		require.Truef(t, ok, "manifest must advertise event %q", key)
		require.Equalf(t, pkgStreaming.TopicName(streamingServiceName, key), topic,
			"manifest topic for %q must converge with pkgStreaming.TopicName", key)
	}
}

// TestBuildStreamingManifestHandler_TopicsIndependentOfCeSource locks the HIGH
// invariant: a NON-BARE STREAMING_CLOUDEVENTS_SOURCE must NOT leak into the
// manifest's advertised topics. The manifest SourceBase is pinned to the bare
// "tracer" service segment, so the served topics stay equal to the EMITTED
// topics (pkgStreaming.TopicName(streamingServiceName, def.Key())) regardless of
// the operator-configured ce-source.
func TestBuildStreamingManifestHandler_TopicsIndependentOfCeSource(t *testing.T) {
	t.Parallel()

	cfg := &Config{StreamingCloudEventsSource: "lerian.midaz.tracer"}
	doc := fetchTracerManifest(t, cfg)

	byKey := make(map[string]string, len(doc.Events))
	for _, ev := range doc.Events {
		byKey[ev.Key] = ev.Topic
	}

	defs := tracerEventDefinitions()

	require.Len(t, doc.Events, len(defs),
		"manifest must advertise exactly the tracer definitions")

	for _, def := range defs {
		key := def.Key()
		topic, ok := byKey[key]
		require.Truef(t, ok, "manifest must advertise event %q", key)
		require.Equalf(t, pkgStreaming.TopicName(streamingServiceName, key), topic,
			"manifest topic for %q must equal the emitted topic even when ce-source is non-bare", key)
	}
}

// TestBuildStreamingManifestHandler_IndependentOfStreamingEnabled asserts the
// manifest is built regardless of STREAMING_ENABLED and regardless of a nil
// config (the descriptor's SourceBase then falls back to the bare service name).
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := fetchTracerManifest(t, tc.cfg)
			require.Len(t, doc.Events, len(defs),
				"manifest must advertise exactly the tracer definitions regardless of STREAMING_ENABLED / nil config")
		})
	}
}
