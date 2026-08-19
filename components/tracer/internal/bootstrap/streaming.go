// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOtel "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for tracer's single
// streaming target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in
// sync.
const streamingPrimaryTargetName = "primary"

// streamingServiceName is this application's name on the streaming roster: the
// ce-source tracer stamps on every event and the single segment its one topic is
// derived from, "lerian.streaming.tracer". A Kafka ACL scoped to the three names
// lib-streaming derives from it (topic, commands queue, DLQ) covers everything
// tracer writes.
const streamingServiceName = "tracer"

// streamingFactRouteKey identifies the single catch-all route that carries every
// fact tracer emits to its application topic. Under one topic per producing
// application there is nothing left to fan out per event, so one route replaces
// the former route-per-catalog-entry table. The key is an operator-facing
// identifier only — it never reaches the wire.
const streamingFactRouteKey = streamingServiceName + ".facts." + streamingPrimaryTargetName

// resolveStreamingSource normalizes the configured CloudEvents source. The
// resolved value is load-bearing three times over and MUST be one value: it is
// stamped as ce-source, it derives the application topic every event rides
// (libStreaming.AppTopic), and it is what the manifest advertises to the
// streaming hub. A divergence between any two of those would point provisioning
// at a stream nothing writes, and a source-verifying consumer would quarantine
// every record it received.
//
// STREAMING_CLOUDEVENTS_SOURCE is REQUIRED when streaming is enabled:
// libStreaming.LoadConfig fail-closes with ErrMissingSource on a genuinely-unset
// value, so BuildStreamingEmitter aborts and the binary never starts without it
// (.env.example recommends the roster name "tracer"). lib-streaming REJECTS a
// source that is not a single dot-free lowercase segment rather than rewriting
// it, so a malformed value fails startup instead of silently colonizing another
// application's topic namespace.
//
// Whitespace is trimmed ONLY to recognize "unset". The value returned is the RAW
// configured one, deliberately: it is what RequireRosterSource compares, and
// lib-streaming's ValidateSource rejects " tracer " because a space is not in the
// ce-source charset. A helper that returned the trimmed value would wave a padded
// source through the gate with streaming OFF and then refuse boot the day the flag
// flips — the exact deferred failure the gate exists to pull forward.
//
// streamingServiceName ("tracer") is a defense-in-depth fallback for a nil or
// whitespace-only config value that slips past LoadConfig's empty-string check,
// NOT the unset-env default.
func resolveStreamingSource(cfg *Config) string {
	if cfg != nil && strings.TrimSpace(cfg.StreamingCloudEventsSource) != "" {
		return cfg.StreamingCloudEventsSource
	}

	return streamingServiceName
}

// noopStreamingCloser is the close hook returned by BuildStreamingEmitter
// when streaming is disabled. It exists only so callers can append a single
// uniform cleanup callback to their existing chain.
func noopStreamingCloser() error { return nil }

