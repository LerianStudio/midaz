// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
)

// TestMidazCatalogRoutesAssembly locks the catalog/routes assembly path: it
// must produce exactly one catalog entry and one required route per event
// definition, each route pointing at its topic under the single "ledger"
// service segment (ledger core, fees, and CRM all collapsed into "ledger"),
// with no duplicate or orphan keys in either direction (the ghost-topic guard).
func TestMidazCatalogRoutesAssembly(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes := buildRoutes(streamingPrimaryTargetName)

	// One catalog entry per domain definition, plus the single shared
	// billing_recorded entry appended by buildCatalog (owned by lib-streaming's
	// billing package, not part of midazEventDefinitions). Routes from buildRoutes
	// remain domain-only; the billing route is wired via Builder.RouteOverrides.
	assert.Equal(t, len(defs)+1, catalog.Len(), "catalog = one entry per domain definition plus the shared billing entry")
	assert.Len(t, routes, len(defs), "route count must equal definition count")

	// The set of definition keys is the source of truth for the bijection, and
	// each key carries its producing service so route topics are verified
	// per-product. Key the checks off DefinitionKey (NOT route.Key, which
	// carries the ".primary" target suffix). Under the ACL-prefix grammar the
	// wire topic derives from the underscore-canonical Key() directly, so the
	// expected topic is simply TopicName(service, DefinitionKey).
	defKeys := make(map[string]struct{}, len(defs))
	serviceByKey := make(map[string]string, len(defs))

	for _, def := range defs {
		key := def.Key()
		_, dup := defKeys[key]
		require.False(t, dup, "duplicate definition key %q in midazEventDefinitions", key)
		defKeys[key] = struct{}{}
		serviceByKey[key] = streamingServiceName
	}

	seenRouteKeys := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		assert.Equal(t, libStreaming.RouteRequired, r.Requirement,
			"route for %q must be RouteRequired", r.DefinitionKey)

		service, ok := serviceByKey[r.DefinitionKey]
		require.Truef(t, ok, "route DefinitionKey %q has no matching event definition (dead/ghost route)", r.DefinitionKey)

		wantTopic := libStreaming.KafkaTopic(pkgStreaming.TopicName(service, r.DefinitionKey))
		assert.Equal(t, wantTopic, r.Destination,
			"route for %q must target topic %q", r.DefinitionKey, wantTopic)

		_, dup := seenRouteKeys[r.DefinitionKey]
		assert.False(t, dup, "duplicate route for DefinitionKey %q", r.DefinitionKey)
		seenRouteKeys[r.DefinitionKey] = struct{}{}
	}

	// No orphan definition: every definition has a route (other direction).
	for key := range defKeys {
		_, ok := seenRouteKeys[key]
		assert.True(t, ok, "definition %q has no route (unroutable event)", key)
	}

	// Single-service routing regression lock (#3388): an INDEPENDENT
	// expected-service map keyed by Definition.Key() with LITERAL service
	// segments, deliberately NOT derived from midazEventDefinitions (the code
	// under test). serviceByKey above is computed from the registry and would
	// tautologically agree with a wrong-service bug; this map enumerates every
	// event's expected service so a regression on ANY event — not just a handful
	// of spot-checks — is caught at unit speed. After the collapse every event
	// (ledger core, fees, CRM) routes under the single "ledger" segment; the
	// wantFee/wantCRM aliases are kept only to document which events used to carry
	// a distinct segment. The literal "ledger" is intentional (not the
	// streamingServiceName constant the production code uses).
	const (
		wantLedger = "ledger"
		wantFee    = "ledger"
		wantCRM    = "ledger"
	)

	expectedService := map[string]string{
		// Ledger core.
		events.OrganizationCreatedDefinition.Key():     wantLedger,
		events.OrganizationUpdatedDefinition.Key():     wantLedger,
		events.OrganizationDeletedDefinition.Key():     wantLedger,
		events.LedgerCreatedDefinition.Key():           wantLedger,
		events.LedgerUpdatedDefinition.Key():           wantLedger,
		events.LedgerDeletedDefinition.Key():           wantLedger,
		events.AccountCreatedDefinition.Key():          wantLedger,
		events.AccountUpdatedDefinition.Key():          wantLedger,
		events.AccountDeletedDefinition.Key():          wantLedger,
		events.AssetCreatedDefinition.Key():            wantLedger,
		events.AssetUpdatedDefinition.Key():            wantLedger,
		events.AssetDeletedDefinition.Key():            wantLedger,
		events.PortfolioCreatedDefinition.Key():        wantLedger,
		events.PortfolioUpdatedDefinition.Key():        wantLedger,
		events.PortfolioDeletedDefinition.Key():        wantLedger,
		events.SegmentCreatedDefinition.Key():          wantLedger,
		events.SegmentUpdatedDefinition.Key():          wantLedger,
		events.SegmentDeletedDefinition.Key():          wantLedger,
		events.OperationRouteCreatedDefinition.Key():   wantLedger,
		events.OperationRouteUpdatedDefinition.Key():   wantLedger,
		events.OperationRouteDeletedDefinition.Key():   wantLedger,
		events.TransactionRouteCreatedDefinition.Key(): wantLedger,
		events.TransactionRouteUpdatedDefinition.Key(): wantLedger,
		events.TransactionRouteDeletedDefinition.Key(): wantLedger,
		events.BalanceCreatedDefinition.Key():          wantLedger,
		events.BalanceChangedDefinition.Key():          wantLedger,
		events.BalanceConfigChangedDefinition.Key():    wantLedger,
		events.BalanceDeletedDefinition.Key():          wantLedger,
		events.BalanceOverdraftDrawnDefinition.Key():   wantLedger,
		events.BalanceOverdraftRepaidDefinition.Key():  wantLedger,
		events.BalanceOverdraftClearedDefinition.Key(): wantLedger,
		events.TransactionPostedDefinition.Key():       wantLedger,
		events.TransactionCommittedDefinition.Key():    wantLedger,
		events.TransactionCanceledDefinition.Key():     wantLedger,
		events.TransactionRevertedDefinition.Key():     wantLedger,
		// Fees.
		events.FeesPackageCreatedDefinition.Key():        wantFee,
		events.FeesPackageUpdatedDefinition.Key():        wantFee,
		events.FeesPackageDeletedDefinition.Key():        wantFee,
		events.FeesBillingPackageCreatedDefinition.Key(): wantFee,
		events.FeesBillingPackageUpdatedDefinition.Key(): wantFee,
		events.FeesBillingPackageDeletedDefinition.Key(): wantFee,
		events.FeesAppliedDefinition.Key():               wantFee,
		// CRM.
		events.HolderCreatedDefinition.Key():                 wantCRM,
		events.HolderUpdatedDefinition.Key():                 wantCRM,
		events.HolderDeletedDefinition.Key():                 wantCRM,
		events.InstrumentCreatedDefinition.Key():             wantCRM,
		events.InstrumentUpdatedDefinition.Key():             wantCRM,
		events.InstrumentDeletedDefinition.Key():             wantCRM,
		events.InstrumentRelatedPartyDeletedDefinition.Key(): wantCRM,
	}

	// The independent map must cover exactly the registry key set: a missing
	// event would silently skip its service check, an extra key would mask a
	// dropped registration.
	assert.Equal(t, len(defKeys), len(expectedService),
		"expectedService must enumerate every registered event exactly once")
	for key := range defKeys {
		_, ok := expectedService[key]
		assert.Truef(t, ok, "registered event %q missing from independent expectedService map", key)
	}
	for key := range expectedService {
		_, ok := defKeys[key]
		assert.Truef(t, ok, "expectedService key %q is not present in the registry", key)
	}

	// Every actual route's topic must match the INDEPENDENT expected service.
	for _, r := range routes {
		want, ok := expectedService[r.DefinitionKey]
		if !assert.Truef(t, ok, "route %q has no independent expected service", r.DefinitionKey) {
			continue
		}

		wantTopic := libStreaming.KafkaTopic(pkgStreaming.TopicName(want, r.DefinitionKey))
		assert.Equalf(t, wantTopic, r.Destination,
			"route %q must target %q (independent service lock)", r.DefinitionKey, wantTopic)
	}

	// Fee-family destination lock: after the collapse every fee event routes
	// under the "ledger" segment, so the fee resource ("fee_packages",
	// "fee_charge", ...) becomes the resource segment of "ledger.<resource>.<event>".
	// Literal broker topics so a regression on the fee namespace is caught directly.
	destByKey := make(map[string]string, len(routes))
	for _, r := range routes {
		destByKey[r.DefinitionKey] = r.Destination.Name
	}

	feeTopics := map[string]string{
		"fee_packages.created":         "ledger.fee_packages.created",
		"fee_packages.updated":         "ledger.fee_packages.updated",
		"fee_packages.deleted":         "ledger.fee_packages.deleted",
		"fee_billing_packages.created": "ledger.fee_billing_packages.created",
		"fee_billing_packages.updated": "ledger.fee_billing_packages.updated",
		"fee_billing_packages.deleted": "ledger.fee_billing_packages.deleted",
		"fee_charge.applied":           "ledger.fee_charge.applied",
	}
	for key, topic := range feeTopics {
		assert.Equalf(t, topic, destByKey[key],
			"fee route %q must target %q", key, topic)
	}
}

