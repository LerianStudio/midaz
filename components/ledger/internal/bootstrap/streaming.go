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
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	billing "github.com/LerianStudio/lib-streaming/v3/billing"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for midaz's single
// streaming target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in
// sync.
const streamingPrimaryTargetName = "primary"

// streamingServiceName is this application's name on the streaming roster: the
// ce-source it stamps on every event and the single segment its one topic is
// derived from, "lerian.streaming.ledger". The monorepo binary emits every
// event it owns (ledger core, fees, CRM) under this one application name, so a
// Kafka ACL scoped to the three names lib-streaming derives from it (topic,
// commands queue, DLQ) covers everything the binary writes.
const streamingServiceName = "ledger"

// streamingFactRouteKey identifies the single catch-all route that carries every
// fact the ledger emits to its application topic. Under one topic per producing
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
// (.env.example recommends the roster name "ledger"). lib-streaming REJECTS a
// source that is not a single dot-free lowercase segment rather than rewriting
// it, so a malformed value fails startup instead of silently colonizing another
// application's topic namespace.
//
// Whitespace is trimmed ONLY to recognize "unset". The value returned is the RAW
// configured one, deliberately: it is what RequireRosterSource compares, and
// lib-streaming's ValidateSource rejects " ledger " because a space is not in the
// ce-source charset. A helper that returned the trimmed value would wave a padded
// source through the gate with streaming OFF and then refuse boot the day the flag
// flips — the exact deferred failure the gate exists to pull forward.
//
// streamingServiceName ("ledger") is a defense-in-depth fallback for a nil or
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

