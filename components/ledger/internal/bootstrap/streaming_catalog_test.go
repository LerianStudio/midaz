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
// definition, each route pointing at its PER-PRODUCT topic (ledger core under
// service "ledger", fees under "fee", CRM under "crm"), with no duplicate or
// orphan keys in either direction (the ghost-topic guard).
func TestMidazCatalogRoutesAssembly(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes := buildRoutes(streamingPrimaryTargetName)

	// One catalog entry and one route per definition.
	assert.Equal(t, len(defs), catalog.Len(), "catalog entry count must equal definition count")
	assert.Len(t, routes, len(defs), "route count must equal definition count")

	// The set of definition keys is the source of truth for the bijection, and
	// each key carries its producing service so route topics are verified
	// per-product. Key the checks off DefinitionKey (NOT route.Key, which
	// carries the ".primary" target suffix).
	defKeys := make(map[string]struct{}, len(defs))
	serviceByKey := make(map[string]string, len(defs))
	// routeKeyByKey folds the underscored canonical Key() back to the hyphenated
	// RouteKey(). Expected topics must derive from the route key (what production
	// feeds into TopicName), not the DefinitionKey — otherwise the "fee-" prefix
	// strip cannot fire and the fee topics would regress to "fee_fee_*".
	routeKeyByKey := make(map[string]string, len(defs))

	for _, rd := range defs {
		key := rd.def.Key()
		_, dup := defKeys[key]
		require.False(t, dup, "duplicate definition key %q in midazEventDefinitions", key)
		defKeys[key] = struct{}{}
		serviceByKey[key] = rd.service
		routeKeyByKey[key] = rd.def.RouteKey()
	}

	seenRouteKeys := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		assert.Equal(t, libStreaming.RouteRequired, r.Requirement,
			"route for %q must be RouteRequired", r.DefinitionKey)

		service, ok := serviceByKey[r.DefinitionKey]
		require.Truef(t, ok, "route DefinitionKey %q has no matching event definition (dead/ghost route)", r.DefinitionKey)

		wantTopic := libStreaming.KafkaTopic(pkgStreaming.TopicName(service, routeKeyByKey[r.DefinitionKey]))
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

	// Per-product routing regression lock (#3388): an INDEPENDENT expected-service
	// map keyed by Definition.Key() with LITERAL service segments, deliberately
	// NOT derived from midazEventDefinitions (the code under test). serviceByKey
	// above is computed from the registry and would tautologically agree with a
	// wrong-service bug; this map enumerates every event's expected service so a
	// regression on ANY event — not just a handful of spot-checks — is caught at
	// unit speed. The literals "ledger"/"fee"/"crm" are intentional (not the
	// serviceLedger/serviceFee/serviceCRM constants the production code uses).
	const (
		wantLedger = "ledger"
		wantFee    = "fee"
		wantCRM    = "crm"
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

		wantTopic := libStreaming.KafkaTopic(pkgStreaming.TopicName(want, routeKeyByKey[r.DefinitionKey]))
		assert.Equalf(t, wantTopic, r.Destination,
			"route %q must target %q (independent service lock)", r.DefinitionKey, wantTopic)
	}

	// Fee-family destination lock (underscore canonicalization): the route key
	// folds the underscored DefinitionKey back to the hyphenated routing handle
	// before TopicName strips the "fee-" prefix, so fee topics keep a single
	// "fee_" segment and never regress to "fee_fee_*". Literal broker topics so
	// feeding the DefinitionKey (instead of the route key) into TopicName fails.
	destByKey := make(map[string]string, len(routes))
	for _, r := range routes {
		destByKey[r.DefinitionKey] = r.Destination.Name
	}

	feeTopics := map[string]string{
		"fee_packages.created":         "lerian.streaming.fee_packages.created",
		"fee_packages.updated":         "lerian.streaming.fee_packages.updated",
		"fee_packages.deleted":         "lerian.streaming.fee_packages.deleted",
		"fee_billing_packages.created": "lerian.streaming.fee_billing_packages.created",
		"fee_billing_packages.updated": "lerian.streaming.fee_billing_packages.updated",
		"fee_billing_packages.deleted": "lerian.streaming.fee_billing_packages.deleted",
		"fee_charge.applied":           "lerian.streaming.fee_charge.applied",
	}
	for key, topic := range feeTopics {
		assert.Equalf(t, topic, destByKey[key],
			"fee route %q must target %q (no fee_fee_* regression)", key, topic)
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