// TestFeesEventsRegistered locks the fee events into the assembled catalog: the
// fee package / billing-package keys plus fee_charge.applied must be a subset of the
// catalog keys, so a dropped fee registration is caught before it becomes a
// silent gap.
func TestFeesEventsRegistered(t *testing.T) {
	t.Parallel()

	expected := []string{
		"fee_packages.created",
		"fee_packages.updated",
		"fee_packages.deleted",
		"fee_billing_packages.created",
		"fee_billing_packages.updated",
		"fee_billing_packages.deleted",
		"fee_charge.applied",
	}

	catalog, err := buildCatalog()
	require.NoError(t, err)

	catalogKeys := make(map[string]struct{}, catalog.Len())
	for _, d := range catalog.Definitions() {
		catalogKeys[d.Key] = struct{}{}
	}

	for _, key := range expected {
		_, ok := catalogKeys[key]
		assert.True(t, ok, "fee event %q must be registered in the streaming catalog", key)
	}

	// Guard against the key strings drifting from the Definition vars.
	assert.Equal(t, "fee_packages.created", events.FeesPackageCreatedDefinition.Key())
	assert.Equal(t, "fee_billing_packages.deleted", events.FeesBillingPackageDeletedDefinition.Key())
	assert.Equal(t, "fee_charge.applied", events.FeesAppliedDefinition.Key())
}

