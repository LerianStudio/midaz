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
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// streamingPrimaryTargetName is the canonical name for midaz's single
// streaming target. Lives as a const so the Builder.Target call, the
// RouteDefinition.Target field, and the route-key suffix all stay in
// sync.
const streamingPrimaryTargetName = "primary"

// Per-product service segments folded into topic names by
// pkgStreaming.TopicName, yielding "lerian.streaming.<service>_<resource>.<event>".
// The monorepo binary emits events on behalf of three products, each keeping the
// service segment it had before consolidation: ledger core, fees, and CRM.
const (
	serviceLedger = "ledger"
	serviceFee    = "fee"
	serviceCRM    = "crm"
)

// routedDefinition pairs an event's pure wire Definition with the producing
// service that owns its topic segment. events.Definition is the wire contract
// and carries no service; the service lives here in the bootstrap registry so
// routing stays a composition-root concern.
type routedDefinition struct {
	def     events.Definition
	service string
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
	// canonical "lerian.streaming.<service>_<resource>.<event>" topic name,
	// where <service> is the event's producing product (ledger/fee/crm).
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
			libLog.String("ce_source", streamingCfg.CloudEventsSource),
			libLog.String("auth", authMode),
			libLog.Int("catalog_size", catalog.Len()),
			libLog.Int("routes", len(routes)),
		)
	}

	return emitter, emitter.Close, nil
}

// midazEventDefinitions returns the canonical, ordered list of midaz event
// Definitions paired with their producing service, registered into both the
// Catalog (service-agnostic) and the Routes (per-product topic). Kept as a
// single source of truth so adding a new event is a one-place change.
func midazEventDefinitions() []routedDefinition {
	return []routedDefinition{
		{events.OrganizationCreatedDefinition, serviceLedger},
		{events.OrganizationUpdatedDefinition, serviceLedger},
		{events.OrganizationDeletedDefinition, serviceLedger},
		{events.LedgerCreatedDefinition, serviceLedger},
		{events.LedgerUpdatedDefinition, serviceLedger},
		{events.LedgerDeletedDefinition, serviceLedger},
		{events.AccountCreatedDefinition, serviceLedger},
		{events.AccountUpdatedDefinition, serviceLedger},
		{events.AccountDeletedDefinition, serviceLedger},
		{events.AssetCreatedDefinition, serviceLedger},
		{events.AssetUpdatedDefinition, serviceLedger},
		{events.AssetDeletedDefinition, serviceLedger},
		{events.PortfolioCreatedDefinition, serviceLedger},
		{events.PortfolioUpdatedDefinition, serviceLedger},
		{events.PortfolioDeletedDefinition, serviceLedger},
		{events.SegmentCreatedDefinition, serviceLedger},
		{events.SegmentUpdatedDefinition, serviceLedger},
		{events.SegmentDeletedDefinition, serviceLedger},
		// account_type.* events are intentionally NOT registered:
		// internal validation config, the type label flows through
		// account.* events as a string field.
		{events.OperationRouteCreatedDefinition, serviceLedger},
		{events.OperationRouteUpdatedDefinition, serviceLedger},
		{events.OperationRouteDeletedDefinition, serviceLedger},
		{events.TransactionRouteCreatedDefinition, serviceLedger},
		{events.TransactionRouteUpdatedDefinition, serviceLedger},
		{events.TransactionRouteDeletedDefinition, serviceLedger},
		{events.BalanceCreatedDefinition, serviceLedger},
		{events.BalanceChangedDefinition, serviceLedger},
		{events.BalanceConfigChangedDefinition, serviceLedger},
		{events.BalanceDeletedDefinition, serviceLedger},
		{events.BalanceOverdraftDrawnDefinition, serviceLedger},
		{events.BalanceOverdraftRepaidDefinition, serviceLedger},
		{events.BalanceOverdraftClearedDefinition, serviceLedger},
		{events.TransactionPostedDefinition, serviceLedger},
		{events.TransactionCommittedDefinition, serviceLedger},
		{events.TransactionCanceledDefinition, serviceLedger},
		{events.TransactionRevertedDefinition, serviceLedger},
		// Fees
		{events.FeesPackageCreatedDefinition, serviceFee},
		{events.FeesPackageUpdatedDefinition, serviceFee},
		{events.FeesPackageDeletedDefinition, serviceFee},
		{events.FeesBillingPackageCreatedDefinition, serviceFee},
		{events.FeesBillingPackageUpdatedDefinition, serviceFee},
		{events.FeesBillingPackageDeletedDefinition, serviceFee},
		{events.FeesAppliedDefinition, serviceFee},
		// CRM
		{events.HolderCreatedDefinition, serviceCRM},
		{events.HolderUpdatedDefinition, serviceCRM},
		{events.HolderDeletedDefinition, serviceCRM},
		{events.InstrumentCreatedDefinition, serviceCRM},
		{events.InstrumentUpdatedDefinition, serviceCRM},
		{events.InstrumentDeletedDefinition, serviceCRM},
		{events.InstrumentRelatedPartyDeletedDefinition, serviceCRM},
	}
}

// buildCatalog constructs the immutable lib-streaming Catalog from
// midaz's event Definitions. Every entry maps the canonical
// "<resource>.<event>" key to its ResourceType / EventType /
// SchemaVersion triple.
func buildCatalog() (libStreaming.Catalog, error) {
	defs := midazEventDefinitions()
	entries := make([]libStreaming.EventDefinition, 0, len(defs))

	for _, rd := range defs {
		entries = append(entries, libStreaming.EventDefinition{
			Key:           rd.def.Key(),
			ResourceType:  rd.def.ResourceType,
			EventType:     rd.def.EventType,
			SchemaVersion: rd.def.SchemaVersion,
		})
	}

	return libStreaming.NewCatalog(entries...)
}

// buildRoutes constructs one RouteRequired route per midaz event,
// targeting the single broker named targetName. Topic names are
// "lerian.streaming.<service>_<resource>.<event>", where the service segment
// is the event's producing product (ledger core / fee / crm) from the
// per-product registry — NOT a single shared segment.
//
// Route Keys are composed as "<definition-key>.<target-name>" (e.g.
// "account.created.primary") — Route.Key must match a lower-case
// dot-delimited pattern, and the target-name suffix guarantees uniqueness
// when the same event is later routed to multiple targets (e.g. a parallel
// shadow route).
func buildRoutes(targetName string) []libStreaming.RouteDefinition {
	defs := midazEventDefinitions()
	routes := make([]libStreaming.RouteDefinition, 0, len(defs))

	for _, rd := range defs {
		key := rd.def.Key()
		routes = append(routes, libStreaming.RouteDefinition{
			Key:           key + "." + targetName,
			DefinitionKey: key,
			Target:        targetName,
			Destination:   libStreaming.KafkaTopic(pkgStreaming.TopicName(rd.service, key)),
			Requirement:   libStreaming.RouteRequired,
		})
	}

	return routes
}
