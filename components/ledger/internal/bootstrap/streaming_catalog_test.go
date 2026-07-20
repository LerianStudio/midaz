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
// definition, each route pointing at the canonical prefixed topic, with no
// duplicate or orphan keys in either direction (the ghost-topic guard).
func TestMidazCatalogRoutesAssembly(t *testing.T) {
	t.Parallel()

	defs := midazEventDefinitions()

	catalog, err := buildCatalog()
	require.NoError(t, err)

	routes := buildRoutes(streamingPrimaryTargetName)

	// One catalog entry and one route per definition.
	assert.Equal(t, len(defs), catalog.Len(), "catalog entry count must equal definition count")
	assert.Len(t, routes, len(defs), "route count must equal definition count")

	// The set of definition keys is the source of truth for the bijection.
	// Key the checks off DefinitionKey (NOT route.Key, which carries the
	// ".primary" target suffix).
	defKeys := make(map[string]struct{}, len(defs))
	for _, d := range defs {
		key := d.Key()
		_, dup := defKeys[key]
		require.False(t, dup, "duplicate definition key %q in midazEventDefinitions", key)
		defKeys[key] = struct{}{}
	}

	seenRouteKeys := make(map[string]struct{}, len(routes))
	for _, r := range routes {
		assert.Equal(t, libStreaming.RouteRequired, r.Requirement,
			"route for %q must be RouteRequired", r.DefinitionKey)
		wantTopic := libStreaming.KafkaTopic(pkgStreaming.TopicName(streamingServiceName, r.DefinitionKey))
		assert.Equal(t, wantTopic, r.Destination,
			"route for %q must target topic %q", r.DefinitionKey, wantTopic)

		_, dup := seenRouteKeys[r.DefinitionKey]
		assert.False(t, dup, "duplicate route for DefinitionKey %q", r.DefinitionKey)
		seenRouteKeys[r.DefinitionKey] = struct{}{}

		// No orphan route: every route maps to a real definition.
		_, ok := defKeys[r.DefinitionKey]
		assert.True(t, ok, "route DefinitionKey %q has no matching event definition (dead/ghost route)", r.DefinitionKey)
	}

	// No orphan definition: every definition has a route (other direction).
	for key := range defKeys {
		_, ok := seenRouteKeys[key]
		assert.True(t, ok, "definition %q has no route (unroutable event)", key)
	}
}

// TestFeesEventsRegistered locks the fee events into the assembled catalog: the
// fee package / billing-package keys plus fees.applied must be a subset of the
// catalog keys, so a dropped fee registration is caught before it becomes a
// silent gap.
func TestFeesEventsRegistered(t *testing.T) {
	t.Parallel()

	expected := []string{
		"fees-package.created",
		"fees-package.updated",
		"fees-package.deleted",
		"fees-billing-package.created",
		"fees-billing-package.updated",
		"fees-billing-package.deleted",
		"fees.applied",
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
	assert.Equal(t, "fees-package.created", events.FeesPackageCreatedDefinition.Key())
	assert.Equal(t, "fees-billing-package.deleted", events.FeesBillingPackageDeletedDefinition.Key())
	assert.Equal(t, "fees.applied", events.FeesAppliedDefinition.Key())
}