// TestTopicConvergesWithEventDefinition proves midaz's pkgStreaming.TopicName
// and lib-streaming's own EventDefinition.Topic derive the SAME Kafka topic for
// every registered event, with the bare service name ("ledger") as the
// CloudEvents source. This convergence is what lets a Kafka ACL scoped to the
// "ledger." prefix — granted from the tenant-manager's SanitizeKafkaSegment —
// cover every topic the producer emits: the two derivations must never diverge.
// Card #3783 Task 5.2.
//
// The convergence asserted here holds ONLY because the service name is pure
// [a-z0-9] (as "ledger" is): midaz's sanitizeServiceSegment keeps [a-z0-9] while
// lib-streaming's sanitizeSourceSegment keeps [a-z0-9._-], so the two legitimately
// diverge for non-alphanumeric input. That is why this test uses the bare service
// name and deliberately does NOT assert a non-identity source case — a source with
// a "." or "-" would produce different segments from each sanitizer by design.
func TestTopicConvergesWithEventDefinition(t *testing.T) {
	t.Parallel()

	const ceSource = streamingServiceName // the bare service name is the ce-source

	for _, def := range midazEventDefinitions() {
		ed := libStreaming.EventDefinition{
			Key:           def.Key(),
			ResourceType:  def.ResourceType,
			EventType:     def.EventType,
			SchemaVersion: def.SchemaVersion,
		}

		want := ed.Topic(ceSource)
		got := pkgStreaming.TopicName(streamingServiceName, def.Key())

		assert.Equalf(t, want, got,
			"TopicName and EventDefinition.Topic must converge for %q", def.Key())
		// Lock the literal invariant the convergence rests on: for a service name
		// that is already [a-z0-9]-only, both sanitizers are the identity, so the
		// topic is exactly service + "." + Key().
		assert.Equalf(t, streamingServiceName+"."+def.Key(), got,
			"topic for %q must be service + \".\" + Key()", def.Key())
	}
}