// BuildStreamingEmitter returns the lib-streaming Emitter the ledger
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
//     the roster gate's posture rather than a degraded start.
//   - Otherwise the function builds a single-target catalog-first Producer
//     via libStreaming.NewBuilder(), wiring the configured CloudEvents
//     source onto the Builder, registering all midaz event definitions in
//     the Catalog, and pointing one catch-all route at the application topic
//     derived from that source.
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

	// Fail closed on a ce-source that is not the roster name, BEFORE the enabled
	// check: the topics and Kafka ACLs exist for the roster name only, so any other
	// value publishes into a stream that neither exists nor is granted — and midaz's
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

	// Delegate env-var loading + defaulting to libStreaming.LoadConfig so
	// every STREAMING_* knob (MaxBufferedRecords, BatchMaxBytes, CB
	// ratios, CloseTimeout, etc.) gets its documented default rather than
	// the zero value of the struct.
	streamingCfg, warnings, err := libStreaming.LoadConfig()
	if err != nil {
		// LoadConfig validates the broker list itself, so the missing-broker case
		// surfaces here rather than at the shared gate below. Route it through the
		// gate anyway: ledger and tracer then refuse boot with one error identity
		// and one piece of operator guidance — including the sentence that stops
		// someone "fixing" it by pointing STREAMING_BROKERS at a dead host. Any
		// OTHER LoadConfig failure is a different config error and propagates wrapped.
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

	// Fail closed on an enabled producer with no broker. LoadConfig validates the
	// broker list itself when it agrees that streaming is enabled, so in a normal
	// env-driven boot this gate is reached only when the two parses of
	// STREAMING_ENABLED disagree — but that is precisely the case that used to fall
	// through to a NoopEmitter, discarding every event while the ledger, which has
	// no streaming readiness prober, stayed Ready throughout.
	if err := pkgStreaming.RequireBrokers(streamingCfg.Brokers); err != nil {
		return nil, noopStreamingCloser, err
	}

	// A Lerian-hosted deployment must not dial the broker in cleartext. Postgres,
	// Mongo, Redis, Vault and RabbitMQ are gated by ValidateSaaSTLS during readyz
	// wiring; the Kafka flag belongs to lib-streaming's env contract and exists
	// only once LoadConfig has run, so the same rule is applied here — the first
	// point where STREAMING_TLS_ENABLED is known. This is a BOOT gate only: the
	// ledger has no streaming readiness prober, so there is no /readyz checker to
	// hang a streaming TLS reporter off (the readyz `tls` field is a byproduct of
	// a live DependencyChecker, and inventing a Kafka health probe here would be a
	// new readiness dependency, not a TLS report).
	if err := ValidateSaaSStreamingTLS(cfg.DeploymentMode, streamingCfg.TLSEnabled); err != nil {
		return nil, noopStreamingCloser, err
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

	source := resolveStreamingSource(cfg)

	// Build the route table: ONE required catch-all route carrying every fact
	// the ledger emits to its single application topic.
	routes, err := buildRoutes(streamingPrimaryTargetName, source)
	if err != nil {
		return nil, noopStreamingCloser, err
	}

	builder := libStreaming.NewBuilder().
		Source(source).
		Catalog(catalog).
		Routes(routes...).
		// The shared billing_recorded route targets a FIXED literal topic owned
		// by the billing package, not the ledger's application topic, so it is
		// wired explicitly here rather than through buildRoutes. It is scoped to
		// the billing_recorded DefinitionKey on the SAME target as the catch-all,
		// which is precisely how lib-streaming expresses "this one event goes
		// somewhere else": the scoped route REPLACES the catch-all for that
		// definition on that target, so billing_recorded rides the billing topic
		// only and never double-publishes onto the ledger's own stream.
		RouteOverrides(billing.Route()).
		Target(libStreaming.TargetConfig{
			Name:    streamingPrimaryTargetName,
			Kind:    libStreaming.TransportKafkaLike,
			Brokers: streamingCfg.Brokers,
		})

	// SASL/TLS are owned by lib-streaming: TLSFromConfig and SASLFromConfig
	// read the STREAMING_TLS_* and STREAMING_SASL_* knobs already parsed by
	// LoadConfig and wire the broker dial. midaz does not parse these itself.
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

// buildBillingSerializerFromEnv loads the streaming config from the environment
// and delegates to buildBillingSerializer. It is the composition-root entry
// point (config.go) uses; the env read is separated from the network-free
// decision core so the latter stays unit-testable with a hand-built config.
//
// A LoadConfig failure degrades gracefully to a nil serializer (billing
// disabled) rather than failing boot — mirroring the builder's posture.
func buildBillingSerializerFromEnv(ctx context.Context, logger libLog.Logger) *billing.Serializer {
	cfg, _, err := libStreaming.LoadConfig()
	if err != nil {
		warnBillingDisabled(ctx, logger, err)

		return nil
	}

	return buildBillingSerializer(ctx, cfg, logger)
}

// buildBillingSerializer builds the billing Serializer with graceful
// degradation: any wiring failure yields a nil serializer (billing disabled)
// and a single WARN, never a boot failure. This preserves backward-compatible
// startup for deployments without a Schema Registry.
//
// It returns the concrete *billing.Serializer (not the command seam) so the
// caller can nil-guard the interface assignment at the injection site and avoid
// the typed-nil-interface trap: a nil *billing.Serializer assigned straight to
// an interface compares NON-nil, so the caller assigns only when non-nil.
//
// Branches:
//   - streaming disabled (Enabled=false): return nil, no registry contact.
//   - context canceled/expired: WARN + nil, before any registry contact.
//   - NewSchemaRegistryClient fails (empty URL or partial credentials, both
//     fail-closed): WARN + nil.
//   - billing.NewSerializer fails (registry round-trip): WARN + nil.
//   - success: the constructed serializer.
func buildBillingSerializer(ctx context.Context, cfg libStreaming.Config, logger libLog.Logger) *billing.Serializer {
	if !cfg.Enabled {
		return nil
	}

	if err := ctx.Err(); err != nil {
		warnBillingDisabled(ctx, logger, err)

		return nil
	}

	client, err := libStreaming.NewSchemaRegistryClient(cfg)
	if err != nil {
		warnBillingDisabled(ctx, logger, err)

		return nil
	}

	serializer, err := billing.NewSerializer(ctx, client)
	if err != nil {
		warnBillingDisabled(ctx, logger, err)

		return nil
	}

	return serializer
}

// warnBillingDisabled logs the single, uniform graceful-degradation WARN shared
// by every billing-serializer failure branch. The err is attached; no secret is
// ever included (NewSchemaRegistryClient's error never carries the password).
func warnBillingDisabled(ctx context.Context, logger libLog.Logger, err error) {
	if logger == nil {
		return
	}

	logger.Log(ctx, libLog.LevelWarn, "Billing serializer disabled",
		libLog.Bool("billing_enabled", false), libLog.Err(err))
}

// midazEventDefinitions returns the canonical, ordered list of midaz event
// Definitions, registered into both the Catalog and the Routes. Every event is
// routed under the single "ledger" service segment, so the list carries no
// per-product service. Kept as a single source of truth so adding a new event
// is a one-place change.
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
		events.BalanceChangedDefinition,
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
		// CRM
		events.HolderCreatedDefinition,
		events.HolderUpdatedDefinition,
		events.HolderDeletedDefinition,
		events.InstrumentCreatedDefinition,
		events.InstrumentUpdatedDefinition,
		events.InstrumentDeletedDefinition,
		events.InstrumentRelatedPartyDeletedDefinition,
	}
}

