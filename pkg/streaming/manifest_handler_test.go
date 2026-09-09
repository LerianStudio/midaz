// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// manifestEnvelope captures only the fields the shared-helper contract asserts.
// Under one topic per producing application the topic pair is a DOCUMENT-level
// fact, not a per-event one; each event instead names the dispatch selector
// (eventKey) and its class.
type manifestEnvelope struct {
	Publisher struct {
		ServiceName     string `json:"serviceName"`
		Source          string `json:"source"`
		OutboxSupported bool   `json:"outboxSupported"`
	} `json:"publisher"`
	Topic         string `json:"topic"`
	DLQTopic      string `json:"dlqTopic"`
	CommandsTopic string `json:"commandsTopic"`
	Events        []struct {
		Key           string `json:"key"`
		EventKey      string `json:"eventKey"`
		Class         string `json:"class"`
		DefaultPolicy struct {
			Enabled bool   `json:"enabled"`
			Direct  string `json:"direct"`
			Outbox  string `json:"outbox"`
			DLQ     string `json:"dlq"`
		} `json:"defaultPolicy"`
	} `json:"events"`
}

// sampleDefs returns a small, deterministic set of event Definitions to exercise
// the shared manifest helper without depending on any binary's event registry.
// The mixed schema versions are deliberate: the manifest must advertise the same
// single application topic regardless of any definition's schema version.
func sampleDefs() []events.Definition {
	return []events.Definition{
		{ResourceType: "account", EventType: "created", SchemaVersion: "1.0.0"},
		{ResourceType: "account", EventType: "deleted", SchemaVersion: "1.0.0"},
		{ResourceType: "rule", EventType: "activated", SchemaVersion: "1.2.0"},
	}
}

func serveManifest(t *testing.T, handler nethttp.Handler) manifestEnvelope {
	t.Helper()

	req := httptest.NewRequest(nethttp.MethodGet, pkgStreaming.ManifestRoutePath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, nethttp.StatusOK, rec.Code, "manifest GET must return 200")

	var doc manifestEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc), "manifest body must be valid JSON")

	return doc
}

// TestNewManifestHandler_AdvertisesApplicationTopic is the pkg-level lock on the
// one-topic-per-application contract: the manifest advertises ONE topic for the
// whole catalog, derived from the source the helper is given, plus its DLQ. Every
// event names only its dispatch selector and its class — no event carries a topic
// of its own, and a fact-only catalog advertises no commands queue, so
// provisioning is never pointed at a stream nothing writes.
func TestNewManifestHandler_AdvertisesApplicationTopic(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"ledger", "tracer"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			defs := sampleDefs()

			handler, err := pkgStreaming.NewManifestHandler(source, source, defs)
			require.NoError(t, err)
			require.NotNil(t, handler)

			doc := serveManifest(t, handler)

			wantTopic, err := libStreaming.AppTopic(source)
			require.NoError(t, err)

			wantDLQ, err := libStreaming.AppDLQTopic(source)
			require.NoError(t, err)

			require.Equal(t, source, doc.Publisher.Source,
				"publisher source must be the ce-source the emitter publishes under")
			require.Equal(t, source, doc.Publisher.ServiceName,
				"publisher serviceName must carry the roster identity")
			require.Equal(t, wantTopic, doc.Topic,
				"manifest must advertise the application topic derived from the source")
			require.Equal(t, wantDLQ, doc.DLQTopic,
				"manifest must advertise the DLQ derived from the application topic")
			require.Empty(t, doc.CommandsTopic,
				"a fact-only catalog must not advertise a commands queue")

			require.Len(t, doc.Events, len(defs),
				"manifest must advertise exactly the supplied definitions")

			byKey := make(map[string]string, len(doc.Events))
			classByKey := make(map[string]string, len(doc.Events))

			for _, ev := range doc.Events {
				byKey[ev.Key] = ev.EventKey
				classByKey[ev.Key] = ev.Class
			}

			for _, def := range defs {
				key := def.Key()

				eventKey, ok := byKey[key]
				require.Truef(t, ok, "manifest must advertise event %q", key)
				require.Equalf(t, key, eventKey,
					"manifest eventKey for %q must be the <resource>.<event> dispatch selector", key)
				require.Equalf(t, "fact", classByKey[key],
					"midaz emits facts only; %q must not be classified as a command", key)
			}
		})
	}
}

