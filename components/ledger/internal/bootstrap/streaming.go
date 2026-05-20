// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for midaz's single
// streaming target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in
// sync.
const streamingPrimaryTargetName = "primary"

// streamingTopicPrefix is the canonical prefix every topic name uses.
// Topic names take the shape "lerian.streaming.<resource>.<event>".
const streamingTopicPrefix = "lerian.streaming."

// noopStreamingCloser is the close hook returned by BuildStreamingEmitter
// when streaming is disabled. It exists only so callers can append a single
// uniform cleanup callback to their existing chain.
func noopStreamingCloser() error { return nil }

// BuildStreamingEmitter returns the lib-streaming Emitter the ledger
// component should inject into its command UseCase, plus a close hook the
// caller must run on shutdown (or on bootstrap failure).
//
// Behaviour:
//   - When cfg.StreamingEnabled is false (the documented default for this
//     pilot) the function returns libStreaming.NewNoopEmitter() and a no-op
//     close hook. No transport client is constructed and no broker
//     connection is attempted.
//   - When STREAMING_BROKERS is empty (LoadConfig surfaces this as an empty
//     Brokers slice) the function ALSO returns a NoopEmitter — the Builder
//     would otherwise reject construction with ErrMissingTarget Brokers.
//   - Otherwise the function builds a single-target catalog-first Producer
//     via libStreaming.NewBuilder(), wiring the configured CloudEvents
//     source onto the Builder and registering all midaz event definitions
//     in the Catalog with a matching RouteDefinition per event.
func BuildStreamingEmitter(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) (libStreaming.Emitter, func() error, error) {
	if cfg == nil {
		return nil, noopStreamingCloser, fmt.Errorf("BuildStreamingEmitter: nil config")
	}

	_ = telemetry

	if !cfg.StreamingEnabled {
		if logger != nil {
			logger.Log(ctx, libLog.LevelInfo, "Streaming disabled (STREAMING_ENABLED=false); using NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so
	// every STREAMING_* knob (MaxBufferedRecords, BatchMaxBytes, CB
	// ratios, CloseTimeout, etc.) gets its documented default rather than
	// the zero value of the struct.
	streamingCfg, warnings, err := libStreaming.LoadConfig()
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to load streaming config: %w", err)
	}

	if logger != nil {
		for _, warning := range warnings {
			logger.Log(ctx, libLog.LevelWarn, "Streaming config warning: "+warning)
		}
	}

	if len(streamingCfg.Brokers) == 0 {
		if logger != nil {
			logger.Log(ctx, libLog.LevelWarn,
				"STREAMING_ENABLED=true but STREAMING_BROKERS is empty; falling back to NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// Build the immutable Catalog of every event midaz emits. Catalog
	// lookup at emit time resolves ResourceType/EventType/SchemaVersion
	// from these entries via the EmitRequest.DefinitionKey, so the
	// Catalog and the per-event Definition vars in pkg/streaming/events
	// MUST stay in sync (the test suite locks the key strings).
	catalog, err := buildCatalog()
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to build streaming catalog: %w", err)
	}

	// Build the route table. One required route per event keyed to the
	// canonical "lerian.streaming.<resource>.<event>" topic name.
	routes, err := buildRoutes(streamingPrimaryTargetName)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to build streaming routes: %w", err)
	}

	emitter, err := libStreaming.NewBuilder().
		Source(streamingCfg.CloudEventsSource).
		Catalog(catalog).
		Routes(routes...).
		Target(libStreaming.TargetConfig{
			Name:    streamingPrimaryTargetName,
			Kind:    libStreaming.TransportKafkaLike,
			Brokers: streamingCfg.Brokers,
		}).
		Build(ctx)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to construct streaming emitter: %w", err)
	}

	if logger != nil {
		logger.Log(ctx, libLog.LevelInfo, "Streaming emitter constructed",
			libLog.String("brokers", strings.Join(streamingCfg.Brokers, ",")),
			libLog.String("client_id", streamingCfg.ClientID),
			libLog.String("ce_source", streamingCfg.CloudEventsSource),
			libLog.Int("catalog_size", catalog.Len()),
			libLog.Int("routes", len(routes)),
		)
	}

	return emitter, emitter.Close, nil
}

// midazEventDefinitions returns the canonical, ordered list of midaz
// event Definitions registered into both the Catalog and the Routes.
// Kept as a single source of truth so adding a new event is a one-place
// change.
func midazEventDefinitions() []events.Definition {
	return []events.Definition{
		events.OrganizationCreatedDefinition,
		events.OrganizationUpdatedDefinition,
		events.OrganizationDeletedDefinition,
		events.LedgerCreatedDefinition,
		events.LedgerUpdatedDefinition,
		events.LedgerDeletedDefinition,
		events.AccountCreatedDefinition,
		events.AccountUpdatedDefinition,
		events.AccountDeletedDefinition,
		events.AssetCreatedDefinition,
		events.AssetUpdatedDefinition,
		events.AssetDeletedDefinition,
		events.PortfolioCreatedDefinition,
		events.PortfolioUpdatedDefinition,
		events.PortfolioDeletedDefinition,
		events.SegmentCreatedDefinition,
		events.SegmentUpdatedDefinition,
		events.SegmentDeletedDefinition,
		events.AccountTypeCreatedDefinition,
		events.AccountTypeUpdatedDefinition,
		events.AccountTypeDeletedDefinition,
		events.OperationRouteCreatedDefinition,
		events.OperationRouteUpdatedDefinition,
		events.OperationRouteDeletedDefinition,
		events.TransactionRouteCreatedDefinition,
		events.TransactionRouteUpdatedDefinition,
		events.TransactionRouteDeletedDefinition,
	}
}

// buildCatalog constructs the immutable lib-streaming Catalog from
// midaz's event Definitions. Every entry maps the canonical
// "<resource>.<event>" key to its ResourceType / EventType /
// SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	defs := midazEventDefinitions()
	entries := make([]libStreaming.EventDefinition, 0, len(defs))

	for _, d := range defs {
		entries = append(entries, libStreaming.EventDefinition{
			Key:           d.Key(),
			ResourceType:  d.ResourceType,
			EventType:     d.EventType,
			SchemaVersion: d.SchemaVersion,
		})
	}

	return libStreaming.NewCatalog(entries...)
}

// buildRoutes constructs one RouteRequired route per midaz event,
// targeting the single broker named targetName. Topic names are
// "lerian.streaming.<resource>.<event>".
//
// Route Keys are composed as "<definition-key>.<target-name>" (e.g.
// "account.created.primary") — Route.Key must match a lower-case
// dot-delimited pattern, and the target-name suffix guarantees uniqueness
// when the same event is later routed to multiple targets (e.g. a parallel
// shadow route).
func buildRoutes(targetName string) ([]libStreaming.RouteDefinition, error) {
	defs := midazEventDefinitions()
	routes := make([]libStreaming.RouteDefinition, 0, len(defs))

	for _, d := range defs {
		key := d.Key()
		routes = append(routes, libStreaming.RouteDefinition{
			Key:           key + "." + targetName,
			DefinitionKey: key,
			Target:        targetName,
			Destination:   libStreaming.KafkaTopic(streamingTopicPrefix + key),
			Requirement:   libStreaming.RouteRequired,
		})
	}

	return routes, nil
}