// buildCatalog constructs the immutable lib-streaming Catalog from
// midaz's event Definitions. Every entry maps the canonical
// "<resource>.<event>" key to its ResourceType / EventType /
// SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	entries := pkgStreaming.CatalogEntriesFromDefinitions(midazEventDefinitions())

	// The shared billing_recorded event is owned by lib-streaming's billing
	// package: its Definition is a ready EventDefinition (Confluent-framed
	// protobuf content type), added as-is rather than via the midaz registry.
	entries = append(entries, billing.Definition())

	return libStreaming.NewCatalog(entries...)
}

// BuildStreamingManifestHandler builds the catalog-only lib-streaming manifest
// HTTP handler the ledger serves at pkgStreaming.ManifestRoutePath. It is a thin
// wrapper over pkgStreaming.NewManifestHandler.
//
// The descriptor Source is the SAME resolved ce-source the emitter publishes
// under, so the application topic the manifest advertises is by construction the
// topic the ledger writes: under one topic per application the manifest carries
// that topic at document level, derived from the descriptor's Source. The
// manifest stays INDEPENDENT of STREAMING_ENABLED — it is served whether or not
// a producer was built, falling back to the roster name when no source is
// configured.
//
// The manifest excludes the shared billing entry (billing.recorded rides a FIXED
// literal topic owned by the billing package, not the ledger's application
// topic) by advertising only midazEventDefinitions().
func BuildStreamingManifestHandler(cfg *Config) (nethttp.Handler, error) {
	return pkgStreaming.NewManifestHandler(streamingServiceName, resolveStreamingSource(cfg), midazEventDefinitions())
}

// buildRoutes constructs the ledger's single RouteRequired catch-all route,
// carrying every fact in the catalog to the one application topic
// "lerian.streaming.<source>" on the broker named targetName.
//
// DefinitionKey is deliberately EMPTY: that is what makes the route a catch-all
// serving every definition. One topic per producing application leaves nothing
// to fan out per event — every event has the same destination — so a single
// route replaces the former one-route-per-catalog-entry table. A definition that
// needs a different destination is expressed as a scoped RouteOverrides entry on
// this same target (see the billing route in BuildStreamingEmitter).
//
// The destination is derived through libStreaming.AppTopic, which VALIDATES the
// source and returns an error rather than handing back a topic name built from a
// malformed one.
func buildRoutes(targetName, source string) ([]libStreaming.RouteDefinition, error) {
	topic, err := libStreaming.AppTopic(source)
	if err != nil {
		return nil, fmt.Errorf("failed to derive ledger application topic: %w", err)
	}

	return []libStreaming.RouteDefinition{{
		Key:         streamingFactRouteKey,
		Target:      targetName,
		Destination: libStreaming.KafkaTopic(topic),
		Requirement: libStreaming.RouteRequired,
	}}, nil
}
