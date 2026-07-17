// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/LerianStudio/midaz/v3/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for CRM's single streaming
// target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in sync.
const streamingPrimaryTargetName = "primary"

// streamingTopicPrefix is the canonical prefix every topic name uses. Topic
// names take the shape "lerian.streaming.<service>_<resource>.<event>", where
// <service> is streamingServiceName and hyphens in the <resource>.<event>
// route key are converted to underscores in the topic name only (the route key
// and ce-type stay hyphenated).
const streamingTopicPrefix = "lerian.streaming."

// streamingServiceName is the component service segment embedded in every topic
// name (e.g. "crm" -> lerian.streaming.crm_<resource>.<event>).
const streamingServiceName = "crm"

// noopStreamingCloser is the close hook returned by BuildStreamingEmitter when
// streaming is disabled. It exists only so callers can append a single uniform
// cleanup callback to their existing chain.
func noopStreamingCloser() error { return nil }

// closeStreamingOnBootFailure runs the streaming producer's close hook during a
// partial-boot cleanup and logs a Warn if the drain fails, so a cleanup failure
// on an aborted bootstrap is visible rather than silently dropped. It never
// propagates. On a successful boot the caller disarms it by swapping the hook to
// noopStreamingCloser, which returns nil and logs nothing.
func closeStreamingOnBootFailure(logger libLog.Logger, cleanup func() error) {
	if err := cleanup(); err != nil && logger != nil {
		logger.Log(
			context.Background(), libLog.LevelWarn,
			"Failed to close streaming emitter during bootstrap cleanup",
			libLog.Err(err),
		)
	}
}

// BuildStreamingEmitter returns the lib-streaming Emitter the CRM component
// should inject into its UseCase, plus a close hook the caller must run on
// shutdown (or on bootstrap failure).
//
// Behaviour:
//   - When cfg.StreamingEnabled is false (the documented default for this
//     pilot) the function returns libStreaming.NewNoopEmitter() and a no-op
//     close hook. No transport client is constructed and no broker connection
//     is attempted.
//   - When STREAMING_BROKERS is empty (LoadConfig surfaces this as an empty
//     Brokers slice) the function ALSO returns a NoopEmitter — the Builder
//     would otherwise reject construction with a missing-target error.
//   - Otherwise the function builds a single-target catalog-first Producer via
//     libStreaming.NewBuilder(), wiring the configured CloudEvents source onto
//     the Builder and registering all CRM event definitions in the Catalog
//     with a matching RouteDefinition per event.
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

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so every
	// STREAMING_* knob gets its documented franz-go default rather than the
	// zero value of the struct.
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

	// Build the immutable Catalog of every event CRM emits. Catalog lookup at
	// emit time resolves ResourceType/EventType/SchemaVersion from these
	// entries via the EmitRequest.DefinitionKey, so the Catalog and the
	// per-event Definition vars in pkg/streaming/events MUST stay in sync.
	catalog, err := buildCatalog()
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to build streaming catalog: %w", err)
	}

	// Build the route table. One required route per event keyed to the
	// canonical "lerian.streaming.<service>_<resource>.<event>" topic name.
	routes := buildRoutes(streamingPrimaryTargetName)

	builder := libStreaming.NewBuilder().
		Source(streamingCfg.CloudEventsSource).
		Catalog(catalog).
		Routes(routes...).
		Target(libStreaming.TargetConfig{
			Name:    streamingPrimaryTargetName,
			Kind:    libStreaming.TransportKafkaLike,
			Brokers: streamingCfg.Brokers,
		})

	// Apply TLS from STREAMING_TLS_* env. No-op when STREAMING_TLS_ENABLED=false,
	// so plaintext dev brokers are unaffected. When enabled, the private-CA dial
	// is built from STREAMING_TLS_CA_CERT inside lib-streaming.
	builder = builder.TLSFromConfig(streamingCfg)

	// Apply SASL from STREAMING_SASL_* env via lib-streaming (SASLFromConfig).
	// No-op when STREAMING_SASL_MECHANISM is empty. SASL over plaintext needs
	// STREAMING_SASL_ALLOW_PLAINTEXT=true (dev brokers only); otherwise
	// lib-streaming pairs SASL with TLS and fails closed at Build.
	builder = builder.SASLFromConfig(streamingCfg)

	emitter, err := builder.Build(ctx)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to construct streaming emitter: %w", err)
	}

	if logger != nil {
		// NOTE: only the mechanism name is logged. Username and password are
		// NEVER logged, even at debug level.
		authMode := "none"
		if streamingCfg.SASLMechanism != "" {
			authMode = streamingCfg.SASLMechanism
			if streamingCfg.SASLAllowPlaintext {
				authMode += " (plaintext)"
			}
		}

		logger.Log(
			ctx, libLog.LevelInfo, "Streaming emitter constructed",
			libLog.String("brokers", strings.Join(streamingCfg.Brokers, ",")),
			libLog.String("client_id", streamingCfg.ClientID),
			libLog.String("ce_source", streamingCfg.CloudEventsSource),
			libLog.String("auth", authMode),
			libLog.Int("catalog_size", catalog.Len()),
			libLog.Int("routes", len(routes)),
		)
	}

	return emitter, emitter.Close, nil
}

