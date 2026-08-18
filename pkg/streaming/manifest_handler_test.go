// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming_test

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// manifestEnvelope captures only the fields the shared-helper contract asserts:
// the per-event key and the advertised topic.
type manifestEnvelope struct {
	Events []struct {
		Key   string `json:"key"`
		Topic string `json:"topic"`
	} `json:"events"`
}

// sampleDefs returns a small, deterministic set of event Definitions to exercise
// the shared manifest helper without depending on any binary's event registry.
// SchemaVersion stays at major 1 to mirror every shipped midaz/tracer event:
// lib-streaming's Topic derivation appends a ".v<major>" suffix for major >= 2,
// which pkgStreaming.TopicName (version-unaware) never does, so convergence is a
// contract only for the major-1 schemas the binaries actually publish.
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

// TestNewManifestHandler_TopicsPinnedToServiceName is the pkg-level lock on the
// HIGH fix: the manifest's advertised topic for every Definition equals
// pkgStreaming.TopicName(serviceName, def.Key()). This holds only because the
// helper pins the descriptor SourceBase to serviceName; any other SourceBase
// would make lib-streaming derive a different namespace segment.
func TestNewManifestHandler_TopicsPinnedToServiceName(t *testing.T) {
	t.Parallel()

	for _, service := range []string{"ledger", "tracer"} {
		t.Run(service, func(t *testing.T) {
			t.Parallel()

			defs := sampleDefs()

			handler, err := pkgStreaming.NewManifestHandler(service, defs)
			require.NoError(t, err)
			require.NotNil(t, handler)

			doc := serveManifest(t, handler)

			byKey := make(map[string]string, len(doc.Events))
			for _, ev := range doc.Events {
				byKey[ev.Key] = ev.Topic
			}

			require.Len(t, doc.Events, len(defs),
				"manifest must advertise exactly the supplied definitions")

			for _, def := range defs {
				key := def.Key()
				topic, ok := byKey[key]
				require.Truef(t, ok, "manifest must advertise event %q", key)
				require.Equalf(t, pkgStreaming.TopicName(service, key), topic,
					"manifest topic for %q must equal pkgStreaming.TopicName(%q, key)", key, service)
			}
		})
	}
}

// TestNewManifestHandler_MethodAllowlist confirms the lib handler's GET/HEAD
// allowlist is preserved through the shared helper: a POST is rejected 405.
func TestNewManifestHandler_MethodAllowlist(t *testing.T) {
	t.Parallel()

	handler, err := pkgStreaming.NewManifestHandler("ledger", sampleDefs())
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
