// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
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

// streamingServiceName is the producing-service segment folded into every
// tracer topic name via pkgStreaming.TopicName. Topic names take the shape
// "lerian.streaming.<service>_<resource>.<event>" (service = tracer).
const streamingServiceName = "tracer"

// streamingSource is the default CloudEvents source stamped on every event
// tracer emits. Distinct from the ledger component's source so downstream
// consumers can attribute events to the emitting service. Overridable via
// STREAMING_CLOUDEVENTS_SOURCE (see resolveStreamingSource).
const streamingSource = "lerian.midaz.tracer"

// resolveStreamingSource returns the CloudEvents source to stamp on emitted
// events. The configured STREAMING_CLOUDEVENTS_SOURCE value wins when set
// (after trimming surrounding whitespace); otherwise the in-code default
// streamingSource is used so an unset var never breaks the historical
// behaviour.
func resolveStreamingSource(cfg *Config) string {
	if cfg != nil {
		if source := strings.TrimSpace(cfg.StreamingCloudEventsSource); source != "" {
			return source
		}
	}

	return streamingSource
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
//     with ErrMissingBrokers. The function treats that as an
//     operator-correctable misconfiguration and degrades to a NoopEmitter
//     (no error, Warn logged) rather than aborting bootstrap. Any OTHER
//     LoadConfig failure propagates as a wrapped error.
//   - When tracerEventDefinitions() is empty the function returns a
//     NoopEmitter. An empty catalog can never build a live producer; this
//     is a defensive guard so a future edit that empties the definition set
//     degrades to Noop rather than failing bootstrap.
//   - Otherwise the function builds a single-target catalog-first Producer
//     via libStreaming.NewBuilder(), wiring the tracer CloudEvents source
//     onto the Builder and registering all tracer event definitions in the
//     Catalog with a matching RouteDefinition per event.
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
		if errors.Is(err, libStreaming.ErrMissingBrokers) {
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

	// Build the route table. One required route per event keyed to the
	// canonical "lerian.streaming.<service>_<resource>.<event>" topic name
	// (service = tracer).
	routes := buildRoutes(streamingPrimaryTargetName)

	source := resolveStreamingSource(cfg)

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
	defs := tracerEventDefinitions()
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

// buildRoutes constructs one RouteRequired route per tracer event,
// targeting the single broker named targetName. Topic names are
// "lerian.streaming.<service>_<resource>.<event>" (service = tracer),
// rendered via pkgStreaming.TopicName.
//
// Route Keys are composed as "<definition-key>.<target-name>" (e.g.
// "rule.created.primary") — Route.Key must match a lower-case dot-delimited
// pattern, and the target-name suffix guarantees uniqueness when the same
// event is later routed to multiple targets (e.g. a parallel shadow route).
func buildRoutes(targetName string) []libStreaming.RouteDefinition {
	defs := tracerEventDefinitions()
	routes := make([]libStreaming.RouteDefinition, 0, len(defs))

	for _, d := range defs {
		key := d.Key()
		routes = append(routes, libStreaming.RouteDefinition{
			Key:           key + "." + targetName,
			DefinitionKey: key,
			Target:        targetName,
			Destination:   libStreaming.KafkaTopic(pkgStreaming.TopicName(streamingServiceName, key)),
			Requirement:   libStreaming.RouteRequired,
		})
	}

	return routes
}
