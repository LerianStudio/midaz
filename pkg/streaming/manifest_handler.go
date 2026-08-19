// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"fmt"
	nethttp "net/http"

	libStreaming "github.com/LerianStudio/lib-streaming/v2"

	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// CatalogEntriesFromDefinitions maps midaz/tracer event Definitions onto
// lib-streaming EventDefinition catalog entries, carrying the canonical
// "<resource>.<event>" key and the ResourceType / EventType / SchemaVersion
// triple. It is the single source of truth for how a Definition becomes a
// catalog entry, shared by the per-binary emitter catalog (which may append
// extra entries, e.g. billing) and the catalog-only manifest built here.
func CatalogEntriesFromDefinitions(defs []events.Definition) []libStreaming.EventDefinition {
	entries := make([]libStreaming.EventDefinition, 0, len(defs))

	for _, def := range defs {
		entries = append(entries, libStreaming.EventDefinition{
			Key:           def.Key(),
			ResourceType:  def.ResourceType,
			EventType:     def.EventType,
			SchemaVersion: def.SchemaVersion,
		})
	}

	return entries
}

// NewManifestHandler builds the catalog-only lib-streaming manifest HTTP handler
// a producing binary serves at ManifestRoutePath. It is the SINGLE shared helper
// behind both ledger's and tracer's BuildStreamingManifestHandler.
//
// The PublisherDescriptor's SourceBase is pinned to serviceName — the bare,
// ACL-scoped service segment ("ledger" / "tracer") — the SAME value that feeds
// the emitter's route Destination via TopicName. lib-streaming derives each
// advertised topic from EventDefinition.Topic(SourceBase), so pinning SourceBase
// to the bare service name makes the manifest-advertised topics equal the
// emitted topics UNCONDITIONALLY, independent of the operator-configured
// STREAMING_CLOUDEVENTS_SOURCE. ServiceName is the same bare segment.
//
// No WithManifestRoutes option is passed, so the document is catalog-only and
// discloses no broker topology. The lib handler pre-marshals once and enforces a
// GET/HEAD method allowlist plus hardening headers (Cache-Control: no-store,
// X-Content-Type-Options, X-Frame-Options). It performs NO authentication; the
// caller wraps it in the binary's authz chain. Degraded-safe wiring is the
// composition root's responsibility: a non-nil error here must leave the route
// unmounted (logged at Warn), never fail boot.
func NewManifestHandler(serviceName string, defs []events.Definition) (nethttp.Handler, error) {
	catalog, err := libStreaming.NewCatalog(CatalogEntriesFromDefinitions(defs)...)
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest catalog: %w", err)
	}

	descriptor := libStreaming.PublisherDescriptor{
		ServiceName: serviceName,
		SourceBase:  serviceName,
		RoutePath:   ManifestRoutePath,
	}

	handler, err := libStreaming.NewStreamingHandler(descriptor, catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest handler: %w", err)
	}

	return handler, nil
}