// TestNewManifestHandler_RejectsIllegalSource locks the invalid-source
// invariant: a malformed ce-source must fail handler construction instead of
// producing a manifest handler.
func TestNewManifestHandler_RejectsIllegalSource(t *testing.T) {
	t.Parallel()

	for _, source := range []string{"", "Ledger", "lerian.midaz.ledger", "//lerian.midaz/ledger", "ledger service"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()

			handler, err := pkgStreaming.NewManifestHandler("ledger", source, sampleDefs())
			require.Error(t, err, "an illegal ce-source must not produce a manifest handler")
			require.Nil(t, handler)
		})
	}
}

// TestNewManifestHandler_MethodAllowlist confirms the lib handler's GET/HEAD
// allowlist is preserved through the shared helper: a POST is rejected 405.
func TestNewManifestHandler_MethodAllowlist(t *testing.T) {
	t.Parallel()

	handler, err := pkgStreaming.NewManifestHandler("ledger", "ledger", sampleDefs())
	require.NoError(t, err)

	req := httptest.NewRequest(nethttp.MethodPost, pkgStreaming.ManifestRoutePath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, nethttp.StatusMethodNotAllowed, rec.Code, "POST must return 405")
}

// TestCatalogEntriesFromDefinitions_Mapping locks the Definition→EventDefinition
// mapping shared by the emitter catalog and the manifest catalog.
func TestCatalogEntriesFromDefinitions_Mapping(t *testing.T) {
	t.Parallel()

	defs := sampleDefs()

	entries := pkgStreaming.CatalogEntriesFromDefinitions(defs)
	require.Len(t, entries, len(defs))

	for i, def := range defs {
		require.Equal(t, def.Key(), entries[i].Key)
		require.Equal(t, def.ResourceType, entries[i].ResourceType)
		require.Equal(t, def.EventType, entries[i].EventType)
		require.Equal(t, def.SchemaVersion, entries[i].SchemaVersion)
	}
}

// TestManifestAdvertisesNoOutboxFallback pins the delivery policy the manifest
// publishes for every event. Midaz wires no outbox writer and no relay, so an
// advertised outbox fallback is a durability guarantee no deployment can honour:
// a consumer reading defaultPolicy.outbox would build reconciliation on a
// retained copy that is never written. The policy must therefore name the
// outbox mode midaz implements (never), while delivery itself stays enabled --
// a partial policy that forgets Enabled makes lib-streaming drop every event.
func TestManifestAdvertisesNoOutboxFallback(t *testing.T) {
	t.Parallel()

	handler, err := pkgStreaming.NewManifestHandler("ledger", "ledger", sampleDefs())
	require.NoError(t, err)

	doc := serveManifest(t, handler)
	require.Len(t, doc.Events, len(sampleDefs()))

	require.False(t, doc.Publisher.OutboxSupported,
		"midaz declares no outbox support; the per-event policy must agree with it")

	for _, event := range doc.Events {
		require.Equal(t, "never", event.DefaultPolicy.Outbox,
			"event %q must not advertise an outbox path midaz never writes to", event.EventKey)
		require.True(t, event.DefaultPolicy.Enabled,
			"event %q must stay enabled; a disabled policy silently drops every emit", event.EventKey)
		require.Equal(t, "direct", event.DefaultPolicy.Direct,
			"event %q publishes straight to the broker", event.EventKey)
		require.Equal(t, "on_routable_failure", event.DefaultPolicy.DLQ,
			"event %q keeps the library DLQ routing", event.EventKey)
	}
}

// TestCatalogEntriesDeclareNoOutbox locks the same policy on the shared mapper,
// which feeds both binaries' emitter catalogs as well as their manifests, and
// resolves it through the function the emit hot path calls. lib-streaming
// rejects an event whose resolved policy is not Enabled, so this is the guard
// that a declared policy never turns the producer into a silent no-op.
func TestCatalogEntriesDeclareNoOutbox(t *testing.T) {
	t.Parallel()

	for _, entry := range pkgStreaming.CatalogEntriesFromDefinitions(sampleDefs()) {
		require.Equal(t, libStreaming.OutboxModeNever, entry.DefaultPolicy.Outbox, entry.Key)

		resolved, err := libStreaming.ResolveDeliveryPolicy(
			entry,
			libStreaming.DeliveryPolicyOverride{},
			libStreaming.DeliveryPolicyOverride{},
		)
		require.NoError(t, err, entry.Key)
		require.True(t, resolved.Enabled, "%s must stay deliverable", entry.Key)
		require.Equal(t, libStreaming.OutboxModeNever, resolved.Outbox, entry.Key)
	}
}
