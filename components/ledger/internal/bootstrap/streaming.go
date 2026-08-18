// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	nethttp "net/http"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	billing "github.com/LerianStudio/lib-streaming/v2/billing"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for midaz's single
// streaming target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in
// sync.
const streamingPrimaryTargetName = "primary"

// streamingServiceName is the leading, ACL-scoped service segment of every
// topic name produced by pkgStreaming.TopicName, yielding
// "ledger.<resource>.<event>". The monorepo binary emits every event it owns
// (ledger core, fees, CRM) under this one segment: the per-product segments
// were collapsed into "ledger" so the whole binary shares one namespace and a
// single Kafka ACL prefix "ledger." covers every topic.
const streamingServiceName = "ledger"

// resolveStreamingSource normalizes the configured CloudEvents source to stamp
// on emitted events. STREAMING_CLOUDEVENTS_SOURCE is REQUIRED when streaming is
// enabled: libStreaming.LoadConfig fail-closes with ErrMissingSource on a
// genuinely-unset value, so BuildStreamingEmitter aborts and the binary never
// starts without it (.env.example recommends the bare service name "ledger" so
// ce-source matches the leading ACL-scoped topic segment "ledger." — the route
// Destination topics are "ledger.<resource>.<event>"). This helper only trims
// the configured value and returns it verbatim; the bare service name
// streamingServiceName ("ledger") is a defense-in-depth fallback for a nil or
// whitespace-only config value that slips past LoadConfig's empty-string check,
// NOT the unset-env default.
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
	// canonical "ledger.<resource>.<event>" topic name: every event routes
	// under the single "ledger" service segment.
	routes := buildRoutes(streamingPrimaryTargetName)

	source := resolveStreamingSource(cfg)

	builder := libStreaming.NewBuilder().
		Source(source).
		Catalog(catalog).
		Routes(routes...).
		// The shared billing_recorded route targets a FIXED literal topic owned
		// by the billing package (not a per-product topic from pkgStreaming.TopicName),
		// so it is wired explicitly here rather than through buildRoutes. Merge is
		// replace-by-DefinitionKey: billing_recorded matches no domain route, so it
		// is appended and every domain route stays intact.
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

// catalogEntriesFromDefinitions maps midaz event Definitions onto lib-streaming
// EventDefinition catalog entries, carrying the canonical "<resource>.<event>"
// key and the ResourceType / EventType / SchemaVersion triple. It is shared by
// buildCatalog (the emitter's catalog, which also appends the billing entry) and
// buildManifestCatalog (the manifest's catalog-only view) so the two stay in
// sync on how a Definition becomes a catalog entry.
func catalogEntriesFromDefinitions(defs []events.Definition) []libStreaming.EventDefinition {
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

// buildCatalog constructs the immutable lib-streaming Catalog from
// midaz's event Definitions. Every entry maps the canonical
// "<resource>.<event>" key to its ResourceType / EventType /
// SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	entries := catalogEntriesFromDefinitions(midazEventDefinitions())

	// The shared billing_recorded event is owned by lib-streaming's billing
	// package: its Definition is a ready EventDefinition (Confluent-framed
	// protobuf content type), added as-is rather than via the midaz registry.
	entries = append(entries, billing.Definition())

	return libStreaming.NewCatalog(entries...)
}

// buildManifestCatalog builds the catalog the ledger manifest advertises: every
// midaz event Definition, independent of STREAMING_ENABLED so the manifest is
// served even when publication is off. Unlike buildCatalog it does NOT append
// the shared billing entry — billing.recorded rides a FIXED literal topic owned
// by lib-streaming's billing package, not a pkgStreaming.TopicName-derived one,
// so advertising it here would break the manifest's per-event topic-convergence
// invariant (manifest topic == pkgStreaming.TopicName(service, def.Key())).
func buildManifestCatalog() (libStreaming.Catalog, error) {
	return libStreaming.NewCatalog(catalogEntriesFromDefinitions(midazEventDefinitions())...)
}

// buildPublisherDescriptor builds the lib-streaming PublisherDescriptor for the
// ledger manifest. ServiceName is the bare, ACL-scoped service segment
// (streamingServiceName, "ledger"); SourceBase is the CloudEvents source
// lib-streaming stamps on emitted events (resolveStreamingSource(cfg), also the
// bare "ledger"). SourceBase feeds EventDefinition.Topic(source), so aligning it
// with the service name makes the manifest's advertised per-event topic converge
// with pkgStreaming.TopicName. RoutePath advertises where the manifest is served.
func buildPublisherDescriptor(cfg *Config) libStreaming.PublisherDescriptor {
	return libStreaming.PublisherDescriptor{
		ServiceName: streamingServiceName,
		SourceBase:  resolveStreamingSource(cfg),
		RoutePath:   pkgStreaming.ManifestRoutePath,
	}
}

// BuildStreamingManifestHandler builds the catalog-only lib-streaming manifest
// HTTP handler the ledger serves at pkgStreaming.ManifestRoutePath. It is
// INDEPENDENT of STREAMING_ENABLED: the manifest advertises the event taxonomy
// even when publication is disabled. No WithManifestRoutes option is passed, so
// the document is catalog-only and discloses no broker topology. The lib handler
// pre-marshals once and enforces a GET/HEAD method allowlist plus the hardening
// headers (Cache-Control: no-store, X-Content-Type-Options, X-Frame-Options).
//
// The lib handler performs NO authentication; the caller wraps it in the midaz
// authz chain. Degraded-safe wiring is the composition root's responsibility: a
// non-nil error here must leave the route unmounted (logged at Warn), never fail
// boot.
func BuildStreamingManifestHandler(cfg *Config) (nethttp.Handler, error) {
	catalog, err := buildManifestCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest catalog: %w", err)
	}

	handler, err := libStreaming.NewStreamingHandler(buildPublisherDescriptor(cfg), catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to build streaming manifest handler: %w", err)
	}

	return handler, nil
}

// buildRoutes constructs one RouteRequired route per midaz event,
// targeting the single broker named targetName. Topic names are
// "ledger.<resource>.<event>": every event routes under the single "ledger"
// service segment, with the underscore-canonical Definition.Key() as the
// two trailing segments.
//
// Route Keys are composed as "<route-key>.<target-name>" (e.g.
// "account.created.primary"), where <route-key> is the hyphenated routing
// handle (RouteKey()) — Route.Key must match lib-streaming's lower-case
// hyphenated dot-delimited grammar, and the target-name suffix guarantees
// uniqueness when the same event is later routed to multiple targets (e.g. a
// parallel shadow route). The wire topic, by contrast, derives from the
// underscore-canonical Key() so it converges with EventDefinition.Topic.
func buildRoutes(targetName string) []libStreaming.RouteDefinition {
	defs := midazEventDefinitions()
	routes := make([]libStreaming.RouteDefinition, 0, len(defs))

	for _, def := range defs {
		key := def.Key()
		routeKey := def.RouteKey()
		routes = append(routes, libStreaming.RouteDefinition{
			Key:           routeKey + "." + targetName,
			DefinitionKey: key,
			Target:        targetName,
			Destination:   libStreaming.KafkaTopic(pkgStreaming.TopicName(streamingServiceName, key)),
			Requirement:   libStreaming.RouteRequired,
		})
	}

	return routes
}
