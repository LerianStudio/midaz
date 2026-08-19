// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v3"
	billing "github.com/LerianStudio/lib-streaming/v3/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingEventWiredIntoCatalog locks that the shared billing_recorded
// definition owned by lib-streaming's billing package is registered in the midaz
// streaming catalog, and that it resolves to the FIXED shared billing topic
// INSTEAD OF — never in addition to — the ledger's own application topic.
//
// Under one topic per producing application the ledger publishes through a single
// catch-all route. lib-streaming resolves routes additively per target, so a
// scoped route only overrides the catch-all when it names the SAME target: the
// billing override sharing "primary" is exactly what keeps billing_recorded off
// the ledger's own stream. A billing route on any other target would ADD, and
// every billable usage event would be published twice.
func TestBillingEventWiredIntoCatalog(t *testing.T) {
	t.Parallel()

	catalog, err := buildCatalog()
	require.NoError(t, err)

	// (1) The catalog must contain the shared billing definition key. This is the
	// production wiring under test: buildCatalog appends billing.Definition().
	catalogKeys := make(map[string]struct{}, catalog.Len())
	for _, d := range catalog.Definitions() {
		catalogKeys[d.Key] = struct{}{}
	}

	_, ok := catalogKeys[billing.Definition().Key]
	assert.Truef(t, ok, "billing definition %q must be registered in the streaming catalog", billing.Definition().Key)

	// Contract lock: the shared billing key and topic must not drift.
	assert.Equal(t, "billing_recorded", billing.Definition().Key,
		"billing definition key drifted from the shared contract")
	assert.Equal(t, "lerian.streaming.billing.recorded", billing.Topic,
		"billing topic literal drifted from the shared contract")

	billingRoute := billing.Route()
	assert.Equal(t, billing.Definition().Key, billingRoute.DefinitionKey,
		"billing route DefinitionKey must equal the billing definition key")
	assert.Equal(t, libStreaming.KafkaTopic(billing.Topic), billingRoute.Destination,
		"billing route must target the fixed shared billing topic")
	assert.Equal(t, streamingPrimaryTargetName, billingRoute.Target,
		"the billing override must share the catch-all's target, or it would ADD a second publish instead of replacing it")

	// (2) Resolve the production route table exactly as Builder.Build does:
	// the domain catch-all plus the billing override. The two carry different
	// DefinitionKeys, so the merge appends rather than replacing.
	domainRoutes, err := buildRoutes(streamingPrimaryTargetName, streamingServiceName)
	require.NoError(t, err)
	require.Len(t, domainRoutes, 1, "the ledger publishes through exactly one catch-all route")
	require.Empty(t, domainRoutes[0].DefinitionKey,
		"an empty DefinitionKey is what makes the domain route a catch-all")

	table, err := libStreaming.NewRouteTable(append(domainRoutes, billingRoute)...)
	require.NoError(t, err)

	appTopic, err := libStreaming.AppTopic(streamingServiceName)
	require.NoError(t, err)

	// billing_recorded resolves to the billing topic ONLY.
	resolved := table.Routes(billing.Definition().Key)
	require.Len(t, resolved, 1,
		"billing_recorded must resolve to exactly one route; a second would double-publish every billable event")
	assert.Equal(t, billing.Topic, resolved[0].Destination.Name,
		"billing_recorded must ride the fixed billing topic")

	// Every other definition resolves to the catch-all on the application topic.
	for _, def := range midazEventDefinitions() {
		routes := table.Routes(def.Key())
		require.Lenf(t, routes, 1, "%q must resolve to exactly one route", def.Key())
		assert.Equalf(t, appTopic, routes[0].Destination.Name,
			"%q must ride the ledger application topic", def.Key())
	}
}
