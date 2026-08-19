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
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// SASL mechanism names accepted by STREAMING_SASL_MECHANISM. Compared
// case-insensitively at parse time so operators can write any casing.
const (
	saslMechanismPlain    = "PLAIN"
	saslMechanismScram256 = "SCRAM-SHA-256"
	saslMechanismScram512 = "SCRAM-SHA-512"
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
// application's topic namespace. This helper only trims the configured value and
// returns it verbatim; streamingServiceName ("tracer") is a defense-in-depth
// fallback for a nil or whitespace-only config value that slips past
// LoadConfig's empty-string check, NOT the unset-env default.
func resolveStreamingSource(cfg *Config) string {
	if cfg != nil {
		if source := strings.TrimSpace(cfg.StreamingCloudEventsSource); source != "" {
			return source
		}
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
//   - When STREAMING_BROKERS is empty, libStreaming.LoadConfig fails closed
//     with ErrProducerMissingBrokers. The function treats that as an
//     operator-correctable misconfiguration and degrades to a NoopEmitter
//     (no error, Warn logged) rather than aborting bootstrap. Any OTHER
//     LoadConfig failure propagates as a wrapped error.
//   - When tracerEventDefinitions() is empty the function returns a
//     NoopEmitter. An empty catalog can never build a live producer; this
//     is a defensive guard so a future edit that empties the definition set
//     degrades to Noop rather than failing bootstrap.
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

	if !cfg.StreamingEnabled {
		if logger != nil {
			logger.Log(ctx, libLog.LevelInfo, "Streaming disabled (STREAMING_ENABLED=false); using NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// An empty catalog can never build a live producer. Defensive guard so a
	// future edit that empties the definition set degrades to Noop rather
	// than reaching the Builder with zero routes.
	if len(tracerEventDefinitions()) == 0 {
		if logger != nil {
			logger.Log(ctx, libLog.LevelInfo,
				"Streaming enabled but no tracer events are registered yet; using NoopEmitter")
		}

		return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
	}

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so
	// every STREAMING_* knob (MaxBufferedRecords, BatchMaxBytes, CB
	// ratios, CloseTimeout, etc.) gets its documented default rather than
	// the zero value of the struct.
	streamingCfg, warnings, err := libStreaming.LoadConfig()
	if err != nil {
		// A missing broker list is an operator-correctable misconfiguration,
		// not a reason to abort bootstrap: degrade to a NoopEmitter so the
		// service starts with streaming disabled. Any OTHER LoadConfig
		// failure is a genuine config error and propagates.
		if errors.Is(err, libStreaming.ErrProducerMissingBrokers) {
			if logger != nil {
				logger.Log(ctx, libLog.LevelWarn,
					"STREAMING_ENABLED=true but STREAMING_BROKERS is empty; falling back to NoopEmitter")
			}

			return libStreaming.NewNoopEmitter(), noopStreamingCloser, nil
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

	// Apply SASL/TLS auth knobs from cfg. resolveSASLMechanism returns a
	// nil mechanism (and an empty mechanism name) when SASL is disabled,
	// in which case the Builder is left untouched and the producer dials
	// the broker without authentication — matching the historical local/dev
	// behaviour. When SASL is enabled but TLS is not, lib-streaming
	// rejects construction with ErrPlaintextSASLNotAllowed unless the
	// caller also opts into AllowPlaintextSASL — gated behind
	// STREAMING_ALLOW_PLAINTEXT_SASL=true for dev brokers.
	mechanism, mechanismName, err := resolveSASLMechanism(cfg)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to resolve streaming SASL mechanism: %w", err)
	}

	if mechanism != nil {
		builder = builder.SASL(mechanism)

		if cfg.StreamingAllowPlaintextSASL {
			builder = builder.AllowPlaintextSASL()
		}
	}

	emitter, err := builder.Build(ctx)
	if err != nil {
		return nil, noopStreamingCloser, fmt.Errorf("failed to construct streaming emitter: %w", err)
	}

	if logger != nil {
		// NOTE: only mechanism name is logged. Username and password are
		// NEVER logged, even at debug level.
		authMode := "none"
		if mechanismName != "" {
			authMode = mechanismName
			if cfg.StreamingAllowPlaintextSASL {
				authMode += " (plaintext)"
			}
		}

		logger.Log(
			ctx, libLog.LevelInfo, "Streaming emitter constructed",
			libLog.String("brokers", strings.Join(streamingCfg.Brokers, ",")),
			libLog.String("client_id", streamingCfg.ClientID),
			libLog.String("ce_source", source),
			libLog.String("auth", authMode),
			libLog.Int("catalog_size", catalog.Len()),
			libLog.Int("routes", len(routes)),
		)
	}

	return emitter, emitter.Close, nil
}

// resolveSASLMechanism inspects the streaming SASL knobs on cfg and
// returns the matching franz-go sasl.Mechanism plus its canonical name.
//
// Behaviour:
//   - StreamingSASLMechanism empty (after trimming) → returns (nil, "", nil).
//     The Builder stays unauthenticated, matching the existing local/dev
//     default.
//   - StreamingSASLMechanism set but USERNAME or PASSWORD empty → returns
//     a config error. SASL with empty credentials would either be rejected
//     by the broker after I/O (PLAIN) or panic inside franz-go's SCRAM
//     handshake; failing closed at bootstrap is the safer contract.
//   - StreamingSASLMechanism unrecognised → returns a config error
//     enumerating the accepted values.
//
// The mechanism name returned is the canonical upper-case form
// ("PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512") — used for the bootstrap
// log line. Username and password are NEVER returned to the caller and
// never logged.
func resolveSASLMechanism(cfg *Config) (sasl.Mechanism, string, error) {
	raw := strings.TrimSpace(cfg.StreamingSASLMechanism)
	if raw == "" {
		return nil, "", nil
	}

	mechanism := strings.ToUpper(raw)

	user := cfg.StreamingSASLUsername
	pass := cfg.StreamingSASLPassword

	if user == "" || pass == "" {
		return nil, "", fmt.Errorf(
			"STREAMING_SASL_MECHANISM=%q requires STREAMING_SASL_USERNAME and STREAMING_SASL_PASSWORD",
			mechanism,
		)
	}

	switch mechanism {
	case saslMechanismPlain:
		return plain.Auth{User: user, Pass: pass}.AsMechanism(), saslMechanismPlain, nil
	case saslMechanismScram256:
		return scram.Auth{User: user, Pass: pass}.AsSha256Mechanism(), saslMechanismScram256, nil
	case saslMechanismScram512:
		return scram.Auth{User: user, Pass: pass}.AsSha512Mechanism(), saslMechanismScram512, nil
	default:
		return nil, "", fmt.Errorf(
			"STREAMING_SASL_MECHANISM=%q is not supported (accepted: %s, %s, %s)",
			raw, saslMechanismPlain, saslMechanismScram256, saslMechanismScram512,
		)
	}
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
	return pkgStreaming.NewManifestHandler(resolveStreamingSource(cfg), tracerEventDefinitions())
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
