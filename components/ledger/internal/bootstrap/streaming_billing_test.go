// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming/v2"
	billing "github.com/LerianStudio/lib-streaming/v2/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingEventWiredIntoCatalog locks that the shared billing_recorded
// definition owned by lib-streaming's billing package is registered in the midaz
// streaming catalog, and its route resolves to the FIXED shared billing topic
// (owned by the billing package, NOT rendered via pkgStreaming.TopicName) without
// colliding with any midaz domain route.
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

	// Contract lock: the shared billing key must not drift.
	assert.Equal(t, "billing_recorded", billing.Definition().Key,
		"billing definition key drifted from the shared contract")

	// (2) The billing route is wired via Builder.RouteOverrides on top of the
	// domain routes. RouteOverrides merges replace-by-DefinitionKey; billing_recorded
	// matches NO domain route, so it is appended and every domain route survives.
	// Assert the no-collision invariant that wiring relies on.
	domainRoutes := buildRoutes(streamingPrimaryTargetName)
	for _, r := range domainRoutes {
		assert.NotEqualf(t, billing.Definition().Key, r.DefinitionKey,
			"billing_recorded must not collide with domain route %q", r.DefinitionKey)
	}

	// The billing route resolves to the fixed shared literal topic (not a
	// per-product topic rendered from pkgStreaming.TopicName).
	billingRoute := billing.Route()
	assert.Equal(t, billing.Definition().Key, billingRoute.DefinitionKey,
		"billing route DefinitionKey must equal the billing definition key")
	assert.Equal(t, libStreaming.KafkaTopic(billing.Topic), billingRoute.Destination,
		"billing route must target the fixed shared billing topic")
	assert.Equal(t, "lerian.streaming.billing.recorded", billing.Topic,
		"billing topic literal drifted from the shared contract")
}
