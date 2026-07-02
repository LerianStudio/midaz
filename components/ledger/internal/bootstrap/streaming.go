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
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// SASL mechanism names accepted by STREAMING_SASL_MECHANISM. Compared
// case-insensitively at parse time so operators can write any casing.
const (
	saslMechanismPlain    = "PLAIN"
	saslMechanismScram256 = "SCRAM-SHA-256"
	saslMechanismScram512 = "SCRAM-SHA-512"
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

		logger.Log(ctx, libLog.LevelInfo, "Streaming emitter constructed",
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
			mechanism)
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
			raw, saslMechanismPlain, saslMechanismScram256, saslMechanismScram512)
	}
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
		// account_type.* events are intentionally NOT registered:
		// internal validation config, the type label flows through
		// account.* events as a string field.
		events.OperationRouteCreatedDefinition,
		events.OperationRouteUpdatedDefinition,
		events.OperationRouteDeletedDefinition,
		events.TransactionRouteCreatedDefinition,
		events.TransactionRouteUpdatedDefinition,
		events.TransactionRouteDeletedDefinition,
		events.BalanceCreatedDefinition,
		events.BalanceConfigChangedDefinition,
		events.BalanceDeletedDefinition,
		events.BalanceOverdraftDrawnDefinition,
		events.BalanceOverdraftRepaidDefinition,
		events.BalanceOverdraftClearedDefinition,
		events.TransactionPostedDefinition,
		events.TransactionCommittedDefinition,
		events.TransactionCanceledDefinition,
		events.TransactionRevertedDefinition,
		// Fees
		events.FeesPackageCreatedDefinition,
		events.FeesPackageUpdatedDefinition,
		events.FeesPackageDeletedDefinition,
		events.FeesBillingPackageCreatedDefinition,
		events.FeesBillingPackageUpdatedDefinition,
		events.FeesBillingPackageDeletedDefinition,
		events.FeesAppliedDefinition,
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
func buildRoutes(targetName string) []libStreaming.RouteDefinition {
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

	return routes
}
