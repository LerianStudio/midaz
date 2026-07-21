// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	for _, rd := range defs {
		key := rd.def.Key()
		_, dup := defKeys[key]
		require.False(t, dup, "duplicate definition key %q in midazEventDefinitions", key)
		defKeys[key] = struct{}{}
		serviceByKey[key] = rd.service
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

	// Per-product routing regression lock (#3388): fees route under the "fee"
	// service segment and CRM under "crm" — NOT the ledger default that the
	// monorepo-consolidation bug folded onto every event.
	assertRouteTopic := func(key, service string) {
		t.Helper()

		want := libStreaming.KafkaTopic(pkgStreaming.TopicName(service, key))
		for _, r := range routes {
			if r.DefinitionKey == key {
				assert.Equalf(t, want, r.Destination, "route %q must target %q", key, want)
				return
			}
		}

		t.Fatalf("no route for key %q", key)
	}

	assertRouteTopic(events.AccountCreatedDefinition.Key(), serviceLedger)
	assertRouteTopic(events.FeesPackageCreatedDefinition.Key(), serviceFee)
	assertRouteTopic(events.FeesAppliedDefinition.Key(), serviceFee)
	assertRouteTopic(events.HolderCreatedDefinition.Key(), serviceCRM)
	assertRouteTopic(events.InstrumentCreatedDefinition.Key(), serviceCRM)
}

// TestFeesEventsRegistered locks the fee events into the assembled catalog: the
// fee package / billing-package keys plus fees.applied must be a subset of the
// catalog keys, so a dropped fee registration is caught before it becomes a
// silent gap.
func TestFeesEventsRegistered(t *testing.T) {
	t.Parallel()

	expected := []string{
		"fee-packages.created",
		"fee-packages.updated",
		"fee-packages.deleted",
		"fee-billing-packages.created",
		"fee-billing-packages.updated",
		"fee-billing-packages.deleted",
		"fee-charge.applied",
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
	assert.Equal(t, "fee-packages.created", events.FeesPackageCreatedDefinition.Key())
	assert.Equal(t, "fee-billing-packages.deleted", events.FeesBillingPackageDeletedDefinition.Key())
	assert.Equal(t, "fee-charge.applied", events.FeesAppliedDefinition.Key())
}