// BuildStreamingEmitter returns the lib-streaming Emitter the tracer
// component should inject into its command UseCase, plus a close hook the
// caller must run on shutdown (or on bootstrap failure).
//
// Behaviour:
//   - When cfg.StreamingEnabled is false (the documented default for this
//     pilot) the function returns libStreaming.NewNoopEmitter() and a no-op
//     close hook. No transport client is constructed and no broker
//     connection is attempted.
//   - When STREAMING_BROKERS is empty the function refuses boot via
//     pkgStreaming.RequireBrokers. An enabled producer with nowhere to publish
//     discards every event silently while readiness reports healthy, so it gets
//     the roster gate's posture rather than a degraded start. Any OTHER
//     LoadConfig failure propagates as a wrapped error.
//   - When tracerEventDefinitions() is empty the function refuses boot. An empty
//     catalog can never build a live producer, so with streaming ENABLED it is
//     the same silent-total-loss condition as a missing broker.
//   - Otherwise the function builds a single-target catalog-first Producer
//     via libStreaming.NewBuilder(), wiring the tracer CloudEvents source
//     onto the Builder, registering all tracer event definitions in the
//     Catalog, and pointing one catch-all route at the application topic
//     derived from that source.
func BuildStreamingEmitter(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	telemetry *libOtel.Telemetry,
) (libStreaming.Emitter, func() error, error) {
	if cfg == nil {
		return nil, noopStreamingCloser, fmt.Errorf("BuildStreamingEmitter: nil config")
	}

	if err := ctx.Err(); err != nil {
		return nil, noopStreamingCloser, err
	}

	_ = telemetry

	// Fail closed on a ce-source that is not the roster name, BEFORE the enabled
	// check: the topics and Kafka ACLs exist for the roster name only, so any other
	// value publishes into a stream that neither exists nor is granted — and the
	// IMPORTANT posture would swallow every one of those failures as a Warn while the
	// pod stays Ready. Checked even when streaming is disabled so a source left over
	// from the pre-v3 dotted or URI shape fails startup instead of waiting in an env
	// file for someone to flip the flag.
	if err := pkgStreaming.RequireRosterSource(resolveStreamingSource(cfg), streamingServiceName); err != nil {
		return nil, noopStreamingCloser, err
	}

	if !cfg.StreamingEnabled {
		if logger != nil {
			logger.Log(ctx, libLog.LevelInfo, "Streaming disabled (STREAMING_ENABLED=false); using NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// An empty catalog can never build a live producer. With streaming ENABLED
	// that is the same condition RequireBrokers refuses: every emit would resolve
	// to nothing while readiness reported the dependency healthy. Fail closed so a
	// future edit that empties the definition set cannot ship as a silent no-op.
	if len(tracerEventDefinitions()) == 0 {
		return nil, noopStreamingCloser, fmt.Errorf(
			"streaming is enabled but no tracer event definitions are registered:" +
				" an empty catalog publishes nothing while readiness reports healthy")
	}

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so
	// every STREAMING_* knob (MaxBufferedRecords, BatchMaxBytes, CB
	// ratios, CloseTimeout, etc.) gets its documented default rather than
	// the zero value of the struct.
	streamingCfg, warnings, err := libStreaming.LoadConfig()
	if err != nil {
		// LoadConfig validates the broker list itself, so the missing-broker case
		// surfaces here rather than at the shared gate below. Route it through the
		// gate anyway: ledger and tracer then refuse boot with one error identity
		// and one piece of operator guidance. Any OTHER LoadConfig failure is a
		// different config error and propagates wrapped.
		if errors.Is(err, libStreaming.ErrProducerMissingBrokers) {
			return nil, noopStreamingCloser, pkgStreaming.RequireBrokers(streamingCfg.Brokers)
		}

		return nil, noopStreamingCloser, fmt.Errorf("failed to load streaming config: %w", err)
	}

	if logger != nil {
		for _, warning := range warnings {
			logger.Log(ctx, libLog.LevelWarn, "Streaming config warning: "+warning)
		}
	}

	return buildLiveStreamingEmitter(ctx, cfg, logger, streamingCfg)
}

// buildLiveStreamingEmitter constructs the single-target, catalog-first
// Producer once BuildStreamingEmitter's early-return guards have passed.
// Split out so the guard-heavy entry point stays within the package
// cyclomatic-complexity budget.
func buildLiveStreamingEmitter(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	streamingCfg libStreaming.Config,
) (libStreaming.Emitter, func() error, error) {
	// Build the immutable Catalog of every event tracer emits. Catalog
	// lookup at emit time resolves ResourceType/EventType/SchemaVersion
	// from these entries via the EmitRequest.DefinitionKey, so the
	// Catalog and the per-event Definition vars in pkg/streaming/events
	// MUST stay in sync (the test suite locks the key strings).
	catalog, err := buildCatalog()
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to build streaming catalog: %w", err)
	}

	source := resolveStreamingSource(cfg)

	// Defense in depth against a config that reaches here with no broker: the
	// enabled flag on the midaz Config and the one libStreaming.LoadConfig reads
	// are two separate parses of STREAMING_ENABLED, so a value only one of them
	// accepts would skip LoadConfig's own broker validation and arrive here with an
	// empty list. Building the Target on it would yield a producer with nowhere to
	// publish, which is silent total event loss.
	if err := pkgStreaming.RequireBrokers(streamingCfg.Brokers); err != nil {
		return nil, noopStreamingCloser, err
	}

	// Build the route table: ONE required catch-all route carrying every fact
	// tracer emits to its single application topic.
	routes, err := buildRoutes(streamingPrimaryTargetName, source)
	if err != nil {
		return nil, noopStreamingCloser, err
	}

	builder := libStreaming.NewBuilder().
		Source(source).
		Catalog(catalog).
		Routes(routes...).
		Target(libStreaming.TargetConfig{
			Name:    streamingPrimaryTargetName,
			Kind:    libStreaming.TransportKafkaLike,
			Brokers: streamingCfg.Brokers,
		})

	// SASL/TLS are owned by lib-streaming: TLSFromConfig and SASLFromConfig read
	// the STREAMING_TLS_* and STREAMING_SASL_* knobs already parsed by LoadConfig
	// and wire the broker dial. tracer does not parse these itself — a hand-rolled
	// SASL mechanism here is what left STREAMING_TLS_ENABLED with no reader at all,
	// making a TLS broker unreachable and forcing every authenticated deployment
	// through the unsafe plaintext opt-in.
	builder = builder.TLSFromConfig(streamingCfg)
	builder = builder.SASLFromConfig(streamingCfg)

	emitter, err := builder.Build(ctx)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to construct streaming emitter: %w", err)
	}

	if logger != nil {
		// NOTE: only mechanism name is logged. Username and password are
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
			libLog.String("ce_source", source),
			libLog.String("auth", authMode),
			libLog.Bool("tls", streamingCfg.TLSEnabled),
			libLog.Int("catalog_size", catalog.Len()),
			libLog.Int("routes", len(routes)),
		)
	}

	return emitter, emitter.Close, nil
}

