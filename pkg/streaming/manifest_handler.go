// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"fmt"
	nethttp "net/http"

	libStreaming "github.com/LerianStudio/lib-streaming/v4"

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
// source is the application's ce-source — the SAME value the emitter's Builder is
// given and from which its route Destination is derived as
// libStreaming.AppTopic(source). Under the one-topic-per-application contract the
// manifest advertises the app topic at DOCUMENT level, derived from
// PublisherDescriptor.Source, so passing the emitter's source here is what makes
// the advertised topic equal the emitted topic. A divergence would point the
// streaming hub and topic provisioning at a stream nothing writes, and a
// source-verifying consumer would quarantine every record.
//
// serviceName is the application's ROSTER identity ("ledger" / "tracer"). The
// bootstrap gate holds the two equal in every deployment that boots, but they are
// kept as separate fields rather than folded into one because they answer
// different questions for the hub: serviceName is who this producer IS on the
// roster, source is the name its topics are derived from. Collapsing them would
// remove the surface the hub reconciles across.
//
// source is REJECTED (never rewritten) by lib-streaming when it is not a single
// dot-free lowercase segment, so an illegal value surfaces as an error here and
// leaves the route unmounted rather than advertising a garbage topic.
//
// No WithManifestRoutes option is passed, so the document is catalog-only and
// discloses no broker topology. The lib handler pre-marshals once and enforces a
// GET/HEAD method allowlist plus hardening headers (Cache-Control: no-store,
// X-Content-Type-Options, X-Frame-Options). It performs NO authentication; the
// caller wraps it in the binary's authz chain. Degraded-safe wiring is the
// composition root's responsibility: a non-nil error here must leave the route
// unmounted (logged at Warn), never fail boot.
func NewManifestHandler(serviceName, source string, defs []events.Definition) (nethttp.Handler, error) {
	catalog, err := libStreaming.NewCatalog(CatalogEntriesFromDefinitions(defs)...)
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest catalog: %w", err)
	}

	descriptor := libStreaming.PublisherDescriptor{
		ServiceName: serviceName,
		Source:      source,
		RoutePath:   ManifestRoutePath,
	}

	handler, err := libStreaming.NewStreamingHandler(descriptor, catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest handler: %w", err)
	}

	return handler, nil
}