// crmEventDefinitions returns the canonical, ordered list of CRM event
// Definitions registered into both the Catalog and the Routes. Kept as a single
// source of truth so adding a new event is a one-place change.
func crmEventDefinitions() []events.Definition {
	return []events.Definition{
		events.HolderCreatedDefinition,
		events.HolderUpdatedDefinition,
		events.HolderDeletedDefinition,
		events.AliasCreatedDefinition,
		events.AliasUpdatedDefinition,
		events.AliasDeletedDefinition,
		events.AliasRelatedPartyDeletedDefinition,
	}
}

// buildCatalog constructs the immutable lib-streaming Catalog from CRM's event
// Definitions. Every entry maps the canonical "<resource>.<event>" key to its
// ResourceType / EventType / SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	defs := crmEventDefinitions()
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

// buildRoutes constructs one RouteRequired route per CRM event, targeting the
// single broker named targetName. Topic names are
// "lerian.streaming.<service>_<resource>.<event>" — hyphens in the route key
// are converted to underscores in the topic name ONLY; the route Key and
// DefinitionKey stay hyphenated (the lib-streaming route-key regex rejects
// underscores).
//
// Route Keys are composed as "<definition-key>.<target-name>" (e.g.
// "holder.created.primary") — Route.Key must match a lower-case dot-delimited
// pattern, and the target-name suffix guarantees uniqueness when the same event
// is later routed to multiple targets.
func buildRoutes(targetName string) []libStreaming.RouteDefinition {
	defs := crmEventDefinitions()
	routes := make([]libStreaming.RouteDefinition, 0, len(defs))

	for _, d := range defs {
		key := d.Key()
		routes = append(routes, libStreaming.RouteDefinition{
			Key:           key + "." + targetName,
			DefinitionKey: key,
			Target:        targetName,
			Destination:   libStreaming.KafkaTopic(streamingTopicName(key)),
			Requirement:   libStreaming.RouteRequired,
		})
	}

	return routes
}

// streamingTopicName renders the consumer-facing Kafka topic name for a
// definition key ("<resource>.<event>").
//
// The streaming-hub ingest consumer subscribes via kgo.ConsumeRegex to
// ^lerian.streaming.<seg>.<seg>$ over the [a-z0-9_] charset — exactly two
// segments, no hyphen. To satisfy that grammar while still namespacing topics by
// producing service, the service is folded into the first segment
// ("<service>_<resource>") and hyphens are normalized to underscores. The route
// Key and the CloudEvents type keep their hyphens: lib-streaming's route-key
// grammar requires hyphens and rejects "_", so the underscore form lives ONLY on
// the wire topic name, not on the event identity.
func streamingTopicName(key string) string {
	return streamingTopicPrefix + streamingServiceName + "_" + strings.ReplaceAll(key, "-", "_")
}