// tracerEventDefinitions returns the canonical, ordered list of tracer
// event Definitions registered into both the Catalog and the Routes.
// Kept as a single source of truth so adding a new event is a one-place
// change.
//
// Registers the full Rule and Limit lifecycle events (created, updated,
// activated, deactivated, drafted, deleted) — six per resource, twelve total.
func tracerEventDefinitions() []events.Definition {
	return []events.Definition{
		events.RuleCreatedDefinition,
		events.RuleUpdatedDefinition,
		events.RuleActivatedDefinition,
		events.RuleDeactivatedDefinition,
		events.RuleDraftedDefinition,
		events.RuleDeletedDefinition,
		events.LimitCreatedDefinition,
		events.LimitUpdatedDefinition,
		events.LimitActivatedDefinition,
		events.LimitDeactivatedDefinition,
		events.LimitDraftedDefinition,
		events.LimitDeletedDefinition,
	}
}

// buildCatalog constructs the immutable lib-streaming Catalog from
// tracer's event Definitions. Every entry maps the canonical
// "<resource>.<event>" key to its ResourceType / EventType /
// SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	return libStreaming.NewCatalog(pkgStreaming.CatalogEntriesFromDefinitions(tracerEventDefinitions())...)
}

// BuildStreamingManifestHandler builds the catalog-only lib-streaming manifest
// HTTP handler the tracer serves at pkgStreaming.ManifestRoutePath. It is a thin
// wrapper over pkgStreaming.NewManifestHandler.
//
// The descriptor Source is the SAME resolved ce-source the emitter publishes
// under, so the application topic the manifest advertises is by construction the
// topic tracer writes: under one topic per application the manifest carries that
// topic at document level, derived from the descriptor's Source. The manifest
// stays INDEPENDENT of STREAMING_ENABLED — it is served whether or not a producer
// was built, falling back to the roster name when no source is configured.
func BuildStreamingManifestHandler(cfg *Config) (nethttp.Handler, error) {
	return pkgStreaming.NewManifestHandler(streamingServiceName, resolveStreamingSource(cfg), tracerEventDefinitions())
}

// buildRoutes constructs tracer's single RouteRequired catch-all route, carrying
// every fact in the catalog to the one application topic
// "lerian.streaming.<source>" on the broker named targetName.
//
// DefinitionKey is deliberately EMPTY: that is what makes the route a catch-all
// serving every definition. One topic per producing application leaves nothing to
// fan out per event — every event has the same destination — so a single route
// replaces the former one-route-per-catalog-entry table.
//
// The destination is derived through libStreaming.AppTopic, which VALIDATES the
// source and returns an error rather than handing back a topic name built from a
// malformed one.
func buildRoutes(targetName, source string) ([]libStreaming.RouteDefinition, error) {
	topic, err := libStreaming.AppTopic(source)
	if err != nil {
		return nil, fmt.Errorf("failed to derive tracer application topic: %w", err)
	}

	return []libStreaming.RouteDefinition{{
		Key:         streamingFactRouteKey,
		Target:      targetName,
		Destination: libStreaming.KafkaTopic(topic),
		Requirement: libStreaming.RouteRequired,
	}}, nil
}
